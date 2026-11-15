package workflow

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/valueobject"
)

func TestAddNodeTopicAndDraft(t *testing.T) {
	h := newHarness(t, baseProject())
	project, err := h.service.AddNode(context.Background(), AddNodeInput{
		Type: valueobject.NodeTypeTopic, Position: entity.Position{X: 1, Y: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if project.Revision != 2 || len(project.Nodes) != 9 {
		t.Fatalf("project = %+v", project)
	}
	added := project.Nodes[len(project.Nodes)-1]
	if added.Type != valueobject.NodeTypeTopic || added.Title != "新选题" || added.Parent != "" || added.ParentCandidate != nil {
		t.Fatalf("added topic = %+v", added)
	}
	if added.Size != entity.DefaultNodeSize(valueobject.NodeTypeTopic) {
		t.Fatalf("added topic size = %+v", added.Size)
	}
	if err := valueobject.ValidateID(added.ID); err != nil {
		t.Fatalf("generated id: %v", err)
	}
	if len(*h.changes) != 1 || (*h.changes)[0].revision != 2 || (*h.changes)[0].reason != ChangeReasonGraph {
		t.Fatalf("changes = %+v", *h.changes)
	}

	project, err = h.service.AddNode(context.Background(), AddNodeInput{
		Type: valueobject.NodeTypePrompt, Position: entity.Position{X: 3, Y: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	draft := project.Nodes[len(project.Nodes)-1]
	if draft.Type != valueobject.NodeTypePrompt || draft.Title != "新提示词" || draft.Parent != "" || draft.ParentCandidate != nil {
		t.Fatalf("draft = %+v", draft)
	}
	if draft.Size != entity.DefaultNodeSize(valueobject.NodeTypePrompt) {
		t.Fatalf("draft size = %+v", draft.Size)
	}
}

func TestAddNodeValidation(t *testing.T) {
	h := newHarness(t, baseProject())
	cases := []struct {
		name  string
		input AddNodeInput
		want  error
	}{
		{"unknown type", AddNodeInput{Type: "render"}, ErrUnknownNodeType},
		{"missing parent", AddNodeInput{Type: valueobject.NodeTypePrompt, Parent: "n_missing"}, ErrNodeNotFound},
		{"parentCandidate unsupported", AddNodeInput{Type: valueobject.NodeTypePrompt, Parent: "n_topic", ParentCandidate: intPtr(3)}, ErrNodeConnectionInvalid},
		{"candidate must be generated", AddNodeInput{Type: valueobject.NodeTypeCandidate}, ErrNodeConnectionInvalid},
		{"invalid position", AddNodeInput{Type: valueobject.NodeTypePrompt, Position: entity.Position{X: math.NaN()}}, ErrInvalidPosition},
		{"invalid output kind", AddNodeInput{Type: valueobject.NodeTypePrompt, Params: entity.NodeParams{OutputKind: "bad"}}, ErrInvalidInput},
		{"negative candidate count", AddNodeInput{Type: valueobject.NodeTypeTopic, Params: entity.NodeParams{CandidateCount: -1}}, ErrInvalidInput},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.service.AddNode(context.Background(), tc.input)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
	if h.session.saveCount != 0 || len(*h.changes) != 0 {
		t.Fatalf("invalid commands persisted changes: saves=%d changes=%d", h.session.saveCount, len(*h.changes))
	}
}

func TestGraphCommandsRejectStaleProjectScope(t *testing.T) {
	h := newHarness(t, baseProject())
	wrongProjectID := "p_old"
	_, err := h.service.AddNode(context.Background(), AddNodeInput{
		ProjectID: wrongProjectID, Type: valueobject.NodeTypeTopic, Position: entity.Position{X: 1, Y: 2},
	})
	if !errors.Is(err, ErrProjectChanged) {
		t.Fatalf("AddNode error = %v, want PROJECT_CHANGED", err)
	}
	if _, err := h.service.Connect(context.Background(), "n_topic", "n_draft", wrongProjectID); !errors.Is(err, ErrProjectChanged) {
		t.Fatalf("Connect error = %v, want PROJECT_CHANGED", err)
	}
	if _, err := h.service.MoveNode(context.Background(), "n_a", 4, 5, wrongProjectID); !errors.Is(err, ErrProjectChanged) {
		t.Fatalf("MoveNode error = %v, want PROJECT_CHANGED", err)
	}
	if _, err := h.service.SetNodeLayout(context.Background(), "n_a", 4, 5, 300, 200, wrongProjectID); !errors.Is(err, ErrProjectChanged) {
		t.Fatalf("SetNodeLayout error = %v, want PROJECT_CHANGED", err)
	}
	if _, err := h.service.SelectNode(context.Background(), "n_a", wrongProjectID); !errors.Is(err, ErrProjectChanged) {
		t.Fatalf("SelectNode error = %v, want PROJECT_CHANGED", err)
	}
	if _, err := h.service.UpdateNodeParams(context.Background(), "n_a", "A2", entity.NodeParams{Instruction: "A2"}, wrongProjectID); !errors.Is(err, ErrProjectChanged) {
		t.Fatalf("UpdateNodeParams error = %v, want PROJECT_CHANGED", err)
	}
	if h.session.saveCount != 0 || len(*h.changes) != 0 {
		t.Fatalf("stale project commands persisted: saves=%d changes=%+v", h.session.saveCount, *h.changes)
	}
}

func TestAddNodeConnectionMatrix(t *testing.T) {
	h := newHarness(t, baseProject())
	project, err := h.service.AddNode(context.Background(), AddNodeInput{
		Type: valueobject.NodeTypeAgent, Parent: "n_a",
	})
	if err != nil {
		t.Fatal(err)
	}
	agent := project.Nodes[len(project.Nodes)-1]
	if agent.Type != valueobject.NodeTypeAgent || parentOf(project, agent.ID) != "n_a" || agent.WorkspaceRootID != "n_topic" {
		t.Fatalf("agent = %+v", agent)
	}

	project, err = h.service.AddNode(context.Background(), AddNodeInput{
		Type: valueobject.NodeTypeScript, Parent: agent.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if added := project.Nodes[len(project.Nodes)-1]; added.Type != valueobject.NodeTypeScript || parentOf(project, added.ID) != agent.ID {
		t.Fatalf("script = %+v", added)
	}

	if _, err := h.service.AddNode(context.Background(), AddNodeInput{
		Type: valueobject.NodeTypePrompt, Parent: "n_a",
	}); err != nil {
		t.Fatalf("arbitrary node connection should be accepted: %v", err)
	}
}

func TestSecondScriptOutputIsConnectedButRunTargetIsAmbiguous(t *testing.T) {
	h := newHarness(t, baseProject())
	project, err := h.service.AddNode(context.Background(), AddNodeInput{
		Type: valueobject.NodeTypeScript, Parent: "n_b",
	})
	if err != nil {
		t.Fatal(err)
	}
	first := project.Nodes[len(project.Nodes)-1].ID
	project, err = h.service.AddNode(context.Background(), AddNodeInput{
		Type: valueobject.NodeTypeScript, Parent: "n_b",
	})
	if err != nil {
		t.Fatalf("second script connection should be accepted: %v", err)
	}
	second := project.Nodes[len(project.Nodes)-1].ID
	if parentOf(project, first) != "n_b" || parentOf(project, second) != "n_b" {
		t.Fatalf("script edges = %+v", project.Edges)
	}
	if _, _, err := h.service.BeginRun(context.Background(), "r_ambiguous", "n_b"); !errors.Is(err, ErrOutputBindingConflict) {
		t.Fatalf("ambiguous output error = %v, want OUTPUT_BINDING_CONFLICT", err)
	}
	if h.service.isNodeLocked("n_b") {
		t.Fatal("ambiguous run left a lock")
	}
}

func TestAddNodeRejectsRunLockedParent(t *testing.T) {
	h := newHarness(t, baseProject())
	if err := h.service.AcquireRunLock(context.Background(), "r_run", "n_topic", "n_a", []string{"n_topic", "n_candidate", "n_a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.AddNode(context.Background(), AddNodeInput{
		Type: valueobject.NodeTypeAgent, Parent: "n_a",
	}); !errors.Is(err, ErrRunningChainLocked) {
		t.Fatalf("error = %v, want RUNNING_CHAIN_LOCKED", err)
	}
	if _, err := h.service.AddNode(context.Background(), AddNodeInput{
		Type: valueobject.NodeTypeAgent, Parent: "n_c",
	}); err != nil {
		t.Fatalf("sibling branch edit failed: %v", err)
	}
	if _, err := h.service.UpdateNodeParams(context.Background(), "n_c", "C2", entity.NodeParams{Instruction: "C2"}); err != nil {
		t.Fatalf("sibling branch params edit failed: %v", err)
	}
	if _, err := h.service.AddNode(context.Background(), AddNodeInput{
		Type: valueobject.NodeTypeTopic,
	}); err != nil {
		t.Fatalf("new topic root failed: %v", err)
	}
}

func TestConnectAddsInputAndKeepsExistingEdges(t *testing.T) {
	h := newHarness(t, baseProject())
	project, err := h.service.Connect(context.Background(), "n_candidate2", "n_b")
	if err != nil {
		t.Fatal(err)
	}
	if project.Revision != 2 || len(h.mover.calls) != 0 {
		t.Fatalf("revision=%d moves=%+v", project.Revision, h.mover.calls)
	}
	if parents := h.parents(t, "n_b"); len(parents) != 2 || parents[0] != "n_a" || parents[1] != "n_candidate2" {
		t.Fatalf("parents = %+v", parents)
	}
	if !h.node(t, "n_b").Stale {
		t.Fatal("target was not marked stale")
	}
	if h.node(t, "n_c").Stale || h.node(t, "n_draft").Stale {
		t.Fatal("node outside the target subtree was marked stale")
	}
	if len(*h.changes) != 1 {
		t.Fatalf("changes = %+v", *h.changes)
	}

	before := h.session.saveCount
	project, err = h.service.Connect(context.Background(), "n_candidate2", "n_b")
	if err != nil {
		t.Fatal(err)
	}
	if project.Revision != 2 || h.session.saveCount != before || len(*h.changes) != 1 {
		t.Fatal("duplicate connection was not idempotent")
	}
}

func TestConnectAllowsDraftNodesInEitherOrder(t *testing.T) {
	h := newHarness(t, baseProject())
	project, err := h.service.Connect(context.Background(), "n_draft", "n_agent_draft")
	if err != nil {
		t.Fatal(err)
	}
	if parentOf(project, "n_agent_draft") != "n_draft" {
		t.Fatalf("edges = %+v", project.Edges)
	}
	if h.node(t, "n_agent_draft").Stale {
		t.Fatal("newly connected draft should not be stale")
	}
	if _, err := h.service.Connect(context.Background(), "n_c", "n_draft"); err != nil {
		t.Fatalf("connecting an existing draft as a target failed: %v", err)
	}
	if parents := h.parents(t, "n_draft"); len(parents) != 1 || parents[0] != "n_c" {
		t.Fatalf("draft parents = %+v", parents)
	}
}

func TestConnectKeepsCandidateSourceProtected(t *testing.T) {
	h := newHarness(t, baseProject())
	project, err := h.service.Connect(context.Background(), "n_candidate2", "n_candidate")
	if err != nil {
		t.Fatal(err)
	}
	source := ""
	for _, edge := range project.Edges {
		if edge.TargetNodeID == "n_candidate" && edge.Kind == entity.EdgeKindCandidateSource {
			source = edge.SourceNodeID
		}
	}
	if source != "n_topic" || h.node(t, "n_candidate").CandidateSource.TopicNodeID != "n_topic" {
		t.Fatalf("candidate source changed: %+v", project.Edges)
	}
}

func TestConnectValidation(t *testing.T) {
	cases := []struct {
		name   string
		source string
		target string
		want   error
	}{
		{"self", "n_a", "n_a", ErrSelfConnect},
		{"cycle", "n_b", "n_a", ErrCycleDetected},
		{"missing source", "n_missing", "n_c", ErrNodeNotFound},
		{"missing target", "n_a", "n_missing", ErrNodeNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, baseProject())
			_, err := h.service.Connect(context.Background(), tc.source, tc.target)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if h.session.saveCount != 0 {
				t.Fatal("invalid command saved")
			}
		})
	}
}

func TestDisconnectAndReplaceProtectSourceEdges(t *testing.T) {
	h := newHarness(t, baseProject())
	sourceEdge := ""
	for _, edge := range h.session.project.Edges {
		if edge.TargetNodeID == "n_candidate" {
			sourceEdge = edge.ID
		}
	}
	if _, _, err := h.service.Disconnect(context.Background(), sourceEdge); !errors.Is(err, ErrProtectedEdge) {
		t.Fatalf("disconnect source error = %v, want PROTECTED_EDGE", err)
	}
	if _, err := h.service.ReplaceConnection(context.Background(), sourceEdge, "n_c"); !errors.Is(err, ErrProtectedEdge) {
		t.Fatalf("replace source error = %v, want PROTECTED_EDGE", err)
	}
	if h.session.saveCount != 0 {
		t.Fatal("protected edge command saved")
	}

	ordinaryEdge := ""
	for _, edge := range h.session.project.Edges {
		if edge.TargetNodeID == "n_b" {
			ordinaryEdge = edge.ID
		}
	}
	project, targetID, err := h.service.Disconnect(context.Background(), ordinaryEdge)
	if err != nil {
		t.Fatal(err)
	}
	if targetID != "n_b" || parentOf(project, "n_b") != "" || !h.node(t, "n_b").Stale {
		t.Fatalf("disconnect result = target %s edges %+v", targetID, project.Edges)
	}
	if h.node(t, "n_b").WorkspaceRootID != "n_topic" {
		t.Fatal("disconnect changed the recorded workspace root")
	}
	if _, err := h.service.Connect(context.Background(), "n_candidate2", "n_b"); err != nil {
		t.Fatalf("reconnect after disconnect failed: %v", err)
	}
}

func TestReplaceConnectionMovesStepsAndWorkspaceRoot(t *testing.T) {
	h := newHarness(t, projectWithSecondTree())
	edgeID := ""
	for _, edge := range h.session.project.Edges {
		if edge.TargetNodeID == "n_a" {
			edgeID = edge.ID
		}
	}
	project, err := h.service.ReplaceConnection(context.Background(), edgeID, "n_x")
	if err != nil {
		t.Fatal(err)
	}
	if len(h.mover.calls) != 1 {
		t.Fatalf("mover calls = %+v", h.mover.calls)
	}
	call := h.mover.calls[0]
	if call.sourceRootID != "n_topic" || call.targetRootID != "n_topic2" {
		t.Fatalf("move = %+v", call)
	}
	if len(call.nodeIDs) != 2 || call.nodeIDs[0] != "n_a" || call.nodeIDs[1] != "n_b" {
		t.Fatalf("move nodes = %+v", call.nodeIDs)
	}
	if parentOf(project, "n_a") != "n_x" || !h.node(t, "n_a").Stale || !h.node(t, "n_b").Stale {
		t.Fatalf("replace wrong: %+v", project.Edges)
	}
	if h.node(t, "n_a").WorkspaceRootID != "n_topic2" || h.node(t, "n_b").WorkspaceRootID != "n_topic2" {
		t.Fatalf("workspace roots = %s %s", h.node(t, "n_a").WorkspaceRootID, h.node(t, "n_b").WorkspaceRootID)
	}
}

func TestReplaceConnectionFailures(t *testing.T) {
	edgeA := func(h *harness) string {
		for _, edge := range h.session.project.Edges {
			if edge.TargetNodeID == "n_a" {
				return edge.ID
			}
		}
		return ""
	}
	t.Run("move failure", func(t *testing.T) {
		h := newHarness(t, projectWithSecondTree())
		h.mover.err = errors.New("disk full")
		_, err := h.service.ReplaceConnection(context.Background(), edgeA(h), "n_x")
		if !errors.Is(err, ErrWorkspaceMoveFailed) {
			t.Fatalf("error = %v, want WORKSPACE_MOVE_FAILED", err)
		}
		if h.session.saveCount != 0 || parentOf(h.session.project, "n_a") != "n_candidate" {
			t.Fatal("graph changed after move failure")
		}
	})
	t.Run("save failure rolls back movement", func(t *testing.T) {
		h := newHarness(t, projectWithSecondTree())
		h.session.saveErr = errors.New("save failed")
		_, err := h.service.ReplaceConnection(context.Background(), edgeA(h), "n_x")
		if err == nil {
			t.Fatal("error = nil")
		}
		if h.mover.rollbacks != 1 {
			t.Fatalf("rollbacks = %d", h.mover.rollbacks)
		}
		if parentOf(h.session.project, "n_a") != "n_candidate" {
			t.Fatal("graph changed after save failure")
		}
	})
	t.Run("shared descendant is refused", func(t *testing.T) {
		project := projectWithSecondTree()
		project.Edges = append(project.Edges, entity.Edge{
			ID: "e_shared_input", SourceNodeID: "n_candidate2", TargetNodeID: "n_b", Kind: entity.EdgeKindConnection,
		})
		h := newHarness(t, project)
		_, err := h.service.ReplaceConnection(context.Background(), edgeA(h), "n_x")
		if !errors.Is(err, ErrWorkspaceMigrationUnsafe) {
			t.Fatalf("error = %v, want WORKSPACE_MIGRATION_UNSAFE", err)
		}
		if len(h.mover.calls) != 0 {
			t.Fatal("steps moved while the subtree had a shared descendant")
		}
	})
	t.Run("locked target source", func(t *testing.T) {
		h := newHarness(t, projectWithSecondTree())
		if err := h.service.AcquireRunLock(context.Background(), "r_two", "n_topic2", "n_x", []string{"n_topic2", "n_x"}); err != nil {
			t.Fatal(err)
		}
		_, err := h.service.ReplaceConnection(context.Background(), edgeA(h), "n_x")
		if !errors.Is(err, ErrRunningChainLocked) {
			t.Fatalf("error = %v, want RUNNING_CHAIN_LOCKED", err)
		}
		if len(h.mover.calls) != 0 {
			t.Fatal("steps moved while target was locked")
		}
	})
	t.Run("locked subtree", func(t *testing.T) {
		h := newHarness(t, projectWithSecondTree())
		if err := h.service.AcquireRunLock(context.Background(), "r_a", "n_topic", "n_a", []string{"n_topic", "n_candidate", "n_a"}); err != nil {
			t.Fatal(err)
		}
		_, err := h.service.ReplaceConnection(context.Background(), edgeA(h), "n_x")
		if !errors.Is(err, ErrRunningChainLocked) {
			t.Fatalf("error = %v, want RUNNING_CHAIN_LOCKED", err)
		}
	})
}

func TestReconnectTargetPreservesEdgeAndStalesAffectedSubgraphs(t *testing.T) {
	project := asProjectV5(baseProject())
	project.Nodes = append(project.Nodes,
		entity.Node{ID: "n_b_child", Type: valueobject.NodeTypePrompt, WorkspaceRootID: "n_topic", Size: entity.DefaultNodeSize(valueobject.NodeTypePrompt), Params: entity.NodeParams{Instruction: "旧分支"}, Rounds: []entity.Round{}},
		entity.Node{ID: "n_c_child", Type: valueobject.NodeTypePrompt, WorkspaceRootID: "n_topic", Size: entity.DefaultNodeSize(valueobject.NodeTypePrompt), Params: entity.NodeParams{Instruction: "新分支"}, Rounds: []entity.Round{}},
		entity.Node{ID: "n_shared", Type: valueobject.NodeTypeAgent, WorkspaceRootID: "n_topic", Size: entity.DefaultNodeSize(valueobject.NodeTypeAgent), Rounds: []entity.Round{}},
	)
	attachEdge(&project, "n_b", "n_b_child")
	attachEdge(&project, "n_c", "n_c_child")
	attachEdge(&project, "n_b_child", "n_shared")
	project.Edges = append(project.Edges, entity.Edge{ID: "e_shared_from_c", SourceNodeID: "n_c_child", TargetNodeID: "n_shared", Kind: entity.EdgeKindConnection})
	h := newHarness(t, project)
	before := h.session.project.Clone()
	expected := edgeByID(t, before, "e_n_b")

	updated, err := h.service.ReconnectTarget(context.Background(), expected, "n_c", before.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != before.Revision+1 || h.session.saveCount != 1 {
		t.Fatalf("revision=%d saves=%d", updated.Revision, h.session.saveCount)
	}
	wantEdges := append([]entity.Edge(nil), before.Edges...)
	for i := range wantEdges {
		if wantEdges[i].ID == expected.ID {
			wantEdges[i].TargetNodeID = "n_c"
		}
	}
	if !reflect.DeepEqual(updated.Edges, wantEdges) {
		t.Fatalf("edges = %+v, want %+v", updated.Edges, wantEdges)
	}
	for id, stale := range map[string]bool{
		"n_b": true, "n_b_child": true, "n_c": true, "n_c_child": true, "n_shared": true,
		"n_a": false, "n_draft": false,
	} {
		if h.node(t, id).Stale != stale {
			t.Fatalf("node %s stale=%t, want %t", id, h.node(t, id).Stale, stale)
		}
	}
	for _, node := range before.Nodes {
		if h.node(t, node.ID).WorkspaceRootID != node.WorkspaceRootID {
			t.Fatalf("node %s workspace root changed from %q to %q", node.ID, node.WorkspaceRootID, h.node(t, node.ID).WorkspaceRootID)
		}
	}
	if len(h.mover.calls) != 0 || len(*h.changes) != 1 || (*h.changes)[0].revision != updated.Revision || (*h.changes)[0].reason != ChangeReasonGraph {
		t.Fatalf("mover calls=%+v changes=%+v", h.mover.calls, *h.changes)
	}
}

func TestReconnectTargetNoopAndStaleIdentity(t *testing.T) {
	project := asProjectV5(baseProject())
	h := newHarness(t, project)
	expected := edgeByID(t, project, "e_n_b")
	unchanged, err := h.service.ReconnectTarget(context.Background(), expected, expected.TargetNodeID, project.ID)
	if err != nil || unchanged.Revision != project.Revision || h.session.saveCount != 0 || len(*h.changes) != 0 {
		t.Fatalf("same-target reconnect err=%v revision=%d saves=%d changes=%+v", err, unchanged.Revision, h.session.saveCount, *h.changes)
	}
	candidateSource := edgeByID(t, project, "e_n_candidate")
	unchanged, err = h.service.ReconnectTarget(context.Background(), candidateSource, candidateSource.TargetNodeID, project.ID)
	if err != nil || unchanged.Revision != project.Revision || h.session.saveCount != 0 || len(*h.changes) != 0 {
		t.Fatalf("same-target protected edge err=%v revision=%d saves=%d changes=%+v", err, unchanged.Revision, h.session.saveCount, *h.changes)
	}

	stale := expected
	stale.TargetNodeID = "n_c"
	before := h.session.project.Clone()
	if _, err := h.service.ReconnectTarget(context.Background(), stale, "n_c", project.ID); !errors.Is(err, ErrEdgeConflict) {
		t.Fatalf("stale edge error = %v, want EDGE_CONFLICT", err)
	}
	if h.session.saveCount != 0 || !reflect.DeepEqual(h.session.project.Clone(), before) || len(*h.changes) != 0 {
		t.Fatal("stale edge identity changed the project")
	}
}

func TestReconnectTargetRejectsChangedEdgeIdentityFields(t *testing.T) {
	project := asProjectV5(baseProject())
	expected := edgeByID(t, project, "e_n_b")
	cases := map[string]func(*entity.Edge){
		"source":         func(edge *entity.Edge) { edge.SourceNodeID = "n_c" },
		"target":         func(edge *entity.Edge) { edge.TargetNodeID = "n_c" },
		"kind":           func(edge *entity.Edge) { edge.Kind = entity.EdgeKindCandidateSource },
		"script version": func(edge *entity.Edge) { edge.ScriptVersion = 1 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, project.Clone())
			stale := expected
			mutate(&stale)
			before := h.session.project.Clone()
			if _, err := h.service.ReconnectTarget(context.Background(), stale, "n_c", project.ID); !errors.Is(err, ErrEdgeConflict) {
				t.Fatalf("changed edge identity error = %v, want EDGE_CONFLICT", err)
			}
			if h.session.saveCount != 0 || !reflect.DeepEqual(h.session.project.Clone(), before) || len(*h.changes) != 0 {
				t.Fatal("changed edge identity modified the project")
			}
		})
	}
}

func TestReconnectTargetPreservesScriptVersion(t *testing.T) {
	for _, version := range []int{0, 1} {
		t.Run(fmt.Sprintf("version_%d", version), func(t *testing.T) {
			project := asProjectV5(baseProject())
			project.Nodes = append(project.Nodes, entity.Node{
				ID: "n_script", Type: valueobject.NodeTypeScript, Title: "脚本", WorkspaceRootID: "n_topic",
				ScriptVersions: []entity.ScriptVersion{
					{N: 1, ArtifactID: "sha256:script-v1", RunID: "r_script_v1"},
					{N: 2, ArtifactID: "sha256:script-v2", RunID: "r_script_v2"},
				},
			})
			for i := range project.Edges {
				if project.Edges[i].ID == "e_n_c" {
					project.Edges[i].SourceNodeID = "n_script"
					project.Edges[i].ScriptVersion = version
				}
			}
			h := newHarness(t, project)
			expected := edgeByID(t, project, "e_n_c")
			updated, err := h.service.ReconnectTarget(context.Background(), expected, "n_b", project.ID)
			if err != nil {
				t.Fatal(err)
			}
			actual := edgeByID(t, updated, expected.ID)
			if actual.SourceNodeID != expected.SourceNodeID || actual.TargetNodeID != "n_b" || actual.ScriptVersion != version {
				t.Fatalf("reconnected edge = %+v, want source %s, target n_b, version %d", actual, expected.SourceNodeID, version)
			}

			stale := expected
			stale.ScriptVersion = 1 - version
			before := h.session.project.Clone()
			if _, err := h.service.ReconnectTarget(context.Background(), stale, "n_b", project.ID); !errors.Is(err, ErrEdgeConflict) {
				t.Fatalf("changed Script version error = %v, want EDGE_CONFLICT", err)
			}
			if h.session.saveCount != 1 || !reflect.DeepEqual(h.session.project.Clone(), before) {
				t.Fatal("stale Script version identity changed the project")
			}
		})
	}
}

func TestReconnectTargetRejectsInvalidRequests(t *testing.T) {
	project := asProjectV5(baseProject())
	expected := edgeByID(t, project, "e_n_b")
	assertRejected := func(t *testing.T, current entity.Project, edge entity.Edge, targetID, projectID string, want error) {
		t.Helper()
		h := newHarness(t, current)
		before := h.session.project.Clone()
		if _, err := h.service.ReconnectTarget(context.Background(), edge, targetID, projectID); !errors.Is(err, want) {
			t.Fatalf("reconnect error = %v, want %v", err, want)
		}
		if h.session.saveCount != 0 || !reflect.DeepEqual(h.session.project.Clone(), before) || len(*h.changes) != 0 {
			t.Fatal("rejected reconnect changed the project")
		}
	}
	t.Run("self connection", func(t *testing.T) {
		assertRejected(t, project, expected, expected.SourceNodeID, project.ID, ErrSelfConnect)
	})
	t.Run("cycle", func(t *testing.T) { assertRejected(t, project, expected, "n_candidate", project.ID, ErrCycleDetected) })
	t.Run("missing target", func(t *testing.T) { assertRejected(t, project, expected, "n_missing", project.ID, ErrNodeNotFound) })
	t.Run("missing edge", func(t *testing.T) {
		missing := expected
		missing.ID = "e_missing"
		assertRejected(t, project, missing, "n_c", project.ID, ErrEdgeNotFound)
	})
	t.Run("candidate source", func(t *testing.T) {
		candidateSource := edgeByID(t, project, "e_n_candidate")
		assertRejected(t, project, candidateSource, "n_c", project.ID, ErrProtectedEdge)
	})
	t.Run("project changed", func(t *testing.T) { assertRejected(t, project, expected, "n_c", "p_other", ErrProjectChanged) })
	t.Run("duplicate source and target", func(t *testing.T) {
		withDuplicate := project.Clone()
		withDuplicate.Edges = append(withDuplicate.Edges, entity.Edge{
			ID: "e_existing_ac", SourceNodeID: expected.SourceNodeID, TargetNodeID: "n_c", Kind: entity.EdgeKindConnection,
		})
		assertRejected(t, withDuplicate, expected, "n_c", project.ID, ErrEdgeConflict)
	})
}

func TestReconnectTargetChecksLocksOnSourceAndBothSubgraphs(t *testing.T) {
	project := asProjectV5(baseProject())
	project.Nodes = append(project.Nodes,
		entity.Node{ID: "n_b_child", Type: valueobject.NodeTypePrompt, WorkspaceRootID: "n_topic", Size: entity.DefaultNodeSize(valueobject.NodeTypePrompt), Params: entity.NodeParams{Instruction: "旧分支"}, Rounds: []entity.Round{}},
		entity.Node{ID: "n_c_child", Type: valueobject.NodeTypePrompt, WorkspaceRootID: "n_topic", Size: entity.DefaultNodeSize(valueobject.NodeTypePrompt), Params: entity.NodeParams{Instruction: "新分支"}, Rounds: []entity.Round{}},
		entity.Node{ID: "n_shared", Type: valueobject.NodeTypeAgent, WorkspaceRootID: "n_topic", Size: entity.DefaultNodeSize(valueobject.NodeTypeAgent), Rounds: []entity.Round{}},
	)
	attachEdge(&project, "n_b", "n_b_child")
	attachEdge(&project, "n_c", "n_c_child")
	attachEdge(&project, "n_b_child", "n_shared")
	project.Edges = append(project.Edges, entity.Edge{ID: "e_shared_from_c", SourceNodeID: "n_c_child", TargetNodeID: "n_shared", Kind: entity.EdgeKindConnection})
	for _, nodeID := range []string{"n_a", "n_b", "n_b_child", "n_c", "n_c_child", "n_shared"} {
		t.Run(nodeID, func(t *testing.T) {
			h := newHarness(t, project.Clone())
			h.service.locks = map[string]runLock{"r_locked": {Members: map[string]struct{}{nodeID: {}}}}
			expected := edgeByID(t, h.session.project, "e_n_b")
			before := h.session.project.Clone()
			if _, err := h.service.ReconnectTarget(context.Background(), expected, "n_c", project.ID); !errors.Is(err, ErrRunningChainLocked) {
				t.Fatalf("reconnect error = %v, want RUNNING_CHAIN_LOCKED", err)
			}
			if h.session.saveCount != 0 || !reflect.DeepEqual(h.session.project.Clone(), before) {
				t.Fatal("locked reconnect changed the project")
			}
		})
	}
}

func TestReconnectTargetKeepsWorkspaceIdentityAndBlocksMismatchedOutput(t *testing.T) {
	project := asProjectV5(projectWithSecondTree())
	project.Nodes = append(project.Nodes,
		entity.Node{ID: "n_old_script", Type: valueobject.NodeTypeScript, Title: "旧脚本", WorkspaceRootID: "n_topic", Rounds: []entity.Round{}},
		entity.Node{ID: "n_new_script", Type: valueobject.NodeTypeScript, Title: "新脚本", WorkspaceRootID: "n_topic2", Rounds: []entity.Round{}},
	)
	attachEdge(&project, "n_b", "n_old_script")
	graph, err := entity.NewGraph(&project)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := graph.SnapshotForRun("n_b"); err != nil {
		t.Fatalf("baseline output should be runnable: %v", err)
	}

	h := newHarness(t, project)
	expected := edgeByID(t, project, "e_n_old_script")
	updated, err := h.service.ReconnectTarget(context.Background(), expected, "n_new_script", project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if h.node(t, "n_old_script").WorkspaceRootID != "n_topic" || h.node(t, "n_new_script").WorkspaceRootID != "n_topic2" || len(h.mover.calls) != 0 {
		t.Fatalf("workspace roots changed or mover used: old=%s new=%s moves=%+v", h.node(t, "n_old_script").WorkspaceRootID, h.node(t, "n_new_script").WorkspaceRootID, h.mover.calls)
	}
	graph, err = entity.NewGraph(&updated)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := graph.SnapshotForRun("n_b"); !errors.Is(err, entity.ErrRunGraphUnsupported) {
		t.Fatalf("mismatched workspace output error = %v, want RUN_GRAPH_UNSUPPORTED", err)
	}
}

func TestReconnectTargetSaveFailureLeavesGraphUnchanged(t *testing.T) {
	project := asProjectV5(baseProject())
	h := newHarness(t, project)
	h.session.saveErr = errors.New("disk full")
	before := h.session.project.Clone()
	expected := edgeByID(t, project, "e_n_b")
	if _, err := h.service.ReconnectTarget(context.Background(), expected, "n_c", project.ID); err == nil {
		t.Fatal("save error was ignored")
	}
	if h.session.saveCount != 0 || !reflect.DeepEqual(h.session.project.Clone(), before) || len(*h.changes) != 0 || len(h.mover.calls) != 0 {
		t.Fatalf("failed save changed state: saves=%d changes=%+v moves=%+v", h.session.saveCount, *h.changes, h.mover.calls)
	}
}

func TestMoveNode(t *testing.T) {
	h := newHarness(t, baseProject())
	project, err := h.service.MoveNode(context.Background(), "n_a", 11, 12)
	if err != nil {
		t.Fatal(err)
	}
	if project.Revision != 2 || h.node(t, "n_a").Position != (entity.Position{X: 11, Y: 12}) {
		t.Fatalf("position = %+v revision=%d", h.node(t, "n_a").Position, project.Revision)
	}
	before := h.session.saveCount
	project, err = h.service.MoveNode(context.Background(), "n_a", 11, 12)
	if err != nil || project.Revision != 2 || h.session.saveCount != before {
		t.Fatalf("same position was not idempotent: %v rev=%d saves=%d", err, project.Revision, h.session.saveCount)
	}
	if _, err := h.service.MoveNode(context.Background(), "n_a", math.Inf(1), 0); !errors.Is(err, ErrInvalidPosition) {
		t.Fatalf("error = %v, want INVALID_POSITION", err)
	}
	if _, err := h.service.MoveNode(context.Background(), "n_missing", 1, 1); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("error = %v, want NODE_NOT_FOUND", err)
	}
	if err := h.service.AcquireRunLock(context.Background(), "r_run", "n_topic", "n_a", []string{"n_topic", "n_candidate", "n_a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.MoveNode(context.Background(), "n_a", 13, 14); err != nil {
		t.Fatalf("moving a locked node position should be allowed: %v", err)
	}
}

func TestSetNodeLayout(t *testing.T) {
	h := newHarness(t, baseProject())
	before := h.node(t, "n_a").Clone()
	project, err := h.service.SetNodeLayout(context.Background(), "n_a", 41, 42, 340, 260, "p_test")
	if err != nil {
		t.Fatal(err)
	}
	updated := h.node(t, "n_a")
	if project.Revision != 2 || updated.Position != (entity.Position{X: 41, Y: 42}) || updated.Size != (entity.NodeSize{Width: 340, Height: 260}) {
		t.Fatalf("layout = %+v revision=%d", updated, project.Revision)
	}
	if updated.Type != before.Type || updated.Parent != before.Parent || updated.Title != before.Title || updated.Params != before.Params ||
		!reflect.DeepEqual(updated.Rounds, before.Rounds) || updated.Locked != before.Locked || updated.Stale != before.Stale {
		t.Fatalf("layout change mutated node content: before=%+v after=%+v", before, updated)
	}
	if h.session.saveCount != 1 || len(*h.changes) != 1 {
		t.Fatalf("layout was not committed once: saves=%d changes=%d", h.session.saveCount, len(*h.changes))
	}

	beforeSaves := h.session.saveCount
	project, err = h.service.SetNodeLayout(context.Background(), "n_a", 41, 42, 340, 260, "p_test")
	if err != nil || project.Revision != 2 || h.session.saveCount != beforeSaves {
		t.Fatalf("unchanged layout was not idempotent: err=%v revision=%d saves=%d", err, project.Revision, h.session.saveCount)
	}

	invalidSizes := []entity.NodeSize{
		{Width: math.NaN(), Height: 200},
		{Width: 200, Height: math.Inf(1)},
		{Width: entity.MinNodeWidth - 1, Height: entity.MinNodeHeight},
		{Width: entity.MaxNodeWidth + 1, Height: entity.MaxNodeHeight},
		{Width: entity.MinNodeWidth, Height: entity.MinNodeHeight - 1},
		{Width: entity.MaxNodeWidth, Height: entity.MaxNodeHeight + 1},
	}
	for _, size := range invalidSizes {
		if _, err := h.service.SetNodeLayout(context.Background(), "n_a", 1, 2, size.Width, size.Height, "p_test"); !errors.Is(err, ErrInvalidNodeSize) {
			t.Fatalf("size %+v error = %v, want INVALID_NODE_SIZE", size, err)
		}
	}
	if _, err := h.service.SetNodeLayout(context.Background(), "n_a", math.NaN(), 2, 300, 200, "p_test"); !errors.Is(err, ErrInvalidPosition) {
		t.Fatalf("invalid position error = %v, want INVALID_POSITION", err)
	}
	if _, err := h.service.SetNodeLayout(context.Background(), "n_missing", 1, 2, 300, 200, "p_test"); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("missing node error = %v, want NODE_NOT_FOUND", err)
	}
	if h.session.saveCount != beforeSaves {
		t.Fatalf("invalid layouts were persisted: saves=%d", h.session.saveCount)
	}

	if err := h.service.AcquireRunLock(context.Background(), "r_run", "n_topic", "n_a", []string{"n_topic", "n_candidate", "n_a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.SetNodeLayout(context.Background(), "n_a", 45, 46, 350, 270, "p_test"); err != nil {
		t.Fatalf("layout change under run lock should be allowed: %v", err)
	}

	failed := newHarness(t, baseProject())
	beforeFailure := failed.node(t, "n_a").Clone()
	failed.session.saveErr = errors.New("disk write failed")
	if _, err := failed.service.SetNodeLayout(context.Background(), "n_a", 5, 6, 360, 280, "p_test"); !errors.Is(err, failed.session.saveErr) {
		t.Fatalf("write error = %v, want %v", err, failed.session.saveErr)
	}
	if failed.node(t, "n_a").Position != beforeFailure.Position || failed.node(t, "n_a").Size != beforeFailure.Size ||
		failed.session.project.Revision != 1 || len(*failed.changes) != 0 {
		t.Fatalf("failed layout left a partial mutation: node=%+v changes=%+v", failed.node(t, "n_a"), *failed.changes)
	}
}

func TestSetNodeCollapsed(t *testing.T) {
	h := newHarness(t, baseProject())
	before := h.node(t, "n_a").Clone()
	project, err := h.service.SetNodeCollapsed(context.Background(), "n_a", true, "p_test")
	if err != nil {
		t.Fatal(err)
	}
	updated := h.node(t, "n_a")
	if project.Revision != 2 || !updated.Collapsed || updated.Size != before.Size || updated.Position != before.Position {
		t.Fatalf("collapsed node = %+v revision=%d", updated, project.Revision)
	}
	if updated.Parent != before.Parent || updated.Title != before.Title || updated.Params != before.Params ||
		!reflect.DeepEqual(updated.Rounds, before.Rounds) || updated.Locked != before.Locked || updated.Stale != before.Stale {
		t.Fatalf("collapse mutated node content: before=%+v after=%+v", before, updated)
	}
	if h.session.saveCount != 1 || len(*h.changes) != 1 {
		t.Fatalf("collapse was not committed once: saves=%d changes=%d", h.session.saveCount, len(*h.changes))
	}

	beforeSaves := h.session.saveCount
	project, err = h.service.SetNodeCollapsed(context.Background(), "n_a", true, "p_test")
	if err != nil || project.Revision != 2 || h.session.saveCount != beforeSaves {
		t.Fatalf("unchanged collapse was not idempotent: err=%v revision=%d saves=%d", err, project.Revision, h.session.saveCount)
	}
	if _, err := h.service.SetNodeCollapsed(context.Background(), "n_missing", true, "p_test"); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("missing node error = %v, want NODE_NOT_FOUND", err)
	}
	if _, err := h.service.SetNodeCollapsed(context.Background(), "n_a", false, "p_other"); !errors.Is(err, ErrProjectChanged) {
		t.Fatalf("project scope error = %v, want PROJECT_CHANGED", err)
	}

	if err := h.service.AcquireRunLock(context.Background(), "r_run", "n_topic", "n_a", []string{"n_topic", "n_candidate", "n_a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.SetNodeCollapsed(context.Background(), "n_a", false, "p_test"); err != nil {
		t.Fatalf("collapse state change under run lock should be allowed: %v", err)
	}

	failed := newHarness(t, baseProject())
	failed.session.saveErr = errors.New("disk write failed")
	if _, err := failed.service.SetNodeCollapsed(context.Background(), "n_a", true, "p_test"); !errors.Is(err, failed.session.saveErr) {
		t.Fatalf("write error = %v, want %v", err, failed.session.saveErr)
	}
	if failed.node(t, "n_a").Collapsed || failed.session.project.Revision != 1 || len(*failed.changes) != 0 {
		t.Fatalf("failed collapse left a partial mutation: node=%+v changes=%+v", failed.node(t, "n_a"), *failed.changes)
	}
}

func TestUpdateNodeParams(t *testing.T) {
	h := newHarness(t, baseProject())
	params := entity.NodeParams{Instruction: "新指令", OutputKind: valueobject.OutputKindScript}
	project, err := h.service.UpdateNodeParams(context.Background(), "n_a", "A2", params)
	if err != nil {
		t.Fatal(err)
	}
	if project.Revision != 2 {
		t.Fatalf("revision = %d", project.Revision)
	}
	node := h.node(t, "n_a")
	if node.Title != "A2" || node.Params.Instruction != "新指令" {
		t.Fatalf("node = %+v", node)
	}
	before := h.session.saveCount
	project, err = h.service.UpdateNodeParams(context.Background(), "n_a", "", params)
	if err != nil || project.Revision != 2 || h.session.saveCount != before {
		t.Fatalf("same params were not idempotent: %v rev=%d saves=%d", err, project.Revision, h.session.saveCount)
	}

	if _, err := h.service.UpdateNodeParams(context.Background(), "n_topic", "", entity.NodeParams{}); !errors.Is(err, ErrNodeLocked) {
		t.Fatalf("error = %v, want NODE_LOCKED", err)
	}
	if err := h.service.AcquireRunLock(context.Background(), "r_run", "n_topic", "n_a", []string{"n_topic", "n_candidate", "n_a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.UpdateNodeParams(context.Background(), "n_a", "", params); !errors.Is(err, ErrRunningChainLocked) {
		t.Fatalf("error = %v, want RUNNING_CHAIN_LOCKED", err)
	}
	if _, err := h.service.UpdateNodeParams(context.Background(), "n_missing", "", params); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("error = %v, want NODE_NOT_FOUND", err)
	}
	if _, err := h.service.UpdateNodeParams(context.Background(), "n_b", strings.Repeat("长", valueobject.MaxNodeTitleLength+1), params); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("error = %v, want INVALID_INPUT", err)
	}
	if _, err := h.service.UpdateNodeParams(context.Background(), "n_b", "", entity.NodeParams{OutputKind: "bad"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("error = %v, want INVALID_INPUT", err)
	}
}

func TestSelectNodeDoesNotIncrementRevisionOrBroadcast(t *testing.T) {
	h := newHarness(t, baseProject())
	project, err := h.service.SelectNode(context.Background(), "n_c")
	if err != nil {
		t.Fatal(err)
	}
	if project.ActiveNodeID != "n_c" || project.Revision != 1 {
		t.Fatalf("project = %+v", project)
	}
	if len(*h.changes) != 0 {
		t.Fatalf("SelectNode broadcast: %+v", *h.changes)
	}
	before := h.session.saveCount
	if _, err := h.service.SelectNode(context.Background(), "n_c"); err != nil || h.session.saveCount != before {
		t.Fatalf("same selection was not idempotent: %v saves=%d", err, h.session.saveCount)
	}
	if _, err := h.service.SelectNode(context.Background(), "n_missing"); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("error = %v, want NODE_NOT_FOUND", err)
	}
	if err := h.service.AcquireRunLock(context.Background(), "r_run", "n_topic", "n_a", []string{"n_topic", "n_candidate", "n_a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.SelectNode(context.Background(), "n_b"); err != nil {
		t.Fatalf("SelectNode under run lock should be allowed: %v", err)
	}
}

func TestEnsureExecutable(t *testing.T) {
	h := newHarness(t, baseProject())
	if err := h.service.EnsureExecutable(context.Background(), "n_topic"); !errors.Is(err, ErrNodeLocked) {
		t.Fatalf("locked topic error = %v, want NODE_LOCKED", err)
	}
	if err := h.service.EnsureExecutable(context.Background(), "n_a"); !errors.Is(err, ErrRunEntryUnavailable) {
		t.Fatalf("prompt error = %v, want RUN_ENTRY_UNAVAILABLE", err)
	}
	if err := h.service.EnsureExecutable(context.Background(), "n_draft"); !errors.Is(err, ErrRunEntryUnavailable) {
		t.Fatalf("draft error = %v, want RUN_ENTRY_UNAVAILABLE", err)
	}
	if err := h.service.EnsureExecutable(context.Background(), "n_missing"); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("missing error = %v, want NODE_NOT_FOUND", err)
	}
	if err := h.service.AcquireRunLock(context.Background(), "r_run", "n_topic", "n_a", []string{"n_topic", "n_candidate", "n_a"}); err != nil {
		t.Fatal(err)
	}
	if err := h.service.EnsureExecutable(context.Background(), "n_a"); !errors.Is(err, ErrRunningChainLocked) {
		t.Fatalf("run-locked error = %v, want RUNNING_CHAIN_LOCKED", err)
	}
}

func TestBeginRunRejectsPromptAndAllowsAgentEntry(t *testing.T) {
	h := newHarness(t, baseProject())
	if _, _, err := h.service.BeginRun(context.Background(), "r_prompt", "n_a"); !errors.Is(err, ErrRunEntryUnavailable) {
		t.Fatalf("BeginRun(prompt) error = %v, want RUN_ENTRY_UNAVAILABLE", err)
	}
	_, snapshot, err := h.service.BeginRun(context.Background(), "r_agent", "n_b")
	if err != nil {
		t.Fatalf("BeginRun(agent) error = %v", err)
	}
	if snapshot.NodeID != "n_b" || !slices.Equal(snapshot.ChainPath, []string{"n_topic", "n_candidate", "n_a", "n_b"}) {
		t.Fatalf("agent snapshot = %+v", snapshot)
	}
	if len(h.service.locks) != 1 {
		t.Fatalf("agent run acquired %d locks, want 1", len(h.service.locks))
	}
	if err := h.service.ReleaseRunLock(context.Background(), "r_agent"); err != nil {
		t.Fatal(err)
	}
	if locked := h.service.LockedNodeIDs(); len(locked) != 0 {
		t.Fatalf("agent run left path locks: %+v", locked)
	}
}

func TestBeginRunLocksBoundScriptOutputAndKeepsSiblingEditable(t *testing.T) {
	project := baseProject()
	project.Nodes = append(project.Nodes,
		entity.Node{ID: "n_output", Type: valueobject.NodeTypeScript, Title: "输出", WorkspaceRootID: "n_topic", Rounds: []entity.Round{}},
		entity.Node{ID: "n_prompt_alt", Type: valueobject.NodeTypePrompt, Title: "另一任务", WorkspaceRootID: "n_topic", Params: entity.NodeParams{Instruction: "另一任务"}},
		entity.Node{ID: "n_agent_alt", Type: valueobject.NodeTypeAgent, Title: "另一 Agent", WorkspaceRootID: "n_topic"},
	)
	attachEdge(&project, "n_b", "n_output")
	attachEdge(&project, "n_candidate", "n_prompt_alt")
	attachEdge(&project, "n_prompt_alt", "n_agent_alt")
	h := newHarness(t, project)
	_, snapshot, err := h.service.BeginRun(context.Background(), "r_agent", "n_b")
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.OutputBinding.Bound || snapshot.OutputBinding.NodeID != "n_output" || snapshot.OutputBinding.ParentNodeID != "n_b" {
		t.Fatalf("output binding = %+v", snapshot.OutputBinding)
	}
	for _, id := range []string{"n_topic", "n_candidate", "n_a", "n_b", "n_output"} {
		if !h.service.isNodeLocked(id) {
			t.Errorf("run did not lock %s", id)
		}
	}
	if _, err := h.service.Connect(context.Background(), "n_agent_alt", "n_output"); !errors.Is(err, ErrRunningChainLocked) {
		t.Fatalf("moving a bound output during run error = %v, want RUNNING_CHAIN_LOCKED", err)
	}
	if _, err := h.service.UpdateNodeParams(context.Background(), "n_c", "C", entity.NodeParams{Instruction: "兄弟可编辑"}); err != nil {
		t.Fatalf("unrelated sibling edit was blocked: %v", err)
	}
	if err := h.service.ReleaseRunLock(context.Background(), "r_agent"); err != nil {
		t.Fatal(err)
	}
	if h.service.isNodeLocked("n_output") {
		t.Fatal("run lock release kept the bound output locked")
	}
}

func TestLockedNodeTopologyCanChange(t *testing.T) {
	h := newHarness(t, baseProject())
	h.node(t, "n_c").Locked = true
	project, err := h.service.Connect(context.Background(), "n_candidate", "n_c")
	if err != nil {
		t.Fatalf("connecting a content-locked node should be allowed: %v", err)
	}
	if parents := h.parents(t, "n_c"); len(parents) != 2 || !h.node(t, "n_c").Stale {
		t.Fatalf("child parents = %+v", parents)
	}
	ordinary := ""
	for _, edge := range project.Edges {
		if edge.TargetNodeID == "n_c" && edge.SourceNodeID == "n_candidate" {
			ordinary = edge.ID
		}
	}
	if ordinary == "" {
		t.Fatal("new edge missing")
	}
	if _, _, err := h.service.Disconnect(context.Background(), ordinary); err != nil {
		t.Fatalf("disconnecting a content-locked node should be allowed: %v", err)
	}
	if parentOf(h.session.project, "n_c") != "n_candidate2" {
		t.Fatalf("remaining edge = %+v", h.session.project.Edges)
	}
}

func TestRunLockLifecycle(t *testing.T) {
	h := newHarness(t, projectWithSecondTree())
	if err := h.service.AcquireRunLock(context.Background(), "r_one", "n_topic", "n_a", []string{"n_topic", "n_candidate", "n_a"}); err != nil {
		t.Fatal(err)
	}
	if err := h.service.AcquireRunLock(context.Background(), "r_two", "n_topic2", "n_x", []string{"n_topic2", "n_x"}); err != nil {
		t.Fatal(err)
	}
	locked := h.service.LockedNodeIDs()
	if len(locked) != 5 || locked[0] != "n_a" || locked[1] != "n_candidate" || locked[2] != "n_topic" || locked[3] != "n_topic2" || locked[4] != "n_x" {
		t.Fatalf("locked = %+v", locked)
	}
	if err := h.service.ReleaseRunLock(context.Background(), "r_one"); err != nil {
		t.Fatal(err)
	}
	locked = h.service.LockedNodeIDs()
	if len(locked) != 2 || locked[0] != "n_topic2" || locked[1] != "n_x" {
		t.Fatalf("locked after release = %+v", locked)
	}
	if err := h.service.ReleaseRunLock(context.Background(), "r_missing"); err != nil {
		t.Fatalf("releasing missing run: %v", err)
	}
	if err := h.service.AcquireRunLock(context.Background(), "r_bad", "n_a", "n_topic", []string{"n_a", "n_topic"}); !errors.Is(err, ErrNotInChain) {
		t.Fatalf("error = %v, want NOT_IN_CHAIN", err)
	}
	if err := h.service.AcquireRunLock(context.Background(), "r_bad", "n_topic", "n_b", []string{"n_topic", "n_b"}); !errors.Is(err, ErrNotInChain) {
		t.Fatalf("error = %v, want NOT_IN_CHAIN", err)
	}
	if err := h.service.AcquireRunLock(context.Background(), "bad/id", "n_topic", "n_a", []string{"n_topic", "n_candidate", "n_a"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("error = %v, want INVALID_INPUT", err)
	}
	if err := h.service.AcquireRunLock(context.Background(), "r_bad", "n_topic", "n_missing", []string{"n_topic", "n_missing"}); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("error = %v, want NODE_NOT_FOUND", err)
	}
}

func TestBeginRunRejectsStaleProjectBeforeLocking(t *testing.T) {
	h := newHarness(t, baseProject())
	beforeChanges := len(*h.changes)
	if _, _, err := h.service.BeginRun(context.Background(), "r_run", "n_a", "p_other"); !errors.Is(err, ErrProjectChanged) {
		t.Fatalf("BeginRun error = %v, want PROJECT_CHANGED", err)
	}
	if locked := h.service.LockedNodeIDs(); len(locked) != 0 {
		t.Fatalf("stale project start acquired path locks: %+v", locked)
	}
	if len(*h.changes) != beforeChanges {
		t.Fatalf("stale project start published changes: %+v", *h.changes)
	}
}

func TestRunLockReleaseEventCarriesProjectAndLatestGraphRevision(t *testing.T) {
	h := newHarness(t, baseProject())
	if err := h.service.AcquireRunLock(context.Background(), "r_one", "n_topic", "n_a", []string{"n_topic", "n_candidate", "n_a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.UpdateNodeParams(context.Background(), "n_c", "C", entity.NodeParams{Instruction: "updated sibling"}); err != nil {
		t.Fatal(err)
	}
	if err := h.service.ReleaseRunLock(context.Background(), "r_one"); err != nil {
		t.Fatal(err)
	}
	changes := *h.changes
	if len(changes) != 3 {
		t.Fatalf("change events = %+v", changes)
	}
	last := changes[len(changes)-1]
	if last.projectID != h.session.project.ID || last.revision != h.session.project.Revision || last.reason != ChangeReasonRunLock {
		t.Fatalf("run lock release event = %+v, project=%+v", last, h.session.project)
	}
}
