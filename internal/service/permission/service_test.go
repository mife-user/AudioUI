package permission

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/infra/paths"
)

type fakeAudit struct {
	records []port.AuditRecord
	err     error
}

func (f *fakeAudit) Record(_ context.Context, record port.AuditRecord) error {
	f.records = append(f.records, record)
	return f.err
}

func newTestService(t *testing.T, timeout time.Duration, audit *fakeAudit) *Service {
	t.Helper()
	root := t.TempDir()
	guard, err := paths.NewGuard(root, filepath.Join(root, "workspace"))
	if err != nil {
		t.Fatal(err)
	}
	return New(Options{
		ChainID: "chain_default", Paths: guard, Audit: audit, Timeout: timeout,
		Publish: func(valueobject.AgentEvent) {},
	})
}

func TestPermissionAllowAndAudit(t *testing.T) {
	audit := &fakeAudit{}
	service := newTestService(t, time.Second, audit)
	ctx := context.Background()
	result := make(chan valueobject.PermissionDecision, 1)
	go func() {
		decision, err := service.Request(ctx, valueobject.PermissionRequest{Tool: "Write", Tier: valueobject.PermissionConfirm, Summary: "Write x", Paths: []string{"x"}})
		if err != nil {
			t.Errorf("Request error = %v", err)
		}
		result <- decision
	}()
	deadline := time.Now().Add(time.Second)
	for {
		service.mu.Lock()
		var id string
		for key := range service.pending {
			id = key
			break
		}
		service.mu.Unlock()
		if id != "" {
			if err := service.ResolvePermission(id, valueobject.DecisionAllow); err != nil {
				t.Fatal(err)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("pending permission not registered")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := <-result; got != valueobject.DecisionAllow {
		t.Fatalf("decision = %s", got)
	}
	if len(audit.records) != 1 || audit.records[0].Decision != "allow" {
		t.Fatalf("audit = %+v", audit.records)
	}
}

func TestPermissionTimeoutDenies(t *testing.T) {
	audit := &fakeAudit{}
	service := newTestService(t, 30*time.Millisecond, audit)
	decision, err := service.Request(context.Background(), valueobject.PermissionRequest{Tool: "Write", Tier: valueobject.PermissionConfirm})
	if err != nil {
		t.Fatal(err)
	}
	if decision != valueobject.DecisionDeny {
		t.Fatalf("decision = %s", decision)
	}
	if len(audit.records) != 1 || audit.records[0].Reason != "timeout" {
		t.Fatalf("audit = %+v", audit.records)
	}
}

func TestAuditFailureFailsClosed(t *testing.T) {
	audit := &fakeAudit{err: errors.New("audit down")}
	service := newTestService(t, time.Second, audit)
	result := make(chan valueobject.PermissionDecision, 1)
	go func() {
		decision, _ := service.Request(context.Background(), valueobject.PermissionRequest{Tool: "Write", Tier: valueobject.PermissionConfirm})
		result <- decision
	}()
	deadline := time.Now().Add(time.Second)
	for {
		service.mu.Lock()
		var id string
		for key := range service.pending {
			id = key
			break
		}
		service.mu.Unlock()
		if id != "" {
			_ = service.ResolvePermission(id, valueobject.DecisionAllow)
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("pending permission not registered")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := <-result; got != valueobject.DecisionDeny {
		t.Fatalf("decision = %s, want deny", got)
	}
}

func TestQuestionAnswerAndTimeout(t *testing.T) {
	audit := &fakeAudit{}
	service := newTestService(t, time.Second, audit)
	result := make(chan valueobject.QuestionAnswer, 1)
	go func() {
		answer, _ := service.Ask(context.Background(), valueobject.QuestionRequest{Question: "Pick", Options: []string{"A", "B"}})
		result <- answer
	}()
	deadline := time.Now().Add(time.Second)
	for {
		service.mu.Lock()
		var id string
		for key := range service.questions {
			id = key
			break
		}
		service.mu.Unlock()
		if id != "" {
			_ = service.ResolveQuestion(id, valueobject.QuestionAnswer{Answer: "A", Via: "option"})
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("pending question not registered")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := <-result; got.Answer != "A" {
		t.Fatalf("answer = %+v", got)
	}
}

func TestSessionChangeClearsAllowedSession(t *testing.T) {
	service := newTestService(t, time.Second, &fakeAudit{})
	service.mu.Lock()
	service.allowedSession["write|x"] = true
	service.mu.Unlock()
	service.SetSessionID("session-two")
	service.mu.Lock()
	defer service.mu.Unlock()
	if len(service.allowedSession) != 0 {
		t.Fatalf("allowedSession = %v, want empty after session change", service.allowedSession)
	}
}

func TestQuestionTimeout(t *testing.T) {
	service := newTestService(t, 30*time.Millisecond, &fakeAudit{})
	_, err := service.Ask(context.Background(), valueobject.QuestionRequest{Question: "Pick", Options: []string{"A"}})
	if !errors.Is(err, ErrQuestionTimeout) {
		t.Fatalf("Ask error = %v, want QUESTION_TIMEOUT", err)
	}
}
