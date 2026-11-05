package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/infra/persistence"
)

type fakeArtifactProvider struct {
	store port.ArtifactStore
}

func (p *fakeArtifactProvider) ForProject(ctx context.Context, projectID string) (port.ArtifactStore, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p.store == nil {
		return nil, context.Canceled
	}
	return p.store, nil
}

type mismatchedArtifactStore struct {
	port.ArtifactStore
	payload []byte
}

func (s mismatchedArtifactStore) Get(ctx context.Context, _ string) (entity.Artifact, []byte, error) {
	if err := ctx.Err(); err != nil {
		return entity.Artifact{}, nil, err
	}
	return entity.Artifact{ID: "sha256:" + strings.Repeat("f", 64)}, append([]byte(nil), s.payload...), nil
}

func artifactWithTopics(t *testing.T, store port.ArtifactStore, payload string) string {
	t.Helper()
	artifact, err := store.Put(context.Background(), []byte(payload), entity.ArtifactMeta{
		Kind: valueobject.OutputKindTopics, NodeID: "n_topic", CreatedAt: "2026-11-01T10:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	return artifact.ID
}

func TestArtifactCandidateCounter(t *testing.T) {
	store := persistence.NewArtifactStore(t.TempDir())
	counter := NewArtifactCandidateCounter(&fakeArtifactProvider{store: store})
	first := artifactWithTopics(t, store, `{"topics":[{"title":"a","angle":"a","hook":"a"},{"title":"b","angle":"b","hook":"b"},{"title":"c","angle":"c","hook":"c"}]}`)
	second := artifactWithTopics(t, store, `{"topics":[{"title":"a","angle":"a","hook":"a"},{"title":"b","angle":"b","hook":"b"},{"title":"c","angle":"c","hook":"c"},{"title":"d","angle":"d","hook":"d"}]}`)
	node := entity.Node{
		ID: "n_topic", Type: valueobject.NodeTypeTopic,
		Rounds: []entity.Round{
			{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: first},
			{N: 2, Kind: valueobject.OutputKindTopics, ArtifactID: second},
		},
	}
	count, err := counter.Count(context.Background(), "p_test", node)
	if err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Fatalf("count = %d, want latest round 4", count)
	}
}

func TestArtifactTopicCandidateReaderReturnsStableIndexes(t *testing.T) {
	store := persistence.NewArtifactStore(t.TempDir())
	reader := NewArtifactCandidateCounter(&fakeArtifactProvider{store: store})
	artifactID := artifactWithTopics(t, store, `{"topics":[{"title":"夜班","angle":"人物观察","hook":"凌晨出发"},{"title":"末班车","angle":"城市声音","hook":"最后一趟"},{"title":"修理铺","angle":"手艺记录","hook":"灯仍亮着"}]}`)
	candidates, err := reader.Read(context.Background(), "p_test", entity.Node{
		ID: "n_topic", Type: valueobject.NodeTypeTopic,
		Rounds: []entity.Round{{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: artifactID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 3 || candidates[0] != (entity.TopicCandidate{Index: 0, Title: "夜班", Angle: "人物观察", Hook: "凌晨出发"}) || candidates[2].Index != 2 {
		t.Fatalf("candidates = %+v", candidates)
	}
}

func TestGetTopicCandidatesUsesAppliedArtifactAndProjectScope(t *testing.T) {
	store := persistence.NewArtifactStore(t.TempDir())
	older := artifactWithTopics(t, store, `{"topics":[{"title":"旧一","angle":"a","hook":"a"},{"title":"旧二","angle":"b","hook":"b"},{"title":"旧三","angle":"c","hook":"c"}]}`)
	latest := artifactWithTopics(t, store, `{"topics":[{"title":"新一","angle":"a","hook":"a"},{"title":"新二","angle":"b","hook":"b"},{"title":"新三","angle":"c","hook":"c"}]}`)
	project := baseProject()
	project.Nodes[0].Rounds = []entity.Round{
		{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: older},
		{N: 2, Kind: valueobject.OutputKindTopics, ArtifactID: latest},
	}
	session := &fakeSession{project: project}
	reader := NewArtifactCandidateCounter(&fakeArtifactProvider{store: store})
	service := New(Options{Project: session, CandidateReader: reader})

	candidates, err := service.GetTopicCandidates(context.Background(), "n_topic", "p_test")
	if err != nil || len(candidates) != 3 || candidates[0].Title != "新一" || candidates[0].Index != 0 {
		t.Fatalf("candidates = %+v err=%v", candidates, err)
	}
	if _, err := service.GetTopicCandidates(context.Background(), "n_topic", "p_other"); !errors.Is(err, ErrProjectChanged) {
		t.Fatalf("cross-project error = %v", err)
	}
	if _, err := service.GetTopicCandidates(context.Background(), "n_a", "p_test"); !errors.Is(err, ErrUnknownNodeType) {
		t.Fatalf("prompt candidate read error = %v", err)
	}
	reloaded := New(Options{
		Project:         &fakeSession{project: session.project.Clone()},
		CandidateReader: reader,
	})
	candidates, err = reloaded.GetTopicCandidates(context.Background(), "n_topic", "p_test")
	if err != nil || len(candidates) != 3 || candidates[0].Title != "新一" {
		t.Fatalf("reloaded candidates = %+v err=%v", candidates, err)
	}

	session.project.Nodes[0].Rounds = nil
	candidates, err = service.GetTopicCandidates(context.Background(), "n_topic", "p_test")
	if err != nil || len(candidates) != 0 {
		t.Fatalf("unapplied topic candidates = %+v err=%v", candidates, err)
	}
}

func TestGetTopicCandidatesFailsClosedForBrokenArtifacts(t *testing.T) {
	validPayload := []byte(`{"topics":[{"title":"a","angle":"a","hook":"a"},{"title":"b","angle":"b","hook":"b"},{"title":"c","angle":"c","hook":"c"}]}`)
	missingStore := persistence.NewArtifactStore(t.TempDir())
	corruptStore := persistence.NewArtifactStore(t.TempDir())
	corruptID := artifactWithTopics(t, corruptStore, `{"topics":"invalid"}`)

	cases := map[string]struct {
		provider port.ArtifactStoreProvider
		kind     valueobject.OutputKind
		artifact string
	}{
		"missing artifact": {
			provider: &fakeArtifactProvider{store: missingStore},
			kind:     valueobject.OutputKindTopics,
			artifact: "sha256:" + strings.Repeat("0", 64),
		},
		"invalid payload": {
			provider: &fakeArtifactProvider{store: corruptStore},
			kind:     valueobject.OutputKindTopics,
			artifact: corruptID,
		},
		"wrong round kind": {
			provider: &fakeArtifactProvider{store: missingStore},
			kind:     valueobject.OutputKindScript,
			artifact: "sha256:" + strings.Repeat("1", 64),
		},
		"missing artifact ID": {
			provider: &fakeArtifactProvider{store: missingStore},
			kind:     valueobject.OutputKindTopics,
		},
		"mismatched artifact ID": {
			provider: &fakeArtifactProvider{store: mismatchedArtifactStore{payload: validPayload}},
			kind:     valueobject.OutputKindTopics,
			artifact: "sha256:" + strings.Repeat("2", 64),
		},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			project := baseProject()
			project.Nodes[0].Rounds = []entity.Round{{N: 1, Kind: test.kind, ArtifactID: test.artifact}}
			service := New(Options{
				Project:         &fakeSession{project: project},
				CandidateReader: NewArtifactCandidateCounter(test.provider),
			})
			if _, err := service.GetTopicCandidates(context.Background(), "n_topic", "p_test"); !errors.Is(err, ErrCandidateInvalid) {
				t.Fatalf("candidate read error = %v, want CANDIDATE_INVALID", err)
			}
		})
	}
}

func TestArtifactCandidateCounterEmptyCases(t *testing.T) {
	store := persistence.NewArtifactStore(t.TempDir())
	counter := NewArtifactCandidateCounter(&fakeArtifactProvider{store: store})
	scriptArtifact, err := store.Put(context.Background(), []byte(`{"topics":[{"title":"a","angle":"a","hook":"a"},{"title":"b","angle":"b","hook":"b"},{"title":"c","angle":"c","hook":"c"}]}`), entity.ArtifactMeta{
		Kind: valueobject.OutputKindScript, NodeID: "n_topic", CreatedAt: "2026-11-01T10:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, node := range map[string]entity.Node{
		"no rounds":    {ID: "n_topic", Type: valueobject.NodeTypeTopic},
		"prompt node":  {ID: "n_prompt", Type: valueobject.NodeTypePrompt, Rounds: []entity.Round{{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: "sha256:x"}}},
		"script round": {ID: "n_topic", Type: valueobject.NodeTypeTopic, Rounds: []entity.Round{{N: 1, Kind: valueobject.OutputKindScript, ArtifactID: "sha256:x"}}},
	} {
		count, err := counter.Count(context.Background(), "p_test", node)
		if err != nil || count != 0 {
			t.Fatalf("%s: count=%d err=%v", name, count, err)
		}
	}
	count, err := counter.Count(context.Background(), "p_test", entity.Node{
		ID: "n_topic", Type: valueobject.NodeTypeTopic,
		Rounds: []entity.Round{{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: scriptArtifact.ID}},
	})
	if err != nil || count != 3 {
		t.Fatalf("current round kind should override first artifact metadata: count=%d err=%v", count, err)
	}
}

func TestArtifactCandidateCounterErrors(t *testing.T) {
	store := persistence.NewArtifactStore(t.TempDir())
	counter := NewArtifactCandidateCounter(&fakeArtifactProvider{store: store})
	invalid := artifactWithTopics(t, store, `{"topics":"bad"}`)
	if _, err := counter.Count(context.Background(), "p_test", entity.Node{
		ID: "n_topic", Type: valueobject.NodeTypeTopic,
		Rounds: []entity.Round{{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: invalid}},
	}); err == nil {
		t.Fatal("invalid topics payload error = nil")
	}
	if _, err := counter.Count(context.Background(), "p_test", entity.Node{
		ID: "n_topic", Type: valueobject.NodeTypeTopic,
		Rounds: []entity.Round{{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: "sha256:" + "0"}},
	}); err == nil {
		t.Fatal("missing artifact error = nil")
	}
	wrongKind, err := store.Put(context.Background(), []byte(`{"topics":"bad"}`), entity.ArtifactMeta{
		Kind: valueobject.OutputKindScript, NodeID: "n_topic", CreatedAt: "2026-11-01T10:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := counter.Count(context.Background(), "p_test", entity.Node{
		ID: "n_topic", Type: valueobject.NodeTypeTopic,
		Rounds: []entity.Round{{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: wrongKind.ID}},
	}); err == nil {
		t.Fatal("invalid cross-kind payload error = nil")
	}
	noProvider := NewArtifactCandidateCounter(nil)
	if _, err := noProvider.Count(context.Background(), "p_test", entity.Node{
		ID: "n_topic", Type: valueobject.NodeTypeTopic,
		Rounds: []entity.Round{{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: invalid}},
	}); err == nil {
		t.Fatal("nil provider error = nil")
	}
}
