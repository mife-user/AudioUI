package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/valueobject"
	runservice "audio-ui/internal/service/run"
)

type RunService struct {
	manager *runservice.Manager
}

func NewRunService(manager *runservice.Manager) *RunService {
	return &RunService{manager: manager}
}

func (h *RunService) Start(nodeID string) (entity.RunRecord, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return entity.RunRecord{}, errors.New("节点 ID 不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return h.manager.Start(ctx, nodeID)
}

func (h *RunService) ListRuns() ([]entity.RunSummary, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return h.manager.List(ctx)
}

func (h *RunService) GetRun(runID string) (entity.RunRecord, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return entity.RunRecord{}, errors.New("运行 ID 不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return h.manager.Get(ctx, runID)
}

func (h *RunService) GetRunSnapshot(runID string) (entity.RunSnapshot, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return entity.RunSnapshot{}, errors.New("运行 ID 不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return h.manager.GetSnapshot(ctx, runID)
}

func (h *RunService) Interrupt(runID string) error {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return errors.New("运行 ID 不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return h.manager.Interrupt(ctx, runID)
}

func (h *RunService) RespondPermission(runID, requestID, decision string) error {
	parsed := valueobject.PermissionDecision(strings.TrimSpace(decision))
	switch parsed {
	case valueobject.DecisionAllow, valueobject.DecisionAllowSession, valueobject.DecisionDeny:
	default:
		return fmt.Errorf("无效的权限决定: %s", decision)
	}
	return h.manager.RespondPermission(strings.TrimSpace(runID), strings.TrimSpace(requestID), parsed)
}

func (h *RunService) RespondQuestion(runID, requestID, answer, via string) error {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return errors.New("回答不能为空")
	}
	if via == "" {
		via = "other"
	}
	return h.manager.RespondQuestion(strings.TrimSpace(runID), strings.TrimSpace(requestID), valueobject.QuestionAnswer{Answer: answer, Via: via})
}
