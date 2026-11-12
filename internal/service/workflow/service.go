package workflow

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"unicode/utf8"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
)

const (
	ChangeReasonProject = "project"
	ChangeReasonGraph   = "graph"
	ChangeReasonPending = "pending"
	ChangeReasonRun     = "run"
	ChangeReasonRunLock = "run_lock"
)

type Options struct {
	Project         port.ProjectSession
	CandidateReader TopicCandidateReader
	Workspace       port.WorkspaceMover
	Pending         port.PendingStoreProvider
	Artifacts       port.ArtifactStoreProvider
	ContextHasher   port.ContextHasher
	OnChange        func(projectID string, revision int64, reason string)
}

type Service struct {
	opts  Options
	mu    sync.Mutex
	locks map[string]runLock
}

func New(opts Options) *Service {
	return &Service{opts: opts, locks: make(map[string]runLock)}
}

type AddNodeInput struct {
	ProjectID       string
	Type            valueobject.NodeType
	Title           string
	Parent          string
	ParentCandidate *int
	Position        entity.Position
	Params          entity.NodeParams
}

func (s *Service) AddNode(ctx context.Context, input AddNodeInput) (entity.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return entity.Project{}, err
	}
	if !input.Type.Valid() {
		return entity.Project{}, fmt.Errorf("%w: %q", ErrUnknownNodeType, input.Type)
	}
	if err := validatePosition(input.Position.X, input.Position.Y); err != nil {
		return entity.Project{}, err
	}
	if err := validateParams(input.Params); err != nil {
		return entity.Project{}, err
	}
	title, err := normalizeTitle(input.Title, input.Type)
	if err != nil {
		return entity.Project{}, err
	}
	project, graph, err := s.loadCurrentForProject(ctx, input.ProjectID)
	if err != nil {
		return entity.Project{}, err
	}
	node := entity.Node{
		Type:     input.Type,
		Title:    title,
		Position: input.Position,
		Size:     entity.DefaultNodeSize(input.Type),
		Params:   input.Params,
		Rounds:   []entity.Round{},
	}
	if input.ParentCandidate != nil {
		return entity.Project{}, fmt.Errorf("%w: parentCandidate is not supported", ErrNodeConnectionInvalid)
	}
	switch input.Type {
	case valueobject.NodeTypeTopic:
		if input.Parent != "" {
			return entity.Project{}, fmt.Errorf("%w: topic must be a root", ErrTopicRootParent)
		}
	case valueobject.NodeTypeCandidate:
		return entity.Project{}, fmt.Errorf("%w: candidate nodes are application-generated", ErrNodeConnectionInvalid)
	case valueobject.NodeTypePrompt, valueobject.NodeTypeAgent, valueobject.NodeTypeScript:
		if input.Parent != "" {
			parent, err := requireNode(graph, input.Parent, "parent")
			if err != nil {
				return entity.Project{}, err
			}
			if s.isNodeLocked(parent.ID) {
				return entity.Project{}, fmt.Errorf("%w: %s", ErrRunningChainLocked, parent.ID)
			}
			if _, err := chainRoot(graph, parent.ID); err != nil {
				return entity.Project{}, err
			}
			if err := entity.ValidateNodeConnection(*parent, node); err != nil {
				return entity.Project{}, err
			}
			node.Parent = parent.ID
		}
	}
	id, err := valueobject.NewID(valueobject.NodeIDPrefix)
	if err != nil {
		return entity.Project{}, err
	}
	node.ID = id
	project.Nodes = append(project.Nodes, node)
	if err := validateFinalGraph(&project); err != nil {
		return entity.Project{}, fmt.Errorf("图校验失败: %w", err)
	}
	project.Revision++
	saved, err := s.opts.Project.Save(ctx, project)
	if err != nil {
		return entity.Project{}, err
	}
	s.changed(saved.ID, saved.Revision)
	return saved, nil
}

func (s *Service) Connect(ctx context.Context, parentID, childID string, parentCandidate *int, replace bool, expectedProjectID ...string) (entity.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return entity.Project{}, err
	}
	project, graph, err := s.loadCurrentForProject(ctx, firstExpectedProjectID(expectedProjectID))
	if err != nil {
		return entity.Project{}, err
	}
	child, err := requireNode(graph, childID, "node")
	if err != nil {
		return entity.Project{}, err
	}
	if child.Type == valueobject.NodeTypeTopic {
		return entity.Project{}, fmt.Errorf("%w: topic %s cannot be a child", ErrTopicRootParent, childID)
	}
	if child.Type == valueobject.NodeTypeCandidate {
		return entity.Project{}, fmt.Errorf("%w: candidate source is immutable", ErrNodeConnectionInvalid)
	}
	parent, err := requireNode(graph, parentID, "parent")
	if err != nil {
		return entity.Project{}, err
	}
	if parentID == childID {
		return entity.Project{}, fmt.Errorf("%w: %s", ErrSelfConnect, childID)
	}
	if graph.IsAncestor(childID, parentID) {
		return entity.Project{}, fmt.Errorf("%w: %s is an ancestor of %s", ErrCycleDetected, childID, parentID)
	}
	if _, err := chainRoot(graph, parentID); err != nil {
		return entity.Project{}, err
	}
	if parentCandidate != nil {
		return entity.Project{}, fmt.Errorf("%w: parentCandidate is not supported", ErrNodeConnectionInvalid)
	}
	if err := entity.ValidateNodeConnection(*parent, *child); err != nil {
		return entity.Project{}, err
	}
	if child.Parent == parentID {
		return project, nil
	}

	subtree, err := graph.Subtree(childID)
	if err != nil {
		return entity.Project{}, err
	}
	if err := s.ensureEditable(subtree...); err != nil {
		return entity.Project{}, err
	}
	if err := s.ensureEditable(parentID); err != nil {
		return entity.Project{}, err
	}
	if child.Parent != "" && !replace {
		return entity.Project{}, fmt.Errorf("%w: %s -> %s", ErrReparentConfirmRequired, child.Parent, parentID)
	}

	sourceRoot := ""
	if child.Parent != "" {
		sourceRoot, err = chainRoot(graph, childID)
		if err != nil {
			return entity.Project{}, err
		}
	}
	targetRoot, err := chainRoot(graph, parentID)
	if err != nil {
		return entity.Project{}, err
	}

	child.Parent = parentID
	child.ParentCandidate = nil
	markSubtreeStale(&project, subtree)
	project.Revision++

	var move port.WorkspaceMove
	if sourceRoot != "" && sourceRoot != targetRoot {
		if s.opts.Workspace == nil {
			return entity.Project{}, fmt.Errorf("%w: workspace mover unavailable", ErrWorkspaceMoveFailed)
		}
		move, err = s.opts.Workspace.MoveSubtree(ctx, project.ID, sourceRoot, targetRoot, subtree)
		if err != nil {
			return entity.Project{}, fmt.Errorf("%w: %v", ErrWorkspaceMoveFailed, err)
		}
	}
	if err := validateFinalGraph(&project); err != nil {
		if rollbackErr := rollbackMove(move); rollbackErr != nil {
			return entity.Project{}, errors.Join(fmt.Errorf("图校验失败: %w", err), rollbackErr)
		}
		return entity.Project{}, fmt.Errorf("图校验失败: %w", err)
	}
	saved, err := s.opts.Project.Save(ctx, project)
	if err != nil {
		if rollbackErr := rollbackMove(move); rollbackErr != nil {
			return entity.Project{}, errors.Join(err, rollbackErr)
		}
		return entity.Project{}, err
	}
	s.changed(saved.ID, saved.Revision)
	return saved, nil
}

func (s *Service) MoveNode(ctx context.Context, id string, x, y float64, expectedProjectID ...string) (entity.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return entity.Project{}, err
	}
	if err := validatePosition(x, y); err != nil {
		return entity.Project{}, err
	}
	project, graph, err := s.loadCurrentForProject(ctx, firstExpectedProjectID(expectedProjectID))
	if err != nil {
		return entity.Project{}, err
	}
	node, err := requireNode(graph, id, "node")
	if err != nil {
		return entity.Project{}, err
	}
	if node.Position.X == x && node.Position.Y == y {
		return project, nil
	}
	node.Position = entity.Position{X: x, Y: y}
	project.Revision++
	saved, err := s.opts.Project.Save(ctx, project)
	if err != nil {
		return entity.Project{}, err
	}
	s.changed(saved.ID, saved.Revision)
	return saved, nil
}

func (s *Service) SetNodeLayout(ctx context.Context, id string, x, y, width, height float64, expectedProjectID ...string) (entity.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return entity.Project{}, err
	}
	if err := validatePosition(x, y); err != nil {
		return entity.Project{}, err
	}
	size := entity.NodeSize{Width: width, Height: height}
	if err := validateNodeSize(size); err != nil {
		return entity.Project{}, err
	}
	project, graph, err := s.loadCurrentForProject(ctx, firstExpectedProjectID(expectedProjectID))
	if err != nil {
		return entity.Project{}, err
	}
	node, err := requireNode(graph, id, "node")
	if err != nil {
		return entity.Project{}, err
	}
	position := entity.Position{X: x, Y: y}
	if node.Position == position && node.Size == size {
		return project, nil
	}
	node.Position = position
	node.Size = size
	project.Revision++
	saved, err := s.opts.Project.Save(ctx, project)
	if err != nil {
		return entity.Project{}, err
	}
	s.changed(saved.ID, saved.Revision)
	return saved, nil
}

func (s *Service) SetNodeCollapsed(ctx context.Context, id string, collapsed bool, expectedProjectID ...string) (entity.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return entity.Project{}, err
	}
	project, graph, err := s.loadCurrentForProject(ctx, firstExpectedProjectID(expectedProjectID))
	if err != nil {
		return entity.Project{}, err
	}
	node, err := requireNode(graph, id, "node")
	if err != nil {
		return entity.Project{}, err
	}
	if node.Collapsed == collapsed {
		return project, nil
	}
	node.Collapsed = collapsed
	project.Revision++
	saved, err := s.opts.Project.Save(ctx, project)
	if err != nil {
		return entity.Project{}, err
	}
	s.changed(saved.ID, saved.Revision)
	return saved, nil
}

func (s *Service) UpdateNodeParams(ctx context.Context, id, title string, params entity.NodeParams, expectedProjectID ...string) (entity.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return entity.Project{}, err
	}
	if err := validateParams(params); err != nil {
		return entity.Project{}, err
	}
	project, graph, err := s.loadCurrentForProject(ctx, firstExpectedProjectID(expectedProjectID))
	if err != nil {
		return entity.Project{}, err
	}
	node, err := requireNode(graph, id, "node")
	if err != nil {
		return entity.Project{}, err
	}
	if node.Type == valueobject.NodeTypeCandidate {
		return entity.Project{}, fmt.Errorf("%w: candidate content is artifact-bound", ErrNodeLocked)
	}
	if s.isNodeLocked(id) {
		return entity.Project{}, fmt.Errorf("%w: %s", ErrRunningChainLocked, id)
	}
	if node.Locked {
		return entity.Project{}, fmt.Errorf("%w: %s", ErrNodeLocked, id)
	}
	newTitle := node.Title
	if trimmed := strings.TrimSpace(title); trimmed != "" {
		if utf8.RuneCountInString(trimmed) > valueobject.MaxNodeTitleLength {
			return entity.Project{}, fmt.Errorf("%w: title is too long", ErrInvalidInput)
		}
		newTitle = trimmed
	}
	if newTitle == node.Title && reflect.DeepEqual(node.Params, params) {
		return project, nil
	}
	node.Title = newTitle
	node.Params = params
	project.Revision++
	saved, err := s.opts.Project.Save(ctx, project)
	if err != nil {
		return entity.Project{}, err
	}
	s.changed(saved.ID, saved.Revision)
	return saved, nil
}

// EnsureExecutable is the M2-07 pre-run guard: locked nodes and nodes on a
// run path cannot start another run. Draft nodes are not runnable.
func (s *Service) EnsureExecutable(ctx context.Context, nodeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	_, graph, err := s.loadCurrent(ctx)
	if err != nil {
		return err
	}
	node, err := requireNode(graph, nodeID, "node")
	if err != nil {
		return err
	}
	if s.isNodeLocked(nodeID) {
		return fmt.Errorf("%w: %s", ErrRunningChainLocked, nodeID)
	}
	if node.Locked {
		return fmt.Errorf("%w: %s", ErrNodeLocked, nodeID)
	}
	if err := validateRunEntry(node); err != nil {
		return err
	}
	if _, err := chainRoot(graph, nodeID); err != nil {
		return err
	}
	return nil
}

func (s *Service) SelectNode(ctx context.Context, id string, expectedProjectID ...string) (entity.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return entity.Project{}, err
	}
	project, graph, err := s.loadCurrentForProject(ctx, firstExpectedProjectID(expectedProjectID))
	if err != nil {
		return entity.Project{}, err
	}
	if _, err := requireNode(graph, id, "node"); err != nil {
		return entity.Project{}, err
	}
	if project.ActiveNodeID == id {
		return project, nil
	}
	project.ActiveNodeID = id
	saved, err := s.opts.Project.Save(ctx, project)
	if err != nil {
		return entity.Project{}, err
	}
	return saved, nil
}

func (s *Service) changed(projectID string, revision int64) {
	s.changedWithReason(projectID, revision, ChangeReasonGraph)
}

func (s *Service) changedPending(projectID string, revision int64) {
	s.changedWithReason(projectID, revision, ChangeReasonPending)
}

func (s *Service) changedWithReason(projectID string, revision int64, reason string) {
	s.changedForProject(projectID, revision, reason)
}

func (s *Service) changedForProject(projectID string, revision int64, reason string) {
	for runID, lock := range s.locks {
		if lock.ProjectID == projectID {
			lock.ProjectRevision = revision
			s.locks[runID] = lock
		}
	}
	if s.opts.OnChange != nil {
		s.opts.OnChange(projectID, revision, reason)
	}
}

func rollbackMove(move port.WorkspaceMove) error {
	if move == nil {
		return nil
	}
	if err := move.Rollback(context.Background()); err != nil {
		return fmt.Errorf("steps 回滚失败: %w", err)
	}
	return nil
}
