package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/infra/persistence"
)

const validScriptBody = `{"title":"旁白","scenes":[{"narration":"第一行\n第二行","duration":12}]}`

func scriptVersionService(t *testing.T, artifactID string, provider port.ArtifactStoreProvider) *Service {
	t.Helper()
	project := baseProject()
	project.Nodes = append(project.Nodes, entity.Node{
		ID: "n_script", Type: valueobject.NodeTypeScript, Title: "脚本", Parent: "n_b",
		Size: entity.DefaultNodeSize(valueobject.NodeTypeScript), Rounds: []entity.Round{},
		ScriptVersions: []entity.ScriptVersion{{N: 1, ArtifactID: artifactID, RunID: "r_script"}},
	})
	return New(Options{Project: &fakeSession{project: project}, Artifacts: provider})
}

func TestGetScriptVersionUsesProjectNodeAndExactVersionReference(t *testing.T) {
	store := persistence.NewArtifactStore(t.TempDir())
	artifact, err := store.Put(context.Background(), []byte(validScriptBody), entity.ArtifactMeta{
		Kind: valueobject.OutputKindScript, NodeID: "n_script", CreatedAt: "2026-11-09T10:00:00Z", SourceRunID: "r_script",
	})
	if err != nil {
		t.Fatal(err)
	}
	service := scriptVersionService(t, artifact.ID, &fakeArtifactProvider{store: store})

	detail, err := service.GetScriptVersion(context.Background(), "n_script", 1, "p_test")
	if err != nil {
		t.Fatal(err)
	}
	if detail.NodeID != "n_script" || detail.Version != (entity.ScriptVersion{N: 1, ArtifactID: artifact.ID, RunID: "r_script"}) || detail.Payload != validScriptBody {
		t.Fatalf("script version detail = %+v", detail)
	}
	if _, err := service.GetScriptVersion(context.Background(), "n_script", 1, "p_other"); !errors.Is(err, ErrProjectChanged) {
		t.Fatalf("cross-project error = %v", err)
	}
	if _, err := service.GetScriptVersion(context.Background(), "n_a", 1, "p_test"); !errors.Is(err, ErrUnknownNodeType) {
		t.Fatalf("non-script error = %v", err)
	}
	if _, err := service.GetScriptVersion(context.Background(), "n_missing", 1, "p_test"); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("missing node error = %v", err)
	}
	if _, err := service.GetScriptVersion(context.Background(), "n_script", 2, "p_test"); !errors.Is(err, ErrScriptVersionNotFound) {
		t.Fatalf("missing version error = %v", err)
	}
	if _, err := service.GetScriptVersion(context.Background(), "n_script", 0, "p_test"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid version error = %v", err)
	}
}

func TestGetScriptVersionFailsClosedForInvalidArtifacts(t *testing.T) {
	store := persistence.NewArtifactStore(t.TempDir())
	invalidSchema, err := store.Put(context.Background(), []byte(`{"title":"空","scenes":[]}`), entity.ArtifactMeta{
		Kind: valueobject.OutputKindScript, NodeID: "n_script", CreatedAt: "2026-11-09T10:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	oversizedPayload := []byte(fmt.Sprintf(`{"title":"大","scenes":[{"narration":"%s","duration":1}]}`, strings.Repeat("x", valueobject.MaxPendingPayloadSize+1)))
	oversized, err := store.Put(context.Background(), oversizedPayload, entity.ArtifactMeta{
		Kind: valueobject.OutputKindScript, NodeID: "n_script", CreatedAt: "2026-11-09T10:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string]struct {
		service *Service
		wantErr error
	}{
		"missing artifact": {
			service: scriptVersionService(t, "sha256:"+strings.Repeat("0", 64), &fakeArtifactProvider{store: store}),
			wantErr: ErrScriptVersionCorrupt,
		},
		"mismatched artifact ID": {
			service: scriptVersionService(t, "sha256:"+strings.Repeat("2", 64), &fakeArtifactProvider{store: mismatchedArtifactStore{payload: []byte(validScriptBody)}}),
			wantErr: ErrScriptVersionCorrupt,
		},
		"invalid schema": {
			service: scriptVersionService(t, invalidSchema.ID, &fakeArtifactProvider{store: store}),
			wantErr: ErrScriptVersionCorrupt,
		},
		"oversized payload": {
			service: scriptVersionService(t, oversized.ID, &fakeArtifactProvider{store: store}),
			wantErr: ErrScriptVersionCorrupt,
		},
		"missing artifact provider": {
			service: scriptVersionService(t, invalidSchema.ID, nil),
			wantErr: ErrArtifactStoreUnavailable,
		},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := test.service.GetScriptVersion(context.Background(), "n_script", 1, "p_test"); !errors.Is(err, test.wantErr) {
				t.Fatalf("script read error = %v, want %v", err, test.wantErr)
			}
		})
	}
}
