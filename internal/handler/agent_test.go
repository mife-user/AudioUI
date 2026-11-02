package handler

import (
	"context"
	"errors"
	"testing"

	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/service/session"
)

type stubStore struct{}

func (stubStore) Load(context.Context) (valueobject.SessionsFile, error) {
	return valueobject.SessionsFile{}, nil
}
func (stubStore) Save(context.Context, valueobject.SessionsFile) error { return nil }

var _ port.SessionStore = stubStore{}

func newAgentHandler(t *testing.T, mutate func(*session.Options)) *AgentService {
	t.Helper()
	root := t.TempDir()
	opts := session.Options{
		Layout: valueobject.Layout{
			Root: root, DataDir: root + "/data", WorkspaceDir: root + "/data/workspace/chain_default",
			AgentDir: root + "/data/agent", SessionsFile: root + "/data/sessions.json",
		},
		Store:        stubStore{},
		CLIAvailable: true,
	}
	if mutate != nil {
		mutate(&opts)
	}
	service := session.New(opts)
	t.Cleanup(service.Close)
	return NewAgentService(service)
}

func TestSendMessageRejectsBlank(t *testing.T) {
	handler := NewAgentService(nil)
	for _, input := range []string{"", "   ", "\n\t"} {
		if err := handler.SendMessage(input); err == nil {
			t.Fatalf("SendMessage(%q) returned nil", input)
		}
	}
}

func TestSendMessageMapsSessionError(t *testing.T) {
	handler := newAgentHandler(t, func(o *session.Options) {
		o.CLIAvailable = false
		o.CLIError = "native claude.exe not found"
	})
	err := handler.SendMessage("你好")
	if !errors.Is(err, session.ErrAgentUnavailable) {
		t.Fatalf("error = %v, want ErrAgentUnavailable", err)
	}
}

func TestRespondPermissionRejectsUnknownDecision(t *testing.T) {
	handler := NewAgentService(nil)
	if err := handler.RespondPermission("p1", "maybe"); err == nil {
		t.Fatal("unknown decision accepted")
	}
}

func TestRespondPermissionAcceptsKnownDecisions(t *testing.T) {
	handler := newAgentHandler(t, nil)
	for _, decision := range []string{"allow", "deny", "allow_session", " allow "} {
		err := handler.RespondPermission("p1", decision)
		if err != nil && err.Error() == "无效的权限决定: "+decision {
			t.Fatalf("known decision %q rejected as invalid", decision)
		}
	}
}

func TestRespondQuestionRejectsBlankAnswer(t *testing.T) {
	handler := NewAgentService(nil)
	if err := handler.RespondQuestion("q1", "  ", "option"); err == nil {
		t.Fatal("blank answer accepted")
	}
}

func TestGetStateExposesDefaults(t *testing.T) {
	handler := newAgentHandler(t, nil)
	state := handler.GetState()
	if state.Version != 1 || state.Adapter != "claude" {
		t.Fatalf("state = %+v", state)
	}
	if state.Status != valueobject.SessionIdle {
		t.Fatalf("status = %s", state.Status)
	}
}
