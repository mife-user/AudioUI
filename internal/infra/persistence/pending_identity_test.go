package persistence

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestPendingStoreRejectsIdentityCollisionWithoutOverwritingPayload(t *testing.T) {
	ctx := context.Background()
	store := NewPendingStore(t.TempDir())
	pending := samplePending("pa_test")
	original := []byte(`{"original":true}`)
	if err := store.Save(ctx, pending, original); err != nil {
		t.Fatal(err)
	}
	conflicting := pending
	conflicting.RunID = "r_other"
	if err := store.Save(ctx, conflicting, []byte(`{"replacement":true}`)); !errors.Is(err, ErrPendingConflict) {
		t.Fatalf("conflicting save error = %v, want PENDING_CONFLICT", err)
	}
	stored, err := store.GetPayload(ctx, pending.ID)
	if err != nil || !bytes.Equal(stored, original) {
		t.Fatalf("payload changed after conflict: %s err=%v", stored, err)
	}
}
