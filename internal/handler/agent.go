package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/service/session"
)

type AgentService struct {
	session *session.Service
}

func NewAgentService(sessionService *session.Service) *AgentService {
	return &AgentService{session: sessionService}
}

func (h *AgentService) GetState() valueobject.AgentState {
	return h.session.State()
}

func (h *AgentService) SendMessage(text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("消息不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return h.session.Send(ctx, text)
}

func (h *AgentService) RespondPermission(id, decision string) error {
	var parsed valueobject.PermissionDecision
	switch valueobject.PermissionDecision(strings.TrimSpace(decision)) {
	case valueobject.DecisionAllow:
		parsed = valueobject.DecisionAllow
	case valueobject.DecisionAllowSession:
		parsed = valueobject.DecisionAllowSession
	case valueobject.DecisionDeny:
		parsed = valueobject.DecisionDeny
	default:
		return fmt.Errorf("无效的权限决定: %s", decision)
	}
	return h.session.RespondPermission(id, parsed)
}

func (h *AgentService) RespondQuestion(id, answer, via string) error {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return errors.New("回答不能为空")
	}
	if via == "" {
		via = "other"
	}
	return h.session.RespondQuestion(id, valueobject.QuestionAnswer{Answer: answer, Via: via})
}

func (h *AgentService) NewSession() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return h.session.NewSession(ctx)
}

func (h *AgentService) CancelResume() {
	h.session.CancelResume()
}

func (h *AgentService) Interrupt() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return h.session.Interrupt(ctx)
}
