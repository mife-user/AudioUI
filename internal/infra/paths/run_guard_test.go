package paths

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func runGuardFixture(t *testing.T) (string, *RunGuard) {
	t.Helper()
	tree := filepath.Join(t.TempDir(), "trees", "n_root")
	if err := os.MkdirAll(filepath.Join(tree, "steps", "n_current"), 0o755); err != nil {
		t.Fatal(err)
	}
	guard, err := NewRunGuard(tree, "n_current", []string{"n_root", "n_parent", "n_current"})
	if err != nil {
		t.Fatal(err)
	}
	return tree, guard
}

func TestRunGuardReadAndWriteScope(t *testing.T) {
	tree, guard := runGuardFixture(t)

	for _, input := range []string{
		"steps/n_root/root.txt",
		"steps/n_parent/parent.txt",
		"steps/n_current/current.txt",
		filepath.Join(tree, "steps", "n_current", "absolute.txt"),
	} {
		if _, err := guard.ResolveRead(input); err != nil {
			t.Fatalf("ResolveRead(%q) error = %v", input, err)
		}
	}
	for _, input := range []string{".", "steps/n_sibling/secret.txt", "../other-tree/steps/n_current/a.txt"} {
		if _, err := guard.ResolveRead(input); !errors.Is(err, ErrPathDenied) {
			t.Fatalf("ResolveRead(%q) error = %v, want PATH_DENIED", input, err)
		}
	}

	if _, err := guard.ResolveWrite("steps/n_current/output.md"); err != nil {
		t.Fatalf("ResolveWrite(current) error = %v", err)
	}
	for _, input := range []string{"output.md", "steps/n_parent/output.md", "steps/n_sibling/output.md"} {
		if _, err := guard.ResolveWrite(input); !errors.Is(err, ErrPathDenied) {
			t.Fatalf("ResolveWrite(%q) error = %v, want PATH_DENIED", input, err)
		}
	}

	// Bash remains a cooperative tree-wide boundary in M2 (RM-017).
	if _, err := guard.Resolve("steps/n_sibling/readme.md"); err != nil {
		t.Fatalf("Resolve(sibling for Bash boundary) error = %v", err)
	}
}

func TestRunGuardRequiresActiveNodeAndValidPath(t *testing.T) {
	tree := t.TempDir()
	cases := []struct {
		name    string
		current string
		path    []string
	}{
		{name: "no active node", path: []string{"n_root"}},
		{name: "empty path", current: "n_current"},
		{name: "wrong tail", current: "n_current", path: []string{"n_root", "n_other"}},
		{name: "duplicate", current: "n_root", path: []string{"n_root", "n_root"}},
		{name: "invalid id", current: "n_current", path: []string{"../escape", "n_current"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewRunGuard(tree, tc.current, tc.path)
			if !errors.Is(err, ErrWorkspaceScopeInvalid) {
				t.Fatalf("NewRunGuard error = %v, want WORKSPACE_SCOPE_INVALID", err)
			}
		})
	}
	if _, err := NewRunGuard(filepath.Join(tree, "missing"), "n_current", []string{"n_root", "n_current"}); !errors.Is(err, ErrWorkspaceScopeInvalid) {
		t.Fatalf("missing tree error = %v", err)
	}
}

func TestRunGuardRejectsJunctionInScopedPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junction test is Windows-only")
	}
	tree, guard := runGuardFixture(t)
	outside := t.TempDir()
	link := filepath.Join(tree, "steps", "n_current", "linked")
	if err := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", link, outside).Run(); err != nil {
		t.Skipf("cannot create junction: %v", err)
	}
	if _, err := guard.ResolveWrite("steps/n_current/linked/output.md"); !errors.Is(err, ErrPathDenied) {
		t.Fatalf("junction write error = %v, want PATH_DENIED", err)
	}
}
