package paths

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"audio-ui/internal/domain/valueobject"
)

func TestRunConfigStoreUsesPerRunDirectoryAndCleansIt(t *testing.T) {
	root := t.TempDir()
	layout := valueobject.Layout{Root: root, AgentDir: filepath.Join(root, "data", "agent")}
	store := NewRunConfigStore(layout)
	path, err := store.Prepare(context.Background(), "p_test", "r_test")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(layout.AgentDir, "p_test", "r_test", "mcp.json")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	if err := os.WriteFile(path, []byte(`{"token":"secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Cleanup(context.Background(), "p_test", "r_test"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatalf("per-run directory remains, stat error = %v", err)
	}
}

func TestRunConfigStoreRejectsInvalidIDs(t *testing.T) {
	store := NewRunConfigStore(valueobject.Layout{Root: t.TempDir(), AgentDir: filepath.Join(t.TempDir(), "agent")})
	if _, err := store.Prepare(context.Background(), "../outside", "r_test"); err == nil {
		t.Fatal("invalid project id was accepted")
	}
	if err := store.Cleanup(context.Background(), "p_test", "../outside"); err == nil {
		t.Fatal("invalid run id was accepted")
	}
}
