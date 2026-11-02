package permission

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
)

var (
	ErrPermissionNotFound = errors.New("permission request not found")
	ErrQuestionNotFound   = errors.New("question request not found")
	ErrQuestionTimeout    = errors.New("QUESTION_TIMEOUT")
)

type Options struct {
	ChainID   string
	SessionID string
	Paths     port.PathResolver
	Audit     port.AuditSink
	Publish   func(valueobject.AgentEvent)
	Now       func() time.Time
	Timeout   time.Duration
}

type Service struct {
	options Options
	policy  *Policy

	mu             sync.Mutex
	pending        map[string]*pendingPermission
	questions      map[string]*pendingQuestion
	allowedSession map[string]bool
	sessionID      string
	onResolved     func(kind, id string)
	counter        atomic.Uint64
}

type pendingPermission struct {
	request valueobject.PermissionRequest
	result  chan valueobject.PermissionDecision
}

type pendingQuestion struct {
	request valueobject.QuestionRequest
	result  chan valueobject.QuestionAnswer
}

func New(options Options) *Service {
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Timeout <= 0 {
		options.Timeout = valueobject.PermissionTimeout
	}
	service := &Service{
		options:        options,
		pending:        map[string]*pendingPermission{},
		questions:      map[string]*pendingQuestion{},
		allowedSession: map[string]bool{},
		sessionID:      options.SessionID,
	}
	service.policy = &Policy{
		Paths: options.Paths,
		AllowSession: func(key string) bool {
			service.mu.Lock()
			defer service.mu.Unlock()
			return service.allowedSession[key]
		},
	}
	return service
}

func (s *Service) SetResolvedCallback(callback func(kind, id string)) {
	s.mu.Lock()
	s.onResolved = callback
	s.mu.Unlock()
}

func (s *Service) notifyResolved(kind, id string) {
	s.mu.Lock()
	callback := s.onResolved
	s.mu.Unlock()
	if callback != nil {
		callback(kind, id)
	}
}

func (s *Service) SetSessionID(id string) {
	s.mu.Lock()
	if s.sessionID != id {
		s.allowedSession = map[string]bool{}
	}
	s.sessionID = id
	s.mu.Unlock()
}

func (s *Service) currentSessionID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessionID
}

func (s *Service) Check(ctx context.Context, tool string, input map[string]any) port.PolicyResult {
	result := s.policy.Check(ctx, tool, input)
	decision := ""
	tier := string(result.Tier)
	switch result.Decision {
	case port.PolicyDeny:
		decision = "deny"
	case port.PolicyAllow:
		decision = "auto_allow"
	default:
		return result
	}
	err := s.audit(ctx, port.AuditRecord{
		Event:     "permission",
		ChainID:   s.options.ChainID,
		SessionID: s.currentSessionID(),
		Tool:      tool,
		Tier:      tier,
		Decision:  decision,
		Reason:    result.Reason,
		Paths:     result.Paths,
		Command:   result.Command,
		InputHash: inputHash(input),
	})
	if err != nil && result.Decision == port.PolicyAllow {
		return port.PolicyResult{Decision: port.PolicyDeny, Tier: valueobject.PermissionHardDeny, Reason: "AUDIT_UNAVAILABLE"}
	}
	return result
}

func (s *Service) Request(ctx context.Context, request valueobject.PermissionRequest) (valueobject.PermissionDecision, error) {
	if len(request.Paths) > 0 && s.options.Paths != nil {
		normalized := make([]string, 0, len(request.Paths))
		for _, item := range request.Paths {
			if resolved, err := s.options.Paths.Resolve(item); err == nil {
				normalized = append(normalized, resolved)
			} else {
				normalized = append(normalized, item)
			}
		}
		request.Paths = normalized
	}
	if request.ID == "" {
		request.ID = fmt.Sprintf("perm_%d_%d", s.options.Now().UnixNano(), s.counter.Add(1))
	}
	if request.ExpiresAt == "" {
		request.ExpiresAt = s.options.Now().Add(s.options.Timeout).Format(time.RFC3339)
	}
	pending := &pendingPermission{request: request, result: make(chan valueobject.PermissionDecision, 1)}
	s.mu.Lock()
	s.pending[request.ID] = pending
	s.mu.Unlock()

	s.publish(valueobject.AgentEvent{Version: 1, Type: valueobject.EventPermission, Permission: &request})
	timer := time.NewTimer(s.options.Timeout)
	defer timer.Stop()

	select {
	case decision := <-pending.result:
		effective := s.finishPermission(ctx, request, decision, "resolved")
		return effective, nil
	case <-timer.C:
		s.removePermission(request.ID)
		s.finishPermission(ctx, request, valueobject.DecisionDeny, "timeout")
		return valueobject.DecisionDeny, nil
	case <-ctx.Done():
		s.removePermission(request.ID)
		s.finishPermission(ctx, request, valueobject.DecisionDeny, "aborted")
		return valueobject.DecisionDeny, ctx.Err()
	}
}

func (s *Service) ResolvePermission(id string, decision valueobject.PermissionDecision) error {
	s.mu.Lock()
	pending := s.pending[id]
	if pending == nil {
		s.mu.Unlock()
		return ErrPermissionNotFound
	}
	delete(s.pending, id)
	s.mu.Unlock()
	pending.result <- decision
	return nil
}

func (s *Service) Ask(ctx context.Context, request valueobject.QuestionRequest) (valueobject.QuestionAnswer, error) {
	if request.ID == "" {
		request.ID = fmt.Sprintf("question_%d_%d", s.options.Now().UnixNano(), s.counter.Add(1))
	}
	if request.ExpiresAt == "" {
		request.ExpiresAt = s.options.Now().Add(s.options.Timeout).Format(time.RFC3339)
	}
	pending := &pendingQuestion{request: request, result: make(chan valueobject.QuestionAnswer, 1)}
	s.mu.Lock()
	if len(s.questions) > 0 {
		s.mu.Unlock()
		return valueobject.QuestionAnswer{}, errors.New("QUESTION_BUSY: only one question may be pending")
	}
	s.questions[request.ID] = pending
	s.mu.Unlock()

	s.publish(valueobject.AgentEvent{Version: 1, Type: valueobject.EventQuestion, Question: &request})
	timer := time.NewTimer(s.options.Timeout)
	defer timer.Stop()
	select {
	case answer := <-pending.result:
		s.removeQuestion(request.ID)
		s.notifyResolved("question", request.ID)
		return answer, nil
	case <-timer.C:
		s.removeQuestion(request.ID)
		s.notifyResolved("question", request.ID)
		return valueobject.QuestionAnswer{}, ErrQuestionTimeout
	case <-ctx.Done():
		s.removeQuestion(request.ID)
		s.notifyResolved("question", request.ID)
		return valueobject.QuestionAnswer{}, ctx.Err()
	}
}

func (s *Service) ResolveQuestion(id string, answer valueobject.QuestionAnswer) error {
	s.mu.Lock()
	pending := s.questions[id]
	if pending == nil {
		s.mu.Unlock()
		return ErrQuestionNotFound
	}
	delete(s.questions, id)
	s.mu.Unlock()
	pending.result <- answer
	return nil
}

func (s *Service) Close() {
	s.mu.Lock()
	pending := s.pending
	questions := s.questions
	s.pending = map[string]*pendingPermission{}
	s.questions = map[string]*pendingQuestion{}
	s.mu.Unlock()

	for _, item := range pending {
		select {
		case item.result <- valueobject.DecisionDeny:
		default:
		}
	}
	for _, item := range questions {
		select {
		case item.result <- valueobject.QuestionAnswer{Via: "cancelled"}:
		default:
		}
	}
}

func (s *Service) finishPermission(ctx context.Context, request valueobject.PermissionRequest, decision valueobject.PermissionDecision, outcome string) valueobject.PermissionDecision {
	effective := decision
	record := port.AuditRecord{
		Event:     "permission",
		ChainID:   s.options.ChainID,
		SessionID: s.currentSessionID(),
		RequestID: request.ID,
		Tool:      request.Tool,
		Tier:      string(request.Tier),
		Decision:  string(decision),
		Reason:    outcome,
		Paths:     request.Paths,
		Command:   request.Command,
		InputHash: inputHash(map[string]any{"tool": request.Tool, "command": request.Command, "paths": request.Paths}),
	}
	if err := s.audit(ctx, record); err != nil && effective != valueobject.DecisionDeny {
		// Fail closed when an allow decision cannot be audited.
		effective = valueobject.DecisionDeny
		_ = s.audit(context.Background(), port.AuditRecord{
			Event:     "permission",
			ChainID:   s.options.ChainID,
			SessionID: s.currentSessionID(),
			RequestID: request.ID,
			Tool:      request.Tool,
			Tier:      string(request.Tier),
			Decision:  "deny",
			Reason:    "AUDIT_UNAVAILABLE",
			InputHash: record.InputHash,
		})
	}
	if effective == valueobject.DecisionAllowSession {
		s.mu.Lock()
		s.allowedSession[permissionKey(request)] = true
		s.mu.Unlock()
	}
	s.notifyResolved("permission", request.ID)
	return effective
}

func permissionKey(request valueobject.PermissionRequest) string {
	tool := strings.ToLower(request.Tool)
	key := tool + "|" + request.Command
	if key == tool+"|" && len(request.Paths) > 0 {
		key = tool + "|" + request.Paths[0]
	}
	return key
}

func (s *Service) removePermission(id string) {
	s.mu.Lock()
	delete(s.pending, id)
	s.mu.Unlock()
}

func (s *Service) removeQuestion(id string) {
	s.mu.Lock()
	delete(s.questions, id)
	s.mu.Unlock()
}

func (s *Service) audit(ctx context.Context, record port.AuditRecord) error {
	if s.options.Audit == nil {
		return nil
	}
	return s.options.Audit.Record(ctx, record)
}

func (s *Service) publish(event valueobject.AgentEvent) {
	if s.options.Publish != nil {
		s.options.Publish(event)
	}
}

func inputHash(input map[string]any) string {
	data, _ := json.Marshal(input)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
