package mcp

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/infra/workspace"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type fakeQuestions struct{}

func (f *fakeQuestions) Ask(_ context.Context, request valueobject.QuestionRequest) (valueobject.QuestionAnswer, error) {
	return valueobject.QuestionAnswer{Answer: "A", Via: "option"}, nil
}
func (f *fakeQuestions) ResolveQuestion(string, valueobject.QuestionAnswer) error { return nil }

func TestMCPAuthToolsAndAskUser(t *testing.T) {
	root := t.TempDir()
	workspaceDir := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspaceDir, "notes.md"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	server, err := New(Options{Root: root, Workspace: workspaceDir, ChainID: valueobject.DefaultChainID, DisplayID: "s1", QuestionGate: &fakeQuestions{}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer server.Stop(context.Background())

	unauthorized := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test", Version: "1"}, nil)
	if _, err := unauthorized.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: server.URL()}, nil); err == nil {
		t.Fatal("expected unauthorized connect to fail")
	}

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test", Version: "1"}, nil)
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req = req.Clone(req.Context())
		req.Header.Set("Authorization", "Bearer "+server.Token())
		return http.DefaultTransport.RoundTrip(req)
	})}
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: server.URL(), HTTPClient: httpClient}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	names := map[string]bool{}
	for _, tool := range tools.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{"get_chain", "reply", "read_file", "list_dir", "grep", "stat", "ask_user"} {
		if !names[want] {
			t.Fatalf("missing tool %s in %v", want, names)
		}
	}

	if _, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "read_file", Arguments: map[string]any{"path": "notes.md"}}); err != nil {
		t.Fatalf("read_file: %v", err)
	}
	denied, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "read_file", Arguments: map[string]any{"path": "../x"}})
	if err != nil {
		t.Fatalf("path denial call: %v", err)
	}
	if !denied.IsError {
		t.Fatalf("expected tool error, got %+v", denied)
	}
	grepResult, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "grep", Arguments: map[string]any{"pattern": "TOPSECRET", "path": "."}})
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	if strings.Contains(fmt.Sprint(grepResult.StructuredContent), "TOPSECRET") {
		t.Fatalf("grep leaked protected dirs: %v", grepResult.StructuredContent)
	}
	if _, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "ask_user", Arguments: map[string]any{"question": "Pick", "options": []string{"A", "B"}}}); err != nil {
		t.Fatalf("ask_user: %v", err)
	}
}

func TestMCPReadToolsUseRunOverlay(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	layout := valueobject.Layout{
		Root:        root,
		DataDir:     dataDir,
		ProjectsDir: filepath.Join(dataDir, "projects"),
		RuntimeDir:  filepath.Join(root, "runtime"),
	}
	treeDir := filepath.Join(layout.ProjectsDir, "p_test", "trees", "n_root")
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(treeDir, "steps", "n_root", "root.txt"), "VISIBLE root")
	write(filepath.Join(treeDir, "steps", "n_parent", "shared.txt"), "HIDDEN parent")
	write(filepath.Join(treeDir, "steps", "n_current", "shared.txt"), "VISIBLE current")
	write(filepath.Join(treeDir, "steps", "n_sibling", "secret.txt"), "VISIBLE sibling")
	write(filepath.Join(layout.RuntimeDir, "runtime.txt"), "VISIBLE runtime")

	view, err := workspace.NewRunView(layout, "p_test", "n_root", "n_current", []string{"n_root", "n_parent", "n_current"})
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(Options{Root: root, Workspace: treeDir, WorkspaceReader: view})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, read, err := server.readFile(ctx, nil, ReadFileInput{Path: "shared.txt"})
	if err != nil || read.Content != "VISIBLE current" {
		t.Fatalf("read_file = %+v err=%v", read, err)
	}
	_, listing, err := server.listDir(ctx, nil, ListDirInput{Path: "."})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range listing.Entries {
		if entry.Name == "secret.txt" {
			t.Fatalf("list_dir leaked sibling: %+v", listing.Entries)
		}
	}
	_, matches, err := server.grep(ctx, nil, GrepInput{Pattern: "VISIBLE|HIDDEN", Path: "."})
	if err != nil {
		t.Fatal(err)
	}
	joined := fmt.Sprint(matches.Hits)
	if !strings.Contains(joined, "VISIBLE current") || !strings.Contains(joined, "VISIBLE root") || !strings.Contains(joined, "VISIBLE runtime") {
		t.Fatalf("grep missing visible layers: %+v", matches.Hits)
	}
	if strings.Contains(joined, "HIDDEN parent") || strings.Contains(joined, "VISIBLE sibling") {
		t.Fatalf("grep leaked shadowed or sibling content: %+v", matches.Hits)
	}
}
