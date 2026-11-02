package context

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"audio-ui/internal/domain/port"
)

func TestManifestSortedWithoutReadingContents(t *testing.T) {
	f := newFixture()
	first, err := collectManifest(context.Background(), f.workspace)
	if err != nil {
		t.Fatal(err)
	}
	want := []WorkspaceFile{{Path: "a.txt", Size: 3}, {Path: "docs/note.md", Size: 8}, {Path: "z.bin", Size: 12}}
	if !reflect.DeepEqual(first.Files, want) || first.Truncated || f.workspace.reads != 0 {
		t.Fatalf("manifest = %+v, content reads = %d", first, f.workspace.reads)
	}
	listing := f.workspace.dirs["."]
	listing.Entries[0], listing.Entries[2] = listing.Entries[2], listing.Entries[0]
	f.workspace.dirs["."] = listing
	second, err := collectManifest(context.Background(), f.workspace)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("manifest changed with listing order: %+v %v", second, err)
	}
}

func TestManifestLimitsAndInvalidEntries(t *testing.T) {
	f := newFixture()
	entries := make([]port.WorkspaceEntry, MaxManifestEntries+1)
	for i := range entries {
		entries[i] = port.WorkspaceEntry{Name: fmt.Sprintf("f_%05d.bin", i), Size: int64(i)}
	}
	f.workspace.dirs = map[string]port.WorkspaceList{".": {Entries: entries}}
	result, err := collectManifest(context.Background(), f.workspace)
	if err != nil || !result.Truncated || len(result.Files) != MaxManifestEntries {
		t.Fatalf("limited manifest: files=%d truncated=%v err=%v", len(result.Files), result.Truncated, err)
	}
	f.workspace.dirs = map[string]port.WorkspaceList{".": {Entries: []port.WorkspaceEntry{{Name: "../outside", Size: 1}}}}
	if _, err := collectManifest(context.Background(), f.workspace); !errors.Is(err, ErrWorkspaceScope) {
		t.Fatalf("invalid name: %v", err)
	}
	f.workspace.dirs = map[string]port.WorkspaceList{}
	if _, err := collectManifest(context.Background(), f.workspace); err == nil {
		t.Fatal("listing failure was silently ignored")
	}
}
