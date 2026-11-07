package mcp

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/infra/workspace"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type Options struct {
	Root            string
	Workspace       string
	ChainID         string
	DisplayID       string
	Token           string
	Version         string
	QuestionGate    port.QuestionGate
	OnReply         func(text string)
	WorkspaceReader port.WorkspaceReader
	RunScope        *port.RunScope
	RunTools        port.RunToolService
	RunLifecycle    port.RunLifecycle
}

type Server struct {
	options  Options
	token    string
	reader   port.WorkspaceReader
	sdk      *sdkmcp.Server
	http     *http.Server
	listener net.Listener
	url      string
	mu       sync.Mutex
}

func New(options Options) (*Server, error) {
	if options.RunScope != nil {
		scope := options.RunScope.Clone()
		options.RunScope = &scope
		if err := validateRunToolOptions(options); err != nil {
			return nil, err
		}
	}
	reader := options.WorkspaceReader
	var err error
	if reader == nil {
		reader, err = workspace.NewDirectView(options.Root, options.Workspace)
		if err != nil {
			return nil, err
		}
	}
	token := options.Token
	if token == "" {
		token, err = randomToken()
		if err != nil {
			return nil, err
		}
	}
	version := options.Version
	if version == "" {
		version = "dev"
	}
	s := &Server{
		options: options,
		token:   token,
		reader:  reader,
		sdk:     sdkmcp.NewServer(&sdkmcp.Implementation{Name: "audioui", Version: version}, nil),
	}
	s.registerTools()
	return s, nil
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func (s *Server) Start(ctx context.Context) error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	handler := sdkmcp.NewStreamableHTTPHandler(
		func(*http.Request) *sdkmcp.Server { return s.sdk },
		&sdkmcp.StreamableHTTPOptions{Stateless: true, MaxRequestBodyBytes: 1 << 20},
	)
	mux := http.NewServeMux()
	mux.Handle("/mcp", s.withAuth(handler))
	s.mu.Lock()
	s.listener = listener
	s.url = "http://" + listener.Addr().String() + "/mcp"
	s.http = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	s.mu.Unlock()
	go func() {
		_ = s.http.Serve(listener)
	}()
	return nil
}

func (s *Server) URL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.url
}

func (s *Server) Token() string { return s.token }

func (s *Server) WriteConfig(path string) error { return WriteConfig(path, s.URL(), s.token) }

func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	server := s.http
	listener := s.listener
	s.mu.Unlock()
	if server == nil {
		return nil
	}
	if err := server.Shutdown(ctx); err != nil {
		_ = server.Close()
	}
	if listener != nil {
		_ = listener.Close()
	}
	return nil
}

func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		expected := "Bearer " + s.token
		if len(header) != len(expected) || subtle.ConstantTimeCompare([]byte(header), []byte(expected)) != 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"TOKEN_INVALID"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

type GetChainInput struct{}
type ChainOutput struct {
	ChainID   string `json:"chainId"`
	Workspace string `json:"workspace"`
	DisplayID string `json:"displayId"`
}

type ReplyInput struct {
	NodeID  string         `json:"node_id,omitempty"`
	Kind    string         `json:"kind,omitempty"`
	Text    string         `json:"text,omitempty"`
	Payload map[string]any `json:"payload,omitempty"`
}
type ReplyOutput struct {
	Accepted bool `json:"accepted"`
}

type ReadFileInput struct {
	Path string `json:"path"`
}
type ReadFileOutput struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	Bytes     int    `json:"bytes"`
	Truncated bool   `json:"truncated"`
}

type ListDirInput struct {
	Path string `json:"path,omitempty"`
}
type DirEntry struct {
	Name string `json:"name"`
	Dir  bool   `json:"dir"`
	Size int64  `json:"size,omitempty"`
}
type ListDirOutput struct {
	Path      string     `json:"path"`
	Entries   []DirEntry `json:"entries"`
	Truncated bool       `json:"truncated"`
}

type GrepInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
}
type GrepHit struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}
type GrepOutput struct {
	Hits      []GrepHit `json:"hits"`
	Truncated bool      `json:"truncated"`
}

type StatInput struct {
	Path string `json:"path"`
}
type StatOutput struct {
	Path    string `json:"path"`
	Dir     bool   `json:"dir"`
	Size    int64  `json:"size"`
	ModTime string `json:"modTime"`
}

type AskUserInput struct {
	Question   string   `json:"question"`
	Options    []string `json:"options,omitempty"`
	AllowOther bool     `json:"allowOther,omitempty"`
}
type AskUserOutput struct {
	Answer string `json:"answer"`
	Via    string `json:"via"`
}

func (s *Server) registerTools() {
	if s.options.RunScope == nil {
		sdkmcp.AddTool(s.sdk, &sdkmcp.Tool{Name: "get_chain", Description: "Return the active AudioUI chain and workspace."}, s.getChain)
		sdkmcp.AddTool(s.sdk, &sdkmcp.Tool{Name: "reply", Description: "Send a chat message to the AudioUI session. Only kind=message is supported in M1."}, s.reply)
		sdkmcp.AddTool(s.sdk, &sdkmcp.Tool{Name: "read_file", Description: "Read a UTF-8 text file inside the workspace."}, s.readFile)
		sdkmcp.AddTool(s.sdk, &sdkmcp.Tool{Name: "list_dir", Description: "List a directory inside the workspace."}, s.listDir)
		sdkmcp.AddTool(s.sdk, &sdkmcp.Tool{Name: "grep", Description: "Search text files inside the workspace with a regular expression."}, s.grep)
		sdkmcp.AddTool(s.sdk, &sdkmcp.Tool{Name: "stat", Description: "Return metadata for a path inside the workspace."}, s.stat)
	} else {
		s.registerRunTools()
	}
	sdkmcp.AddTool(s.sdk, &sdkmcp.Tool{Name: "ask_user", Description: "Ask the user a question with optional choices. Blocks until answered or timed out."}, s.askUser)
}

func (s *Server) getChain(_ context.Context, _ *sdkmcp.CallToolRequest, _ GetChainInput) (*sdkmcp.CallToolResult, ChainOutput, error) {
	return nil, ChainOutput{ChainID: s.options.ChainID, Workspace: s.options.Workspace, DisplayID: s.options.DisplayID}, nil
}

func (s *Server) reply(_ context.Context, _ *sdkmcp.CallToolRequest, input ReplyInput) (*sdkmcp.CallToolResult, ReplyOutput, error) {
	if input.Kind != "" && input.Kind != "message" {
		return nil, ReplyOutput{}, fmt.Errorf("UNSUPPORTED_IN_M1: kind %q is not available until M2", input.Kind)
	}
	text := input.Text
	if text == "" && input.Payload != nil {
		if value, ok := input.Payload["text"].(string); ok {
			text = value
		}
	}
	if strings.TrimSpace(text) == "" {
		return nil, ReplyOutput{}, errors.New("empty message")
	}
	if s.options.OnReply != nil {
		s.options.OnReply(text)
	}
	return nil, ReplyOutput{Accepted: true}, nil
}

func (s *Server) readFile(ctx context.Context, _ *sdkmcp.CallToolRequest, input ReadFileInput) (*sdkmcp.CallToolResult, ReadFileOutput, error) {
	const maxBytes = int64(256 * 1024)
	var read port.WorkspaceRead
	err := s.withActiveRun(ctx, func() error {
		var err error
		read, err = s.reader.ReadFile(ctx, input.Path, maxBytes)
		return err
	})
	if err != nil {
		return nil, ReadFileOutput{}, err
	}
	return nil, ReadFileOutput{
		Path:      read.Path,
		Content:   string(read.Data),
		Bytes:     len(read.Data),
		Truncated: read.Truncated,
	}, nil
}

func (s *Server) listDir(ctx context.Context, _ *sdkmcp.CallToolRequest, input ListDirInput) (*sdkmcp.CallToolResult, ListDirOutput, error) {
	if strings.TrimSpace(input.Path) == "" {
		input.Path = "."
	}
	const maxEntries = 200
	var listing port.WorkspaceList
	err := s.withActiveRun(ctx, func() error {
		var err error
		listing, err = s.reader.ListDir(ctx, input.Path, maxEntries)
		return err
	})
	if err != nil {
		return nil, ListDirOutput{}, err
	}
	result := ListDirOutput{
		Path:      listing.Path,
		Entries:   make([]DirEntry, 0, len(listing.Entries)),
		Truncated: listing.Truncated,
	}
	for _, entry := range listing.Entries {
		result.Entries = append(result.Entries, DirEntry{Name: entry.Name, Dir: entry.Dir, Size: entry.Size})
	}
	return nil, result, nil
}

func (s *Server) grep(ctx context.Context, _ *sdkmcp.CallToolRequest, input GrepInput) (*sdkmcp.CallToolResult, GrepOutput, error) {
	if strings.TrimSpace(input.Path) == "" {
		input.Path = "."
	}
	if len(input.Pattern) > 256 {
		return nil, GrepOutput{}, errors.New("pattern too long")
	}
	var search port.WorkspaceGrep
	err := s.withActiveRun(ctx, func() error {
		var err error
		search, err = s.reader.Grep(ctx, input.Pattern, input.Path, port.WorkspaceGrepOptions{
			MaxHits:      200,
			MaxFiles:     5000,
			MaxFileBytes: 512 * 1024,
		})
		return err
	})
	if err != nil {
		return nil, GrepOutput{}, err
	}
	result := GrepOutput{Hits: make([]GrepHit, 0, len(search.Hits)), Truncated: search.Truncated}
	for _, hit := range search.Hits {
		result.Hits = append(result.Hits, GrepHit{Path: hit.Path, Line: hit.Line, Text: hit.Text})
	}
	return nil, result, nil
}

func (s *Server) stat(ctx context.Context, _ *sdkmcp.CallToolRequest, input StatInput) (*sdkmcp.CallToolResult, StatOutput, error) {
	var stat port.WorkspaceStat
	err := s.withActiveRun(ctx, func() error {
		var err error
		stat, err = s.reader.Stat(ctx, input.Path)
		return err
	})
	if err != nil {
		return nil, StatOutput{}, err
	}
	return nil, StatOutput{
		Path:    stat.Path,
		Dir:     stat.Dir,
		Size:    stat.Size,
		ModTime: stat.ModTime.Format(time.RFC3339),
	}, nil
}

func (s *Server) askUser(ctx context.Context, _ *sdkmcp.CallToolRequest, input AskUserInput) (*sdkmcp.CallToolResult, AskUserOutput, error) {
	if s.options.QuestionGate == nil {
		return nil, AskUserOutput{}, errors.New("QUESTION_TIMEOUT: question gate unavailable")
	}
	if strings.TrimSpace(input.Question) == "" {
		return nil, AskUserOutput{}, errors.New("question is empty")
	}
	if len(input.Options) == 0 && !input.AllowOther {
		return nil, AskUserOutput{}, errors.New("ask_user needs options or allowOther")
	}
	var answer valueobject.QuestionAnswer
	err := s.withActiveRun(ctx, func() error {
		var err error
		answer, err = s.options.QuestionGate.Ask(ctx, valueobject.QuestionRequest{
			Question:   input.Question,
			Options:    input.Options,
			AllowOther: input.AllowOther,
		})
		return err
	})
	if err != nil {
		return nil, AskUserOutput{}, err
	}
	return nil, AskUserOutput{Answer: answer.Answer, Via: answer.Via}, nil
}

func trimLine(line string) string {
	line = strings.TrimSpace(line)
	count := 0
	for index := range line {
		if count == 240 {
			return line[:index]
		}
		count++
	}
	return line
}

func WriteConfig(path, url, token string) error {
	config := map[string]any{
		"mcpServers": map[string]any{
			"audioui": map[string]any{
				"type":       "http",
				"url":        url,
				"headers":    map[string]string{"Authorization": "Bearer " + token},
				"alwaysLoad": true,
			},
		},
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".mcp-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
