package handler

import (
	"context"
	"errors"
	"slices"
	"time"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/service/project"
	runservice "audio-ui/internal/service/run"
	"audio-ui/internal/service/workflow"
)

var ErrWorkspaceChangedDuringSnapshot = errors.New("WORKSPACE_CHANGED: retry snapshot")

type WorkspaceRevisionSource interface {
	Read() (uint64, string)
}

type WorkspaceService struct {
	workflow *workflow.Service
	runs     *runservice.Manager
	revision WorkspaceRevisionSource
}

func NewWorkspaceService(workflowService *workflow.Service, runs *runservice.Manager, revision WorkspaceRevisionSource) *WorkspaceService {
	return &WorkspaceService{workflow: workflowService, runs: runs, revision: revision}
}

// GetWorkspaceSnapshot returns a stable project-scoped view. It retries when
// a project switch, graph/pending commit, or run-lock change overlaps the read.
func (h *WorkspaceService) GetWorkspaceSnapshot() (entity.WorkspaceSnapshotResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if h.workflow == nil || h.runs == nil || h.revision == nil {
		return entity.WorkspaceSnapshotResult{}, errors.New("WORKSPACE_SNAPSHOT_DEPENDENCY_MISSING")
	}
	for attempt := 0; attempt < 3; attempt++ {
		beforeRevision, beforeProjectID := h.revision.Read()
		view, err := h.workflow.WorkspaceView(ctx)
		if errors.Is(err, project.ErrNoActiveProject) {
			afterRevision, afterProjectID := h.revision.Read()
			if beforeRevision == afterRevision && beforeProjectID == "" && afterProjectID == "" {
				return entity.WorkspaceSnapshotResult{
					Revision: afterRevision,
					Snapshot: entity.WorkspaceSnapshot{
						Nodes: []entity.Node{}, Edges: []entity.Edge{}, NodePaths: map[string][]string{}, Pending: []entity.PendingSummary{},
						Runs: []entity.WorkspaceRunSummary{}, Chains: []entity.WorkspaceChain{},
						LockedNodeIDs: []string{},
					},
				}, nil
			}
			continue
		}
		if err != nil {
			return entity.WorkspaceSnapshotResult{}, err
		}
		runs, err := h.runs.ListForProject(ctx, view.Project.ID)
		if err != nil {
			return entity.WorkspaceSnapshotResult{}, err
		}
		afterRevision, afterProjectID := h.revision.Read()
		if beforeRevision != afterRevision || beforeProjectID != afterProjectID || afterProjectID != view.Project.ID {
			continue
		}
		return entity.WorkspaceSnapshotResult{
			Revision: afterRevision,
			Snapshot: makeWorkspaceSnapshot(view, runs),
		}, nil
	}
	return entity.WorkspaceSnapshotResult{}, ErrWorkspaceChangedDuringSnapshot
}

func makeWorkspaceSnapshot(view workflow.WorkspaceView, runs []entity.RunSummary) entity.WorkspaceSnapshot {
	projectValue := &entity.WorkspaceProject{
		ID: view.Project.ID, Name: view.Project.Name,
		Revision: view.Project.Revision, ActiveNodeID: view.Project.ActiveNodeID,
	}
	nodes := make([]entity.Node, len(view.Project.Nodes))
	for i, node := range view.Project.Nodes {
		nodes[i] = node.Clone()
	}
	pending := make([]entity.PendingSummary, 0, len(view.Pending))
	for _, item := range view.Pending {
		var outputBinding *entity.RunOutputBinding
		if item.OutputBinding != nil {
			binding := *item.OutputBinding
			outputBinding = &binding
		}
		pending = append(pending, entity.PendingSummary{
			ID: item.ID, NodeID: item.NodeID, RunID: item.RunID, ChainID: item.ChainID,
			Kind: item.Kind, Status: item.Status, Summary: item.Summary, Error: item.Error,
			OutputBinding: outputBinding, ResultNodeID: item.ResultNodeID,
		})
	}
	runSummaries := make([]entity.WorkspaceRunSummary, 0, len(runs))
	for _, run := range runs {
		var actualInputTokens *int
		if run.UsageReported {
			value := run.ActualInputTokens
			actualInputTokens = &value
		}
		runSummaries = append(runSummaries, entity.WorkspaceRunSummary{
			ID: run.ID, NodeID: run.NodeID, Status: run.Status, ChainID: run.ChainID,
			ChainPath: append([]string(nil), run.ChainPath...), ContextHash: run.ContextHash,
			EstimatedTokens: run.EstimatedTokens, ActualInputTokens: actualInputTokens,
			StartedAt: run.StartedAt, FinishedAt: run.FinishedAt,
		})
	}
	knownRuns := make(map[string]struct{}, len(runSummaries))
	for _, run := range runSummaries {
		knownRuns[run.ID] = struct{}{}
	}
	for _, lock := range view.RunLocks {
		if _, exists := knownRuns[lock.RunID]; exists {
			continue
		}
		runSummaries = append(runSummaries, entity.WorkspaceRunSummary{
			ID: lock.RunID, NodeID: lock.NodeID, Status: valueobject.RunStarting,
			ChainID: "c_" + lock.NodeID, ChainPath: append([]string(nil), lock.ChainPath...),
		})
		knownRuns[lock.RunID] = struct{}{}
	}
	chains := make([]entity.WorkspaceChain, 0, len(view.Chains))
	nodeTitles := make(map[string]string, len(nodes))
	for _, node := range nodes {
		nodeTitles[node.ID] = node.Title
	}
	for _, chain := range view.Chains {
		status := chainStatus(chain, runSummaries, pending)
		chains = append(chains, entity.WorkspaceChain{
			ID: chain.ID, TailNodeID: chain.TailNodeID, Path: append([]string(nil), chain.Path...),
			NodeIDs: append([]string(nil), chain.NodeIDs...), Ambiguous: chain.Ambiguous,
			Label: nodeTitles[chain.TailNodeID], Status: status,
		})
	}
	return entity.WorkspaceSnapshot{
		Project: projectValue, Nodes: nodes, Edges: append([]entity.Edge{}, view.Project.Edges...), NodePaths: view.NodePaths,
		Pending: pending, Runs: runSummaries,
		Chains: chains, LockedNodeIDs: append([]string{}, view.LockedNodeIDs...),
	}
}

func chainStatus(chain entity.Chain, runs []entity.WorkspaceRunSummary, pending []entity.PendingSummary) string {
	for _, run := range runs {
		if run.Status != valueobject.RunStarting && run.Status != valueobject.RunRunning {
			continue
		}
		if chain.Ambiguous && slices.Contains(chain.NodeIDs, run.NodeID) || !chain.Ambiguous && pathHasPrefix(chain.Path, run.ChainPath) {
			return "running"
		}
	}
	for _, item := range pending {
		if item.Status != valueobject.PendingPending && item.Status != valueobject.PendingInvalid {
			continue
		}
		if slices.Contains(chain.NodeIDs, item.NodeID) {
			return "pending"
		}
	}
	return "idle"
}

func pathHasPrefix(path, prefix []string) bool {
	return len(prefix) > 0 && len(path) >= len(prefix) && slices.Equal(path[:len(prefix)], prefix)
}
