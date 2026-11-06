package workflow

import (
	"context"
	"errors"
	"math"
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
}

func TestAddNodeValidation(t *testing.T) {
	h := newHarness(t, baseProject())
	cases := []struct {
		name  string
		input AddNodeInput
		want  error
	}{
		{"unknown type", AddNodeInput{Type: "render"}, ErrUnknownNodeType},
		{"topic with parent", AddNodeInput{Type: valueobject.NodeTypeTopic, Parent: "n_topic"}, ErrTopicRootParent},
		{"missing parent", AddNodeInput{Type: valueobject.NodeTypePrompt, Parent: "n_missing"}, ErrNodeNotFound},
		{"draft parent", AddNodeInput{Type: valueobject.NodeTypePrompt, Parent: "n_draft"}, ErrNotInChain},
		{"topic connection invalid", AddNodeInput{Type: valueobject.NodeTypePrompt, Parent: "n_topic"}, ErrNodeConnectionInvalid},
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
	if _, err := h.service.Connect(context.Background(), "n_topic", "n_draft", nil, false, wrongProjectID); !errors.Is(err, ErrProjectChanged) {
		t.Fatalf("Connect error = %v, want PROJECT_CHANGED", err)
	}
	if _, err := h.service.MoveNode(context.Background(), "n_a", 4, 5, wrongProjectID); !errors.Is(err, ErrProjectChanged) {
		t.Fatalf("MoveNode error = %v, want PROJECT_CHANGED", err)
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
	if agent.Type != valueobject.NodeTypeAgent || agent.Parent != "n_a" {
		t.Fatalf("agent = %+v", agent)
	}

	project, err = h.service.AddNode(context.Background(), AddNodeInput{
		Type: valueobject.NodeTypeScript, Parent: agent.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if added := project.Nodes[len(project.Nodes)-1]; added.Type != valueobject.NodeTypeScript || added.Parent != agent.ID {
		t.Fatalf("script = %+v", added)
	}

	if _, err := h.service.AddNode(context.Background(), AddNodeInput{
		Type: valueobject.NodeTypePrompt, Parent: "n_a",
	}); !errors.Is(err, ErrNodeConnectionInvalid) {
		t.Fatalf("prompt-to-prompt error = %v, want NODE_CONNECTION_INVALID", err)
	}
}

func TestAddNodeEnforcesSingleScriptOutput(t *testing.T) {
	h := newHarness(t, baseProject())
	if _, err := h.service.AddNode(context.Background(), AddNodeInput{
		Type: valueobject.NodeTypeScript, Parent: "n_b",
	}); err != nil {
		t.Fatal(err)
	}
	saves := h.session.saveCount
	if _, err := h.service.AddNode(context.Background(), AddNodeInput{
		Type: valueobject.NodeTypeScript, Parent: "n_b",
	}); !errors.Is(err, ErrOutputBindingConflict) {
		t.Fatalf("error = %v, want OUTPUT_BINDING_CONFLICT", err)
	}
	if h.session.saveCount != saves {
		t.Fatal("conflicting output binding was saved")
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

func TestConnectRequiresConfirmAndMovesSubtreeStale(t *testing.T) {
	h := newHarness(t, baseProject())
	_, err := h.service.Connect(context.Background(), "n_candidate2", "n_a", nil, false)
	if !errors.Is(err, ErrReparentConfirmRequired) {
		t.Fatalf("error = %v, want REPARENT_CONFIRM_REQUIRED", err)
	}
	if h.session.saveCount != 0 || len(*h.changes) != 0 {
		t.Fatal("confirm-required command persisted a change")
	}

	project, err := h.service.Connect(context.Background(), "n_candidate2", "n_a", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if project.Revision != 2 {
		t.Fatalf("revision = %d", project.Revision)
	}
	child := h.node(t, "n_a")
	if child.Parent != "n_candidate2" || child.ParentCandidate != nil {
		t.Fatalf("child = %+v", child)
	}
	if !child.Stale || !h.node(t, "n_b").Stale {
		t.Fatal("subtree was not marked stale")
	}
	if h.node(t, "n_c").Stale || h.node(t, "n_draft").Stale {
		t.Fatal("node outside moved subtree was marked stale")
	}
	if len(h.mover.calls) != 0 {
		t.Fatalf("same-tree move should not touch steps: %+v", h.mover.calls)
	}
	if len(*h.changes) != 1 {
		t.Fatalf("changes = %+v", *h.changes)
	}

	before := h.session.saveCount
	project, err = h.service.Connect(context.Background(), "n_candidate2", "n_a", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if project.Revision != 2 || h.session.saveCount != before || len(*h.changes) != 1 {
		t.Fatal("same parent/candidate was not idempotent")
	}
}

func TestConnectRejectsCandidateReparent(t *testing.T) {
	h := newHarness(t, baseProject())
	_, err := h.service.Connect(context.Background(), "n_candidate2", "n_candidate", nil, true)
	if !errors.Is(err, ErrNodeConnectionInvalid) {
		t.Fatalf("error = %v, want NODE_CONNECTION_INVALID", err)
	}
	if h.session.saveCount != 0 || h.node(t, "n_candidate").Parent != "n_topic" {
		t.Fatal("candidate source changed")
	}
}

func TestConnectValidation(t *testing.T) {
	cases := []struct {
		name      string
		parent    string
		child     string
		candidate *int
		want      error
	}{
		{"self", "n_a", "n_a", nil, ErrSelfConnect},
		{"cycle", "n_b", "n_a", nil, ErrCycleDetected},
		{"topic child", "n_a", "n_topic", nil, ErrTopicRootParent},
		{"draft parent", "n_draft", "n_c", nil, ErrNotInChain},
		{"missing parent", "n_missing", "n_c", nil, ErrNodeNotFound},
		{"missing child", "n_a", "n_missing", nil, ErrNodeNotFound},
		{"invalid candidate parameter", "n_topic", "n_c", intPtr(9), ErrNodeConnectionInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, baseProject())
			_, err := h.service.Connect(context.Background(), tc.parent, tc.child, tc.candidate, false)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if h.session.saveCount != 0 {
				t.Fatal("invalid command saved")
			}
		})
	}
}

func TestConnectDraftAgentToPrompt(t *testing.T) {
	h := newHarness(t, baseProject())
	project, err := h.service.Connect(context.Background(), "n_c", "n_agent_draft", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	agent := h.node(t, "n_agent_draft")
	if agent.Parent != "n_c" || agent.ParentCandidate != nil || !agent.Stale {
		t.Fatalf("agent = %+v", agent)
	}
	if project.Revision != 2 {
		t.Fatalf("revision = %d", project.Revision)
	}
}

func TestConnectCrossTreeMovesSteps(t *testing.T) {
	h := newHarness(t, projectWithSecondTree())
	if _, err := h.service.Connect(context.Background(), "n_x", "n_a", nil, true); err != nil {
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
	child := h.node(t, "n_a")
	if child.Parent != "n_x" || !child.Stale || !h.node(t, "n_b").Stale {
		t.Fatalf("cross-tree move wrong: %+v", child)
	}
}

func TestConnectCrossTreeFailures(t *testing.T) {
	t.Run("move failure", func(t *testing.T) {
		h := newHarness(t, projectWithSecondTree())
		h.mover.err = errors.New("disk full")
		_, err := h.service.Connect(context.Background(), "n_x", "n_a", nil, true)
		if !errors.Is(err, ErrWorkspaceMoveFailed) {
			t.Fatalf("error = %v, want WORKSPACE_MOVE_FAILED", err)
		}
		if h.session.saveCount != 0 || h.node(t, "n_a").Parent != "n_candidate" {
			t.Fatal("graph changed after move failure")
		}
	})
	t.Run("save failure rolls back movement", func(t *testing.T) {
		h := newHarness(t, projectWithSecondTree())
		h.session.saveErr = errors.New("save failed")
		_, err := h.service.Connect(context.Background(), "n_x", "n_a", nil, true)
		if err == nil {
			t.Fatal("error = nil")
		}
		if h.mover.rollbacks != 1 {
			t.Fatalf("rollbacks = %d", h.mover.rollbacks)
		}
		if h.node(t, "n_a").Parent != "n_candidate" {
			t.Fatal("graph changed after save failure")
		}
	})
	t.Run("locked target parent", func(t *testing.T) {
		h := newHarness(t, projectWithSecondTree())
		if err := h.service.AcquireRunLock(context.Background(), "r_two", "n_topic2", "n_x", []string{"n_topic2", "n_x"}); err != nil {
			t.Fatal(err)
		}
		_, err := h.service.Connect(context.Background(), "n_x", "n_a", nil, true)
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
		_, err := h.service.Connect(context.Background(), "n_x", "n_a", nil, true)
		if !errors.Is(err, ErrRunningChainLocked) {
			t.Fatalf("error = %v, want RUNNING_CHAIN_LOCKED", err)
		}
	})
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

func TestBeginRunRejectsUnwiredPromptAndAgentEntries(t *testing.T) {
	h := newHarness(t, baseProject())
	for _, nodeID := range []string{"n_a", "n_b"} {
		if _, _, err := h.service.BeginRun(context.Background(), "r_unavailable", nodeID); !errors.Is(err, ErrRunEntryUnavailable) {
			t.Fatalf("BeginRun(%s) error = %v, want RUN_ENTRY_UNAVAILABLE", nodeID, err)
		}
	}
	if len(h.service.locks) != 0 || len(*h.changes) != 0 {
		t.Fatal("unavailable run entry acquired a lock")
	}
}

func TestLockedNodeTopologyCanChange(t *testing.T) {
	h := newHarness(t, baseProject())
	h.node(t, "n_c").Locked = true
	if _, err := h.service.Connect(context.Background(), "n_candidate", "n_c", nil, true); err != nil {
		t.Fatalf("reparenting a content-locked node should be allowed: %v", err)
	}
	if h.node(t, "n_c").Parent != "n_candidate" || !h.node(t, "n_c").Stale {
		t.Fatalf("child = %+v", h.node(t, "n_c"))
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
