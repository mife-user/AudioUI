package session

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
)

type harness struct {
	svc        *Service
	store      *fakeStore
	driver     *fakeDriver
	controller *fakeController
	tools      *fakeTools
	pub        *fakePublisher
}

func newHarness(t *testing.T, mutate func(*Options)) *harness {
	t.Helper()
	root := t.TempDir()
	h := &harness{
		store:      &fakeStore{},
		driver:     &fakeDriver{},
		controller: &fakeController{},
		tools:      &fakeTools{},
		pub:        &fakePublisher{},
	}
	opts := Options{
		Layout:       valueobject.Layout{Root: root, DataDir: root + "/data", WorkspaceDir: root + "/data/workspace/chain_default", AgentDir: root + "/data/agent", SessionsFile: root + "/data/sessions.json"},
		Driver:       h.driver,
		Store:        h.store,
		Publisher:    h.pub,
		Tools:        h.tools,
		Permissions:  h.controller,
		Questions:    h.controller,
		CLIAvailable: true,
	}
	if mutate != nil {
		mutate(&opts)
	}
	h.svc = New(opts)
	t.Cleanup(h.svc.Close)
	return h
}

func (h *harness) permission() valueobject.AgentEvent {
	return valueobject.AgentEvent{Version: 1, Type: valueobject.EventPermission, Permission: &valueobject.PermissionRequest{
		ID: "p1", Tool: "Write", Tier: valueobject.PermissionConfirm, Summary: "write notes.txt",
		ExpiresAt: time.Now().Add(time.Minute).Format(time.RFC3339),
	}}
}

func (h *harness) question() valueobject.AgentEvent {
	return valueobject.AgentEvent{Version: 1, Type: valueobject.EventQuestion, Question: &valueobject.QuestionRequest{
		ID: "q1", Question: "选择节奏", Options: []string{"174", "87"},
		ExpiresAt: time.Now().Add(time.Minute).Format(time.RFC3339),
	}}
}

func statusEvent(state valueobject.SessionState) valueobject.AgentEvent {
	return valueobject.AgentEvent{Version: 1, Type: valueobject.EventStatus, State: &valueobject.AgentState{Status: state}}
}

func TestSendRejectsWhileRunning(t *testing.T) {
	h := newHarness(t, nil)
	h.svc.Start(context.Background())
	h.svc.HandleEvent(statusEvent(valueobject.SessionRunning))
	if err := h.svc.Send(context.Background(), "hi"); !errors.Is(err, ErrBusy) {
		t.Fatalf("Send error = %v, want ErrBusy", err)
	}
}

func TestSendWhenCLIUnavailable(t *testing.T) {
	h := newHarness(t, func(o *Options) {
		o.CLIAvailable = false
		o.CLIError = "native claude.exe not found"
	})
	h.svc.Start(context.Background())
	if h.svc.State().Status != valueobject.SessionUnavailable {
		t.Fatalf("status = %s", h.svc.State().Status)
	}
	err := h.svc.Send(context.Background(), "hi")
	if !errors.Is(err, ErrAgentUnavailable) {
		t.Fatalf("Send error = %v, want ErrAgentUnavailable", err)
	}
	if !strings.Contains(err.Error(), "native claude.exe not found") {
		t.Fatalf("error lost CLI detail: %v", err)
	}
}

func TestSendAfterCloseFails(t *testing.T) {
	h := newHarness(t, nil)
	h.svc.Start(context.Background())
	h.svc.Close()
	if err := h.svc.Send(context.Background(), "hi"); !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("Send error = %v, want ErrSessionClosed", err)
	}
}

func TestSendAfterResumeFailureIsBlocked(t *testing.T) {
	h := newHarness(t, nil)
	h.svc.Start(context.Background())
	h.svc.HandleEvent(statusEvent(valueobject.SessionResumeFailed))
	err := h.svc.Send(context.Background(), "hi")
	if err == nil || !strings.Contains(err.Error(), "SESSION_RESUME_FAILED") {
		t.Fatalf("Send error = %v, want SESSION_RESUME_FAILED", err)
	}
}

func TestRespondQuestionRoutesAndClears(t *testing.T) {
	h := newHarness(t, nil)
	h.svc.Start(context.Background())
	h.svc.HandleEvent(h.question())
	if len(h.svc.State().Questions) != 1 {
		t.Fatalf("questions = %+v", h.svc.State().Questions)
	}

	answer := valueobject.QuestionAnswer{Answer: "174", Via: "option"}
	if err := h.svc.RespondQuestion("q1", answer); err != nil {
		t.Fatalf("RespondQuestion: %v", err)
	}
	if len(h.svc.State().Questions) != 0 {
		t.Fatalf("pending question not cleared: %+v", h.svc.State().Questions)
	}
	if len(h.controller.questions) != 1 || h.controller.questions[0] != answer {
		t.Fatalf("gate answers = %+v", h.controller.questions)
	}
}

func TestRespondQuestionWithoutGate(t *testing.T) {
	h := newHarness(t, func(o *Options) { o.Questions = nil })
	h.svc.HandleEvent(h.question())
	if err := h.svc.RespondQuestion("q1", valueobject.QuestionAnswer{Answer: "x"}); err == nil {
		t.Fatal("expected error without question gate")
	}
}

func TestRespondPermissionRoutesAndClears(t *testing.T) {
	h := newHarness(t, nil)
	h.svc.Start(context.Background())
	h.svc.HandleEvent(h.permission())

	if err := h.svc.RespondPermission("p1", valueobject.DecisionDeny); err != nil {
		t.Fatalf("RespondPermission: %v", err)
	}
	if len(h.svc.State().Permissions) != 0 {
		t.Fatalf("pending permission not cleared: %+v", h.svc.State().Permissions)
	}
	if len(h.controller.permissions) != 1 || h.controller.permissions[0] != valueobject.DecisionDeny {
		t.Fatalf("gate decisions = %+v", h.controller.permissions)
	}
}

func TestRespondPermissionGateErrorKeepsPending(t *testing.T) {
	h := newHarness(t, nil)
	h.svc.Start(context.Background())
	h.svc.HandleEvent(h.permission())
	h.controller.resolveErr = errors.New("permission expired")

	if err := h.svc.RespondPermission("p1", valueobject.DecisionAllow); err == nil {
		t.Fatal("expected gate error")
	}
	if len(h.svc.State().Permissions) != 1 {
		t.Fatalf("pending permission dropped on error: %+v", h.svc.State().Permissions)
	}
}

func TestCancelResumeResetsFailedState(t *testing.T) {
	h := newHarness(t, nil)
	h.svc.Start(context.Background())
	h.svc.onResumeRef(json.RawMessage(`{"sessionId":"old-session"}`))
	h.svc.HandleEvent(statusEvent(valueobject.SessionResumeFailed))
	if h.svc.State().SessionID != "old-session" {
		t.Fatalf("session id = %q", h.svc.State().SessionID)
	}

	h.svc.CancelResume()

	state := h.svc.State()
	if state.Status != valueobject.SessionIdle || state.ResumeError != "" || len(state.Transcript) != 0 || state.SessionID != "" {
		t.Fatalf("state after cancel = %+v", state)
	}
	if len(h.store.snapshot().Chains) > 0 && len(h.store.snapshot().Chains[valueobject.DefaultChainID].ResumeRef) != 0 {
		t.Fatal("resume ref not cleared from store")
	}
}

func TestNewSessionClearsTranscriptAndRestartsDriver(t *testing.T) {
	h := newHarness(t, nil)
	h.svc.Start(context.Background())
	if err := h.svc.Send(context.Background(), "hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	h.svc.onResumeRef(json.RawMessage(`{"sessionId":"s-1"}`))
	first := h.driver.session

	if err := h.svc.NewSession(context.Background()); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if h.driver.startCount != 2 {
		t.Fatalf("driver starts = %d, want 2", h.driver.startCount)
	}
	if first.closeCount != 1 {
		t.Fatalf("old session close count = %d", first.closeCount)
	}
	state := h.svc.State()
	if len(state.Transcript) != 0 || state.SessionID != "" || state.Status != valueobject.SessionIdle {
		t.Fatalf("state after new session = %+v", state)
	}
	if first == h.driver.session {
		t.Fatal("driver session was not replaced")
	}
}

func TestOnResumeRefPersistsSession(t *testing.T) {
	h := newHarness(t, nil)
	h.svc.onResumeRef(json.RawMessage(`{"sessionId":"abc-123"}`))

	state := h.svc.State()
	if state.SessionID != "abc-123" || !strings.Contains(state.DisplayID, "abc-123") {
		t.Fatalf("state = session %q display %q", state.SessionID, state.DisplayID)
	}
	record := h.store.snapshot().Chains[valueobject.DefaultChainID]
	if string(record.ResumeRef) != `{"sessionId":"abc-123"}` {
		t.Fatalf("stored ref = %s", record.ResumeRef)
	}

	h.svc.onResumeRef(json.RawMessage(`not-json`))
	h.svc.onResumeRef(nil)
	if h.svc.State().SessionID != "abc-123" {
		t.Fatalf("invalid ref changed session: %q", h.svc.State().SessionID)
	}
}

func TestStreamingMessageAccumulatesUntilFinal(t *testing.T) {
	h := newHarness(t, nil)
	h.svc.Start(context.Background())

	h.svc.HandleEvent(valueobject.AgentEvent{Version: 1, Type: valueobject.EventMessage, Message: &valueobject.TranscriptItem{
		ID: "s-1", Kind: valueobject.TranscriptMessage, Role: "assistant", Text: "你", Streaming: true,
	}})
	h.svc.HandleEvent(valueobject.AgentEvent{Version: 1, Type: valueobject.EventMessage, Message: &valueobject.TranscriptItem{
		ID: "s-1", Kind: valueobject.TranscriptMessage, Role: "assistant", Text: "好", Streaming: true,
	}})

	state := h.svc.State()
	if len(state.Transcript) != 1 || state.Transcript[0].Text != "你好" {
		t.Fatalf("transcript = %+v", state.Transcript)
	}

	h.svc.HandleEvent(valueobject.AgentEvent{Version: 1, Type: valueobject.EventMessage, Message: &valueobject.TranscriptItem{
		ID: "s-1", Kind: valueobject.TranscriptMessage, Role: "assistant", Text: "你好世界", Streaming: false,
	}})
	state = h.svc.State()
	if state.Transcript[0].Text != "你好世界" || state.Transcript[0].Streaming {
		t.Fatalf("final message = %+v", state.Transcript[0])
	}
}

func TestTranscriptTrimmedToLimit(t *testing.T) {
	h := newHarness(t, nil)
	h.svc.Start(context.Background())
	total := valueobject.MaxTranscriptItems + 5
	for i := range total {
		h.svc.HandleEvent(valueobject.AgentEvent{Version: 1, Type: valueobject.EventMessage, Message: &valueobject.TranscriptItem{
			ID: "m-" + strconv.Itoa(i), Kind: valueobject.TranscriptMessage, Role: "assistant", Text: "x",
		}})
	}

	state := h.svc.State()
	if len(state.Transcript) != valueobject.MaxTranscriptItems {
		t.Fatalf("transcript = %d, want %d", len(state.Transcript), valueobject.MaxTranscriptItems)
	}
	if state.Transcript[0].ID != "m-5" {
		t.Fatalf("oldest kept id = %q, want m-5", state.Transcript[0].ID)
	}
}

func TestToolEventsMergeIntoOneItem(t *testing.T) {
	h := newHarness(t, nil)
	h.svc.Start(context.Background())

	h.svc.HandleEvent(valueobject.AgentEvent{Version: 1, Type: valueobject.EventTool, Tool: &valueobject.TranscriptItem{
		ID: "t1", Kind: valueobject.TranscriptTool, Role: "assistant", ToolName: "Write", ToolStatus: "running", ToolSummary: "Write notes.txt",
	}})
	h.svc.HandleEvent(valueobject.AgentEvent{Version: 1, Type: valueobject.EventTool, Tool: &valueobject.TranscriptItem{
		ID: "t1", Kind: valueobject.TranscriptTool, Role: "assistant", ToolStatus: "ok", ToolSummary: "done",
	}})

	state := h.svc.State()
	if len(state.Transcript) != 1 {
		t.Fatalf("transcript = %+v", state.Transcript)
	}
	item := state.Transcript[0]
	if item.ToolName != "Write" || item.ToolStatus != "ok" || item.ToolSummary != "done" {
		t.Fatalf("tool item = %+v", item)
	}
}

func TestErrorEventAddsNoticeAndMarksError(t *testing.T) {
	h := newHarness(t, nil)
	h.svc.Start(context.Background())
	h.svc.HandleEvent(statusEvent(valueobject.SessionRunning))
	h.svc.HandleEvent(valueobject.AgentEvent{Version: 1, Type: valueobject.EventError, Error: &valueobject.AgentError{
		Code: "TURN_FAILED", Message: "模型不可用", Recoverable: true,
	}})

	state := h.svc.State()
	if state.Status != valueobject.SessionError {
		t.Fatalf("status = %s", state.Status)
	}
	if len(state.Notices) != 1 || state.Notices[0] != "模型不可用" {
		t.Fatalf("notices = %v", state.Notices)
	}
}

type brokenStore struct{}

func (brokenStore) Load(context.Context) (valueobject.SessionsFile, error) {
	return valueobject.SessionsFile{}, errors.New("permission denied")
}
func (brokenStore) Save(context.Context, valueobject.SessionsFile) error { return nil }

func TestCorruptStoreSurfacesResumeFailure(t *testing.T) {
	h := newHarness(t, func(o *Options) { o.Store = brokenStore{} })
	h.svc.Start(context.Background())

	state := h.svc.State()
	if state.Status != valueobject.SessionResumeFailed {
		t.Fatalf("status = %s", state.Status)
	}
	if !strings.Contains(state.ResumeError, "permission denied") {
		t.Fatalf("resume error = %q", state.ResumeError)
	}
	if len(state.Notices) == 0 {
		t.Fatal("missing notice for corrupt metadata")
	}
}

func TestStaleEventSinkIgnoredAfterNewSession(t *testing.T) {
	h := newHarness(t, nil)
	h.svc.Start(context.Background())
	if err := h.svc.Send(context.Background(), "hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	staleSink := h.driver.spec.EventSink
	if err := h.svc.NewSession(context.Background()); err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	before := h.svc.State()
	staleSink(valueobject.AgentEvent{Version: 1, Type: valueobject.EventMessage, Message: &valueobject.TranscriptItem{
		ID: "stale", Kind: valueobject.TranscriptMessage, Role: "assistant", Text: "旧会话",
	}})
	after := h.svc.State()

	if after.Seq != before.Seq || len(after.Transcript) != len(before.Transcript) {
		t.Fatalf("stale sink applied: before=%d/%d after=%d/%d", before.Seq, len(before.Transcript), after.Seq, len(after.Transcript))
	}
}

func TestCloseStopsRuntimeAndRejectsSend(t *testing.T) {
	h := newHarness(t, nil)
	h.svc.Start(context.Background())
	if err := h.svc.Send(context.Background(), "hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	session := h.driver.session

	h.svc.Close()
	if session.closeCount != 1 {
		t.Fatalf("driver close count = %d", session.closeCount)
	}
	if h.tools.last == nil || h.tools.last.stopCount != 1 {
		t.Fatalf("tool server not stopped: %+v", h.tools.last)
	}
	if err := h.svc.Send(context.Background(), "again"); !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("Send after close = %v", err)
	}
}

func TestInterruptWithoutDriverIsNoop(t *testing.T) {
	h := newHarness(t, nil)
	if err := h.svc.Interrupt(context.Background()); err != nil {
		t.Fatalf("Interrupt without driver: %v", err)
	}
	h.svc.Start(context.Background())
	if err := h.svc.Send(context.Background(), "hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if err := h.svc.Interrupt(context.Background()); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	if h.driver.session.interruptCount != 1 {
		t.Fatalf("interrupt count = %d", h.driver.session.interruptCount)
	}
}

func TestStartWithoutRecordPublishesStatusOnly(t *testing.T) {
	h := newHarness(t, nil)
	h.svc.Start(context.Background())
	time.Sleep(20 * time.Millisecond)
	if h.driver.startCount != 0 || h.driver.resumeSeen != nil {
		t.Fatalf("driver started without record: %d", h.driver.startCount)
	}
	h.pub.mu.Lock()
	defer h.pub.mu.Unlock()
	if len(h.pub.events) == 0 {
		t.Fatal("no status event published")
	}
}

func TestStartRestoresPersistedTranscript(t *testing.T) {
	store := &fakeStore{file: valueobject.SessionsFile{Version: 1, ActiveChainID: valueobject.DefaultChainID, Chains: map[string]valueobject.SessionRecord{
		valueobject.DefaultChainID: {
			Adapter: "claude", ResumeRef: json.RawMessage(`{"sessionId":"restored"}`), DisplayID: "restored",
			Status: valueobject.SessionIdle,
			Transcript: []valueobject.TranscriptItem{
				{ID: "u-1", Kind: valueobject.TranscriptMessage, Role: "user", Text: "之前的问题"},
			},
		},
	}}}
	h := newHarness(t, func(o *Options) { o.Store = store })
	h.svc.Start(context.Background())

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		state := h.svc.State()
		if state.SessionID == "restored" && state.Status == valueobject.SessionIdle {
			if len(state.Transcript) != 1 || state.Transcript[0].Text != "之前的问题" {
				t.Fatalf("transcript = %+v", state.Transcript)
			}
			if string(h.driver.resumeSeen) != `{"sessionId":"restored"}` {
				t.Fatalf("resume ref = %s", h.driver.resumeSeen)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("resume did not complete: %+v", h.svc.State())
}

var _ port.SessionStore = brokenStore{}
