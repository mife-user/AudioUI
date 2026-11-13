package handler

import (
	"context"
	"errors"
	"slices"
	"testing"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/port"
	contextservice "audio-ui/internal/service/context"
	runservice "audio-ui/internal/service/run"
	"audio-ui/internal/service/workflow"
)

func TestRunStartErrorMapsKnownCodes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code string
	}{
		{name: "agent unavailable", err: runservice.ErrAgentUnavailable, code: "AGENT_UNAVAILABLE"},
		{name: "busy", err: runservice.ErrBusy, code: "BUSY"},
		{name: "content locked", err: workflow.ErrNodeLocked, code: "NODE_LOCKED"},
		{name: "run path locked", err: workflow.ErrRunningChainLocked, code: "RUNNING_CHAIN_LOCKED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			failure := runStartError(test.err)
			if failure == nil || failure.Code != test.code {
				t.Fatalf("failure = %+v, want code %s", failure, test.code)
			}
		})
	}
}

func TestRunStartErrorCarriesContextEstimateAndBudget(t *testing.T) {
	failure := runStartError(&contextservice.ContextTooLargeError{EstimatedTokens: 1_200_001, TokenBudget: 1_000_000})
	if failure == nil || failure.Code != "CONTEXT_TOO_LARGE" {
		t.Fatalf("failure = %+v, want CONTEXT_TOO_LARGE", failure)
	}
	if failure.EstimatedTokens == nil || *failure.EstimatedTokens != 1_200_001 {
		t.Fatalf("estimated tokens = %v", failure.EstimatedTokens)
	}
	if failure.TokenBudget == nil || *failure.TokenBudget != 1_000_000 {
		t.Fatalf("token budget = %v", failure.TokenBudget)
	}
}

func TestRunStartRequiresNodeAndProjectScope(t *testing.T) {
	service := NewRunService(nil)
	for _, test := range []struct {
		name      string
		nodeID    string
		projectID string
	}{
		{name: "missing node", projectID: "p_test"},
		{name: "missing project", nodeID: "n_topic"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := service.Start(test.nodeID, test.projectID, entity.RunStartOptions{})
			if result.Run != nil || result.Error == nil || result.Error.Code != "INVALID_INPUT" {
				t.Fatalf("start result = %+v, want structured INVALID_INPUT", result)
			}
		})
	}
}

func TestRunStartErrorFallsBackToStableFailureCode(t *testing.T) {
	failure := runStartError(errors.New("unexpected failure"))
	if failure == nil || failure.Code != "RUN_FAILED" || failure.Message != "unexpected failure" {
		t.Fatalf("failure = %+v", failure)
	}
}

func TestRunStartWithInputsPassesSelectedEdgeOrderAndProjectScope(t *testing.T) {
	workflow := &inputCapturingWorkflow{err: errors.New("CAPTURED_RUN_START")}
	manager := runservice.New(runservice.Options{
		Projects: runProjectServiceStub{}, Workflow: workflow, Contexts: runContextBuilderStub{},
		Workspaces: runWorkspaceProviderStub{}, RunStores: runStoreProviderStub{}, RunConfigs: runConfigPathsStub{},
		ToolServers: runToolServerFactoryStub{}, Permissions: runPermissionFactoryStub{}, Driver: runAgentDriverStub{},
	})
	result := NewRunService(manager).Start("n_agent", " p_test ", entity.RunStartOptions{InputEdgeIDs: []string{"e_second", "e_first"}})
	if result.Run != nil || result.Error == nil || result.Error.Message != "CAPTURED_RUN_START" {
		t.Fatalf("run start result = %+v", result)
	}
	if !slices.Equal(workflow.inputEdgeIDs, []string{"e_second", "e_first"}) || !slices.Equal(workflow.projectIDs, []string{"p_test"}) {
		t.Fatalf("workflow input edges=%v project IDs=%v", workflow.inputEdgeIDs, workflow.projectIDs)
	}
}

type inputCapturingWorkflow struct {
	runservice.WorkflowService
	inputEdgeIDs []string
	projectIDs   []string
	err          error
}

func (w *inputCapturingWorkflow) BeginRunWithOptions(_ context.Context, _, _ string, options entity.RunStartOptions, projectIDs ...string) (entity.Project, entity.RunExecutionSnapshot, error) {
	w.inputEdgeIDs = append([]string(nil), options.InputEdgeIDs...)
	w.projectIDs = append([]string(nil), projectIDs...)
	return entity.Project{}, entity.RunExecutionSnapshot{}, w.err
}

type runProjectServiceStub struct{ runservice.ProjectService }
type runContextBuilderStub struct{ port.RunContextBuilder }
type runWorkspaceProviderStub struct{ port.RunWorkspaceProvider }
type runStoreProviderStub struct{ port.RunStoreProvider }
type runConfigPathsStub struct{ port.RunConfigPaths }
type runToolServerFactoryStub struct{ port.RunToolServerFactory }
type runPermissionFactoryStub struct{ port.RunPermissionFactory }
type runAgentDriverStub struct{ port.AgentDriver }
