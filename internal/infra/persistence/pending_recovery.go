package persistence

import (
	"context"
	"fmt"
)

func recoverPendingTransaction(ctx context.Context, projectDir string, revision int64) error {
	store := NewPendingStore(projectDir)
	transaction, found, err := store.LoadTransaction(ctx)
	if err != nil || !found {
		return err
	}
	if revision == transaction.BaseRevision {
		if err := store.DeleteTransaction(ctx); err != nil {
			return fmt.Errorf("discard uncommitted pending transaction: %w", err)
		}
		return nil
	}
	if revision != transaction.TargetRevision {
		return fmt.Errorf("%w: project revision %d is neither %d nor %d", ErrPendingTransactionConflict, revision, transaction.BaseRevision, transaction.TargetRevision)
	}
	if transaction.ArtifactMeta != nil {
		artifact, err := NewArtifactStore(projectDir).Put(ctx, transaction.ArtifactPayload, *transaction.ArtifactMeta)
		if err != nil {
			return fmt.Errorf("recover pending artifact: %w", err)
		}
		if artifact.ID != transaction.Pending.ArtifactID {
			return fmt.Errorf("%w: recovered artifact id %s does not match %s", ErrPendingTransactionConflict, artifact.ID, transaction.Pending.ArtifactID)
		}
	}
	if err := store.Save(ctx, transaction.Pending, transaction.Payload); err != nil {
		return fmt.Errorf("recover pending index: %w", err)
	}
	if err := store.DeleteTransaction(ctx); err != nil {
		return fmt.Errorf("finish pending recovery: %w", err)
	}
	return nil
}
