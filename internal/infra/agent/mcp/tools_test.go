package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"audio-ui/internal/domain/valueobject"
)

func newToolServer(t *testing.T, mutate func(*Options)) *Server {
	t.Helper()
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(filepath.Join(workspace, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "notes.md"), []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	options := Options{
		Root: root, Workspace: workspace, ChainID: valueobject.DefaultChainID, DisplayID: "s1",
		QuestionGate: &fakeQuestions{}, Token: "fixed-token",
	}
	if mutate != nil {
		mutate(&options)
	}
	server, err := New(options)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return server
}

func TestGetChainReturnsOptions(t *testing.T) {
	server := newToolServer(t, nil)
	_, out, err := server.getChain(context.Background(), nil, GetChainInput{})
	if err != nil {
		t.Fatalf("getChain: %v", err)
	}
	if out.ChainID != valueobject.DefaultChainID || out.DisplayID != "s1" || out.Workspace == "" {
		t.Fatalf("output = %+v", out)
	}
}

func TestReplyForwardsMessage(t *testing.T) {
	var received string
	server := newToolServer(t, func(o *Options) { o.OnReply = func(text string) { received = text } })

	_, out, err := server.reply(context.Background(), nil, ReplyInput{Text: "完成"})
	if err != nil || !out.Accepted {
		t.Fatalf("reply: out=%+v err=%v", out, err)
	}
	if received != "完成" {
		t.Fatalf("callback text = %q", received)
	}

	_, _, err = server.reply(context.Background(), nil, ReplyInput{Kind: "graph", Text: "x"})
	if err == nil || !strings.Contains(err.Error(), "UNSUPPORTED_IN_M1") {
		t.Fatalf("graph kind error = %v", err)
	}

	_, _, err = server.reply(context.Background(), nil, ReplyInput{Text: "   "})
	if err == nil {
		t.Fatal("empty reply accepted")
	}

	_, out, err = server.reply(context.Background(), nil, ReplyInput{Payload: map[string]any{"text": "payload"}})
	if err != nil || !out.Accepted || received != "payload" {
		t.Fatalf("payload reply: out=%+v err=%v received=%q", out, err, received)
	}
}

func TestReadFileAndGuards(t *testing.T) {
	server := newToolServer(t, nil)
	_, out, err := server.readFile(context.Background(), nil, ReadFileInput{Path: "notes.md"})
	if err != nil {
		t.Fatalf("readFile: %v", err)
	}
	if out.Content != "hello world" || out.Bytes != 11 || out.Truncated {
		t.Fatalf("output = %+v", out)
	}

	if _, _, err := server.readFile(context.Background(), nil, ReadFileInput{Path: "../escape.txt"}); err == nil {
		t.Fatal("escape path accepted")
	}
	if _, _, err := server.readFile(context.Background(), nil, ReadFileInput{Path: "sub"}); err == nil {
		t.Fatal("directory accepted as file")
	}
	if _, _, err := server.readFile(context.Background(), nil, ReadFileInput{Path: "missing.txt"}); err == nil {
		t.Fatal("missing file accepted")
	}

	large := strings.Repeat("x", 256*1024+10)
	if err := os.WriteFile(filepath.Join(server.options.Workspace, "big.txt"), []byte(large), 0o644); err != nil {
		t.Fatal(err)
	}
	_, truncated, err := server.readFile(context.Background(), nil, ReadFileInput{Path: "big.txt"})
	if err != nil {
		t.Fatalf("readFile big: %v", err)
	}
	if !truncated.Truncated || truncated.Bytes != 256*1024 {
		t.Fatalf("truncation = %+v", truncated)
	}
}

func TestListDirAndStat(t *testing.T) {
	server := newToolServer(t, nil)
	_, out, err := server.listDir(context.Background(), nil, ListDirInput{Path: "."})
	if err != nil {
		t.Fatalf("listDir: %v", err)
	}
	names := map[string]bool{}
	for _, entry := range out.Entries {
		names[entry.Name] = true
	}
	if !names["notes.md"] || !names["sub"] {
		t.Fatalf("entries = %+v", out.Entries)
	}
	if _, _, err := server.listDir(context.Background(), nil, ListDirInput{Path: "../"}); err == nil {
		t.Fatal("escape path accepted by list_dir")
	}

	_, stat, err := server.stat(context.Background(), nil, StatInput{Path: "sub"})
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !stat.Dir {
		t.Fatalf("stat = %+v", stat)
	}
	_, stat, err = server.stat(context.Background(), nil, StatInput{Path: "notes.md"})
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if stat.Dir || stat.Size != 11 || stat.ModTime == "" {
		t.Fatalf("stat = %+v", stat)
	}
}

func TestAskUserValidation(t *testing.T) {
	server := newToolServer(t, nil)

	if _, _, err := server.askUser(context.Background(), nil, AskUserInput{Question: "  "}); err == nil {
		t.Fatal("empty question accepted")
	}
	if _, _, err := server.askUser(context.Background(), nil, AskUserInput{Question: "pick"}); err == nil {
		t.Fatal("question without options or allowOther accepted")
	}

	_, out, err := server.askUser(context.Background(), nil, AskUserInput{Question: "pick", Options: []string{"A"}})
	if err != nil || out.Answer != "A" || out.Via != "option" {
		t.Fatalf("askUser = %+v err=%v", out, err)
	}

	noGate := newToolServer(t, func(o *Options) { o.QuestionGate = nil })
	_, _, err = noGate.askUser(context.Background(), nil, AskUserInput{Question: "pick", Options: []string{"A"}})
	if err == nil || !strings.Contains(err.Error(), "QUESTION_TIMEOUT") {
		t.Fatalf("missing gate error = %v", err)
	}
}

func TestWriteConfigProducesHttpEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "mcp.json")
	const url = "http://127.0.0.1:37999/mcp"
	if err := WriteConfig(path, url, "fixed-token"); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Servers map[string]struct {
			Type       string            `json:"type"`
			URL        string            `json:"url"`
			Headers    map[string]string `json:"headers"`
			AlwaysLoad bool              `json:"alwaysLoad"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("config json: %v", err)
	}
	entry, ok := config.Servers["audioui"]
	if !ok {
		t.Fatalf("missing audioui entry: %s", data)
	}
	if entry.Type != "http" || entry.URL != url || !entry.AlwaysLoad {
		t.Fatalf("entry = %+v", entry)
	}
	if entry.Headers["Authorization"] != "Bearer fixed-token" {
		t.Fatalf("headers = %+v", entry.Headers)
	}
}

func TestTrimLineKeepsRunesIntact(t *testing.T) {
	line := strings.Repeat("中", 300)
	got := trimLine(line)
	if len([]rune(got)) != 240 {
		t.Fatalf("runes = %d, want 240", len([]rune(got)))
	}
	if strings.HasSuffix(got, "�") {
		t.Fatal("trimLine split a multi-byte character")
	}
	if got := trimLine(" 短文本 "); got != "短文本" {
		t.Fatalf("trim = %q", got)
	}
}

func TestServerUsesProvidedToken(t *testing.T) {
	server := newToolServer(t, nil)
	if server.Token() != "fixed-token" {
		t.Fatalf("token = %q", server.Token())
	}
	generated := newToolServer(t, func(o *Options) { o.Token = "" })
	if len(generated.Token()) != 64 {
		t.Fatalf("generated token length = %d, want 64 hex chars", len(generated.Token()))
	}
}
