package persistence

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/valueobject"
)

func TestPendingStoreRoundTripPreservesInvalidPayload(t *testing.T) {
	dir := t.TempDir()
	store := NewPendingStore(dir)
	pending := samplePending("pa_test")
	payload := []byte(`{"scenes":[`)
	if err := store.Save(context.Background(), pending, payload); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(context.Background(), "pa_test")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Payload != "pending/pa_test.json" {
		t.Fatalf("payload path = %q", loaded.Payload)
	}
	got, err := store.GetPayload(context.Background(), "pa_test")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload = %q, want %q", got, payload)
	}
	list, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "pa_test" {
		t.Fatalf("list = %+v", list)
	}
}

func TestPendingStoreUpdateStatus(t *testing.T) {
	dir := t.TempDir()
	store := NewPendingStore(dir)
	pending := samplePending("pa_test")
	if err := store.Save(context.Background(), pending, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	pending.Status = valueobject.PendingApplied
	pending.ArtifactID = "sha256:abc"
	if err := store.Save(context.Background(), pending, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	list, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Status != valueobject.PendingApplied || list[0].ArtifactID != "sha256:abc" {
		t.Fatalf("list = %+v", list)
	}
}

func TestPendingStorePayloadTooLarge(t *testing.T) {
	dir := t.TempDir()
	store := NewPendingStore(dir)
	payload := make([]byte, valueobject.MaxPendingPayloadSize+1)
	err := store.Save(context.Background(), samplePending("pa_test"), payload)
	if !errors.Is(err, ErrPendingTooLarge) {
		t.Fatalf("error = %v, want ErrPendingTooLarge", err)
	}
	if _, err := store.Load(context.Background(), "pa_test"); !errors.Is(err, ErrPendingNotFound) {
		t.Fatalf("Load error = %v, want ErrPendingNotFound", err)
	}
}

func TestPendingStoreCorruptIndex(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "pending.json"), "{")
	store := NewPendingStore(dir)
	if _, err := store.List(context.Background()); !errors.Is(err, ErrCorruptPending) {
		t.Fatalf("error = %v, want ErrCorruptPending", err)
	}
}

func TestPendingStoreUnsupportedVersion(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "pending.json"), `{"version":2,"pending":[]}`)
	if _, err := NewPendingStore(dir).List(context.Background()); !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("error = %v, want ErrUnsupportedVersion", err)
	}
}

func TestPendingStoreDuplicateIDsAreCorrupt(t *testing.T) {
	dir := t.TempDir()
	pending := samplePending("pa_test")
	writeJSONFileForTest(t, filepath.Join(dir, "pending.json"), entity.PendingFile{
		Version: valueobject.PendingFileVersion,
		Pending: []entity.Pending{pending, pending},
	})
	if _, err := NewPendingStore(dir).List(context.Background()); !errors.Is(err, ErrCorruptPending) {
		t.Fatalf("error = %v, want ErrCorruptPending", err)
	}
}

func TestPendingStoreNotFoundAndInvalidID(t *testing.T) {
	store := NewPendingStore(t.TempDir())
	if _, err := store.Load(context.Background(), "pa_missing"); !errors.Is(err, ErrPendingNotFound) {
		t.Fatalf("error = %v, want ErrPendingNotFound", err)
	}
	if _, err := store.GetPayload(context.Background(), "pa_missing"); !errors.Is(err, ErrPendingNotFound) {
		t.Fatalf("error = %v, want ErrPendingNotFound", err)
	}
	if _, err := store.Load(context.Background(), "../escape"); !errors.Is(err, ErrPendingNotFound) {
		t.Fatalf("error = %v, want ErrPendingNotFound", err)
	}
}

func TestPendingStoreRejectsInvalidRecord(t *testing.T) {
	store := NewPendingStore(t.TempDir())
	pending := samplePending("pa_test")
	pending.Kind = "unknown"
	if err := store.Save(context.Background(), pending, nil); !errors.Is(err, ErrInvalidPending) {
		t.Fatalf("error = %v, want ErrInvalidPending", err)
	}
	pending = samplePending("pa_test")
	pending.CreatedAt = ""
	if err := store.Save(context.Background(), pending, nil); !errors.Is(err, ErrInvalidPending) {
		t.Fatalf("error = %v, want ErrInvalidPending", err)
	}
}

func TestPendingStoreNormalizesPayloadPath(t *testing.T) {
	dir := t.TempDir()
	store := NewPendingStore(dir)
	pending := samplePending("pa_test")
	pending.Payload = "../../outside.json"
	if err := store.Save(context.Background(), pending, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(context.Background(), "pa_test")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Payload != "pending/pa_test.json" {
		t.Fatalf("payload = %q", loaded.Payload)
	}
	if _, err := os.Stat(filepath.Join(dir, "outside.json")); !os.IsNotExist(err) {
		t.Fatalf("payload escaped project dir: %v", err)
	}
}

func TestPendingStoreAllowsEmptyPayload(t *testing.T) {
	dir := t.TempDir()
	store := NewPendingStore(dir)
	if err := store.Save(context.Background(), samplePending("pa_empty"), nil); err != nil {
		t.Fatal(err)
	}
	payload, err := store.GetPayload(context.Background(), "pa_empty")
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) != 0 {
		t.Fatalf("payload = %q", payload)
	}
}
