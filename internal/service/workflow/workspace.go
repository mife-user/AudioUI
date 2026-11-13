package workflow

import (
	"context"
	"fmt"
	"sort"

	"audio-ui/internal/domain/entity"
)

// WorkspaceView is one project-scoped source read for a UI workspace
// snapshot. It contains only pending index entries, never pending payloads.
type WorkspaceView struct {
	Project       entity.Project
	NodePaths     map[string][]string
	Pending       []entity.Pending
	Chains        []entity.Chain
	RunLocks      []entity.WorkspaceRunLock
	LockedNodeIDs []string
}

// WorkspaceView reads the graph, pending index, derived chains, and path locks
// while holding the same workflow mutex used by all graph/pending commands.
func (s *Service) WorkspaceView(ctx context.Context) (WorkspaceView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return WorkspaceView{}, err
	}
	project, err := s.opts.Project.Current(ctx)
	if err != nil {
		return WorkspaceView{}, err
	}
	graph, err := entity.NewGraph(&project)
	if err != nil {
		return WorkspaceView{}, fmt.Errorf("图状态无效: %w", err)
	}
	if err := graph.Validate(); err != nil {
		return WorkspaceView{}, fmt.Errorf("图状态无效: %w", err)
	}
	pendingStore, err := s.pendingStore(ctx, project.ID)
	if err != nil {
		return WorkspaceView{}, err
	}
	pending, err := pendingStore.List(ctx)
	if err != nil {
		return WorkspaceView{}, err
	}
	for i := range pending {
		pending[i].ChainPath = append([]string(nil), pending[i].ChainPath...)
	}
	chains := graph.LeafChains()
	for i := range chains {
		chains[i].Path = append([]string(nil), chains[i].Path...)
		chains[i].NodeIDs = append([]string(nil), chains[i].NodeIDs...)
	}
	nodePaths := make(map[string][]string, len(project.Nodes))
	for _, node := range project.Nodes {
		if path, err := graph.PathToRoot(node.ID); err == nil {
			nodePaths[node.ID] = path
		}
	}
	runLocks := make([]entity.WorkspaceRunLock, 0)
	for _, lock := range s.locks {
		if lock.ProjectID != project.ID {
			continue
		}
		runLocks = append(runLocks, entity.WorkspaceRunLock{
			RunID: lock.RunID, TreeID: lock.TreeID, NodeID: lock.NodeID,
			ChainPath: append([]string(nil), lock.Path...), InputNodeIDs: append([]string(nil), lock.InputNodes...),
		})
	}
	sort.Slice(runLocks, func(i, j int) bool { return runLocks[i].RunID < runLocks[j].RunID })
	return WorkspaceView{
		Project: project.Clone(), NodePaths: nodePaths, Pending: pending, Chains: chains, RunLocks: runLocks,
		LockedNodeIDs: s.lockedNodeIDsLocked(project.ID),
	}, nil
}
