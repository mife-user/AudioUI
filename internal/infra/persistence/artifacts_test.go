package persistence

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/infra/paths"
)

func sampleArtifactMeta() entity.ArtifactMeta {
	return entity.ArtifactMeta{
		Kind:        valueobject.OutputKindScript,
		NodeID:      "n_root",
		CreatedAt:   "2026-11-01T10:00:00Z",
		SourceRunID: "r_test",
	}
}

func TestArtifactStorePutGet(t *testing.T) {
	dir := t.TempDir()
	store := NewArtifactStore(dir)
	payload := []byte(`{"title":"示例"}`)
	artifact, err := store.Put(context.Background(), payload, sampleArtifactMeta())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(artifact.ID, "sha256:") || len(artifact.ID) != len("sha256:")+64 {
		t.Fatalf("artifact id = %q", artifact.ID)
	}
	if artifact.Path != "artifacts/"+strings.TrimPrefix(artifact.ID, "sha256:")+"/payload.json" {
		t.Fatalf("artifact path = %q", artifact.Path)
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(artifact.Path))); err != nil {
		t.Fatalf("payload missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "artifacts", strings.TrimPrefix(artifact.ID, "sha256:"), "meta.json")); err != nil {
		t.Fatalf("meta missing: %v", err)
	}
	got, gotPayload, err := store.Get(context.Background(), artifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != artifact || string(gotPayload) != string(payload) {
		t.Fatalf("got = %+v payload = %q", got, gotPayload)
	}
	exists, err := store.Exists(context.Background(), artifact.ID)
	if err != nil || !exists {
		t.Fatalf("exists = %v err = %v", exists, err)
	}
}

func TestArtifactStoreDeduplicatesPayload(t *testing.T) {
	dir := t.TempDir()
	store := NewArtifactStore(dir)
	payload := []byte(`{"title":"示例"}`)
	first, err := store.Put(context.Background(), payload, sampleArtifactMeta())
	if err != nil {
		t.Fatal(err)
	}
	meta := sampleArtifactMeta()
	meta.CreatedAt = "2026-11-02T10:00:00Z"
	meta.NodeID = "n_other"
	second, err := store.Put(context.Background(), payload, meta)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Fatalf("ids differ: %q vs %q", first.ID, second.ID)
	}
	if second.CreatedAt != first.CreatedAt || second.NodeID != first.NodeID {
		t.Fatalf("artifact meta was overwritten: %+v", second)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "artifacts"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("artifact dirs = %d, want 1", len(entries))
	}
}

func TestArtifactStoreDifferentPayloadsDiffer(t *testing.T) {
	store := NewArtifactStore(t.TempDir())
	first, err := store.Put(context.Background(), []byte(`{"a":1}`), sampleArtifactMeta())
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Put(context.Background(), []byte(`{"a":2}`), sampleArtifactMeta())
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatalf("ids are equal: %q", first.ID)
	}
}

func TestArtifactStoreDetectsTampering(t *testing.T) {
	dir := t.TempDir()
	store := NewArtifactStore(dir)
	artifact, err := store.Put(context.Background(), []byte(`{"a":1}`), sampleArtifactMeta())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "artifacts", strings.TrimPrefix(artifact.ID, "sha256:"), "payload.json")
	if err := os.WriteFile(path, []byte(`{"a":2}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Get(context.Background(), artifact.ID); !errors.Is(err, ErrArtifactCorrupt) {
		t.Fatalf("error = %v, want ErrArtifactCorrupt", err)
	}
	if _, err := store.Put(context.Background(), []byte(`{"a":1}`), sampleArtifactMeta()); !errors.Is(err, ErrArtifactCorrupt) {
		t.Fatalf("Put error = %v, want ErrArtifactCorrupt", err)
	}
}

func TestArtifactStoreCorruptMeta(t *testing.T) {
	dir := t.TempDir()
	store := NewArtifactStore(dir)
	artifact, err := store.Put(context.Background(), []byte(`{"a":1}`), sampleArtifactMeta())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "artifacts", strings.TrimPrefix(artifact.ID, "sha256:"), "meta.json")
	writeTestFile(t, path, "{")
	if _, _, err := store.Get(context.Background(), artifact.ID); !errors.Is(err, ErrArtifactCorrupt) {
		t.Fatalf("error = %v, want ErrArtifactCorrupt", err)
	}
}

func TestArtifactStoreNotFoundAndInvalidID(t *testing.T) {
	store := NewArtifactStore(t.TempDir())
	missing := "sha256:" + strings.Repeat("0", 64)
	if _, _, err := store.Get(context.Background(), missing); !errors.Is(err, ErrArtifactNotFound) {
		t.Fatalf("error = %v, want ErrArtifactNotFound", err)
	}
	for _, id := range []string{"", "abc", "sha256:xyz", "sha256:" + strings.Repeat("g", 64), "sha256:" + strings.Repeat("A", 64)} {
		if _, _, err := store.Get(context.Background(), id); !errors.Is(err, ErrArtifactInvalid) {
			t.Fatalf("Get(%q) error = %v, want ErrArtifactInvalid", id, err)
		}
	}
	exists, err := store.Exists(context.Background(), missing)
	if err != nil || exists {
		t.Fatalf("exists = %v err = %v", exists, err)
	}
}

func TestArtifactStoreRejectsInvalidMeta(t *testing.T) {
	store := NewArtifactStore(t.TempDir())
	meta := sampleArtifactMeta()
	meta.CreatedAt = ""
	if _, err := store.Put(context.Background(), []byte(`{}`), meta); !errors.Is(err, ErrArtifactInvalid) {
		t.Fatalf("error = %v, want ErrArtifactInvalid", err)
	}
	meta = sampleArtifactMeta()
	meta.NodeID = "../escape"
	if _, err := store.Put(context.Background(), []byte(`{}`), meta); !errors.Is(err, ErrArtifactInvalid) {
		t.Fatalf("error = %v, want ErrArtifactInvalid", err)
	}
}

func TestArtifactStoreProviderPerProject(t *testing.T) {
	layout := testLayout(t.TempDir())
	projectPaths, err := paths.ResolveProject(layout, "p_test")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(projectPaths.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := NewArtifactStoreProvider(layout)
	store, err := provider.ForProject(context.Background(), "p_test")
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := store.Put(context.Background(), []byte(`{"topics":[]}`), sampleArtifactMeta())
	if err != nil {
		t.Fatal(err)
	}
	if _, payload, err := store.Get(context.Background(), artifact.ID); err != nil || string(payload) != `{"topics":[]}` {
		t.Fatalf("payload = %q err=%v", payload, err)
	}
	if _, err := provider.ForProject(context.Background(), "../escape"); err == nil {
		t.Fatal("invalid project id accepted")
	}
}

func TestArtifactStoreLeavesNoStagingDirs(t *testing.T) {
	dir := t.TempDir()
	store := NewArtifactStore(dir)
	if _, err := store.Put(context.Background(), []byte(`{"a":1}`), sampleArtifactMeta()); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "artifacts"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".artifact-") || strings.HasPrefix(entry.Name(), ".tmp-") {
			t.Fatalf("staging dir left behind: %q", entry.Name())
		}
	}
}
