package persistence

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/infra/paths"
)

func TestProjectLoadDiscardsUncommittedPendingTransaction(t *testing.T) {
	ctx := context.Background()
	layout := testLayout(t.TempDir())
	projects := NewProjectStore(layout)
	project := sampleProject("p_test")
	if err := projects.Create(ctx, project); err != nil {
		t.Fatal(err)
	}
	paths, err := paths.ResolveProject(layout, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	store := NewPendingStore(paths.Dir)
	pending := samplePending("pa_test")
	pending.ContextHash = "context"
	pending.AncestorHash = "ancestor"
	pending.ParamsHash = "params"
	payload := []byte(`{"unvalidated":true}`)
	transaction := entity.PendingTransaction{
		Version: valueobject.PendingTransactionFileVersion, BaseRevision: 1, TargetRevision: 2,
		Pending: pending, Payload: payload,
	}
	if err := store.SaveTransaction(ctx, transaction); err != nil {
		t.Fatal(err)
	}
	if err := store.WritePayload(ctx, pending.ID, payload); err != nil {
		t.Fatal(err)
	}

	loaded, err := projects.Load(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Revision != 1 {
		t.Fatalf("revision = %d, want uncommitted base revision", loaded.Revision)
	}
	if _, err := store.Load(ctx, pending.ID); !errors.Is(err, ErrPendingNotFound) {
		t.Fatalf("pending index error = %v, want PENDING_NOT_FOUND", err)
	}
	if _, found, err := store.LoadTransaction(ctx); err != nil || found {
		t.Fatalf("transaction remains: found=%v err=%v", found, err)
	}
	if _, err := store.GetPayload(ctx, pending.ID); err != nil {
		t.Fatalf("orphan raw payload should be harmless: %v", err)
	}
}

func TestProjectLoadCompletesCommittedPendingTransaction(t *testing.T) {
	ctx := context.Background()
	layout := testLayout(t.TempDir())
	projects := NewProjectStore(layout)
	project := sampleProject("p_test")
	zero := 0
	project.ActiveNodeID = "n_prompt"
	project.Nodes = append(project.Nodes, entity.Node{
		ID: "n_prompt", Type: valueobject.NodeTypePrompt, Title: "脚本",
		Parent: "n_root", ParentCandidate: &zero, Rounds: []entity.Round{},
	})
	if err := projects.Create(ctx, project); err != nil {
		t.Fatal(err)
	}
	projectPaths, err := paths.ResolveProject(layout, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	pendingStore := NewPendingStore(projectPaths.Dir)
	artifactStore := NewArtifactStore(projectPaths.Dir)
	rawPayload := []byte(` { "title" : "脚本", "scenes" : [{"narration":"旁白","duration":0}] } `)
	artifactPayload := []byte(`{"scenes":[{"duration":0,"narration":"旁白"}],"title":"脚本"}`)
	pending := entity.Pending{
		ID: "pa_test", RunID: "r_test", ChainID: "c_n_prompt", ChainPath: []string{"n_root", "n_prompt"},
		NodeID: "n_prompt", Kind: valueobject.OutputKindScript, Status: valueobject.PendingApplied,
		ContextHash: "context", AncestorHash: "ancestor", ParamsHash: "params",
		ArtifactID: "sha256:" + hashPayload(artifactPayload), CreatedAt: "2026-11-01T10:00:00Z",
	}
	meta := entity.ArtifactMeta{
		Kind: valueobject.OutputKindScript, NodeID: pending.NodeID, CreatedAt: pending.CreatedAt,
		SourceRunID: pending.RunID, SourcePendingID: pending.ID,
	}
	transaction := entity.PendingTransaction{
		Version: valueobject.PendingTransactionFileVersion, BaseRevision: 1, TargetRevision: 2,
		Pending: pending, Payload: rawPayload, ArtifactPayload: artifactPayload, ArtifactMeta: &meta,
	}
	if err := pendingStore.SaveTransaction(ctx, transaction); err != nil {
		t.Fatal(err)
	}
	project.Revision = 2
	project.Nodes[1].Locked = true
	project.Nodes[1].Rounds = []entity.Round{{
		N: 1, Kind: pending.Kind, ArtifactID: pending.ArtifactID, PendingID: pending.ID, RunID: pending.RunID,
	}}
	if err := projects.Save(ctx, project); err != nil {
		t.Fatal(err)
	}

	loaded, err := projects.Load(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Revision != 2 || !loaded.Nodes[1].Locked || len(loaded.Nodes[1].Rounds) != 1 {
		t.Fatalf("project commit was not preserved: %+v", loaded)
	}
	stored, err := pendingStore.Load(ctx, pending.ID)
	if err != nil || stored.Status != valueobject.PendingApplied || stored.ArtifactID != pending.ArtifactID {
		t.Fatalf("pending recovery = %+v err=%v", stored, err)
	}
	storedPayload, err := pendingStore.GetPayload(ctx, pending.ID)
	if err != nil || string(storedPayload) != string(rawPayload) {
		t.Fatalf("raw payload = %q err=%v", storedPayload, err)
	}
	artifact, payload, err := artifactStore.Get(ctx, pending.ArtifactID)
	if err != nil || artifact.Kind != valueobject.OutputKindScript || string(payload) != string(artifactPayload) {
		t.Fatalf("artifact recovery = %+v payload=%s err=%v", artifact, payload, err)
	}
	if _, found, err := pendingStore.LoadTransaction(ctx); err != nil || found {
		t.Fatalf("transaction remains: found=%v err=%v", found, err)
	}
	if _, err := projects.Load(ctx, project.ID); err != nil {
		t.Fatalf("repeated recovery: %v", err)
	}
}

func TestProjectLoadRejectsAmbiguousOrCorruptPendingTransaction(t *testing.T) {
	ctx := context.Background()
	t.Run("revision conflict", func(t *testing.T) {
		layout := testLayout(t.TempDir())
		projects := NewProjectStore(layout)
		project := sampleProject("p_test")
		if err := projects.Create(ctx, project); err != nil {
			t.Fatal(err)
		}
		projectPaths, err := paths.ResolveProject(layout, project.ID)
		if err != nil {
			t.Fatal(err)
		}
		store := NewPendingStore(projectPaths.Dir)
		pending := samplePending("pa_test")
		pending.ContextHash, pending.AncestorHash, pending.ParamsHash = "context", "ancestor", "params"
		if err := store.SaveTransaction(ctx, entity.PendingTransaction{
			Version: valueobject.PendingTransactionFileVersion, BaseRevision: 1, TargetRevision: 2,
			Pending: pending, Payload: []byte(`{}`),
		}); err != nil {
			t.Fatal(err)
		}
		project.Revision = 3
		if err := projects.Save(ctx, project); err != nil {
			t.Fatal(err)
		}
		if _, err := projects.Load(ctx, project.ID); !errors.Is(err, ErrPendingTransactionConflict) {
			t.Fatalf("Load error = %v, want PENDING_TRANSACTION_CONFLICT", err)
		}
	})

	t.Run("corrupt journal", func(t *testing.T) {
		layout := testLayout(t.TempDir())
		projects := NewProjectStore(layout)
		project := sampleProject("p_test")
		if err := projects.Create(ctx, project); err != nil {
			t.Fatal(err)
		}
		projectPaths, err := paths.ResolveProject(layout, project.ID)
		if err != nil {
			t.Fatal(err)
		}
		journal := filepath.Join(projectPaths.Dir, "pending.txn.json")
		if err := os.WriteFile(journal, []byte("{"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := projects.Load(ctx, project.ID); !errors.Is(err, ErrCorruptPendingTransaction) {
			t.Fatalf("Load error = %v, want PENDING_TRANSACTION_CORRUPT", err)
		}
		contents, err := os.ReadFile(journal)
		if err != nil || string(contents) != "{" {
			t.Fatalf("corrupt journal was modified: %q err=%v", contents, err)
		}
	})
}
