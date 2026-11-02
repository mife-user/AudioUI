package workflow

import (
	"context"
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
	first := artifactWithTopics(t, store, `{"topics":[{"title":"a"},{"title":"b"}]}`)
	second := artifactWithTopics(t, store, `{"topics":[{"title":"a"},{"title":"b"},{"title":"c"},{"title":"d"}]}`)
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

func TestArtifactCandidateCounterEmptyCases(t *testing.T) {
	store := persistence.NewArtifactStore(t.TempDir())
	counter := NewArtifactCandidateCounter(&fakeArtifactProvider{store: store})
	scriptArtifact, err := store.Put(context.Background(), []byte(`{"topics":[{"title":"a"}]}`), entity.ArtifactMeta{
		Kind: valueobject.OutputKindScript, NodeID: "n_topic", CreatedAt: "2026-11-01T10:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, node := range map[string]entity.Node{
		"no rounds":     {ID: "n_topic", Type: valueobject.NodeTypeTopic},
		"prompt node":   {ID: "n_prompt", Type: valueobject.NodeTypePrompt, Rounds: []entity.Round{{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: "sha256:x"}}},
		"script round":  {ID: "n_topic", Type: valueobject.NodeTypeTopic, Rounds: []entity.Round{{N: 1, Kind: valueobject.OutputKindScript, ArtifactID: "sha256:x"}}},
		"artifact kind": {ID: "n_topic", Type: valueobject.NodeTypeTopic, Rounds: []entity.Round{{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: scriptArtifact.ID}}},
	} {
		count, err := counter.Count(context.Background(), "p_test", node)
		if err != nil || count != 0 {
			t.Fatalf("%s: count=%d err=%v", name, count, err)
		}
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
	noProvider := NewArtifactCandidateCounter(nil)
	if _, err := noProvider.Count(context.Background(), "p_test", entity.Node{
		ID: "n_topic", Type: valueobject.NodeTypeTopic,
		Rounds: []entity.Round{{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: invalid}},
	}); err == nil {
		t.Fatal("nil provider error = nil")
	}
}
