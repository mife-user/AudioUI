package run

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
)

type runPreparation struct {
	store      port.RunStore
	workspace  port.RunWorkspace
	runContext port.RunContext
	scope      port.RunScope
	record     entity.RunRecord
}

func (m *Manager) prepareRun(ctx context.Context, runID, nodeID string, project entity.Project, snapshot entity.RunExecutionSnapshot) (runPreparation, error) {
	snapshot = snapshot.Clone()
	if snapshot.ProjectID != project.ID || snapshot.NodeID != nodeID {
		return runPreparation{}, errors.New("RUN_SNAPSHOT_SCOPE_INVALID")
	}
	if err := snapshot.Validate(); err != nil {
		return runPreparation{}, fmt.Errorf("RUN_SNAPSHOT_INVALID: %w", err)
	}
	chainPath := snapshot.ChainPath
	if snapshot.Version < 3 && len(chainPath) < 1 {
		return runPreparation{}, errors.New("NOT_IN_CHAIN: empty run path")
	}
	workspace, err := m.opts.Workspaces.OpenRun(ctx, project.ID, snapshot)
	if err != nil {
		return runPreparation{}, err
	}
	if !workspaceScopeMatchesSnapshot(workspace.Scope(), project.ID, snapshot) {
		return runPreparation{}, errors.New("RUN_WORKSPACE_SCOPE_INVALID")
	}
	config, err := m.opts.Projects.Config(ctx)
	if err != nil {
		return runPreparation{}, err
	}
	budget := config.ContextTokenBudget
	if budget == 0 {
		budget = valueobject.DefaultContextTokenBudget
	}
	runContext, err := m.opts.Contexts.BuildRunContext(ctx, project, nodeID, snapshot, workspace, budget)
	if err != nil {
		return runPreparation{}, err
	}
	if runContext.TreeID != snapshot.TreeID || runContext.NodeID != snapshot.NodeID || !slices.Equal(runContext.ChainPath, chainPath) ||
		snapshot.Version >= 3 && !slices.Equal(runContext.InputNodeIDs, snapshot.ScopeNodeIDs()) ||
		runContext.Hashes.ContextHash == "" || runContext.Hashes.AncestorHash == "" || runContext.Hashes.ParamsHash == "" {
		return runPreparation{}, errors.New("RUN_CONTEXT_SCOPE_INVALID")
	}
	store, err := m.opts.RunStores.ForProject(ctx, project.ID)
	if err != nil {
		return runPreparation{}, err
	}
	now := m.now().UTC()
	preview, truncated := contextPreview(runContext.InitialPrompt)
	scope := port.RunScope{
		RunID: runID, ProjectID: project.ID, TreeID: snapshot.TreeID, NodeID: snapshot.NodeID,
		ChainID: "c_" + snapshot.NodeID, ChainPath: append([]string(nil), chainPath...),
		InputNodeIDs: append([]string(nil), snapshot.ScopeNodeIDs()...),
		ContextHash:  runContext.Hashes.ContextHash, AncestorHash: runContext.Hashes.AncestorHash,
		ParamsHash: runContext.Hashes.ParamsHash, ExecutionSnapshot: snapshot.Clone(),
	}
	record := entity.RunRecord{
		RunSummary: entity.RunSummary{
			ID: runID, TreeID: snapshot.TreeID, NodeID: snapshot.NodeID, ChainID: scope.ChainID,
			ChainPath: append([]string(nil), chainPath...), Status: valueobject.RunStarting,
			ContextVersion: runContext.ContextVersion, EstimatorVersion: runContext.Estimate.EstimatorVersion,
			ContextHash: runContext.Hashes.ContextHash, EstimatedTokens: runContext.Estimate.EstimatedTokens,
			TokenBudget: runContext.Estimate.TokenBudget, StartedAt: now.Format(time.RFC3339Nano),
		},
		ContextPreview: preview, ContextPreviewTruncated: truncated,
		ExecutionSnapshot: &snapshot,
		Transcript:        []valueobject.TranscriptItem{},
	}
	if err := store.Save(ctx, record); err != nil {
		return runPreparation{}, err
	}
	return runPreparation{store: store, workspace: workspace, runContext: runContext, scope: scope, record: record}, nil
}

func workspaceScopeMatchesSnapshot(scope port.WorkspaceScope, projectID string, snapshot entity.RunExecutionSnapshot) bool {
	return scope.ProjectID == projectID && scope.TreeID == snapshot.TreeID && scope.NodeID == snapshot.NodeID &&
		slices.Equal(scope.ChainPath, snapshot.ChainPath) && slices.Equal(scope.InputNodeIDs, snapshot.ScopeNodeIDs())
}

func (m *Manager) startPreparedRun(ctx context.Context, state *runState, driver port.AgentDriver, prepared runPreparation) error {
	state.setupMu.Lock()
	setupHeld := true
	defer func() {
		if setupHeld {
			state.setupMu.Unlock()
		}
	}()
	failStartup := func(startErr error) error {
		if setupHeld {
			state.setupMu.Unlock()
			setupHeld = false
		}
		m.finish(state, valueobject.RunFailed, errorCode(startErr), nil)
		return startErr
	}
	if err := state.runContext.Err(); err != nil {
		return failStartup(err)
	}

	setupCtx, setupCancel := context.WithTimeout(ctx, m.opts.RunTimeout)
	stopSetupOnRunEnd := context.AfterFunc(state.runContext, setupCancel)
	defer func() {
		stopSetupOnRunEnd()
		setupCancel()
	}()
	state.mu.Lock()
	state.configReady = true
	state.mu.Unlock()
	mcpConfigPath, err := m.opts.RunConfigs.Prepare(setupCtx, state.scope.ProjectID, state.scope.RunID)
	if err != nil {
		return failStartup(err)
	}
	spec := RunSpec{
		Scope: state.scope.Clone(), Workspace: prepared.workspace, Context: prepared.runContext, MCPConfigPath: mcpConfigPath,
		SystemPrompt: runSystemPrompt,
		BuiltinTools: append([]string(nil), runBuiltinTools...),
		AllowedTools: append([]string(nil), runAllowedTools...),
	}
	state.mu.Lock()
	state.spec = spec
	state.mu.Unlock()

	permissions, err := m.opts.Permissions.NewRunPermissions(state.scope.Clone(), prepared.workspace.Resolver(), func(event valueobject.AgentEvent) {
		m.onAgentEvent(state, event)
	})
	if err != nil {
		return failStartup(err)
	}
	state.mu.Lock()
	state.permissions = permissions
	state.mu.Unlock()
	permissions.SetResolvedCallback(func(kind, id string) {
		if !state.acquireLease() {
			return
		}
		defer state.releaseLease()
		m.resolveRequest(state, kind, id)
	})

	tool, err := m.opts.ToolServers.NewForRun(state.scope.Clone(), prepared.workspace.CWD(), state.scope.RunID, permissions, func(message string) {
		m.onReply(state, message)
	}, prepared.workspace, m)
	if err != nil {
		return failStartup(err)
	}
	state.mu.Lock()
	state.tool = tool
	state.mu.Unlock()
	if err := tool.Start(setupCtx); err != nil {
		return failStartup(err)
	}
	if err := tool.WriteConfig(mcpConfigPath); err != nil {
		return failStartup(err)
	}

	driverSpec := port.DriverSpec{
		ChainID: state.scope.ChainID, Workspace: prepared.workspace.CWD(), SystemPrompt: spec.SystemPrompt,
		BuiltinTools: append([]string(nil), spec.BuiltinTools...), AllowedTools: append([]string(nil), spec.AllowedTools...),
		MCPConfigPath: mcpConfigPath, MCPURL: tool.URL(), MCPToken: tool.Token(),
		PermissionGate: permissions, QuestionGate: permissions, ToolPolicy: permissions,
		EventSink: func(event valueobject.AgentEvent) { m.onAgentEvent(state, event) },
	}
	session, err := driver.Start(setupCtx, driverSpec)
	if err != nil {
		return failStartup(err)
	}
	if err := state.runContext.Err(); err != nil {
		_ = session.Close()
		return failStartup(err)
	}
	state.mu.Lock()
	state.session = session
	state.mu.Unlock()

	m.emitWith(state, valueobject.RunEvent{Type: valueobject.EventStatus, Status: valueobject.RunRunning}, func() {
		state.record.Status = valueobject.RunRunning
	})
	if err := m.persist(state); err != nil {
		return failStartup(fmt.Errorf("RUN_STORE_FAILED: %w", err))
	}
	state.mu.Lock()
	state.timer = m.opts.NewTimer(m.opts.RunTimeout, func() {
		m.finish(state, valueobject.RunFailed, "RUN_TIMEOUT", nil)
	})
	state.mu.Unlock()
	if err := session.Send(state.runContext, prepared.runContext.InitialPrompt); err != nil {
		return failStartup(err)
	}
	return nil
}
