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

func baseProject() entity.Project {
	artifactID := "sha256:" + strings.Repeat("0", 64)
	return entity.Project{
		Version:      valueobject.ProjectFileVersion,
		ID:           "p_test",
		Name:         "测试工程",
		Revision:     1,
		ActiveNodeID: "n_a",
		Nodes: []entity.Node{
			{
				ID: "n_topic", Type: valueobject.NodeTypeTopic, Title: "选题",
				Position: entity.Position{X: 0, Y: 0}, Params: entity.NodeParams{Idea: "想法", CandidateCount: 3},
				Rounds: []entity.Round{{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: artifactID}},
				Locked: true,
			},
			{
				ID: "n_candidate", Type: valueobject.NodeTypeCandidate, Title: "候选一",
				Parent: "n_topic", CandidateSource: &entity.CandidateSource{TopicNodeID: "n_topic", ArtifactID: artifactID, Index: 0},
				Rounds: []entity.Round{},
			},
			{
				ID: "n_a", Type: valueobject.NodeTypePrompt, Title: "A",
				Parent: "n_candidate", Position: entity.Position{X: 10, Y: 10},
				Params: entity.NodeParams{Instruction: "A"}, Rounds: []entity.Round{},
			},
			{
				ID: "n_b", Type: valueobject.NodeTypeAgent, Title: "B",
				Parent: "n_a", Position: entity.Position{X: 20, Y: 20},
				Rounds: []entity.Round{},
			},
			{
				ID: "n_candidate2", Type: valueobject.NodeTypeCandidate, Title: "候选二",
				Parent: "n_topic", CandidateSource: &entity.CandidateSource{TopicNodeID: "n_topic", ArtifactID: artifactID, Index: 1},
				Rounds: []entity.Round{},
			},
			{
				ID: "n_c", Type: valueobject.NodeTypePrompt, Title: "C",
				Parent: "n_candidate2", Position: entity.Position{X: 30, Y: 10},
				Params: entity.NodeParams{Instruction: "C"}, Rounds: []entity.Round{},
			},
			{
				ID: "n_draft", Type: valueobject.NodeTypePrompt, Title: "草稿",
				Position: entity.Position{X: 40, Y: 40}, Params: entity.NodeParams{Instruction: "D"},
				Rounds: []entity.Round{},
			},
			{ID: "n_agent_draft", Type: valueobject.NodeTypeAgent, Title: "Agent 草稿", Rounds: []entity.Round{}},
		},
	}
}

func projectWithSecondTree() entity.Project {
	project := baseProject()
	artifactID := "sha256:" + strings.Repeat("1", 64)
	project.Nodes = append(project.Nodes,
		entity.Node{
			ID: "n_topic2", Type: valueobject.NodeTypeTopic, Title: "选题 2",
			Params: entity.NodeParams{Idea: "想法 2", CandidateCount: 3},
			Rounds: []entity.Round{{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: artifactID}},
			Locked: true,
		},
		entity.Node{
			ID: "n_x", Type: valueobject.NodeTypeCandidate, Title: "候选 X",
			Parent: "n_topic2", CandidateSource: &entity.CandidateSource{TopicNodeID: "n_topic2", ArtifactID: artifactID, Index: 0},
			Rounds: []entity.Round{},
		},
		entity.Node{
			ID: "n_y", Type: valueobject.NodeTypePrompt, Title: "Y",
			Parent: "n_x", Params: entity.NodeParams{Instruction: "Y"}, Rounds: []entity.Round{},
		},
	)
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
