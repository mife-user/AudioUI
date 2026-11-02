package claudecli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"audio-ui/internal/agenthost"
	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"

	claudecode "github.com/severity1/claude-agent-sdk-go"
)

type Driver struct {
	Native   NativeCLI
	HostPath string
	Logf     func(format string, args ...any)
}

func NewDriver(native NativeCLI, hostPath string, logf func(string, ...any)) *Driver {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &Driver{Native: native, HostPath: hostPath, Logf: logf}
}

func (d *Driver) Start(ctx context.Context, spec port.DriverSpec) (port.AgentSession, error) {
	return d.start(ctx, spec, nil)
}

func (d *Driver) Resume(ctx context.Context, spec port.DriverSpec, ref json.RawMessage) (port.AgentSession, error) {
	return d.start(ctx, spec, ref)
}

func (d *Driver) start(ctx context.Context, spec port.DriverSpec, ref json.RawMessage) (port.AgentSession, error) {
	options, err := d.options(spec, ref)
	if err != nil {
		return nil, err
	}
	client := claudecode.NewClient(options...)
	if err := client.Connect(ctx); err != nil {
		return nil, fmt.Errorf("AGENT_UNAVAILABLE: %w", err)
	}
	sessionCtx, cancel := context.WithCancel(context.Background())
	s := &agentSession{
		driver:   d,
		spec:     spec,
		client:   client,
		ctx:      sessionCtx,
		cancel:   cancel,
		streamID: fmt.Sprintf("stream-%d", time.Now().UnixNano()),
	}
	go s.readLoop()
	return s, nil
}

func (d *Driver) options(spec port.DriverSpec, ref json.RawMessage) ([]claudecode.Option, error) {
	binary := d.Native.Path
	opts := []claudecode.Option{
		claudecode.WithCLIPath(binary),
		claudecode.WithCwd(spec.Workspace),
		// Load the user-level settings only: credentials provided through
		// ~/.claude/settings.json (env / apiKeyHelper) must reach the CLI, while
		// project and local settings stay out of the agent session.
		claudecode.WithSettingSources(claudecode.SettingSourceUser),
		claudecode.WithPermissionMode(claudecode.PermissionModeDefault),
		claudecode.WithTools("Read", "Glob", "Grep", "Write", "Edit", "Bash"),
		claudecode.WithAllowedTools(
			"mcp__audioui__get_chain",
			"mcp__audioui__reply",
			"mcp__audioui__read_file",
			"mcp__audioui__list_dir",
			"mcp__audioui__grep",
			"mcp__audioui__stat",
			"mcp__audioui__ask_user",
		),
		claudecode.WithIncludePartialMessages(true),
		claudecode.WithAppendSystemPrompt("You are AudioUI's local content agent. Work only inside the provided workspace. Use ask_user when a decision is needed. Use reply(kind=message) for chat output; graph tools are not available in M1."),
		claudecode.WithStderrCallback(func(line string) {
			d.Logf("claude stderr: %s", sanitize(line))
		}),
		claudecode.WithCanUseTool(func(ctx context.Context, tool string, input map[string]any, _ claudecode.ToolPermissionContext) (claudecode.PermissionResult, error) {
			if spec.PermissionGate == nil {
				return claudecode.NewPermissionResultDeny("permission gate unavailable"), nil
			}
			paths := permissionPaths(input)
			request := valueobject.PermissionRequest{
				Tool:         tool,
				Tier:         valueobject.PermissionConfirm,
				Risk:         "工具调用",
				Summary:      summarizeTool(tool, input),
				Paths:        paths,
				Command:      commandOf(input),
				AllowSession: tool != "Bash",
			}
			decision, err := spec.PermissionGate.Request(ctx, request)
			if err != nil {
				return claudecode.NewPermissionResultDeny("permission aborted"), nil
			}
			switch decision {
			case valueobject.DecisionAllow, valueobject.DecisionAllowSession:
				return claudecode.NewPermissionResultAllow(), nil
			default:
				return claudecode.NewPermissionResultDeny("permission denied"), nil
			}
		}),
		claudecode.WithHooks(map[claudecode.HookEvent][]claudecode.HookMatcher{
			claudecode.HookEventPreToolUse: {
				{
					Matcher: "",
					Hooks: []claudecode.HookCallback{
						func(ctx context.Context, input any, _ *string, _ claudecode.HookContext) (claudecode.HookJSONOutput, error) {
							return d.preToolUse(ctx, spec, input), nil
						},
					},
				},
			},
		}),
	}

	args := map[string]*string{"mcp-config": &spec.MCPConfigPath}
	args["strict-mcp-config"] = nil
	opts = append(opts, claudecode.WithExtraArgs(args))

	if d.HostPath != "" {
		opts = append(opts,
			claudecode.WithCLIPath(d.HostPath),
			claudecode.WithEnvVar(agenthost.EnvMode, "1"),
			claudecode.WithEnvVar(agenthost.EnvNative, d.Native.Path),
		)
	}

	if len(ref) > 0 {
		var payload struct {
			SessionID string `json:"sessionId"`
		}
		if err := json.Unmarshal(ref, &payload); err != nil || payload.SessionID == "" {
			return nil, errors.New("SESSION_RESUME_FAILED: invalid resumeRef")
		}
		opts = append(opts, claudecode.WithResume(payload.SessionID))
	}
	return opts, nil
}

func (d *Driver) preToolUse(_ context.Context, spec port.DriverSpec, input any) claudecode.HookJSONOutput {
	var tool string
	var toolInput map[string]any
	switch value := input.(type) {
	case *claudecode.PreToolUseHookInput:
		tool, toolInput = value.ToolName, value.ToolInput
	case claudecode.PreToolUseHookInput:
		tool, toolInput = value.ToolName, value.ToolInput
	}
	if spec.ToolPolicy == nil || tool == "" {
		return claudecode.HookJSONOutput{}
	}
	result := spec.ToolPolicy.Check(context.Background(), tool, toolInput)
	decision := "ask"
	if result.Decision == port.PolicyAllow {
		decision = "allow"
	}
	if result.Decision == port.PolicyDeny {
		decision = "deny"
	}
	output := &claudecode.PreToolUseHookSpecificOutput{
		HookEventName:            "PreToolUse",
		PermissionDecision:       &decision,
		PermissionDecisionReason: &result.Reason,
	}
	return claudecode.HookJSONOutput{HookSpecificOutput: output}
}

type agentSession struct {
	driver          *Driver
	spec            port.DriverSpec
	client          claudecode.Client
	ctx             context.Context
	cancel          context.CancelFunc
	streamID        string
	streamFinalized bool
	mu              sync.Mutex
	closed          bool
}

func (s *agentSession) Send(ctx context.Context, text string) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return errors.New("session closed")
	}
	s.mu.Unlock()
	return s.client.Query(ctx, text)
}

func (s *agentSession) Interrupt(ctx context.Context) error {
	return s.client.Interrupt(ctx)
}

func (s *agentSession) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	s.cancel()
	return s.client.Disconnect()
}

func (s *agentSession) readLoop() {
	msgCh := s.client.ReceiveMessages(s.ctx)
	for {
		select {
		case <-s.ctx.Done():
			return
		case msg := <-msgCh:
			if msg == nil {
				if s.ctx.Err() == nil {
					err := s.client.Err()
					s.emit(valueobject.AgentEvent{Version: 1, Type: valueobject.EventError, Error: &valueobject.AgentError{
						Code: "SESSION_ENDED", Message: errorString(err), Recoverable: true,
					}})
				}
				return
			}
			s.handle(msg)
		}
	}
}

func (s *agentSession) handle(msg claudecode.Message) {
	switch m := msg.(type) {
	case *claudecode.SystemMessage:
		if sessionID, ok := m.Data["session_id"].(string); ok && sessionID != "" {
			ref, _ := json.Marshal(map[string]string{"sessionId": sessionID})
			if s.spec.OnResumeRef != nil {
				s.spec.OnResumeRef(ref)
			}
		}
	case *claudecode.StreamEvent:
		s.handleStreamEvent(m)
	case *claudecode.AssistantMessage:
		for _, block := range m.Content {
			switch b := block.(type) {
			case *claudecode.TextBlock:
				s.mu.Lock()
				if s.streamID == "" || s.streamFinalized {
					s.streamID = fmt.Sprintf("stream-%d", time.Now().UnixNano())
					s.streamFinalized = false
				}
				id := s.streamID
				s.streamFinalized = true
				s.mu.Unlock()
				s.emit(valueobject.AgentEvent{Version: 1, Type: valueobject.EventMessage, Message: &valueobject.TranscriptItem{
					ID: id, Kind: valueobject.TranscriptMessage, Role: "assistant", Text: b.Text,
					Streaming: false, CreatedAt: now(),
				}})
			case *claudecode.ToolUseBlock:
				s.mu.Lock()
				s.streamFinalized = true
				s.mu.Unlock()
				s.emit(valueobject.AgentEvent{Version: 1, Type: valueobject.EventTool, Tool: &valueobject.TranscriptItem{
					ID: b.ToolUseID, Kind: valueobject.TranscriptTool, Role: "assistant",
					ToolName: b.Name, ToolStatus: "running", ToolSummary: summarizeTool(b.Name, b.Input), CreatedAt: now(),
				}})
			}
		}
	case *claudecode.UserMessage:
		if blocks, ok := m.Content.([]claudecode.ContentBlock); ok {
			for _, block := range blocks {
				if result, ok := block.(*claudecode.ToolResultBlock); ok {
					status := "ok"
					if result.IsError != nil && *result.IsError {
						status = "error"
					}
					s.emit(valueobject.AgentEvent{Version: 1, Type: valueobject.EventTool, Tool: &valueobject.TranscriptItem{
						ID: result.ToolUseID, Kind: valueobject.TranscriptTool, Role: "assistant",
						ToolStatus: status, ToolSummary: summarizeToolResult(result.Content), CreatedAt: now(),
					}})
				}
			}
		}
	case *claudecode.ResultMessage:
		s.mu.Lock()
		s.streamID = ""
		s.streamFinalized = true
		s.mu.Unlock()
		if m.SessionID != "" {
			ref, _ := json.Marshal(map[string]string{"sessionId": m.SessionID})
			if s.spec.OnResumeRef != nil {
				s.spec.OnResumeRef(ref)
			}
		}
		status := valueobject.SessionIdle
		if m.IsError {
			s.emit(valueobject.AgentEvent{Version: 1, Type: valueobject.EventError, Error: &valueobject.AgentError{
				Code: "TURN_FAILED", Message: strings.Join(m.Errors, "; "), Recoverable: true,
			}})
			status = valueobject.SessionError
		}
		s.emit(valueobject.AgentEvent{Version: 1, Type: valueobject.EventStatus, State: &valueobject.AgentState{Status: status}})
	}
}

func (s *agentSession) handleStreamEvent(event *claudecode.StreamEvent) {
	data := event.Event
	eventType, _ := data["type"].(string)
	if eventType != "content_block_delta" {
		return
	}
	delta, _ := data["delta"].(map[string]any)
	if delta == nil {
		return
	}
	if deltaType, _ := delta["type"].(string); deltaType != "text_delta" {
		return
	}
	text, _ := delta["text"].(string)
	if text == "" {
		return
	}
	s.mu.Lock()
	if s.streamID == "" || s.streamFinalized {
		s.streamID = fmt.Sprintf("stream-%d", time.Now().UnixNano())
		if event.UUID != "" {
			s.streamID = "stream-" + event.UUID
		}
		s.streamFinalized = false
	}
	id := s.streamID
	s.mu.Unlock()
	s.emit(valueobject.AgentEvent{Version: 1, Type: valueobject.EventMessage, Message: &valueobject.TranscriptItem{
		ID: id, Kind: valueobject.TranscriptMessage, Role: "assistant", Text: text,
		Streaming: true, CreatedAt: now(),
	}})
}

func (s *agentSession) emit(event valueobject.AgentEvent) {
	if s.spec.EventSink != nil {
		s.spec.EventSink(event)
	}
}

func summarizeTool(tool string, input map[string]any) string {
	data, _ := json.Marshal(input)
	text := truncate(string(data), 160)
	if text == "" || text == "null" {
		return tool
	}
	return tool + " " + text
}

func summarizeToolResult(content any) string {
	return truncate(fmt.Sprint(content), 160)
}

// truncate keeps at most limit runes so multi-byte text (UI 文案、中文路径) is
// never cut mid-character, and marks the cut with an ellipsis.
func truncate(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	count := 0
	for index := range text {
		if count == limit {
			return text[:index] + "…"
		}
		count++
	}
	return text
}

func commandOf(input map[string]any) string {
	command, _ := input["command"].(string)
	return command
}

func permissionPaths(input map[string]any) []string {
	var paths []string
	for _, key := range []string{"file_path", "path", "notebook_path"} {
		if value, ok := input[key].(string); ok && value != "" {
			paths = append(paths, value)
		}
	}
	return paths
}

func sanitize(line string) string {
	line = truncate(line, 500)
	for _, marker := range []string{"Authorization: Bearer ", "Bearer "} {
		if index := strings.Index(line, marker); index >= 0 {
			line = line[:index+len(marker)] + "***"
		}
	}
	return line
}

func errorString(err error) string {
	if err == nil {
		return "stream ended"
	}
	return err.Error()
}

func now() string {
	return time.Now().Format(time.RFC3339)
}
