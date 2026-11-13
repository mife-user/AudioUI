package workflow

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/valueobject"
)

type runLock struct {
	RunID           string
	ProjectID       string
	ProjectRevision int64
	TreeID          string
	NodeID          string
	Path            []string
	InputNodes      []string
	Members         map[string]struct{}
}

// AcquireRunLock records the topic-root-to-target path of an in-flight run.
// M2-07 calls this when a run starts; M2-02 only enforces the read-only set.
func (s *Service) AcquireRunLock(ctx context.Context, runID, treeID, nodeID string, chainPath []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := valueobject.ValidateID(runID); err != nil {
		return fmt.Errorf("%w: runId: %v", ErrInvalidInput, err)
	}
	if err := valueobject.ValidateID(treeID); err != nil {
		return fmt.Errorf("%w: treeId: %v", ErrInvalidInput, err)
	}
	if err := valueobject.ValidateID(nodeID); err != nil {
		return fmt.Errorf("%w: nodeId: %v", ErrInvalidInput, err)
	}
	if len(chainPath) == 0 || chainPath[0] != treeID || chainPath[len(chainPath)-1] != nodeID {
		return fmt.Errorf("%w: chainPath must run from tree root to target", ErrNotInChain)
	}
	project, graph, err := s.loadCurrent(ctx)
	if err != nil {
		return err
	}
	for i, id := range chainPath {
		if err := valueobject.ValidateID(id); err != nil {
			return fmt.Errorf("%w: chainPath: %v", ErrInvalidInput, err)
		}
		if _, ok := graph.Node(id); !ok {
			return fmt.Errorf("%w: chainPath node %s", ErrNodeNotFound, id)
		}
		if i == 0 {
			if node, _ := graph.Node(id); node.Type != valueobject.NodeTypeTopic {
				return fmt.Errorf("%w: chain root must be a topic", ErrNotInChain)
			}
			continue
		}
		path, err := graph.PathToRoot(id)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrNotInChain, err)
		}
		if len(path) != i+1 || path[i] != id || path[i-1] != chainPath[i-1] {
			return fmt.Errorf("%w: chainPath is not a parent path", ErrNotInChain)
		}
	}
	if existing, ok := s.locks[runID]; ok && existing.ProjectID == project.ID && existing.TreeID == treeID && existing.NodeID == nodeID && slices.Equal(existing.Path, chainPath) {
		return nil
	}
	s.recordRunLock(runID, project.ID, project.Revision, treeID, nodeID, chainPath, "")
	s.changedWithReason(project.ID, project.Revision, ChangeReasonRunLock)
	return nil
}

// BeginRun takes an executable project snapshot and records its path lock
// while holding the same workflow mutex used by graph commands. The caller
// builds context from the returned immutable snapshot.
func (s *Service) BeginRun(ctx context.Context, runID, nodeID string, expectedProjectID ...string) (entity.Project, entity.RunExecutionSnapshot, error) {
	return s.BeginRunWithInputs(ctx, runID, nodeID, nil, expectedProjectID...)
}

func (s *Service) BeginRunWithInputs(ctx context.Context, runID, nodeID string, inputEdgeIDs []string, expectedProjectID ...string) (entity.Project, entity.RunExecutionSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return entity.Project{}, entity.RunExecutionSnapshot{}, err
	}
	if err := valueobject.ValidateID(runID); err != nil {
		return entity.Project{}, entity.RunExecutionSnapshot{}, fmt.Errorf("%w: runId: %v", ErrInvalidInput, err)
	}
	project, graph, err := s.loadCurrentForProject(ctx, firstExpectedProjectID(expectedProjectID))
	if err != nil {
		return entity.Project{}, entity.RunExecutionSnapshot{}, err
	}
	node, err := requireNode(graph, nodeID, "node")
	if err != nil {
		return entity.Project{}, entity.RunExecutionSnapshot{}, err
	}
	if node.Locked {
		return entity.Project{}, entity.RunExecutionSnapshot{}, fmt.Errorf("%w: %s", ErrNodeLocked, nodeID)
	}
	if err := validateRunEntry(node); err != nil {
		return entity.Project{}, entity.RunExecutionSnapshot{}, err
	}
	snapshot, err := graph.SnapshotForRunWithInputs(nodeID, inputEdgeIDs)
	if err != nil {
		return entity.Project{}, entity.RunExecutionSnapshot{}, err
	}
	if snapshot.Version != 3 {
		if err := validateRunPath(graph, snapshot.TreeID, nodeID, snapshot.ChainPath); err != nil {
			return entity.Project{}, entity.RunExecutionSnapshot{}, err
		}
	}
	members := snapshot.ScopeNodeIDs()
	if snapshot.OutputBinding.Bound {
		members = append(members, snapshot.OutputBinding.NodeID)
	}
	for _, id := range members {
		if s.isNodeLocked(id) {
			return entity.Project{}, entity.RunExecutionSnapshot{}, fmt.Errorf("%w: %s", ErrRunningChainLocked, id)
		}
	}
	s.recordRunLockForSnapshot(runID, project.ID, project.Revision, snapshot)
	s.changedWithReason(project.ID, project.Revision, ChangeReasonRunLock)
	return project.Clone(), snapshot.Clone(), nil
}

func validateRunPath(graph *entity.Graph, treeID, nodeID string, chainPath []string) error {
	if len(chainPath) == 0 || chainPath[0] != treeID || chainPath[len(chainPath)-1] != nodeID {
		return fmt.Errorf("%w: chainPath must run from tree root to target", ErrNotInChain)
	}
	for i, id := range chainPath {
		if err := valueobject.ValidateID(id); err != nil {
			return fmt.Errorf("%w: chainPath: %v", ErrInvalidInput, err)
		}
		if _, ok := graph.Node(id); !ok {
			return fmt.Errorf("%w: chainPath node %s", ErrNodeNotFound, id)
		}
		if i == 0 {
			if node, _ := graph.Node(id); node.Type != valueobject.NodeTypeTopic {
				return fmt.Errorf("%w: chain root must be a topic", ErrNotInChain)
			}
			continue
		}
		path, err := graph.PathToRoot(id)
		if err != nil || len(path) != i+1 || path[i] != id || path[i-1] != chainPath[i-1] {
			return fmt.Errorf("%w: chainPath is not a parent path", ErrNotInChain)
		}
	}
	return nil
}

func (s *Service) recordRunLock(runID, projectID string, projectRevision int64, treeID, nodeID string, chainPath []string, outputNodeID string) {
	inputNodes := append([]string(nil), chainPath...)
	members := make(map[string]struct{}, len(inputNodes)+1)
	for _, id := range inputNodes {
		members[id] = struct{}{}
	}
	if outputNodeID != "" {
		members[outputNodeID] = struct{}{}
	}
	s.recordRunLockState(runLock{
		RunID: runID, ProjectID: projectID, ProjectRevision: projectRevision,
		TreeID: treeID, NodeID: nodeID, Path: append([]string(nil), chainPath...),
		InputNodes: inputNodes, Members: members,
	})
}

func (s *Service) recordRunLockForSnapshot(runID, projectID string, projectRevision int64, snapshot entity.RunExecutionSnapshot) {
	inputNodes := snapshot.ScopeNodeIDs()
	members := make(map[string]struct{}, len(inputNodes)+1)
	for _, id := range inputNodes {
		members[id] = struct{}{}
	}
	if snapshot.OutputBinding.Bound {
		members[snapshot.OutputBinding.NodeID] = struct{}{}
	}
	s.recordRunLockState(runLock{
		RunID: runID, ProjectID: projectID, ProjectRevision: projectRevision,
		TreeID: snapshot.TreeID, NodeID: snapshot.NodeID,
		Path: append([]string(nil), snapshot.ChainPath...), InputNodes: inputNodes, Members: members,
	})
}

func (s *Service) recordRunLockState(lock runLock) {
	if s.locks == nil {
		s.locks = make(map[string]runLock)
	}
	s.locks[lock.RunID] = lock
}

func (s *Service) ReleaseRunLock(ctx context.Context, runID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	lock, exists := s.locks[runID]
	if !exists {
		return nil
	}
	delete(s.locks, runID)
	s.changedWithReason(lock.ProjectID, lock.ProjectRevision, ChangeReasonRunLock)
	return nil
}

// LockedNodeIDs returns the union of all in-flight run paths, sorted.
func (s *Service) LockedNodeIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lockedNodeIDsLocked("")
}

// LockedNodeIDsForProject returns only locks associated with one project.
func (s *Service) LockedNodeIDsForProject(projectID string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lockedNodeIDsLocked(projectID)
}

func (s *Service) lockedNodeIDsLocked(projectID string) []string {
	union := make(map[string]struct{})
	for _, lock := range s.locks {
		if projectID != "" && lock.ProjectID != projectID {
			continue
		}
		for id := range lock.Members {
			union[id] = struct{}{}
		}
	}
	result := make([]string, 0, len(union))
	for id := range union {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

// isNodeLocked and ensureEditable require the caller to hold s.mu.
func (s *Service) isNodeLocked(id string) bool {
	for _, lock := range s.locks {
		if _, ok := lock.Members[id]; ok {
			return true
		}
	}
	return false
}

func (s *Service) isNodeLockedByOtherRun(runID, id string) bool {
	for owner, lock := range s.locks {
		if owner == runID {
			continue
		}
		if _, ok := lock.Members[id]; ok {
			return true
		}
	}
	return false
}

func (s *Service) ensureEditable(ids ...string) error {
	for _, id := range ids {
		if s.isNodeLocked(id) {
			return fmt.Errorf("%w: %s", ErrRunningChainLocked, id)
		}
	}
	return nil
}
