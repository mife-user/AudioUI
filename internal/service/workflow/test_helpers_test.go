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

type fakeCandidates struct {
	counts map[string]int
	err    error
}

func (c *fakeCandidates) Count(ctx context.Context, projectID string, node entity.Node) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if c.err != nil {
		return 0, c.err
	}
	return c.counts[node.ID], nil
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
	revision int64
	reason   string
}

func intPtr(value int) *int {
	return &value
}

func baseProject() entity.Project {
	zero := 0
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
				Rounds: []entity.Round{{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: "sha256:" + strings.Repeat("0", 64)}},
				Locked: true,
			},
			{
				ID: "n_a", Type: valueobject.NodeTypePrompt, Title: "A",
				Parent: "n_topic", ParentCandidate: &zero,
				Position: entity.Position{X: 10, Y: 10}, Params: entity.NodeParams{Instruction: "A"},
				Rounds: []entity.Round{},
			},
			{
				ID: "n_b", Type: valueobject.NodeTypePrompt, Title: "B",
				Parent: "n_a", Position: entity.Position{X: 20, Y: 20},
				Params: entity.NodeParams{Instruction: "B"}, Rounds: []entity.Round{},
			},
			{
				ID: "n_c", Type: valueobject.NodeTypePrompt, Title: "C",
				Parent: "n_topic", ParentCandidate: intPtr(0),
				Position: entity.Position{X: 30, Y: 10}, Params: entity.NodeParams{Instruction: "C"},
				Rounds: []entity.Round{},
			},
			{
				ID: "n_draft", Type: valueobject.NodeTypePrompt, Title: "草稿",
				Position: entity.Position{X: 40, Y: 40}, Params: entity.NodeParams{Instruction: "D"},
				Rounds: []entity.Round{},
			},
		},
	}
}

func projectWithSecondTree() entity.Project {
	project := baseProject()
	zero := 0
	project.Nodes = append(project.Nodes,
		entity.Node{
			ID: "n_topic2", Type: valueobject.NodeTypeTopic, Title: "选题 2",
			Params: entity.NodeParams{Idea: "想法 2", CandidateCount: 3},
			Rounds: []entity.Round{{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: "sha256:" + strings.Repeat("1", 64)}},
			Locked: true,
		},
		entity.Node{
			ID: "n_x", Type: valueobject.NodeTypePrompt, Title: "X",
			Parent: "n_topic2", ParentCandidate: &zero,
			Params: entity.NodeParams{Instruction: "X"}, Rounds: []entity.Round{},
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
		Project: session,
		Candidates: &fakeCandidates{counts: map[string]int{
			"n_topic": 3, "n_topic2": 3,
		}},
		Workspace: mover,
		OnChange: func(revision int64, reason string) {
			*changes = append(*changes, changeCall{revision: revision, reason: reason})
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
