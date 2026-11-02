package persistence

import (
	"context"
	"os"
	"testing"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/infra/paths"
)

func TestCommittedPendingRecoveryBlocksUntilIndexCanBeWritten(t *testing.T) {
	ctx := context.Background()
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
	pendingStore := NewPendingStore(projectPaths.Dir)
	pending := samplePending("pa_test")
	pending.Kind = valueobject.OutputKindTopics
	pending.ContextHash, pending.AncestorHash, pending.ParamsHash = "context", "ancestor", "params"
	payload := []byte("{invalid raw output")
	if err := pendingStore.SaveTransaction(ctx, entity.PendingTransaction{
		Version: valueobject.PendingTransactionFileVersion, BaseRevision: 1, TargetRevision: 2,
		Pending: pending, Payload: payload,
	}); err != nil {
		t.Fatal(err)
	}
	project.Revision = 2
	if err := projects.Save(ctx, project); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(projectPaths.PendingFile); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(projectPaths.PendingFile, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := projects.Load(ctx, project.ID); err == nil {
		t.Fatal("recovery exposed project while pending index could not be written")
	}
	if _, found, err := pendingStore.LoadTransaction(ctx); err != nil || !found {
		t.Fatalf("journal was not retained: found=%v err=%v", found, err)
	}
	if err := os.Remove(projectPaths.PendingFile); err != nil {
		t.Fatal(err)
	}
	if _, err := projects.Load(ctx, project.ID); err != nil {
		t.Fatalf("recovery retry failed: %v", err)
	}
	stored, err := pendingStore.Load(ctx, pending.ID)
	if err != nil || stored.Status != valueobject.PendingPending {
		t.Fatalf("recovered pending=%+v err=%v", stored, err)
	}
	storedPayload, err := pendingStore.GetPayload(ctx, pending.ID)
	if err != nil || string(storedPayload) != string(payload) {
		t.Fatalf("recovered raw payload=%q err=%v", storedPayload, err)
	}
	if _, found, err := pendingStore.LoadTransaction(ctx); err != nil || found {
		t.Fatalf("journal remains after recovery: found=%v err=%v", found, err)
	}
}
