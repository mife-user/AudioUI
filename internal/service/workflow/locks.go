package workflow

import (
	"context"
	"fmt"
	"sort"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/valueobject"
)

type runLock struct {
	RunID   string
	TreeID  string
	NodeID  string
	Path    []string
	Members map[string]struct{}
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
	_, graph, err := s.loadCurrent(ctx)
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
	s.recordRunLock(runID, treeID, nodeID, chainPath)
	return nil
}

// BeginRun takes an executable project snapshot and records its path lock
// while holding the same workflow mutex used by graph commands. The caller
// builds context from the returned immutable snapshot.
func (s *Service) BeginRun(ctx context.Context, runID, nodeID string) (entity.Project, []string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return entity.Project{}, nil, err
	}
	if err := valueobject.ValidateID(runID); err != nil {
		return entity.Project{}, nil, fmt.Errorf("%w: runId: %v", ErrInvalidInput, err)
	}
	project, graph, err := s.loadCurrent(ctx)
	if err != nil {
		return entity.Project{}, nil, err
	}
	node, err := requireNode(graph, nodeID, "node")
	if err != nil {
		return entity.Project{}, nil, err
	}
	if node.Locked {
		return entity.Project{}, nil, fmt.Errorf("%w: %s", ErrNodeLocked, nodeID)
	}
	root, err := chainRoot(graph, nodeID)
	if err != nil {
		return entity.Project{}, nil, err
	}
	chainPath, err := graph.PathToRoot(nodeID)
	if err != nil {
		return entity.Project{}, nil, err
	}
	for _, id := range chainPath {
		if s.isNodeLocked(id) {
			return entity.Project{}, nil, fmt.Errorf("%w: %s", ErrRunningChainLocked, id)
		}
	}
	if err := validateRunPath(graph, root, nodeID, chainPath); err != nil {
		return entity.Project{}, nil, err
	}
	s.recordRunLock(runID, root, nodeID, chainPath)
	return project.Clone(), append([]string(nil), chainPath...), nil
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

func (s *Service) recordRunLock(runID, treeID, nodeID string, chainPath []string) {
	members := make(map[string]struct{}, len(chainPath))
	for _, id := range chainPath {
		members[id] = struct{}{}
	}
	if s.locks == nil {
		s.locks = make(map[string]runLock)
	}
	s.locks[runID] = runLock{
		RunID: runID, TreeID: treeID, NodeID: nodeID,
		Path: append([]string(nil), chainPath...), Members: members,
	}
}

func (s *Service) ReleaseRunLock(ctx context.Context, runID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	delete(s.locks, runID)
	return nil
}

// LockedNodeIDs returns the union of all in-flight run paths, sorted.
func (s *Service) LockedNodeIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	union := make(map[string]struct{})
	for _, lock := range s.locks {
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

func (s *Service) ensureEditable(ids ...string) error {
	for _, id := range ids {
		if s.isNodeLocked(id) {
			return fmt.Errorf("%w: %s", ErrRunningChainLocked, id)
		}
	}
	return nil
}
