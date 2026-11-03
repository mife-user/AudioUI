package claudecli

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"

	claudecode "github.com/severity1/claude-agent-sdk-go"
)

type eventRecorder struct {
	events []valueobject.AgentEvent
	refs   []string
}

func newEventSession(rec *eventRecorder) *agentSession {
	return &agentSession{spec: port.DriverSpec{
		EventSink:   func(event valueobject.AgentEvent) { rec.events = append(rec.events, event) },
		OnResumeRef: func(ref json.RawMessage) { rec.refs = append(rec.refs, string(ref)) },
	}}
}

func textDelta(uuid, text string) *claudecode.StreamEvent {
	return &claudecode.StreamEvent{UUID: uuid, Event: map[string]any{
		"type":  "content_block_delta",
		"delta": map[string]any{"type": "text_delta", "text": text},
	}}
}

func lastEvent(t *testing.T, rec *eventRecorder) valueobject.AgentEvent {
	t.Helper()
	if len(rec.events) == 0 {
		t.Fatal("no events emitted")
	}
	return rec.events[len(rec.events)-1]
}

func TestActualInputTokenUsageIncludesCachedInput(t *testing.T) {
	message := struct {
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"usage"`
	}{}
	message.Usage.InputTokens = 20
	message.Usage.CacheCreationInputTokens = 5
	message.Usage.CacheReadInputTokens = 15
	actual := actualInputTokenUsage(message)
	if actual == nil || *actual != 40 {
		t.Fatalf("actual input tokens = %v, want 40", actual)
	}
}

func TestFirstAssistantUsageWinsAcrossToolTurns(t *testing.T) {
	session := &agentSession{}
	first := struct {
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"usage"`
	}{}
	first.Usage.InputTokens = 25
	first.Usage.CacheCreationInputTokens = 5
	session.captureFirstInputUsage(first)

	later := struct {
		Usage struct {
			InputTokens int `json:"input_tokens"`
		} `json:"usage"`
	}{}
	later.Usage.InputTokens = 900
	session.captureFirstInputUsage(later)

	actual := session.firstInputUsage()
	if actual == nil || *actual != 30 {
		t.Fatalf("first request input tokens = %v, want 30", actual)
	}
}

func TestMissingAssistantUsageRemainsUnknown(t *testing.T) {
	session := &agentSession{}
	session.captureFirstInputUsage(struct{}{})
	if actual := session.firstInputUsage(); actual != nil {
		t.Fatalf("missing usage was reported as %d", *actual)
	}
}

func TestStreamDeltasShareMessageID(t *testing.T) {
	rec := &eventRecorder{}
	session := newEventSession(rec)

	session.handle(textDelta("u1", "你"))
	session.handle(textDelta("u1", "好"))

	if len(rec.events) != 2 {
		t.Fatalf("events = %d, want 2", len(rec.events))
	}
	first, second := rec.events[0].Message, rec.events[1].Message
	if first == nil || second == nil {
		t.Fatal("message payload missing")
	}
	if first.ID != second.ID {
		t.Fatalf("delta ids differ: %q vs %q", first.ID, second.ID)
	}
	if first.ID != "stream-u1" {
		t.Fatalf("id = %q, want stream-u1", first.ID)
	}
	if !first.Streaming || !second.Streaming {
		t.Fatal("stream deltas must be marked streaming")
	}
	if second.Text != "好" {
		t.Fatalf("delta text = %q", second.Text)
	}
}

func TestAssistantTextReplacesStreamWithSameID(t *testing.T) {
	rec := &eventRecorder{}
	session := newEventSession(rec)

	session.handle(textDelta("u2", "你好"))
	session.handle(&claudecode.AssistantMessage{Content: []claudecode.ContentBlock{
		&claudecode.TextBlock{Text: "你好世界"},
	}})

	final := rec.events[len(rec.events)-1].Message
	if final == nil {
		t.Fatal("assistant message missing")
	}
	if final.ID != "stream-u2" {
		t.Fatalf("final id = %q, want stream-u2", final.ID)
	}
	if final.Streaming {
		t.Fatal("final assistant text must not be streaming")
	}
	if final.Text != "你好世界" {
		t.Fatalf("text = %q", final.Text)
	}

	// A new turn after the final text must open a fresh stream id.
	session.handle(textDelta("", "下一条"))
	next := rec.events[len(rec.events)-1].Message
	if next.ID == final.ID {
		t.Fatalf("new turn reused stream id %q", next.ID)
	}
}

func TestIgnoredStreamEvents(t *testing.T) {
	rec := &eventRecorder{}
	session := newEventSession(rec)

	session.handle(&claudecode.StreamEvent{Event: map[string]any{"type": "message_start"}})
	session.handle(&claudecode.StreamEvent{Event: map[string]any{
		"type":  "content_block_delta",
		"delta": map[string]any{"type": "input_json_delta", "partial_json": "{}"},
	}})
	session.handle(&claudecode.StreamEvent{Event: map[string]any{
		"type":  "content_block_delta",
		"delta": map[string]any{"type": "text_delta", "text": ""},
	}})

	if len(rec.events) != 0 {
		t.Fatalf("ignored events emitted %d messages", len(rec.events))
	}
}

func TestToolUseAndResultEvents(t *testing.T) {
	rec := &eventRecorder{}
	session := newEventSession(rec)

	session.handle(&claudecode.AssistantMessage{Content: []claudecode.ContentBlock{
		&claudecode.ToolUseBlock{ToolUseID: "t1", Name: "Write", Input: map[string]any{"file_path": "notes.txt"}},
	}})
	running := lastEvent(t, rec)
	if running.Type != valueobject.EventTool || running.Tool == nil {
		t.Fatalf("event = %+v, want tool", running)
	}
	if running.Tool.ToolStatus != "running" || running.Tool.ToolName != "Write" {
		t.Fatalf("tool = %+v", running.Tool)
	}
	if !strings.Contains(running.Tool.ToolSummary, "notes.txt") {
		t.Fatalf("summary = %q", running.Tool.ToolSummary)
	}

	failure := true
	session.handle(&claudecode.UserMessage{Content: []claudecode.ContentBlock{
		&claudecode.ToolResultBlock{ToolUseID: "t1", Content: "denied", IsError: &failure},
	}})
	done := lastEvent(t, rec)
	if done.Type != valueobject.EventTool || done.Tool == nil {
		t.Fatalf("event = %+v, want tool", done)
	}
	if done.Tool.ToolStatus != "error" || done.Tool.ToolSummary != "denied" {
		t.Fatalf("tool result = %+v", done.Tool)
	}
}

func TestResultMessageEmitsResumeRefAndStatus(t *testing.T) {
	rec := &eventRecorder{}
	session := newEventSession(rec)

	session.handle(&claudecode.ResultMessage{SessionID: "session-1", IsError: true, Errors: []string{"boom", "again"}})

	if len(rec.refs) != 1 || rec.refs[0] != `{"sessionId":"session-1"}` {
		t.Fatalf("refs = %v", rec.refs)
	}
	if len(rec.events) != 2 {
		t.Fatalf("events = %+v", rec.events)
	}
	errEvent := rec.events[0]
	if errEvent.Type != valueobject.EventError || errEvent.Error == nil || errEvent.Error.Code != "TURN_FAILED" {
		t.Fatalf("error event = %+v", errEvent)
	}
	if errEvent.Error.Message != "boom; again" {
		t.Fatalf("error message = %q", errEvent.Error.Message)
	}
	status := rec.events[1]
	if status.Type != valueobject.EventStatus || status.State == nil || status.State.Status != valueobject.SessionError {
		t.Fatalf("status event = %+v", status)
	}
}

func TestSystemMessageEmitsResumeRef(t *testing.T) {
	rec := &eventRecorder{}
	session := newEventSession(rec)

	session.handle(&claudecode.SystemMessage{Data: map[string]any{"session_id": "sys-1"}})
	if len(rec.refs) != 1 || rec.refs[0] != `{"sessionId":"sys-1"}` {
		t.Fatalf("refs = %v", rec.refs)
	}

	session.handle(&claudecode.SystemMessage{Data: map[string]any{}})
	if len(rec.refs) != 1 {
		t.Fatalf("empty system message emitted a ref: %v", rec.refs)
	}
}

func TestSanitizeRedactsBearerToken(t *testing.T) {
	got := sanitize("POST /v1 Authorization: Bearer sk-123456 secret-body")
	if strings.Contains(got, "sk-123456") {
		t.Fatalf("token not redacted: %q", got)
	}
	if !strings.HasPrefix(got, "POST /v1 Authorization: Bearer ***") {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestTruncateKeepsRunesIntact(t *testing.T) {
	// 160 个汉字的标题必须整体保留字符边界，不能截断出半个字符。
	text := strings.Repeat("中", 200)
	got := truncate(text, 160)
	if len([]rune(got)) != 161 || !strings.HasSuffix(got, "…") {
		t.Fatalf("truncate = %d runes, suffix ok=%v", len([]rune(got)), strings.HasSuffix(got, "…"))
	}
	if !utf8.ValidString(got) {
		t.Fatalf("truncate produced invalid UTF-8: %q", got)
	}
	if short := truncate("短文本", 160); short != "短文本" {
		t.Fatalf("short text changed: %q", short)
	}
}

func TestSummarizeToolResultTruncatesRunes(t *testing.T) {
	got := summarizeToolResult(strings.Repeat("响", 200))
	if len([]rune(got)) != 161 {
		t.Fatalf("runes = %d, want 161", len([]rune(got)))
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("missing ellipsis: %q", got)
	}
}

func TestSummarizeToolFallsBackToName(t *testing.T) {
	if got := summarizeTool("Read", nil); got != "Read" {
		t.Fatalf("nil input = %q", got)
	}
	if got := summarizeTool("Read", map[string]any{}); got != "Read {}" {
		t.Fatalf("empty input = %q", got)
	}
}

func TestPermissionPathsCollectsKnownKeys(t *testing.T) {
	paths := permissionPaths(map[string]any{
		"file_path":     "a.txt",
		"notebook_path": "b.ipynb",
		"path":          "",
		"command":       "ls",
	})
	if len(paths) != 2 || paths[0] != "a.txt" || paths[1] != "b.ipynb" {
		t.Fatalf("paths = %v", paths)
	}
}
