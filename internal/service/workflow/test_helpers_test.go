package workflow

import (
	"context"
	"strings"
	"testing"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
)

type fakeSession struct {
	project   entity.Project
	saveErr   error
	saveCount int
}

func (s *fakeSession) Current(ctx context.Context) (entity.Project, error) {
	if err := ctx.Err(); err != nil {
		return entity.Project{}, err
	}
	return s.project.Clone(), nil
}

func (s *fakeSession) Save(ctx context.Context, project entity.Project) (entity.Project, error) {
	if err := ctx.Err(); err != nil {
		return entity.Project{}, err
	}
	if s.saveErr != nil {
		return entity.Project{}, s.saveErr
	}
	s.project = project.Clone()
	s.saveCount++
	return project.Clone(), nil
}

func (s *fakeSession) WithExclusive(ctx context.Context, fn func(port.ProjectAccess) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn(s)
}

type moveCall struct {
	projectID    string
	sourceRootID string
	targetRootID string
	nodeIDs      []string
}

type fakeMove struct {
	mover *fakeMover
}

func (m *fakeMove) Rollback(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mover.rollbacks++
	return m.mover.rollbackErr
}

type fakeMover struct {
	calls       []moveCall
	err         error
	rollbackErr error
	rollbacks   int
}

func (m *fakeMover) MoveSubtree(ctx context.Context, projectID, sourceRootID, targetRootID string, nodeIDs []string) (port.WorkspaceMove, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.calls = append(m.calls, moveCall{
		projectID: projectID, sourceRootID: sourceRootID, targetRootID: targetRootID,
		nodeIDs: append([]string(nil), nodeIDs...),
	})
	if m.err != nil {
		return nil, m.err
	}
	return &fakeMove{mover: m}, nil
}

type changeCall struct {
	projectID string
	revision  int64
	reason    string
}

func intPtr(value int) *int {
	return &value
}

func attachEdge(project *entity.Project, sourceID, targetID string) {
	for i := range project.Nodes {
		if project.Nodes[i].ID == targetID && project.Nodes[i].Type == valueobject.NodeTypeCandidate {
			project.Edges = append(project.Edges, entity.Edge{
				ID: "e_" + targetID, SourceNodeID: sourceID, TargetNodeID: targetID, Kind: entity.EdgeKindCandidateSource,
			})
			return
		}
	}
	project.Edges = append(project.Edges, entity.Edge{
		ID: "e_" + targetID, SourceNodeID: sourceID, TargetNodeID: targetID, Kind: entity.EdgeKindConnection,
	})
}

func parentOf(project entity.Project, childID string) string {
	for _, edge := range project.Edges {
		if edge.TargetNodeID == childID {
			return edge.SourceNodeID
		}
	}
	return ""
}

func baseProject() entity.Project {
	artifactID := "sha256:" + strings.Repeat("0", 64)
	project := entity.Project{
		Version:      valueobject.ProjectFileVersion,
		ID:           "p_test",
		Name:         "测试工程",
		Revision:     1,
		ActiveNodeID: "n_a",
		Nodes: []entity.Node{
			{
				ID: "n_topic", Type: valueobject.NodeTypeTopic, Title: "选题", WorkspaceRootID: "n_topic",
				Position: entity.Position{X: 0, Y: 0}, Size: entity.DefaultNodeSize(valueobject.NodeTypeTopic), Params: entity.NodeParams{Idea: "想法", CandidateCount: 3},
				Rounds: []entity.Round{{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: artifactID}},
				Locked: true,
			},
			{
				ID: "n_candidate", Type: valueobject.NodeTypeCandidate, Title: "候选一", WorkspaceRootID: "n_topic",
				CandidateSource: &entity.CandidateSource{TopicNodeID: "n_topic", ArtifactID: artifactID, Index: 0},
				Size:            entity.DefaultNodeSize(valueobject.NodeTypeCandidate),
				Rounds:          []entity.Round{},
			},
			{
				ID: "n_a", Type: valueobject.NodeTypePrompt, Title: "A", WorkspaceRootID: "n_topic",
				Position: entity.Position{X: 10, Y: 10}, Size: entity.DefaultNodeSize(valueobject.NodeTypePrompt),
				Params: entity.NodeParams{Instruction: "A"}, Rounds: []entity.Round{},
			},
			{
				ID: "n_b", Type: valueobject.NodeTypeAgent, Title: "B", WorkspaceRootID: "n_topic",
				Position: entity.Position{X: 20, Y: 20}, Size: entity.DefaultNodeSize(valueobject.NodeTypeAgent),
				Rounds: []entity.Round{},
			},
			{
				ID: "n_candidate2", Type: valueobject.NodeTypeCandidate, Title: "候选二", WorkspaceRootID: "n_topic",
				CandidateSource: &entity.CandidateSource{TopicNodeID: "n_topic", ArtifactID: artifactID, Index: 1},
				Size:            entity.DefaultNodeSize(valueobject.NodeTypeCandidate),
				Rounds:          []entity.Round{},
			},
			{
				ID: "n_c", Type: valueobject.NodeTypePrompt, Title: "C", WorkspaceRootID: "n_topic",
				Position: entity.Position{X: 30, Y: 10}, Size: entity.DefaultNodeSize(valueobject.NodeTypePrompt),
				Params: entity.NodeParams{Instruction: "C"}, Rounds: []entity.Round{},
			},
			{
				ID: "n_draft", Type: valueobject.NodeTypePrompt, Title: "草稿",
				Position: entity.Position{X: 40, Y: 40}, Size: entity.DefaultNodeSize(valueobject.NodeTypePrompt), Params: entity.NodeParams{Instruction: "D"},
				Rounds: []entity.Round{},
			},
			{ID: "n_agent_draft", Type: valueobject.NodeTypeAgent, Title: "Agent 草稿", Size: entity.DefaultNodeSize(valueobject.NodeTypeAgent), Rounds: []entity.Round{}},
		},
	}
	attachEdge(&project, "n_topic", "n_candidate")
	attachEdge(&project, "n_candidate", "n_a")
	attachEdge(&project, "n_a", "n_b")
	attachEdge(&project, "n_topic", "n_candidate2")
	attachEdge(&project, "n_candidate2", "n_c")
	return project
}

func projectWithSecondTree() entity.Project {
	project := baseProject()
	artifactID := "sha256:" + strings.Repeat("1", 64)
	project.Nodes = append(project.Nodes,
		entity.Node{
			ID: "n_topic2", Type: valueobject.NodeTypeTopic, Title: "选题 2", WorkspaceRootID: "n_topic2", Size: entity.DefaultNodeSize(valueobject.NodeTypeTopic),
			Params: entity.NodeParams{Idea: "想法 2", CandidateCount: 3},
			Rounds: []entity.Round{{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: artifactID}},
			Locked: true,
		},
		entity.Node{
			ID: "n_x", Type: valueobject.NodeTypeCandidate, Title: "候选 X", WorkspaceRootID: "n_topic2",
			CandidateSource: &entity.CandidateSource{TopicNodeID: "n_topic2", ArtifactID: artifactID, Index: 0},
			Rounds:          []entity.Round{},
		},
		entity.Node{
			ID: "n_y", Type: valueobject.NodeTypePrompt, Title: "Y", WorkspaceRootID: "n_topic2",
			Size: entity.DefaultNodeSize(valueobject.NodeTypePrompt), Params: entity.NodeParams{Instruction: "Y"}, Rounds: []entity.Round{},
		},
	)
	attachEdge(&project, "n_topic2", "n_x")
	attachEdge(&project, "n_x", "n_y")
	return project
}

type harness struct {
	service *Service
	session *fakeSession
	mover   *fakeMover
	changes *[]changeCall
}

func newHarness(t *testing.T, project entity.Project) *harness {
	t.Helper()
	session := &fakeSession{project: project}
	mover := &fakeMover{}
	changes := &[]changeCall{}
	service := New(Options{
		Project:   session,
		Workspace: mover,
		OnChange: func(projectID string, revision int64, reason string) {
			*changes = append(*changes, changeCall{projectID: projectID, revision: revision, reason: reason})
		},
	})
	return &harness{service: service, session: session, mover: mover, changes: changes}
}

func (h *harness) parents(t *testing.T, id string) []string {
	t.Helper()
	graph, err := entity.NewGraph(&h.session.project)
	if err != nil {
		t.Fatal(err)
	}
	return graph.Parents(id)
}

func (h *harness) node(t *testing.T, id string) *entity.Node {
	t.Helper()
	for i := range h.session.project.Nodes {
		if h.session.project.Nodes[i].ID == id {
			return &h.session.project.Nodes[i]
		}
	}
	t.Fatalf("node %s not found", id)
	return nil
}
