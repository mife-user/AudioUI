package workflow

import (
	"context"
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
	if input.Type == valueobject.NodeTypeCandidate {
		return entity.Project{}, fmt.Errorf("%w: candidate nodes are application-generated", ErrNodeConnectionInvalid)
	}
	if input.Parent != "" {
		parent, err := requireNode(graph, input.Parent, "parent")
		if err != nil {
			return entity.Project{}, err
		}
		if err := s.ensureEditable(parent.ID); err != nil {
			return entity.Project{}, err
		}
	}
	id, err := valueobject.NewID(valueobject.NodeIDPrefix)
	if err != nil {
		return entity.Project{}, err
	}
	node.ID = id
	if node.Type == valueobject.NodeTypeTopic {
		node.WorkspaceRootID = id
	}
	project.Nodes = append(project.Nodes, node)
	if input.Parent != "" {
		edgeID, err := valueobject.NewID(valueobject.EdgeIDPrefix)
		if err != nil {
			return entity.Project{}, err
		}
		parent, _ := graph.Node(input.Parent)
		project.Edges = append(project.Edges, entity.Edge{
			ID: edgeID, SourceNodeID: input.Parent, TargetNodeID: id, Kind: entity.EdgeKindConnection,
			ScriptVersion: scriptVersionAtConnection(project.Version, parent),
		})
	}
	if err := bindUnassignedWorkspaces(&project); err != nil {
		return entity.Project{}, err
	}
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

func (s *Service) Connect(ctx context.Context, sourceID, targetID string, expectedProjectID ...string) (entity.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return entity.Project{}, err
	}
	project, graph, err := s.loadCurrentForProject(ctx, firstExpectedProjectID(expectedProjectID))
	if err != nil {
		return entity.Project{}, err
	}
	source, err := requireNode(graph, sourceID, "source")
	if err != nil {
		return entity.Project{}, err
	}
	if _, err := requireNode(graph, targetID, "target"); err != nil {
		return entity.Project{}, err
	}
	if sourceID == targetID {
		return entity.Project{}, fmt.Errorf("%w: %s", ErrSelfConnect, targetID)
	}
	if _, exists := graph.EdgeBetween(sourceID, targetID); exists {
		return project, nil
	}
	if graph.IsAncestor(targetID, sourceID) {
		return entity.Project{}, fmt.Errorf("%w: %s → %s", ErrCycleDetected, sourceID, targetID)
	}
	descendants, err := graph.Subtree(targetID)
	if err != nil {
		return entity.Project{}, err
	}
	if err := s.ensureEditable(append(descendants, sourceID)...); err != nil {
		return entity.Project{}, err
	}
	hadInputs := len(graph.Parents(targetID)) > 0
	edgeID, err := valueobject.NewID(valueobject.EdgeIDPrefix)
	if err != nil {
		return entity.Project{}, err
	}
	project.Edges = append(project.Edges, entity.Edge{
		ID: edgeID, SourceNodeID: sourceID, TargetNodeID: targetID, Kind: entity.EdgeKindConnection,
		ScriptVersion: scriptVersionAtConnection(project.Version, source),
	})
	if hadInputs {
		markSubtreeStale(&project, descendants)
	}
	if err := bindUnassignedWorkspaces(&project); err != nil {
		return entity.Project{}, err
	}
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

func (s *Service) Disconnect(ctx context.Context, edgeID string, expectedProjectID ...string) (entity.Project, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return entity.Project{}, "", err
	}
	project, graph, err := s.loadCurrentForProject(ctx, firstExpectedProjectID(expectedProjectID))
	if err != nil {
		return entity.Project{}, "", err
	}
	edge, ok := graph.Edge(edgeID)
	if !ok {
		return entity.Project{}, "", fmt.Errorf("%w: %s", ErrEdgeNotFound, edgeID)
	}
	if edge.Kind == entity.EdgeKindCandidateSource {
		return entity.Project{}, "", fmt.Errorf("%w: 候选生成来源不能断开", ErrProtectedEdge)
	}
	descendants, err := graph.Subtree(edge.TargetNodeID)
	if err != nil {
		return entity.Project{}, "", err
	}
	if err := s.ensureEditable(append(descendants, edge.SourceNodeID)...); err != nil {
		return entity.Project{}, "", err
	}
	for i := range project.Edges {
		if project.Edges[i].ID == edgeID {
			project.Edges = append(project.Edges[:i], project.Edges[i+1:]...)
			break
		}
	}
	markSubtreeStale(&project, descendants)
	if err := bindUnassignedWorkspaces(&project); err != nil {
		return entity.Project{}, "", err
	}
	if err := validateFinalGraph(&project); err != nil {
		return entity.Project{}, "", fmt.Errorf("图校验失败: %w", err)
	}
	project.Revision++
	saved, err := s.opts.Project.Save(ctx, project)
	if err != nil {
		return entity.Project{}, "", err
	}
	s.changed(saved.ID, saved.Revision)
	return saved, edge.TargetNodeID, nil
}

func (s *Service) ReconnectTarget(ctx context.Context, expectedEdge entity.Edge, newTargetID string, expectedProjectID ...string) (entity.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return entity.Project{}, err
	}
	project, graph, err := s.loadCurrentForProject(ctx, firstExpectedProjectID(expectedProjectID))
	if err != nil {
		return entity.Project{}, err
	}
	edge, ok := graph.Edge(expectedEdge.ID)
	if !ok {
		return entity.Project{}, fmt.Errorf("%w: %s", ErrEdgeNotFound, expectedEdge.ID)
	}
	if edge != expectedEdge {
		return entity.Project{}, fmt.Errorf("%w: edge %s changed", ErrEdgeConflict, edge.ID)
	}
	if newTargetID == edge.TargetNodeID {
		return project, nil
	}
	if edge.Kind == entity.EdgeKindCandidateSource {
		return entity.Project{}, fmt.Errorf("%w: 候选生成来源不能改接", ErrProtectedEdge)
	}
	if _, err := requireNode(graph, newTargetID, "target"); err != nil {
		return entity.Project{}, err
	}
	if newTargetID == edge.SourceNodeID {
		return entity.Project{}, fmt.Errorf("%w: %s", ErrSelfConnect, newTargetID)
	}
	if _, exists := graph.EdgeBetween(edge.SourceNodeID, newTargetID); exists {
		return entity.Project{}, fmt.Errorf("%w: 目标已连接该上游", ErrEdgeConflict)
	}
	if graph.IsAncestor(newTargetID, edge.SourceNodeID) {
		return entity.Project{}, fmt.Errorf("%w: %s → %s", ErrCycleDetected, edge.SourceNodeID, newTargetID)
	}
	oldAffected, err := graph.Subtree(edge.TargetNodeID)
	if err != nil {
		return entity.Project{}, err
	}
	newAffected, err := graph.Subtree(newTargetID)
	if err != nil {
		return entity.Project{}, err
	}
	affected := make([]string, 0, len(oldAffected)+len(newAffected))
	seen := make(map[string]struct{}, cap(affected))
	addAffected := func(ids []string) {
		for _, id := range ids {
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			affected = append(affected, id)
		}
	}
	addAffected(oldAffected)
	addAffected(newAffected)
	if err := s.ensureEditable(append(affected, edge.SourceNodeID)...); err != nil {
		return entity.Project{}, err
	}
	for i := range project.Edges {
		if project.Edges[i].ID == edge.ID {
			project.Edges[i].TargetNodeID = newTargetID
			break
		}
	}
	markSubtreeStale(&project, affected)
	if err := bindUnassignedWorkspaces(&project); err != nil {
		return entity.Project{}, err
	}
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

func scriptVersionAtConnection(projectVersion int, source *entity.Node) int {
	if projectVersion < valueobject.ProjectFileVersion || source.Type != valueobject.NodeTypeScript || len(source.ScriptVersions) == 0 {
		return 0
	}
	return source.ScriptVersions[len(source.ScriptVersions)-1].N
}

func (s *Service) SetEdgeScriptVersion(ctx context.Context, edgeID string, version int, expectedProjectID ...string) (entity.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return entity.Project{}, err
	}
	if version < 0 {
		return entity.Project{}, ErrInvalidInput
	}
	project, graph, err := s.loadCurrentForProject(ctx, firstExpectedProjectID(expectedProjectID))
	if err != nil {
		return entity.Project{}, err
	}
	if project.Version < valueobject.ProjectFileVersion {
		return entity.Project{}, ErrInvalidInput
	}
	edge, ok := graph.Edge(edgeID)
	if !ok || edge.Kind != entity.EdgeKindConnection {
		return entity.Project{}, ErrEdgeNotFound
	}
	source, _ := graph.Node(edge.SourceNodeID)
	if source.Type != valueobject.NodeTypeScript || version > len(source.ScriptVersions) {
		return entity.Project{}, ErrScriptVersionNotFound
	}
	if edge.ScriptVersion == version {
		return project, nil
	}
	if err := s.ensureEditable(edge.SourceNodeID, edge.TargetNodeID); err != nil {
		return entity.Project{}, err
	}
	for i := range project.Edges {
		if project.Edges[i].ID == edgeID {
			project.Edges[i].ScriptVersion = version
			break
		}
	}
	descendants, err := graph.Subtree(edge.TargetNodeID)
	if err != nil {
		return entity.Project{}, err
	}
	markSubtreeStale(&project, descendants)
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
	if _, err := graph.SnapshotForRun(nodeID); err != nil {
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
