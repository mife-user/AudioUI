package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
)

type fakeStore struct {
	mu   sync.Mutex
	file valueobject.SessionsFile
}

func (s *fakeStore) Load(context.Context) (valueobject.SessionsFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file.Version == 0 {
		return valueobject.SessionsFile{Version: 1, ActiveChainID: valueobject.DefaultChainID, Chains: map[string]valueobject.SessionRecord{}}, nil
	}
	return s.file, nil
}
func (s *fakeStore) Save(_ context.Context, file valueobject.SessionsFile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.file = file
	return nil
}
func (s *fakeStore) snapshot() valueobject.SessionsFile {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.file
}

type fakeTool struct {
	startCount int
	stopCount  int
}

func (f *fakeTool) Start(context.Context) error { f.startCount++; return nil }
func (f *fakeTool) Stop(context.Context) error  { f.stopCount++; return nil }
func (f *fakeTool) URL() string                 { return "http://127.0.0.1:1/mcp" }
func (f *fakeTool) Token() string               { return "token" }
func (f *fakeTool) WriteConfig(string) error    { return nil }

type fakeTools struct {
	last *fakeTool
}

func (f *fakeTools) New(string, string, port.QuestionGate, func(string)) (port.ToolServer, error) {
	f.last = &fakeTool{}
	return f.last, nil
}

type fakeController struct {
	mu          sync.Mutex
	permissions []valueobject.PermissionDecision
	questions   []valueobject.QuestionAnswer
	resolveErr  error
}

func (f *fakeController) Request(context.Context, valueobject.PermissionRequest) (valueobject.PermissionDecision, error) {
	return valueobject.DecisionAllow, nil
}
func (f *fakeController) Check(context.Context, string, map[string]any) port.PolicyResult {
	return port.PolicyResult{Decision: port.PolicyAllow, Tier: valueobject.PermissionAutoAllow}
}
func (f *fakeController) ResolvePermission(_ string, decision valueobject.PermissionDecision) error {
	if f.resolveErr != nil {
		return f.resolveErr
	}
	f.mu.Lock()
	f.permissions = append(f.permissions, decision)
	f.mu.Unlock()
	return nil
}
func (f *fakeController) Ask(context.Context, valueobject.QuestionRequest) (valueobject.QuestionAnswer, error) {
	return valueobject.QuestionAnswer{Answer: "A", Via: "option"}, nil
}
func (f *fakeController) ResolveQuestion(_ string, answer valueobject.QuestionAnswer) error {
	if f.resolveErr != nil {
		return f.resolveErr
	}
	f.mu.Lock()
	f.questions = append(f.questions, answer)
	f.mu.Unlock()
	return nil
}

type fakeDriver struct {
	resumeErr  error
	startErr   error
	spec       port.DriverSpec
	session    *fakeSession
	startCount int
	resumeSeen json.RawMessage
}

func (d *fakeDriver) Start(_ context.Context, spec port.DriverSpec) (port.AgentSession, error) {
	d.startCount++
	d.spec = spec
	if d.startErr != nil {
		return nil, d.startErr
	}
	d.session = &fakeSession{spec: spec}
	return d.session, nil
}
func (d *fakeDriver) Resume(_ context.Context, spec port.DriverSpec, ref json.RawMessage) (port.AgentSession, error) {
	d.resumeSeen = ref
	if d.resumeErr != nil {
		return nil, d.resumeErr
	}
	return d.Start(context.Background(), spec)
}

type fakeSession struct {
	spec           port.DriverSpec
	sent           int
	closeCount     int
	interruptCount int
}

func (s *fakeSession) Send(_ context.Context, _ string) error {
	s.sent++
	s.spec.EventSink(valueobject.AgentEvent{Version: 1, Type: valueobject.EventMessage, Message: &valueobject.TranscriptItem{ID: fmt.Sprintf("m-%d", s.sent), Kind: valueobject.TranscriptMessage, Role: "assistant", Text: "reply"}})
	s.spec.EventSink(valueobject.AgentEvent{Version: 1, Type: valueobject.EventStatus, State: &valueobject.AgentState{Status: valueobject.SessionIdle}})
	return nil
}
func (s *fakeSession) Interrupt(context.Context) error {
	s.interruptCount++
	return nil
}
func (s *fakeSession) Close() error {
	s.closeCount++
	return nil
}

type fakePublisher struct {
	mu     sync.Mutex
	events []valueobject.AgentEvent
}

func (p *fakePublisher) Publish(event valueobject.AgentEvent) {
	p.mu.Lock()
	p.events = append(p.events, event)
	p.mu.Unlock()
}

func newTestService(t *testing.T, driver port.AgentDriver, store *fakeStore) (*Service, *fakePublisher) {
	t.Helper()
	root := t.TempDir()
	pub := &fakePublisher{}
	controller := &fakeController{}
	svc := New(Options{
		Layout: valueobject.Layout{Root: root, DataDir: root + "/data", WorkspaceDir: root + "/data/workspace/chain_default", AgentDir: root + "/data/agent", SessionsFile: root + "/data/sessions.json"},
		Driver: driver, Store: store, Publisher: pub, Tools: &fakeTools{},
		Permissions: controller, Questions: controller, CLIAvailable: true,
	})
	return svc, pub
}

func TestSendProducesUserAndAssistantMessages(t *testing.T) {
	store := &fakeStore{}
	driver := &fakeDriver{}
	svc, _ := newTestService(t, driver, store)
	svc.Start(context.Background())
	if err := svc.Send(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}
	state := svc.State()
	if len(state.Transcript) != 2 || state.Transcript[0].Role != "user" || state.Transcript[1].Text != "reply" {
		t.Fatalf("transcript = %+v", state.Transcript)
	}
	if state.Status != valueobject.SessionIdle {
		t.Fatalf("status = %s", state.Status)
	}
	if len(store.snapshot().Chains) == 0 {
		t.Fatal("session was not persisted")
	}
}

func TestSecondTurnHasIncreasingSeq(t *testing.T) {
	store := &fakeStore{}
	driver := &fakeDriver{}
	svc, _ := newTestService(t, driver, store)
	svc.Start(context.Background())
	if err := svc.Send(context.Background(), "one"); err != nil {
		t.Fatal(err)
	}
	first := svc.State().Seq
	if err := svc.Send(context.Background(), "two"); err != nil {
		t.Fatal(err)
	}
	second := svc.State().Seq
	if second <= first {
		t.Fatalf("seq did not increase: first=%d second=%d", first, second)
	}
	if len(svc.State().Transcript) != 4 {
		t.Fatalf("transcript = %+v", svc.State().Transcript)
	}
}

func TestResumeFailureIsSurface(t *testing.T) {
	store := &fakeStore{file: valueobject.SessionsFile{Version: 1, ActiveChainID: valueobject.DefaultChainID, Chains: map[string]valueobject.SessionRecord{
		valueobject.DefaultChainID: {Adapter: "claude", ResumeRef: json.RawMessage(`{"sessionId":"s1"}`), DisplayID: "s1", Status: valueobject.SessionIdle},
	}}}
	driver := &fakeDriver{resumeErr: errors.New("boom")}
	svc, _ := newTestService(t, driver, store)
	svc.Start(context.Background())
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if svc.State().Status == valueobject.SessionResumeFailed {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("status = %s, want resume_failed", svc.State().Status)
}

func TestPermissionEventIsInSnapshot(t *testing.T) {
	store := &fakeStore{}
	driver := &fakeDriver{}
	svc, pub := newTestService(t, driver, store)
	svc.Start(context.Background())
	svc.HandleEvent(valueobject.AgentEvent{
		Version: 1, Type: valueobject.EventPermission,
		Permission: &valueobject.PermissionRequest{ID: "p1", Tool: "Write", Tier: valueobject.PermissionConfirm, Summary: "write", ExpiresAt: time.Now().Add(time.Minute).Format(time.RFC3339)},
	})
	state := svc.State()
	if len(state.Permissions) != 1 || state.Permissions[0].ID != "p1" {
		t.Fatalf("pending permission missing from snapshot: %+v", state.Permissions)
	}
	if state.Seq == 0 {
		t.Fatal("snapshot seq is zero")
	}
	pub.mu.Lock()
	defer pub.mu.Unlock()
	if len(pub.events) == 0 || pub.events[len(pub.events)-1].Seq == 0 {
		t.Fatalf("permission event seq missing: %+v", pub.events)
	}
}
