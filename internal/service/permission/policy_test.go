package permission

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/infra/paths"
)

func TestPolicyTiers(t *testing.T) {
	root := t.TempDir()
	guard, err := paths.NewGuard(root, filepath.Join(root, "workspace"))
	if err != nil {
		t.Fatal(err)
	}
	policy := &Policy{Paths: guard}
	cases := []struct {
		tool string
		in   map[string]any
		want port.PolicyDecision
	}{
		{"Write", map[string]any{"file_path": "notes.md"}, port.PolicyConfirm},
		{"Edit", map[string]any{"file_path": "notes.md"}, port.PolicyConfirm},
		{"Read", map[string]any{"file_path": "notes.md"}, port.PolicyAllow},
		{"Bash", map[string]any{"command": "ls -la"}, port.PolicyAllow},
		{"Bash", map[string]any{"command": "git status"}, port.PolicyAllow},
		{"Bash", map[string]any{"command": "python script.py"}, port.PolicyConfirm},
		{"Bash", map[string]any{"command": "rm -rf C:\\"}, port.PolicyDeny},
		{"Bash", map[string]any{"command": "cat ../secret"}, port.PolicyDeny},
		{"Bash", map[string]any{"command": "cat ~/.ssh/id_rsa"}, port.PolicyDeny},
		{"Bash", map[string]any{"command": "cat C:/Users/asus/.ssh/id_rsa"}, port.PolicyDeny},
		{"Bash", map[string]any{"command": "cat runtime/secret"}, port.PolicyDeny},
		{"Bash", map[string]any{"command": "find . -fprintf C:/Users/asus/out.txt %p"}, port.PolicyDeny},
		{"Bash", map[string]any{"command": "find . -fprintf out.txt %p"}, port.PolicyConfirm},
		{"Bash", map[string]any{"command": "git branch -D important"}, port.PolicyConfirm},
		{"Bash", map[string]any{"command": "git diff --output=C:/Users/asus/out.diff"}, port.PolicyDeny},
		{"mcp__audioui__grep", map[string]any{"pattern": "x", "path": "."}, port.PolicyAllow},
		{"mcp__audioui__read_file", map[string]any{"path": "notes.md"}, port.PolicyAllow},
		{"mcp__audioui__ask_user", map[string]any{}, port.PolicyAllow},
		{"mcp__audioui__get_chain", map[string]any{}, port.PolicyAllow},
		{"mcp__audioui__get_graph", map[string]any{}, port.PolicyAllow},
		{"mcp__audioui__get_node", map[string]any{"nodeId": "n_target"}, port.PolicyAllow},
		{"mcp__audioui__list_node_types", map[string]any{}, port.PolicyAllow},
		{"mcp__audioui__validate", map[string]any{"kind": "script"}, port.PolicyAllow},
		{"mcp__audioui__read_artifact", map[string]any{"artifactId": "sha256:..."}, port.PolicyAllow},
		{"Unknown", map[string]any{}, port.PolicyConfirm},
	}
	for _, item := range cases {
		got := policy.Check(context.Background(), item.tool, item.in)
		if got.Decision != item.want {
			t.Fatalf("%s %v: got %s (%s), want %s", item.tool, item.in, got.Decision, got.Reason, item.want)
		}
	}
}

func TestPolicyRejectsWriteEscape(t *testing.T) {
	root := t.TempDir()
	guard, err := paths.NewGuard(root, filepath.Join(root, "workspace"))
	if err != nil {
		t.Fatal(err)
	}
	got := (&Policy{Paths: guard}).Check(context.Background(), "Write", map[string]any{"file_path": "../escape.md"})
	if got.Decision != port.PolicyDeny || got.Tier != valueobject.PermissionHardDeny {
		t.Fatalf("got %+v", got)
	}
}

func TestPolicyEnforcesRunScopedBuiltinFileTools(t *testing.T) {
	root := t.TempDir()
	tree := filepath.Join(root, "trees", "n_root")
	if err := os.MkdirAll(filepath.Join(tree, "steps", "n_current"), 0o755); err != nil {
		t.Fatal(err)
	}
	guard, err := paths.NewRunGuard(tree, "n_current", []string{"n_root", "n_parent", "n_current"})
	if err != nil {
		t.Fatal(err)
	}
	policy := &Policy{Paths: guard}
	cases := []struct {
		name string
		tool string
		args map[string]any
		want port.PolicyDecision
	}{
		{name: "read current ancestor", tool: "Read", args: map[string]any{"file_path": "steps/n_parent/notes.md"}, want: port.PolicyAllow},
		{name: "read current step", tool: "Read", args: map[string]any{"file_path": "steps/n_current/notes.md"}, want: port.PolicyAllow},
		{name: "read sibling", tool: "Read", args: map[string]any{"file_path": "steps/n_sibling/secret.md"}, want: port.PolicyDeny},
		{name: "write current step", tool: "Write", args: map[string]any{"file_path": "steps/n_current/out.md"}, want: port.PolicyConfirm},
		{name: "write ancestor", tool: "Edit", args: map[string]any{"file_path": "steps/n_parent/out.md"}, want: port.PolicyDeny},
		{name: "write sibling", tool: "Write", args: map[string]any{"file_path": "steps/n_sibling/out.md"}, want: port.PolicyDeny},
		{name: "logical overlay MCP read", tool: "mcp__audioui__read_file", args: map[string]any{"path": "runtime-fonts/font.txt"}, want: port.PolicyAllow},
		{name: "unknown MCP read tool", tool: "mcp__audioui__read", args: map[string]any{"path": "secret.md"}, want: port.PolicyConfirm},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := policy.Check(context.Background(), tc.tool, tc.args)
			if got.Decision != tc.want {
				t.Fatalf("Check(%s, %v) = %+v, want %s", tc.tool, tc.args, got, tc.want)
			}
			if (tc.want == port.PolicyDeny) != (got.Tier == valueobject.PermissionHardDeny) {
				t.Fatalf("Check(%s) tier = %s, decision = %s", tc.tool, got.Tier, got.Decision)
			}
		})
	}
}
