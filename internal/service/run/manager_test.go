package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
)

func TestTurnGateRequiresOwnerToRelease(t *testing.T) {
	gate := NewTurnGate()
	if err := gate.Acquire("r_one"); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(gate.Acquire("r_two"), ErrBusy) {
		t.Fatal("second owner should receive BUSY")
	}
	if !errors.Is(gate.Release("r_two"), ErrGateOwner) {
		t.Fatal("wrong owner released the gate")
	}
	if err := gate.Release("r_one"); err != nil {
		t.Fatal(err)
	}
	if gate.Busy() {
		t.Fatal("gate remained busy after owner release")
	}
}

func TestErrorCodePreservesPromptTaskEmpty(t *testing.T) {
	err := fmt.Errorf("%w: Prompt n_prompt has no task", entity.ErrPromptTaskEmpty)
	if got := ErrorCode(err); got != "PROMPT_TASK_EMPTY" {
		t.Fatalf("ErrorCode() = %q, want PROMPT_TASK_EMPTY", got)
	}
}

func TestStartRunUsesOneShotSpecAndPersistsTerminalUsage(t *testing.T) {
	h := newRunHarness(t)
	record, err := h.manager.Start(context.Background(), "n_topic")
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != valueobject.RunRunning || record.ChainID != "c_n_topic" || !slices.Equal(record.ChainPath, []string{"n_topic"}) {
		t.Fatalf("run = %+v", record.RunSummary)
	}
	if record.ExecutionSnapshot == nil || record.ExecutionSnapshot.NodeID != "n_topic" || record.ExecutionSnapshot.Validate() != nil {
		t.Fatalf("run did not preserve a valid execution snapshot: %+v", record.ExecutionSnapshot)
	}
	if h.manager.ActiveRunID() != record.ID || !h.manager.IsBusy() {
		t.Fatal("run gate was not held")
	}
	if h.driver.spec.SystemPrompt == "" || len(h.driver.spec.BuiltinTools) == 0 || len(h.driver.spec.AllowedTools) == 0 {
		t.Fatalf("driver did not receive per-run configuration: %+v", h.driver.spec)
	}
	if h.driver.spec.MCPConfigPath == "" || h.driver.spec.MCPToken != "secret-test-token" {
		t.Fatalf("MCP credentials missing from driver spec: %+v", h.driver.spec)
	}
	if _, err := os.Stat(h.driver.spec.MCPConfigPath); err != nil {
		t.Fatalf("MCP config should exist throughout the active run: %v", err)
	}
	if !errors.Is(func() error { _, err := h.manager.Start(context.Background(), "n_other"); return err }(), ErrBusy) {
		t.Fatal("parallel run should be rejected with BUSY")
	}

	actual := 140
	h.driver.spec.EventSink(valueobject.AgentEvent{
		Version: 1, Type: valueobject.EventStatus,
		State: &valueobject.AgentState{Status: valueobject.SessionIdle}, ActualInputTokens: &actual,
	})
	waitFor(t, func() bool { return !h.manager.IsBusy() })
	stored, err := h.store.Load(context.Background(), record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ExecutionSnapshot == nil || stored.ExecutionSnapshot.NodeID != record.ExecutionSnapshot.NodeID || !slices.Equal(stored.ExecutionSnapshot.ChainPath, record.ExecutionSnapshot.ChainPath) {
		t.Fatalf("persisted run snapshot = %+v", stored.ExecutionSnapshot)
	}
	if stored.Status != valueobject.RunSucceeded || !stored.UsageReported || stored.ActualInputTokens != actual || !stored.OverBudget {
		t.Fatalf("terminal run = %+v", stored.RunSummary)
	}
	if !h.workflow.released || !h.configs.cleaned || !h.tool.stopped || !h.session.closed {
		t.Fatalf("cleanup incomplete: workflow=%v config=%v tool=%v session=%v", h.workflow.released, h.configs.cleaned, h.tool.stopped, h.session.closed)
	}
	if _, err := os.Stat(h.driver.spec.MCPConfigPath); !os.IsNotExist(err) {
		t.Fatalf("MCP config should be removed after the run, stat error = %v", err)
	}
	waitFor(t, func() bool { return len(h.publisher.snapshot()) >= 3 })
	events := h.publisher.snapshot()
	if len(events) < 3 {
		t.Fatalf("missing run events: %d", len(events))
	}
	for _, event := range events {
		if event.RunID != record.ID || event.ProjectID != "p_test" || event.TreeID != "n_topic" || event.NodeID != "n_topic" || !slices.Equal(event.ChainPath, []string{"n_topic"}) {
			t.Fatalf("run event identity missing: %+v", event)
		}
	}
}

func TestStartWithInputsSupportsRootlessDAGSnapshot(t *testing.T) {
	h := newRunHarness(t)
	project := entity.Project{
		Version: valueobject.ProjectFileVersion, ID: "p_test", Name: "Rootless", Revision: 1, ActiveNodeID: "n_agent",
		Nodes: []entity.Node{
			{ID: "n_prompt", Type: valueobject.NodeTypePrompt, Title: "任务", Params: entity.NodeParams{Instruction: "整理输入"}, Rounds: []entity.Round{}},
			{ID: "n_agent", Type: valueobject.NodeTypeAgent, Title: "Agent", Rounds: []entity.Round{}},
		},
		Edges: []entity.Edge{{ID: "e_prompt_agent", SourceNodeID: "n_prompt", TargetNodeID: "n_agent", Kind: entity.EdgeKindConnection}},
	}
	h.workflow.project = project
	h.projects.project = project
	inputEdgeIDs := []string{"e_prompt_agent"}
	record, err := h.manager.StartWithInputs(context.Background(), "n_agent", inputEdgeIDs, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.TreeID != "" || len(record.ChainPath) != 0 || record.ExecutionSnapshot == nil || record.ExecutionSnapshot.Version != 4 {
		t.Fatalf("rootless run record = %+v", record.RunSummary)
	}
	if h.toolFactory.scope.TreeID != "" || len(h.toolFactory.scope.ChainPath) != 0 ||
		!slices.Equal(h.toolFactory.scope.InputNodeIDs, record.ExecutionSnapshot.ScopeNodeIDs()) {
		t.Fatalf("rootless run scope = %+v", h.toolFactory.scope)
	}
	if h.toolFactory.scope.ExecutionSnapshot.ProjectID != project.ID ||
		h.toolFactory.scope.ExecutionSnapshot.ProjectRevision != record.ExecutionSnapshot.ProjectRevision ||
		!slices.Equal(h.toolFactory.scope.ExecutionSnapshot.ScopeNodeIDs(), record.ExecutionSnapshot.ScopeNodeIDs()) {
		t.Fatalf("MCP scope did not retain the frozen execution snapshot: %+v", h.toolFactory.scope.ExecutionSnapshot)
	}
	h.workflow.mu.Lock()
	h.workflow.project.Nodes[0].Params.Instruction = "live graph changed"
	h.workflow.mu.Unlock()
	if got := h.toolFactory.scope.ExecutionSnapshot.InputNodes[0].Params.Instruction; got != "整理输入" {
		t.Fatalf("frozen run scope changed with the live project: %q", got)
	}
	if err := h.manager.Interrupt(context.Background(), record.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return !h.manager.IsBusy() })
}

func TestRecordRunPendingRetryAfterStoreFailureDoesNotDuplicateEvent(t *testing.T) {
	h := newRunHarness(t)
	record, err := h.manager.Start(context.Background(), "n_topic")
	if err != nil {
		t.Fatal(err)
	}
	pending := entity.Pending{ID: "pa_test", RunID: record.ID, NodeID: "n_topic", Status: valueobject.PendingPending}
	storeErr := errors.New("injected run index failure")
	h.store.mu.Lock()
	h.store.saveErr = storeErr
	h.store.mu.Unlock()
	if err := h.manager.RecordRunPending(context.Background(), h.toolFactory.scope, pending); !errors.Is(err, storeErr) {
		t.Fatalf("first pending record error = %v, want injected error", err)
	}
	countPendingEvents := func() int {
		count := 0
		for _, event := range h.publisher.snapshot() {
			if event.Type == valueobject.EventPending && event.PendingID == pending.ID {
				count++
			}
		}
		return count
	}
	if count := countPendingEvents(); count != 1 {
		t.Fatalf("pending events after failed index write = %d, want 1", count)
	}

	h.store.mu.Lock()
	h.store.saveErr = nil
	h.store.mu.Unlock()
	if err := h.manager.RecordRunPending(context.Background(), h.toolFactory.scope, pending); err != nil {
		t.Fatalf("retry pending record: %v", err)
	}
	if count := countPendingEvents(); count != 1 {
		t.Fatalf("pending retry duplicated event: %d", count)
	}
	otherPending := pending
	otherPending.ID = "pa_other"
	if err := h.manager.RecordRunPending(context.Background(), h.toolFactory.scope, otherPending); !errors.Is(err, ErrRunPendingConflict) {
		t.Fatalf("different pending identity error = %v, want RUN_PENDING_CONFLICT", err)
	}
	stored, err := h.store.Load(context.Background(), record.ID)
	if err != nil || stored.PendingID != pending.ID {
		t.Fatalf("persisted pending id = %q err=%v", stored.PendingID, err)
	}
	h.driver.spec.EventSink(valueobject.AgentEvent{Type: valueobject.EventStatus, State: &valueobject.AgentState{Status: valueobject.SessionIdle}})
	waitFor(t, func() bool { return !h.manager.IsBusy() })
}

func TestStartResolvesDriverAgainAfterUnavailableProbe(t *testing.T) {
	h := newRunHarness(t)
	h.manager.opts.Driver = nil
	attempts := 0
	h.manager.opts.DriverProvider = func(context.Context) (port.AgentDriver, error) {
		attempts++
		if attempts == 1 {
			return nil, errors.New("native CLI unavailable")
		}
		return h.driver, nil
	}

	if _, err := h.manager.Start(context.Background(), "n_topic"); err == nil || ErrorCode(err) != "AGENT_UNAVAILABLE" {
		t.Fatalf("first Start error = %v, want AGENT_UNAVAILABLE", err)
	}
	if h.manager.IsBusy() || h.workflow.locked {
		t.Fatal("unavailable driver left a run or graph lock active")
	}

	record, err := h.manager.Start(context.Background(), "n_topic")
	if err != nil {
		t.Fatalf("second Start error = %v", err)
	}
	if attempts != 2 {
		t.Fatalf("driver provider attempts = %d, want 2", attempts)
	}
	h.driver.spec.EventSink(valueobject.AgentEvent{Type: valueobject.EventStatus, State: &valueobject.AgentState{Status: valueobject.SessionIdle}})
	waitFor(t, func() bool { return !h.manager.IsBusy() })
	if _, err := h.store.Load(context.Background(), record.ID); err != nil {
		t.Fatalf("load completed run: %v", err)
	}
}

func TestStartHandlesRunEndingBeforeDriverStartReturns(t *testing.T) {
	h := newRunHarness(t)
	h.driver.startEvents = []valueobject.AgentEvent{{
		Type:  valueobject.EventStatus,
		State: &valueobject.AgentState{Status: valueobject.SessionIdle},
	}}
	record, err := h.manager.Start(context.Background(), "n_topic")
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return !h.manager.IsBusy() })
	stored, err := h.store.Load(context.Background(), record.ID)
	if err != nil || stored.Status != valueobject.RunSucceeded {
		t.Fatalf("early terminal run = %+v, err=%v", stored.RunSummary, err)
	}
	events := h.publisher.snapshot()
	if len(events) < 3 || events[len(events)-1].Status != valueobject.RunSucceeded {
		t.Fatalf("early terminal events = %+v", events)
	}
}

func TestStartRejectsStaleProjectScopeBeforeAcquiringRunLock(t *testing.T) {
	h := newRunHarness(t)
	if _, err := h.manager.Start(context.Background(), "n_topic", "p_other"); err == nil || ErrorCode(err) != "PROJECT_CHANGED" {
		t.Fatalf("start error = %v, want PROJECT_CHANGED", err)
	}
	h.store.mu.Lock()
	recordCount := len(h.store.records)
	h.store.mu.Unlock()
	if h.manager.IsBusy() || h.workflow.locked || recordCount != 0 {
		t.Fatalf("stale project start left resources: busy=%v locked=%v records=%d", h.manager.IsBusy(), h.workflow.locked, recordCount)
	}
}

func TestRunSnapshotRecoversTranscriptRequestsAndTerminalSequence(t *testing.T) {
	h := newRunHarness(t)
	record, err := h.manager.Start(context.Background(), "n_topic")
	if err != nil {
		t.Fatal(err)
	}
	h.driver.spec.EventSink(valueobject.AgentEvent{
		Type:    valueobject.EventMessage,
		Message: &valueobject.TranscriptItem{ID: "stream", Kind: valueobject.TranscriptMessage, Role: "assistant", Text: "hello ", Streaming: true},
	})
	h.driver.spec.EventSink(valueobject.AgentEvent{
		Type:    valueobject.EventMessage,
		Message: &valueobject.TranscriptItem{ID: "stream", Kind: valueobject.TranscriptMessage, Role: "assistant", Text: "world", Streaming: true},
	})
	h.driver.spec.EventSink(valueobject.AgentEvent{
		Type:       valueobject.EventPermission,
		Permission: &valueobject.PermissionRequest{ID: "perm_one", Tool: "Write", Paths: []string{"a.txt"}},
	})
	h.driver.spec.EventSink(valueobject.AgentEvent{
		Type:     valueobject.EventQuestion,
		Question: &valueobject.QuestionRequest{ID: "question_one", Question: "Continue?", Options: []string{"yes"}},
	})
	snapshot, err := h.manager.GetSnapshot(context.Background(), record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ProjectID != "p_test" || snapshot.RunID != record.ID || snapshot.LastSeq == 0 {
		t.Fatalf("run snapshot identity/sequence = %+v", snapshot)
	}
	if len(snapshot.Run.Transcript) != 1 || snapshot.Run.Transcript[0].Text != "hello world" {
		t.Fatalf("snapshot transcript = %+v", snapshot.Run.Transcript)
	}
	if len(snapshot.Permissions) != 1 || snapshot.Permissions[0].ID != "perm_one" || len(snapshot.Questions) != 1 || snapshot.Questions[0].ID != "question_one" {
		t.Fatalf("snapshot requests = permissions %+v questions %+v", snapshot.Permissions, snapshot.Questions)
	}
	beforeResolveSeq := snapshot.LastSeq
	if err := h.manager.RespondPermission(record.ID, "perm_one", valueobject.DecisionAllow); err != nil {
		t.Fatal(err)
	}
	h.permissions.resolved("permission", "perm_one")
	resolved, err := h.manager.GetSnapshot(context.Background(), record.ID)
	if err != nil || resolved.LastSeq != beforeResolveSeq+1 || len(resolved.Permissions) != 0 {
		t.Fatalf("permission response snapshot = %+v, err=%v", resolved, err)
	}
	if err := h.manager.RespondQuestion(record.ID, "question_one", valueobject.QuestionAnswer{Answer: "yes", Via: "option"}); err != nil {
		t.Fatal(err)
	}
	h.permissions.resolved("question", "question_one")
	snapshot, err = h.manager.GetSnapshot(context.Background(), record.ID)
	if err != nil || len(snapshot.Permissions) != 0 || len(snapshot.Questions) != 0 {
		t.Fatalf("resolved request snapshot = %+v, err=%v", snapshot, err)
	}
	actual := 24
	h.driver.spec.EventSink(valueobject.AgentEvent{Type: valueobject.EventStatus, State: &valueobject.AgentState{Status: valueobject.SessionIdle}, ActualInputTokens: &actual})
	waitFor(t, func() bool { return !h.manager.IsBusy() })
	snapshot, err = h.manager.GetSnapshot(context.Background(), record.ID)
	if err != nil || snapshot.Run.Status != valueobject.RunSucceeded || len(snapshot.Permissions) != 0 || len(snapshot.Questions) != 0 {
		t.Fatalf("terminal snapshot = %+v, err=%v", snapshot, err)
	}
	events := h.publisher.snapshot()
	if len(events) == 0 || snapshot.LastSeq != events[len(events)-1].Seq {
		t.Fatalf("terminal lastSeq=%d, final event=%+v", snapshot.LastSeq, events)
	}
	for i, event := range events {
		if event.Seq != uint64(i+1) || event.ProjectID != "p_test" {
			t.Fatalf("event sequence/identity at %d = %+v", i, event)
		}
	}
	restarted := New(Options{
		Projects:  h.projects,
		RunStores: &fakeRunStores{stores: map[string]*memoryRunStore{"p_test": h.store}},
	})
	historical, err := restarted.GetSnapshot(context.Background(), record.ID)
	if err != nil || historical.Run.Status != valueobject.RunSucceeded || historical.LastSeq != 0 || len(historical.Permissions) != 0 || len(historical.Questions) != 0 {
		t.Fatalf("restarted historical snapshot = %+v, err=%v", historical, err)
	}
}

func TestConcurrentStreamEventsMatchSnapshotSequenceAtomically(t *testing.T) {
	h := newRunHarness(t)
	record, err := h.manager.Start(context.Background(), "n_topic")
	if err != nil {
		t.Fatal(err)
	}
	const fragments = 64
	var writers sync.WaitGroup
	for i := 0; i < fragments; i++ {
		writers.Add(1)
		go func() {
			defer writers.Done()
			h.driver.spec.EventSink(valueobject.AgentEvent{
				Type:    valueobject.EventMessage,
				Message: &valueobject.TranscriptItem{ID: "concurrent", Kind: valueobject.TranscriptMessage, Role: "assistant", Text: "x", Streaming: true},
			})
		}()
	}
	writers.Wait()
	snapshot, err := h.manager.GetSnapshot(context.Background(), record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Run.Transcript) != 1 || len(snapshot.Run.Transcript[0].Text) != fragments {
		t.Fatalf("stream snapshot transcript = %+v", snapshot.Run.Transcript)
	}
	events := h.publisher.snapshot()
	if len(events) != fragments+2 || snapshot.LastSeq != events[len(events)-1].Seq {
		t.Fatalf("snapshot seq=%d, event count=%d, last=%+v", snapshot.LastSeq, len(events), events[len(events)-1])
	}
	for i, event := range events {
		if event.Seq != uint64(i+1) {
			t.Fatalf("event sequence[%d] = %d", i, event.Seq)
		}
	}
}

func TestRunEventPublisherCanReenterRequestResolution(t *testing.T) {
	h := newRunHarness(t)
	record, err := h.manager.Start(context.Background(), "n_topic")
	if err != nil {
		t.Fatal(err)
	}
	publisher := &resolvingRunPublisher{manager: h.manager, result: make(chan error, 1)}
	h.manager.opts.Publisher = publisher
	done := make(chan struct{})
	go func() {
		h.driver.spec.EventSink(valueobject.AgentEvent{
			Type:       valueobject.EventPermission,
			Permission: &valueobject.PermissionRequest{ID: "perm_reentrant", Tool: "Write"},
		})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("event publisher reentry deadlocked request resolution")
	}
	if err := <-publisher.result; err != nil {
		t.Fatal(err)
	}
	snapshot, err := h.manager.GetSnapshot(context.Background(), record.ID)
	if err != nil || len(snapshot.Permissions) != 0 {
		t.Fatalf("reentrant resolution snapshot = %+v, err=%v", snapshot, err)
	}
}

func TestRunSnapshotRejectsActiveRunOutsideCurrentProject(t *testing.T) {
	h := newRunHarness(t)
	record, err := h.manager.Start(context.Background(), "n_topic")
	if err != nil {
		t.Fatal(err)
	}
	h.projects.project.ID = "p_other"
	if _, err := h.manager.GetSnapshot(context.Background(), record.ID); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("cross-project snapshot error = %v, want RUN_NOT_FOUND", err)
	}
	h.projects.project.ID = "p_test"
	if err := h.manager.Interrupt(context.Background(), record.ID); err != nil {
		t.Fatal(err)
	}
}

func TestSendFailureReleasesRunLockGateAndMCPConfig(t *testing.T) {
	h := newRunHarness(t)
	h.session.sendErr = errors.New("AGENT_SEND_FAILED: unavailable")
	record, err := h.manager.Start(context.Background(), "n_topic")
	if err == nil {
		t.Fatal("expected send failure")
	}
	if record.Status != valueobject.RunFailed || record.Error != "AGENT_SEND_FAILED" {
		t.Fatalf("failed run = %+v", record.RunSummary)
	}
	if h.manager.IsBusy() || !h.workflow.released || !h.configs.cleaned || !h.tool.stopped {
		t.Fatalf("failure leaked resources: busy=%v lock=%v config=%v tool=%v", h.manager.IsBusy(), h.workflow.released, h.configs.cleaned, h.tool.stopped)
	}
}

func TestBeginRunFailureReleasesTurnGate(t *testing.T) {
	h := newRunHarness(t)
	h.workflow.beginErr = errors.New("workflow unavailable")
	if _, err := h.manager.Start(context.Background(), "n_topic"); err == nil {
		t.Fatal("expected BeginRun failure")
	}
	if h.manager.IsBusy() || h.workflow.released {
		t.Fatalf("failed BeginRun leaked gate or released an unacquired lock: busy=%v released=%v", h.manager.IsBusy(), h.workflow.released)
	}
}

func TestDriverStartFailureReleasesRunResources(t *testing.T) {
	h := newRunHarness(t)
	h.driver.startErr = errors.New("AGENT_UNAVAILABLE: test failure")
	record, err := h.manager.Start(context.Background(), "n_topic")
	if err == nil {
		t.Fatal("expected driver start failure")
	}
	if record.Status != valueobject.RunFailed || record.Error != "AGENT_UNAVAILABLE" {
		t.Fatalf("failed run = %+v", record.RunSummary)
	}
	if h.manager.IsBusy() || !h.workflow.released || !h.configs.cleaned || !h.tool.stopped {
		t.Fatalf("startup failure leaked resources: busy=%v lock=%v config=%v tool=%v", h.manager.IsBusy(), h.workflow.released, h.configs.cleaned, h.tool.stopped)
	}
}

func TestStartRejectsWorkspaceScopeOutsideSnapshot(t *testing.T) {
	h := newRunHarness(t)
	h.manager.opts.Workspaces = fakeWorkspaceProvider{scopeTransform: func(scope port.WorkspaceScope) port.WorkspaceScope {
		scope.NodeID = "n_other"
		return scope
	}}
	if _, err := h.manager.Start(context.Background(), "n_topic"); ErrorCode(err) != "RUN_WORKSPACE_SCOPE_INVALID" {
		t.Fatalf("start error = %v, want RUN_WORKSPACE_SCOPE_INVALID", err)
	}
	if h.manager.IsBusy() || !h.workflow.released || h.workflow.locked {
		t.Fatalf("invalid workspace scope leaked run resources: busy=%v released=%v locked=%v", h.manager.IsBusy(), h.workflow.released, h.workflow.locked)
	}
	h.store.mu.Lock()
	recordCount := len(h.store.records)
	h.store.mu.Unlock()
	if recordCount != 0 {
		t.Fatalf("invalid workspace scope persisted %d run records", recordCount)
	}
}

func TestStartPrepareFailureCleansPartialConfig(t *testing.T) {
	h := newRunHarness(t)
	h.configs.prepareErr = errors.New("injected config prepare failure")
	record, err := h.manager.Start(context.Background(), "n_topic")
	if err == nil || record.Status != valueobject.RunFailed {
		t.Fatalf("start record=%+v err=%v, want failed startup", record.RunSummary, err)
	}
	configDir := filepath.Join(h.configs.root, "p_test", record.ID)
	if _, err := os.Stat(configDir); !os.IsNotExist(err) {
		t.Fatalf("partially prepared config remains, stat error = %v", err)
	}
	if h.manager.IsBusy() || !h.workflow.released || h.configs.cleanupCount != 1 {
		t.Fatalf("prepare failure leaked resources: busy=%v lockReleased=%v cleanupCount=%d", h.manager.IsBusy(), h.workflow.released, h.configs.cleanupCount)
	}
}

func TestMCPStartFailureReleasesRunResources(t *testing.T) {
	h := newRunHarness(t)
	h.tool.startErr = errors.New("injected MCP start failure")
	record, err := h.manager.Start(context.Background(), "n_topic")
	if err == nil || record.Status != valueobject.RunFailed {
		t.Fatalf("start record=%+v err=%v, want failed startup", record.RunSummary, err)
	}
	if h.manager.IsBusy() || !h.workflow.released || !h.configs.cleaned || !h.tool.stopped {
		t.Fatalf("MCP start failure leaked resources: busy=%v lock=%v config=%v tool=%v", h.manager.IsBusy(), h.workflow.released, h.configs.cleaned, h.tool.stopped)
	}
}

func TestTerminalCleanupFailuresMarkRunFailedAndReleaseLocks(t *testing.T) {
	for _, failure := range []struct {
		name string
		set  func(*runHarness)
	}{
		{name: "MCP stop", set: func(h *runHarness) { h.tool.stopErr = errors.New("injected MCP stop failure") }},
		{name: "config cleanup", set: func(h *runHarness) { h.configs.cleanupErr = errors.New("injected config cleanup failure") }},
	} {
		t.Run(failure.name, func(t *testing.T) {
			h := newRunHarness(t)
			record, err := h.manager.Start(context.Background(), "n_topic")
			if err != nil {
				t.Fatal(err)
			}
			failure.set(h)
			h.driver.spec.EventSink(valueobject.AgentEvent{Type: valueobject.EventStatus, State: &valueobject.AgentState{Status: valueobject.SessionIdle}})
			waitFor(t, func() bool { return !h.manager.IsBusy() })
			stored, err := h.store.Load(context.Background(), record.ID)
			if err != nil || stored.Status != valueobject.RunFailed || stored.Error != "RUN_CLEANUP_FAILED" {
				t.Fatalf("terminal run=%+v err=%v, want RUN_CLEANUP_FAILED", stored.RunSummary, err)
			}
			if !h.workflow.released || !h.tool.stopped || h.configs.cleanupCount != 1 {
				t.Fatalf("cleanup failure leaked resources: lock=%v tool=%v cleanupCount=%d", h.workflow.released, h.tool.stopped, h.configs.cleanupCount)
			}
		})
	}
}

func TestTerminalPersistenceFailureRetriesAndReleasesLocks(t *testing.T) {
	h := newRunHarness(t)
	record, err := h.manager.Start(context.Background(), "n_topic")
	if err != nil {
		t.Fatal(err)
	}
	h.store.mu.Lock()
	h.store.failSaveOn = h.store.saveCount + 1
	h.store.saveErrOnCall = errors.New("injected terminal persistence failure")
	h.store.mu.Unlock()
	h.driver.spec.EventSink(valueobject.AgentEvent{Type: valueobject.EventStatus, State: &valueobject.AgentState{Status: valueobject.SessionIdle}})
	waitFor(t, func() bool { return !h.manager.IsBusy() })
	stored, err := h.store.Load(context.Background(), record.ID)
	if err != nil || stored.Status != valueobject.RunFailed || stored.Error != "RUN_STORE_FAILED" {
		t.Fatalf("terminal run=%+v err=%v, want RUN_STORE_FAILED", stored.RunSummary, err)
	}
	if !h.workflow.released || !h.configs.cleaned || !h.tool.stopped {
		t.Fatalf("persistence failure leaked resources: lock=%v config=%v tool=%v", h.workflow.released, h.configs.cleaned, h.tool.stopped)
	}
}

func TestWatchdogAndInterruptReleaseRunResources(t *testing.T) {
	t.Run("watchdog", func(t *testing.T) {
		h := newRunHarness(t)
		h.manager.opts.RunTimeout = 40 * time.Millisecond
		record, err := h.manager.Start(context.Background(), "n_topic")
		if err != nil {
			t.Fatal(err)
		}
		waitFor(t, func() bool { return !h.manager.IsBusy() })
		stored, err := h.store.Load(context.Background(), record.ID)
		if err != nil || stored.Status != valueobject.RunFailed || stored.Error != "RUN_TIMEOUT" {
			t.Fatalf("timed out run = %+v, err=%v", stored.RunSummary, err)
		}
		if !h.workflow.released || !h.configs.cleaned || !h.tool.stopped {
			t.Fatal("watchdog did not clean up run resources")
		}
	})
	t.Run("interrupt", func(t *testing.T) {
		h := newRunHarness(t)
		record, err := h.manager.Start(context.Background(), "n_topic")
		if err != nil {
			t.Fatal(err)
		}
		h.session.interruptFn = func() {
			h.driver.spec.EventSink(valueobject.AgentEvent{
				Type: valueobject.EventStatus, State: &valueobject.AgentState{Status: valueobject.SessionIdle},
			})
		}
		if err := h.manager.Interrupt(context.Background(), record.ID); err != nil {
			t.Fatal(err)
		}
		stored, err := h.store.Load(context.Background(), record.ID)
		if err != nil || stored.Status != valueobject.RunInterrupted || h.manager.IsBusy() {
			t.Fatalf("interrupted run = %+v, busy=%v, err=%v", stored.RunSummary, h.manager.IsBusy(), err)
		}
	})
}

func TestWatchdogIsInactivityBasedAndResetsOnProgress(t *testing.T) {
	h := newRunHarness(t)
	timer := &fakeRunTimer{}
	h.manager.opts.RunTimeout = DefaultRunTimeout
	h.manager.opts.NewTimer = func(delay time.Duration, callback func()) RunTimer {
		timer.mu.Lock()
		timer.delay = delay
		timer.callback = callback
		timer.mu.Unlock()
		return timer
	}
	record, err := h.manager.Start(context.Background(), "n_topic")
	if err != nil {
		t.Fatal(err)
	}
	if timer.delay != 60*time.Minute {
		t.Fatalf("watchdog delay = %s, want 60 minutes", timer.delay)
	}
	h.driver.spec.EventSink(valueobject.AgentEvent{
		Type:    valueobject.EventMessage,
		Message: &valueobject.TranscriptItem{ID: "progress", Kind: valueobject.TranscriptMessage, Text: "working", Streaming: true},
	})
	if got := timer.resetCount(); got != 1 {
		t.Fatalf("timer resets after stream progress = %d, want 1", got)
	}
	if err := h.manager.WithActiveRun(context.Background(), h.toolFactory.scope, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if got := timer.resetCount(); got != 2 {
		t.Fatalf("timer resets after MCP activity = %d, want 2", got)
	}
	timer.fire()
	stored, err := h.store.Load(context.Background(), record.ID)
	if err != nil || stored.Status != valueobject.RunFailed || stored.Error != "RUN_TIMEOUT" {
		t.Fatalf("timed out run = %+v, err=%v", stored.RunSummary, err)
	}
}

func TestCloseCancelsQuestionBeforeWaitingForMCPLease(t *testing.T) {
	h := newRunHarness(t)
	record, err := h.manager.Start(context.Background(), "n_topic")
	if err != nil {
		t.Fatal(err)
	}
	h.permissions.askStarted = make(chan struct{})
	h.permissions.askRelease = make(chan struct{})
	questionDone := make(chan error, 1)
	go func() {
		questionDone <- h.manager.WithActiveRun(context.Background(), h.toolFactory.scope, func() error {
			_, err := h.permissions.Ask(context.Background(), valueobject.QuestionRequest{Question: "continue?"})
			return err
		})
	}()
	<-h.permissions.askStarted
	closeDone := make(chan struct{})
	go func() { h.manager.Close(); close(closeDone) }()
	select {
	case <-closeDone:
	case <-time.After(time.Second):
		t.Fatal("Close waited on an ask_user request before closing its broker")
	}
	if err := <-questionDone; err != nil {
		t.Fatal(err)
	}
	stored, err := h.store.Load(context.Background(), record.ID)
	if err != nil || stored.Status != valueobject.RunInterrupted {
		t.Fatalf("shutdown run = %+v, err=%v", stored.RunSummary, err)
	}
}

func TestTranscriptRemainsBounded(t *testing.T) {
	transcript := []valueobject.TranscriptItem{}
	for i := 0; i < valueobject.MaxRunTranscriptItems+1; i++ {
		applyMessage(&transcript, valueobject.TranscriptItem{ID: fmt.Sprintf("m-%d", i), Text: "x"})
	}
	if len(transcript) != valueobject.MaxRunTranscriptItems || transcript[0].ID != "m-1" {
		t.Fatalf("transcript bounds not enforced: len=%d first=%q", len(transcript), transcript[0].ID)
	}
}

func TestRunTerminationWaitsForActiveMCPLease(t *testing.T) {
	h := newRunHarness(t)
	record, err := h.manager.Start(context.Background(), "n_topic")
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	leave := make(chan struct{})
	operationDone := make(chan error, 1)
	go func() {
		operationDone <- h.manager.WithActiveRun(context.Background(), h.toolFactory.scope, func() error {
			close(entered)
			<-leave
			return nil
		})
	}()
	<-entered
	h.driver.spec.EventSink(valueobject.AgentEvent{Type: valueobject.EventStatus, State: &valueobject.AgentState{Status: valueobject.SessionIdle}})
	select {
	case <-h.tool.stopStarted:
		t.Fatal("MCP server stopped before the active reply lease finished")
	case <-time.After(30 * time.Millisecond):
	}
	close(leave)
	if err := <-operationDone; err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return !h.manager.IsBusy() })
	stored, err := h.store.Load(context.Background(), record.ID)
	if err != nil || stored.Status != valueobject.RunSucceeded {
		t.Fatalf("run record=%+v err=%v", stored.RunSummary, err)
	}
}

func TestRecordRunPendingInsideMCPLeaseSurvivesTerminationStart(t *testing.T) {
	h := newRunHarness(t)
	record, err := h.manager.Start(context.Background(), "n_topic")
	if err != nil {
		t.Fatal(err)
	}
	pending := entity.Pending{ID: "pa_leased", RunID: record.ID, NodeID: "n_topic", Status: valueobject.PendingPending}
	entered := make(chan struct{})
	recordPending := make(chan struct{})
	operationDone := make(chan error, 1)
	go func() {
		operationDone <- h.manager.WithActiveRun(context.Background(), h.toolFactory.scope, func() error {
			close(entered)
			<-recordPending
			return h.manager.RecordRunPending(context.Background(), h.toolFactory.scope, pending)
		})
	}()
	<-entered
	state := h.manager.getActive(record.ID)
	finishDone := make(chan struct{})
	go func() {
		h.manager.finish(state, valueobject.RunSucceeded, "", nil)
		close(finishDone)
	}()
	waitFor(t, func() bool {
		state.leaseMu.Lock()
		defer state.leaseMu.Unlock()
		return state.closing
	})
	close(recordPending)
	if err := <-operationDone; err != nil {
		t.Fatalf("record pending under active reply lease: %v", err)
	}
	<-finishDone

	stored, err := h.store.Load(context.Background(), record.ID)
	if err != nil || stored.PendingID != pending.ID {
		t.Fatalf("terminal run pending id = %q err=%v", stored.PendingID, err)
	}
	pendingEvents := 0
	for _, event := range h.publisher.snapshot() {
		if event.Type == valueobject.EventPending && event.PendingID == pending.ID {
			pendingEvents++
		}
	}
	if pendingEvents != 1 {
		t.Fatalf("pending events = %d, want 1", pendingEvents)
	}
}

func TestRecoverInterruptedRunsAcrossProjectsAndKeepsHistory(t *testing.T) {
	started := testRecord("r_start", "p_one", valueobject.RunStarting)
	running := testRecord("r_running", "p_two", valueobject.RunRunning)
	succeeded := testRecord("r_done", "p_two", valueobject.RunSucceeded)
	one := &memoryRunStore{records: map[string]entity.RunRecord{started.ID: started}}
	two := &memoryRunStore{records: map[string]entity.RunRecord{running.ID: running, succeeded.ID: succeeded}}
	projects := &fakeProjects{project: testProject(), list: []entity.ProjectSummary{{ID: "p_one"}, {ID: "p_two"}}}
	provider := &fakeRunStores{stores: map[string]*memoryRunStore{"p_one": one, "p_two": two}}
	configs := &fakeRunConfigs{}
	manager := New(Options{Projects: projects, RunStores: provider, RunConfigs: configs})
	if err := manager.RecoverInterrupted(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		store *memoryRunStore
		id    string
		want  valueobject.RunStatus
	}{{one, started.ID, valueobject.RunInterrupted}, {two, running.ID, valueobject.RunInterrupted}, {two, succeeded.ID, valueobject.RunSucceeded}} {
		record, err := test.store.Load(context.Background(), test.id)
		if err != nil {
			t.Fatal(err)
		}
		if record.Status != test.want {
			t.Fatalf("run %s status=%s, want %s", test.id, record.Status, test.want)
		}
	}
	if len(configs.cleanedIDs) != 2 || !slices.Contains(configs.cleanedIDs, "r_start") || !slices.Contains(configs.cleanedIDs, "r_running") {
		t.Fatalf("interrupted MCP configs were not cleaned: %v", configs.cleanedIDs)
	}
}

type runHarness struct {
	manager     *Manager
	workflow    *fakeWorkflow
	projects    *fakeProjects
	store       *memoryRunStore
	configs     *fakeRunConfigs
	tool        *fakeTool
	toolFactory *fakeToolFactory
	driver      *fakeDriver
	session     *fakeAgentSession
	permissions *fakeRunPermissions
	publisher   *fakeRunPublisher
}

func newRunHarness(t *testing.T) *runHarness {
	t.Helper()
	project := testProject()
	workflow := &fakeWorkflow{project: project, path: []string{"n_topic"}}
	projects := &fakeProjects{project: project, config: valueobject.AppConfig{ContextTokenBudget: 100}}
	store := &memoryRunStore{records: make(map[string]entity.RunRecord)}
	configs := &fakeRunConfigs{root: t.TempDir()}
	tool := &fakeTool{token: "secret-test-token", stopStarted: make(chan struct{}, 1)}
	toolFactory := &fakeToolFactory{tool: tool}
	session := &fakeAgentSession{}
	driver := &fakeDriver{session: session}
	permissions := &fakeRunPermissions{}
	publisher := &fakeRunPublisher{}
	manager := New(Options{
		Projects: projects, Workflow: workflow, Contexts: fakeContextBuilder{},
		Workspaces: fakeWorkspaceProvider{}, RunStores: &fakeRunStores{stores: map[string]*memoryRunStore{"p_test": store}},
		RunConfigs: configs, ToolServers: toolFactory, Permissions: fakePermissionFactory{permissions: permissions},
		Driver: driver, Publisher: publisher, RunTimeout: time.Second,
	})
	return &runHarness{manager: manager, workflow: workflow, projects: projects, store: store, configs: configs, tool: tool, toolFactory: toolFactory, driver: driver, session: session, permissions: permissions, publisher: publisher}
}

func waitFor(t *testing.T, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition did not become true before timeout")
}

func testProject() entity.Project {
	return entity.Project{
		Version: valueobject.ProjectFileVersion, ID: "p_test", Name: "Test", Revision: 1,
		Nodes: []entity.Node{{ID: "n_topic", Type: valueobject.NodeTypeTopic, Title: "Topic", Params: entity.NodeParams{Idea: "idea"}, Rounds: []entity.Round{}}},
	}
}

func testRecord(id, projectID string, status valueobject.RunStatus) entity.RunRecord {
	return entity.RunRecord{RunSummary: entity.RunSummary{
		ID: id, TreeID: "n_topic", NodeID: "n_topic", ChainID: "c_n_topic", ChainPath: []string{"n_topic"},
		Status: status, ContextVersion: 1, StartedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, Transcript: []valueobject.TranscriptItem{}}
}

type fakeProjects struct {
	project entity.Project
	config  valueobject.AppConfig
	list    []entity.ProjectSummary
}

func (p *fakeProjects) Current(context.Context) (entity.Project, error) {
	return p.project.Clone(), nil
}
func (p *fakeProjects) Config(context.Context) (valueobject.AppConfig, error) { return p.config, nil }
func (p *fakeProjects) List(context.Context) ([]entity.ProjectSummary, error) {
	return append([]entity.ProjectSummary(nil), p.list...), nil
}

type fakeWorkflow struct {
	project  entity.Project
	path     []string
	beginErr error
	mu       sync.Mutex
	locked   bool
	released bool
}

func (w *fakeWorkflow) BeginRun(ctx context.Context, runID, nodeID string, expectedProjectID ...string) (entity.Project, entity.RunExecutionSnapshot, error) {
	return w.BeginRunWithOptions(ctx, runID, nodeID, entity.RunStartOptions{}, expectedProjectID...)
}

func (w *fakeWorkflow) BeginRunWithOptions(_ context.Context, _ string, nodeID string, options entity.RunStartOptions, expectedProjectID ...string) (entity.Project, entity.RunExecutionSnapshot, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(expectedProjectID) > 0 && expectedProjectID[0] != "" && expectedProjectID[0] != w.project.ID {
		return entity.Project{}, entity.RunExecutionSnapshot{}, fmt.Errorf("PROJECT_CHANGED: active %s, expected %s", w.project.ID, expectedProjectID[0])
	}
	if w.beginErr != nil {
		return entity.Project{}, entity.RunExecutionSnapshot{}, w.beginErr
	}
	graph, err := entity.NewGraph(&w.project)
	if err != nil {
		return entity.Project{}, entity.RunExecutionSnapshot{}, err
	}
	snapshot, err := graph.SnapshotForRunWithOptions(nodeID, options)
	if err != nil {
		return entity.Project{}, entity.RunExecutionSnapshot{}, err
	}
	w.locked = true
	return w.project.Clone(), snapshot, nil
}

func (w *fakeWorkflow) BeginRunWithInputs(ctx context.Context, runID, nodeID string, inputEdgeIDs []string, expectedProjectID ...string) (entity.Project, entity.RunExecutionSnapshot, error) {
	return w.BeginRunWithOptions(ctx, runID, nodeID, entity.RunStartOptions{InputEdgeIDs: inputEdgeIDs}, expectedProjectID...)
}
func (w *fakeWorkflow) ReleaseRunLock(context.Context, string) error {
	w.mu.Lock()
	w.released = true
	w.locked = false
	w.mu.Unlock()
	return nil
}

type fakeContextBuilder struct{}

func (fakeContextBuilder) BuildRunContext(_ context.Context, project entity.Project, nodeID string, snapshot entity.RunExecutionSnapshot, workspace port.RunWorkspace, budget int) (port.RunContext, error) {
	scope := workspace.Scope()
	return port.RunContext{
		TreeID: scope.TreeID, NodeID: nodeID, ChainPath: scope.ChainPath, InputNodeIDs: snapshot.ScopeNodeIDs(),
		OutputKind: snapshot.OutputContract.Kind, ContextVersion: 1, InitialPrompt: "initial context",
		Hashes:   valueobject.ContextHashes{ContextHash: "context", AncestorHash: "ancestor", ParamsHash: "params"},
		Estimate: valueobject.ContextEstimate{EstimatorVersion: 1, EstimatedTokens: 40, TokenBudget: budget},
	}, nil
}

type fakeWorkspaceProvider struct {
	scopeTransform func(port.WorkspaceScope) port.WorkspaceScope
}

func (p fakeWorkspaceProvider) OpenRun(_ context.Context, projectID string, snapshot entity.RunExecutionSnapshot) (port.RunWorkspace, error) {
	scope := port.WorkspaceScope{
		ProjectID: projectID, TreeID: snapshot.TreeID, NodeID: snapshot.NodeID,
		ChainPath: append([]string(nil), snapshot.ChainPath...), InputNodeIDs: snapshot.ScopeNodeIDs(),
	}
	if p.scopeTransform != nil {
		scope = p.scopeTransform(scope)
	}
	return &fakeRunWorkspace{scope: scope}, nil
}

type fakeRunWorkspace struct{ scope port.WorkspaceScope }

func (w *fakeRunWorkspace) Scope() port.WorkspaceScope         { return w.scope }
func (*fakeRunWorkspace) CWD() string                          { return `C:\workspace\trees\n_topic` }
func (*fakeRunWorkspace) RuntimeReadPath() string              { return `..\..\..\..\..\runtime` }
func (*fakeRunWorkspace) InputSources() []port.WorkspaceSource { return nil }
func (*fakeRunWorkspace) Resolver() port.ScopedPathResolver    { return fakePathResolver{} }
func (w *fakeRunWorkspace) ReadFile(context.Context, string, int64) (port.WorkspaceRead, error) {
	return port.WorkspaceRead{}, nil
}
func (w *fakeRunWorkspace) ListDir(context.Context, string, int) (port.WorkspaceList, error) {
	return port.WorkspaceList{}, nil
}
func (w *fakeRunWorkspace) Grep(context.Context, string, string, port.WorkspaceGrepOptions) (port.WorkspaceGrep, error) {
	return port.WorkspaceGrep{}, nil
}
func (w *fakeRunWorkspace) Stat(context.Context, string) (port.WorkspaceStat, error) {
	return port.WorkspaceStat{}, nil
}

type fakePathResolver struct{}

func (fakePathResolver) Resolve(path string) (string, error)      { return path, nil }
func (fakePathResolver) ResolveRead(path string) (string, error)  { return path, nil }
func (fakePathResolver) ResolveWrite(path string) (string, error) { return path, nil }

type fakeRunStores struct{ stores map[string]*memoryRunStore }

func (p *fakeRunStores) ForProject(_ context.Context, projectID string) (port.RunStore, error) {
	store := p.stores[projectID]
	if store == nil {
		return nil, fmt.Errorf("no store for %s", projectID)
	}
	return store, nil
}

type memoryRunStore struct {
	mu            sync.Mutex
	records       map[string]entity.RunRecord
	saveErr       error
	saveCount     int
	failSaveOn    int
	saveErrOnCall error
}

func (s *memoryRunStore) List(context.Context) ([]entity.RunSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]entity.RunSummary, 0, len(s.records))
	for _, record := range s.records {
		result = append(result, record.RunSummary)
	}
	return result, nil
}
func (s *memoryRunStore) Load(_ context.Context, id string) (entity.RunRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[id]
	if !ok {
		return entity.RunRecord{}, ErrRunNotFound
	}
	return cloneRecord(record), nil
}
func (s *memoryRunStore) Save(_ context.Context, record entity.RunRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveCount++
	if s.saveErr != nil {
		return s.saveErr
	}
	if s.saveCount == s.failSaveOn {
		return s.saveErrOnCall
	}
	s.records[record.ID] = cloneRecord(record)
	return nil
}

type fakeRunConfigs struct {
	root         string
	mu           sync.Mutex
	cleaned      bool
	cleanedIDs   []string
	cleanupCount int
	prepareErr   error
	cleanupErr   error
}

func (c *fakeRunConfigs) Prepare(_ context.Context, projectID, runID string) (string, error) {
	dir := filepath.Join(c.root, projectID, runID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if c.prepareErr != nil {
		return "", c.prepareErr
	}
	return filepath.Join(dir, "mcp.json"), nil
}
func (c *fakeRunConfigs) Cleanup(_ context.Context, projectID, runID string) error {
	c.mu.Lock()
	c.cleanupCount++
	c.cleanedIDs = append(c.cleanedIDs, runID)
	cleanupErr := c.cleanupErr
	c.mu.Unlock()
	if cleanupErr != nil {
		return cleanupErr
	}
	if c.root != "" {
		if err := os.RemoveAll(filepath.Join(c.root, projectID, runID)); err != nil {
			return err
		}
	}
	c.mu.Lock()
	c.cleaned = true
	c.mu.Unlock()
	return nil
}

type fakeToolFactory struct {
	tool  *fakeTool
	scope port.RunScope
}

func (f *fakeToolFactory) New(string, string, port.QuestionGate, func(string)) (port.ToolServer, error) {
	return f.tool, nil
}
func (f *fakeToolFactory) NewForRun(scope port.RunScope, _, _ string, _ port.QuestionGate, _ func(string), _ port.WorkspaceReader, _ port.RunLifecycle) (port.ToolServer, error) {
	f.scope = scope
	return f.tool, nil
}

type fakeTool struct {
	token          string
	started        bool
	stopped        bool
	startErr       error
	stopErr        error
	writeConfigErr error
	stopStarted    chan struct{}
}

func (t *fakeTool) Start(context.Context) error {
	t.started = true
	return t.startErr
}
func (t *fakeTool) Stop(context.Context) error {
	t.stopped = true
	select {
	case t.stopStarted <- struct{}{}:
	default:
	}
	return t.stopErr
}
func (*fakeTool) URL() string     { return "http://127.0.0.1:1/mcp" }
func (t *fakeTool) Token() string { return t.token }
func (t *fakeTool) WriteConfig(path string) error {
	if t.writeConfigErr != nil {
		return t.writeConfigErr
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(`{"mcpServers":{"audioui":{"type":"http","url":"http://127.0.0.1:1/mcp"}}}`), 0o600)
}

type fakePermissionFactory struct{ permissions *fakeRunPermissions }

func (f fakePermissionFactory) NewRunPermissions(_ port.RunScope, _ port.ScopedPathResolver, _ func(valueobject.AgentEvent)) (port.RunPermissionController, error) {
	return f.permissions, nil
}

type fakeRunPermissions struct {
	resolved   func(string, string)
	askStarted chan struct{}
	askRelease chan struct{}
	closeOnce  sync.Once
}

func (*fakeRunPermissions) Request(context.Context, valueobject.PermissionRequest) (valueobject.PermissionDecision, error) {
	return valueobject.DecisionDeny, nil
}
func (p *fakeRunPermissions) Ask(context.Context, valueobject.QuestionRequest) (valueobject.QuestionAnswer, error) {
	if p.askStarted != nil {
		close(p.askStarted)
		<-p.askRelease
	}
	return valueobject.QuestionAnswer{}, nil
}
func (*fakeRunPermissions) Check(context.Context, string, map[string]any) port.PolicyResult {
	return port.PolicyResult{Decision: port.PolicyAllow}
}
func (*fakeRunPermissions) ResolvePermission(string, valueobject.PermissionDecision) error {
	return nil
}
func (*fakeRunPermissions) ResolveQuestion(string, valueobject.QuestionAnswer) error { return nil }
func (p *fakeRunPermissions) SetResolvedCallback(callback func(string, string)) {
	p.resolved = callback
}
func (p *fakeRunPermissions) Close() {
	if p.askRelease != nil {
		p.closeOnce.Do(func() { close(p.askRelease) })
	}
}

type fakeDriver struct {
	spec        port.DriverSpec
	session     *fakeAgentSession
	startEvents []valueobject.AgentEvent
	startErr    error
}

func (d *fakeDriver) Start(_ context.Context, spec port.DriverSpec) (port.AgentSession, error) {
	d.spec = spec
	if _, err := os.Stat(spec.MCPConfigPath); err != nil {
		return nil, fmt.Errorf("MCP_CONFIG_NOT_FOUND: %w", err)
	}
	if d.startErr != nil {
		return nil, d.startErr
	}
	for _, event := range d.startEvents {
		spec.EventSink(event)
	}
	return d.session, nil
}
func (*fakeDriver) Resume(context.Context, port.DriverSpec, json.RawMessage) (port.AgentSession, error) {
	return nil, errors.New("resume is not used")
}

type fakeAgentSession struct {
	sendErr     error
	closed      bool
	interruptFn func()
}

func (s *fakeAgentSession) Send(context.Context, string) error { return s.sendErr }
func (s *fakeAgentSession) Interrupt(context.Context) error {
	if s.interruptFn != nil {
		s.interruptFn()
	}
	return nil
}
func (s *fakeAgentSession) Close() error { s.closed = true; return nil }

type fakeRunPublisher struct {
	mu     sync.Mutex
	events []valueobject.RunEvent
}

type resolvingRunPublisher struct {
	manager *Manager
	result  chan error
	once    sync.Once
}

func (p *resolvingRunPublisher) PublishRun(event valueobject.RunEvent) {
	if event.Type != valueobject.EventPermission || event.Permission == nil {
		return
	}
	p.once.Do(func() {
		p.result <- p.manager.RespondPermission(event.RunID, event.Permission.ID, valueobject.DecisionAllow)
	})
}

func (p *fakeRunPublisher) snapshot() []valueobject.RunEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]valueobject.RunEvent(nil), p.events...)
}

type fakeRunTimer struct {
	mu       sync.Mutex
	delay    time.Duration
	callback func()
	resets   int
	stopped  bool
}

func (t *fakeRunTimer) Stop() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	wasActive := !t.stopped
	t.stopped = true
	return wasActive
}

func (t *fakeRunTimer) Reset(delay time.Duration) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	wasActive := !t.stopped
	t.delay = delay
	t.resets++
	t.stopped = false
	return wasActive
}

func (t *fakeRunTimer) resetCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.resets
}

func (t *fakeRunTimer) fire() {
	t.mu.Lock()
	callback := t.callback
	t.stopped = true
	t.mu.Unlock()
	if callback != nil {
		callback()
	}
}

func (p *fakeRunPublisher) PublishRun(event valueobject.RunEvent) {
	p.mu.Lock()
	p.events = append(p.events, event)
	p.mu.Unlock()
}

func cloneRecord(record entity.RunRecord) entity.RunRecord {
	clone := record
	clone.ChainPath = append([]string(nil), record.ChainPath...)
	clone.Transcript = append([]valueobject.TranscriptItem(nil), record.Transcript...)
	return clone
}
