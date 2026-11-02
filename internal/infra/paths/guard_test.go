package paths

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGuardResolve(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	guard, err := NewGuard(root, workspace)
	if err != nil {
		t.Fatal(err)
	}
	got, err := guard.Resolve("notes/a.md")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != filepath.Join(workspace, "notes", "a.md") {
		t.Fatalf("Resolve() = %q", got)
	}
	if _, err := guard.Resolve("."); err != nil {
		t.Fatalf("Resolve(.) error = %v", err)
	}
}

func TestGuardAllowsAbsoluteInsideWorkspace(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	guard, err := NewGuard(root, workspace)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(workspace, "notes", "a.md")
	got, err := guard.Resolve(target)
	if err != nil {
		t.Fatalf("Resolve(%q) error = %v", target, err)
	}
	if got != target {
		t.Fatalf("Resolve() = %q, want %q", got, target)
	}
	if _, err := guard.Resolve(filepath.Join(root, "outside.md")); !errors.Is(err, ErrPathDenied) {
		t.Fatalf("outside absolute path error = %v", err)
	}
}

func TestGuardRejectsJunction(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junction test is Windows-only")
	}
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(workspace, "link")
	if err := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", link, outside).Run(); err != nil {
		t.Skipf("cannot create junction: %v", err)
	}
	guard, err := NewGuard(root, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := guard.Resolve("link/secret.txt"); !errors.Is(err, ErrPathDenied) {
		t.Fatalf("junction path error = %v, want PATH_DENIED", err)
	}
}

func TestGuardRejectsEscapes(t *testing.T) {
	root := t.TempDir()
	guard, err := NewGuard(root, filepath.Join(root, "workspace"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"", "../x", `C:\Windows\win.ini`, "\\\\server\\share\\x", "runtime/x", "artifacts/x"} {
		if _, err := guard.Resolve(path); !errors.Is(err, ErrPathDenied) {
			t.Fatalf("Resolve(%q) error = %v, want PATH_DENIED", path, err)
		}
	}
}
