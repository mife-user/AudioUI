package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/valueobject"
	contextservice "audio-ui/internal/service/context"
	runservice "audio-ui/internal/service/run"
)

type RunService struct {
	manager *runservice.Manager
}

func NewRunService(manager *runservice.Manager) *RunService {
	return &RunService{manager: manager}
}

func (h *RunService) Start(nodeID, projectID string, options entity.RunStartOptions) entity.RunStartResult {
	nodeID = strings.TrimSpace(nodeID)
	projectID = strings.TrimSpace(projectID)
	if nodeID == "" {
		return entity.RunStartResult{Error: runStartError(errors.New("INVALID_INPUT: 节点 ID 不能为空"))}
	}
	if projectID == "" {
		return entity.RunStartResult{Error: runStartError(errors.New("INVALID_INPUT: 工程 ID 不能为空"))}
	}
	if h.manager == nil {
		return entity.RunStartResult{Error: runStartError(errors.New("RUN_MANAGER_UNAVAILABLE"))}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	record, err := h.manager.StartWithOptions(ctx, nodeID, options, projectID)
	result := entity.RunStartResult{}
	if record.ID != "" {
		result.Run = &record
	}
	if err != nil {
		result.Error = runStartError(err)
	}
	return result
}

func runStartError(err error) *entity.RunStartError {
	if err == nil {
		return nil
	}
	failure := &entity.RunStartError{Code: runservice.ErrorCode(err), Message: err.Error()}
	var tooLarge *contextservice.ContextTooLargeError
	if errors.As(err, &tooLarge) {
		estimatedTokens := tooLarge.EstimatedTokens
		tokenBudget := tooLarge.TokenBudget
		failure.EstimatedTokens = &estimatedTokens
		failure.TokenBudget = &tokenBudget
	}
	return failure
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
