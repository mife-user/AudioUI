package run

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
)

const (
	// DefaultRunTimeout is the inactivity window before an active run times out.
	DefaultRunTimeout      = 60 * time.Minute
	contextPreviewMaxBytes = 16 * 1024
	statusCleanupTimeout   = 5 * time.Second
)

var (
	ErrAgentUnavailable = errors.New("AGENT_UNAVAILABLE")
	ErrRunNotFound      = errors.New("RUN_NOT_FOUND")
)

type ProjectService interface {
	Current(ctx context.Context) (entity.Project, error)
	Config(ctx context.Context) (valueobject.AppConfig, error)
	List(ctx context.Context) ([]entity.ProjectSummary, error)
}

type WorkflowService interface {
	BeginRun(ctx context.Context, runID, nodeID string) (entity.Project, []string, error)
	ReleaseRunLock(ctx context.Context, runID string) error
}

// RunTimer is injectable so inactivity timeout behavior can be tested without sleeping.
type RunTimer interface {
	Stop() bool
	Reset(time.Duration) bool
}

type RunSpec struct {
	Scope         port.RunScope
	Workspace     port.RunWorkspace
	Context       port.RunContext
	MCPConfigPath string
	SystemPrompt  string
	BuiltinTools  []string
	AllowedTools  []string
}

type Options struct {
	Projects    ProjectService
	Workflow    WorkflowService
	Contexts    port.RunContextBuilder
	Workspaces  port.RunWorkspaceProvider
	RunStores   port.RunStoreProvider
	RunConfigs  port.RunConfigPaths
	ToolServers port.RunToolServerFactory
	Permissions port.RunPermissionFactory
	Driver      port.AgentDriver
	Publisher   port.RunEventPublisher
	Gate        *TurnGate
	RunTimeout  time.Duration
	NewTimer    func(time.Duration, func()) RunTimer
	Now         func() time.Time
	Logf        func(format string, args ...any)
}

type Manager struct {
	opts   Options
	gate   *TurnGate
	now    func() time.Time
	mu     sync.Mutex
	active map[string]*runState
	closed bool
	seq    atomic.Uint64
}

type runState struct {
	setupMu      sync.Mutex
	mu           sync.Mutex
	persistMu    sync.Mutex
	leaseMu      sync.Mutex
	leaseCond    *sync.Cond
	leases       int
	closing      bool
	interrupting bool
	finishOnce   sync.Once
	seq          uint64
	scope        port.RunScope
	spec         RunSpec
	record       entity.RunRecord
	store        port.RunStore
	session      port.AgentSession
	tool         port.ToolServer
	permissions  port.RunPermissionController
	runContext   context.Context
	cancel       context.CancelFunc
	timer        RunTimer
	configReady  bool
	messageSeq   atomic.Uint64
}

func New(options Options) *Manager {
	if options.Gate == nil {
		options.Gate = NewTurnGate()
	}
	if options.RunTimeout <= 0 {
		options.RunTimeout = DefaultRunTimeout
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.NewTimer == nil {
		options.NewTimer = func(delay time.Duration, callback func()) RunTimer {
			return time.AfterFunc(delay, callback)
		}
	}
	if options.Logf == nil {
		options.Logf = func(string, ...any) {}
	}
	return &Manager{opts: options, gate: options.Gate, now: options.Now, active: make(map[string]*runState)}
}

func (m *Manager) Start(ctx context.Context, nodeID string) (entity.RunRecord, error) {
	if err := ctx.Err(); err != nil {
		return entity.RunRecord{}, err
	}
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed {
		return entity.RunRecord{}, ErrManagerClosed
	}
	if m.opts.Driver == nil {
		return entity.RunRecord{}, ErrAgentUnavailable
	}
	if err := m.validateDependencies(); err != nil {
		return entity.RunRecord{}, err
	}
	runID, err := valueobject.NewID(valueobject.RunIDPrefix)
	if err != nil {
		return entity.RunRecord{}, err
	}
	if err := m.gate.Acquire(runID); err != nil {
		return entity.RunRecord{}, err
	}
	lockAcquired := false
	managerOwnsGate := false
	defer func() {
		if managerOwnsGate {
			return
		}
		if lockAcquired {
			if err := m.opts.Workflow.ReleaseRunLock(context.Background(), runID); err != nil {
				m.opts.Logf("release run lock after startup failure: %v", err)
			}
		}
		_ = m.gate.Release(runID)
	}()

	project, chainPath, err := m.opts.Workflow.BeginRun(ctx, runID, nodeID)
	if err != nil {
		return entity.RunRecord{}, err
	}
	lockAcquired = true
	if len(chainPath) < 1 {
		return entity.RunRecord{}, errors.New("NOT_IN_CHAIN: empty run path")
	}
	treeID := chainPath[0]
	workspace, err := m.opts.Workspaces.OpenRun(ctx, project.ID, treeID, nodeID, chainPath)
	if err != nil {
		return entity.RunRecord{}, err
	}
	config, err := m.opts.Projects.Config(ctx)
	if err != nil {
		return entity.RunRecord{}, err
	}
	budget := config.ContextTokenBudget
	if budget == 0 {
		budget = valueobject.DefaultContextTokenBudget
	}
	runContext, err := m.opts.Contexts.BuildRunContext(ctx, project, nodeID, workspace, budget)
	if err != nil {
		return entity.RunRecord{}, err
	}
	if runContext.TreeID != treeID || runContext.NodeID != nodeID || !slices.Equal(runContext.ChainPath, chainPath) || runContext.Hashes.ContextHash == "" || runContext.Hashes.AncestorHash == "" || runContext.Hashes.ParamsHash == "" {
		return entity.RunRecord{}, errors.New("RUN_CONTEXT_SCOPE_INVALID")
	}
	store, err := m.opts.RunStores.ForProject(ctx, project.ID)
	if err != nil {
		return entity.RunRecord{}, err
	}
	now := m.now().UTC()
	preview, truncated := contextPreview(runContext.InitialPrompt)
	scope := port.RunScope{
		RunID: runID, ProjectID: project.ID, TreeID: treeID, NodeID: nodeID,
		ChainID: "c_" + nodeID, ChainPath: append([]string(nil), chainPath...),
		ContextHash: runContext.Hashes.ContextHash, AncestorHash: runContext.Hashes.AncestorHash,
		ParamsHash: runContext.Hashes.ParamsHash,
	}
	record := entity.RunRecord{
		RunSummary: entity.RunSummary{
			ID: runID, TreeID: treeID, NodeID: nodeID, ChainID: scope.ChainID,
			ChainPath: append([]string(nil), chainPath...), Status: valueobject.RunStarting,
			ContextVersion: runContext.ContextVersion, EstimatorVersion: runContext.Estimate.EstimatorVersion,
			ContextHash: runContext.Hashes.ContextHash, EstimatedTokens: runContext.Estimate.EstimatedTokens,
			TokenBudget: runContext.Estimate.TokenBudget, StartedAt: now.Format(time.RFC3339Nano),
		},
		ContextPreview: preview, ContextPreviewTruncated: truncated,
		Transcript: []valueobject.TranscriptItem{},
	}
	if err := store.Save(ctx, record); err != nil {
		return entity.RunRecord{}, err
	}
	runCtx, cancel := context.WithCancel(context.Background())
	state := &runState{
		scope: scope, store: store, record: record, runContext: runCtx, cancel: cancel,
	}
	state.leaseCond = sync.NewCond(&state.leaseMu)
	if err := m.register(state); err != nil {
		cancel()
		record.Status = valueobject.RunFailed
		record.Error = errorCode(err)
		record.FinishedAt = m.now().UTC().Format(time.RFC3339Nano)
		_ = store.Save(context.Background(), record)
		return record, err
	}
	managerOwnsGate = true
	m.emitStatus(state, valueobject.RunStarting, nil)
	state.setupMu.Lock()
	setupHeld := true
	defer func() {
		if setupHeld {
			state.setupMu.Unlock()
		}
	}()
	failStartup := func(startErr error) (entity.RunRecord, error) {
		if setupHeld {
			state.setupMu.Unlock()
			setupHeld = false
		}
		m.finish(state, valueobject.RunFailed, errorCode(startErr), nil)
		return state.snapshot(), startErr
	}
	if err := runCtx.Err(); err != nil {
		return failStartup(err)
	}

	setupCtx, setupCancel := context.WithTimeout(ctx, m.opts.RunTimeout)
	stopSetupOnRunEnd := context.AfterFunc(runCtx, setupCancel)
	defer func() {
		stopSetupOnRunEnd()
		setupCancel()
	}()
	mcpConfigPath, err := m.opts.RunConfigs.Prepare(setupCtx, project.ID, runID)
	if err != nil {
		_ = m.opts.RunConfigs.Cleanup(context.Background(), project.ID, runID)
		return failStartup(err)
	}
	state.mu.Lock()
	state.configReady = true
	state.mu.Unlock()
	spec := RunSpec{
		Scope: scope, Workspace: workspace, Context: runContext, MCPConfigPath: mcpConfigPath,
		SystemPrompt: runSystemPrompt,
		BuiltinTools: append([]string(nil), runBuiltinTools...),
		AllowedTools: append([]string(nil), runAllowedTools...),
	}
	state.mu.Lock()
	state.spec = spec
	state.mu.Unlock()

	permissions, err := m.opts.Permissions.NewRunPermissions(scope, workspace.Resolver(), func(event valueobject.AgentEvent) {
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
		m.emit(state, valueobject.RunEvent{Type: valueobject.EventRequestResolved, ResolvedKind: kind, ResolvedID: id})
	})

	tool, err := m.opts.ToolServers.NewForRun(scope, workspace.CWD(), runID, permissions, func(message string) {
		m.onReply(state, message)
	}, workspace, m)
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
		ChainID: scope.ChainID, Workspace: workspace.CWD(), SystemPrompt: spec.SystemPrompt,
		BuiltinTools: append([]string(nil), spec.BuiltinTools...), AllowedTools: append([]string(nil), spec.AllowedTools...),
		MCPConfigPath: mcpConfigPath, MCPURL: tool.URL(), MCPToken: tool.Token(),
		PermissionGate: permissions, QuestionGate: permissions, ToolPolicy: permissions,
		EventSink: func(event valueobject.AgentEvent) { m.onAgentEvent(state, event) },
	}
	session, err := m.opts.Driver.Start(setupCtx, driverSpec)
	if err != nil {
		return failStartup(err)
	}
	if err := runCtx.Err(); err != nil {
		_ = session.Close()
		return failStartup(err)
	}
	state.mu.Lock()
	state.session = session
	state.mu.Unlock()

	state.mu.Lock()
	state.record.Status = valueobject.RunRunning
	state.mu.Unlock()
	if err := m.persist(state); err != nil {
		return failStartup(fmt.Errorf("RUN_STORE_FAILED: %w", err))
	}
	state.mu.Lock()
	state.timer = m.opts.NewTimer(m.opts.RunTimeout, func() {
		m.finish(state, valueobject.RunFailed, "RUN_TIMEOUT", nil)
	})
	state.mu.Unlock()
	m.emitStatus(state, valueobject.RunRunning, nil)

	if err := session.Send(runCtx, runContext.InitialPrompt); err != nil {
		state.setupMu.Unlock()
		setupHeld = false
		m.finish(state, valueobject.RunFailed, errorCode(err), nil)
		return state.snapshot(), err
	}
	state.setupMu.Unlock()
	setupHeld = false
	return state.snapshot(), nil
}

func (m *Manager) validateDependencies() error {
	if m.opts.Projects == nil || m.opts.Workflow == nil || m.opts.Contexts == nil || m.opts.Workspaces == nil || m.opts.RunStores == nil || m.opts.RunConfigs == nil || m.opts.ToolServers == nil || m.opts.Permissions == nil {
		return errors.New("RUN_MANAGER_DEPENDENCY_MISSING")
	}
	return nil
}

func (m *Manager) register(state *runState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrManagerClosed
	}
	if _, exists := m.active[state.scope.RunID]; exists {
		return errors.New("RUN_ID_CONFLICT")
	}
	m.active[state.scope.RunID] = state
	return nil
}

func (m *Manager) IsBusy() bool { return m.gate.Busy() }

func (m *Manager) ActiveRunID() string { return m.gate.Owner() }

func (m *Manager) Interrupt(ctx context.Context, runID string) error {
	state := m.getActive(runID)
	if state == nil || !state.acquireLease() {
		return ErrRunNotFound
	}
	state.leaseMu.Lock()
	state.interrupting = true
	state.leaseMu.Unlock()
	state.mu.Lock()
	session := state.session
	state.mu.Unlock()
	var interruptErr error
	if session != nil {
		interruptErr = session.Interrupt(ctx)
	}
	state.releaseLease()
	m.finish(state, valueobject.RunInterrupted, "RUN_INTERRUPTED", nil)
	return interruptErr
}

func (m *Manager) RespondPermission(runID, id string, decision valueobject.PermissionDecision) error {
	state := m.getActive(runID)
	if state == nil || !state.acquireLease() {
		return ErrRunNotFound
	}
	defer state.releaseLease()
	m.resetWatchdog(state)
	state.mu.Lock()
	permissions := state.permissions
	state.mu.Unlock()
	if permissions == nil {
		return ErrRunNotFound
	}
	return permissions.ResolvePermission(id, decision)
}

func (m *Manager) RespondQuestion(runID, id string, answer valueobject.QuestionAnswer) error {
	state := m.getActive(runID)
	if state == nil || !state.acquireLease() {
		return ErrRunNotFound
	}
	defer state.releaseLease()
	m.resetWatchdog(state)
	state.mu.Lock()
	permissions := state.permissions
	state.mu.Unlock()
	if permissions == nil {
		return ErrRunNotFound
	}
	return permissions.ResolveQuestion(id, answer)
}

func (m *Manager) List(ctx context.Context) ([]entity.RunSummary, error) {
	if m.opts.Projects == nil || m.opts.RunStores == nil {
		return nil, errors.New("RUN_MANAGER_DEPENDENCY_MISSING")
	}
	project, err := m.opts.Projects.Current(ctx)
	if err != nil {
		return nil, err
	}
	store, err := m.opts.RunStores.ForProject(ctx, project.ID)
	if err != nil {
		return nil, err
	}
	return store.List(ctx)
}

func (m *Manager) Get(ctx context.Context, runID string) (entity.RunRecord, error) {
	if err := valueobject.ValidateID(runID); err != nil {
		return entity.RunRecord{}, ErrRunNotFound
	}
	project, err := m.opts.Projects.Current(ctx)
	if err != nil {
		return entity.RunRecord{}, err
	}
	store, err := m.opts.RunStores.ForProject(ctx, project.ID)
	if err != nil {
		return entity.RunRecord{}, err
	}
	return store.Load(ctx, runID)
}

// RecoverInterrupted marks unfinished records from every project before the
// application accepts new runs. Runs are history only; they are never resumed.
func (m *Manager) RecoverInterrupted(ctx context.Context) error {
	if m.opts.Projects == nil || m.opts.RunStores == nil || m.opts.RunConfigs == nil {
		return errors.New("RUN_MANAGER_DEPENDENCY_MISSING")
	}
	projects, err := m.opts.Projects.List(ctx)
	if err != nil {
		return err
	}
	for _, project := range projects {
		if err := ctx.Err(); err != nil {
			return err
		}
		store, err := m.opts.RunStores.ForProject(ctx, project.ID)
		if err != nil {
			return err
		}
		runs, err := store.List(ctx)
		if err != nil {
			return err
		}
		for _, summary := range runs {
			if summary.Status != valueobject.RunStarting && summary.Status != valueobject.RunRunning {
				continue
			}
			record, err := store.Load(ctx, summary.ID)
			if err != nil {
				return err
			}
			record.Status = valueobject.RunInterrupted
			record.Error = "INTERRUPTED_BY_RESTART"
			record.FinishedAt = m.now().UTC().Format(time.RFC3339Nano)
			if err := store.Save(ctx, record); err != nil {
				return err
			}
			if err := m.opts.RunConfigs.Cleanup(ctx, project.ID, record.ID); err != nil {
				return fmt.Errorf("cleanup interrupted run %s: %w", record.ID, err)
			}
			m.publishRecovered(record)
		}
	}
	return nil
}

func (m *Manager) WithActiveRun(ctx context.Context, scope port.RunScope, operation func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if operation == nil {
		return errors.New("RUN_OPERATION_REQUIRED")
	}
	state := m.getActive(scope.RunID)
	if state == nil || !sameScope(state.scope, scope) {
		return port.ErrTurnNotActive
	}
	if !state.acquireLease() {
		return port.ErrTurnNotActive
	}
	defer state.releaseLease()
	m.resetWatchdog(state)
	return operation()
}

func (m *Manager) RecordRunPending(ctx context.Context, scope port.RunScope, pending entity.Pending) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	state := m.getActive(scope.RunID)
	if state == nil || !sameScope(state.scope, scope) || pending.RunID != scope.RunID || pending.NodeID != scope.NodeID {
		return port.ErrTurnNotActive
	}
	if !state.acquireLease() {
		return port.ErrTurnNotActive
	}
	defer state.releaseLease()
	m.resetWatchdog(state)
	state.mu.Lock()
	state.record.PendingID = pending.ID
	state.mu.Unlock()
	if err := m.persist(state); err != nil {
		return err
	}
	m.emit(state, valueobject.RunEvent{Type: valueobject.EventPending, PendingID: pending.ID})
	return nil
}

func (m *Manager) onAgentEvent(state *runState, event valueobject.AgentEvent) {
	if !state.acquireLease() {
		return
	}
	defer state.releaseLease()
	state.leaseMu.Lock()
	interrupting := state.interrupting
	state.leaseMu.Unlock()
	if interrupting {
		return
	}
	m.resetWatchdog(state)
	switch event.Type {
	case valueobject.EventStatus:
		if event.State == nil {
			return
		}
		switch event.State.Status {
		case valueobject.SessionIdle:
			go m.finish(state, valueobject.RunSucceeded, "", event.ActualInputTokens)
		case valueobject.SessionError, valueobject.SessionUnavailable, valueobject.SessionResumeFailed:
			go m.finish(state, valueobject.RunFailed, "RUN_FAILED", event.ActualInputTokens)
		default:
			m.emitAgentEvent(state, event)
		}
	case valueobject.EventError:
		m.emitAgentEvent(state, event)
		code := "RUN_FAILED"
		if event.Error != nil && event.Error.Code != "" {
			code = event.Error.Code
		}
		go m.finish(state, valueobject.RunFailed, code, event.ActualInputTokens)
	case valueobject.EventMessage:
		if event.Message != nil {
			state.mu.Lock()
			applyMessage(&state.record.Transcript, *event.Message)
			state.mu.Unlock()
			if !event.Message.Streaming {
				if err := m.persist(state); err != nil {
					m.opts.Logf("persist transcript for run %s: %v", state.scope.RunID, err)
				}
			}
		}
		m.emitAgentEvent(state, event)
	case valueobject.EventTool:
		if event.Tool != nil {
			state.mu.Lock()
			applyTool(&state.record.Transcript, *event.Tool)
			state.mu.Unlock()
			if err := m.persist(state); err != nil {
				m.opts.Logf("persist tool event for run %s: %v", state.scope.RunID, err)
			}
		}
		m.emitAgentEvent(state, event)
	case valueobject.EventPermission, valueobject.EventQuestion:
		m.emitAgentEvent(state, event)
	}
}

func (m *Manager) onReply(state *runState, message string) {
	if strings.TrimSpace(message) == "" {
		return
	}
	if !state.acquireLease() {
		return
	}
	defer state.releaseLease()
	m.resetWatchdog(state)
	item := valueobject.TranscriptItem{
		ID:   fmt.Sprintf("reply-%s-%d", state.scope.RunID, state.messageSeq.Add(1)),
		Kind: valueobject.TranscriptMessage, Role: "assistant", Text: message,
		CreatedAt: m.now().UTC().Format(time.RFC3339Nano),
	}
	state.mu.Lock()
	applyMessage(&state.record.Transcript, item)
	state.mu.Unlock()
	if err := m.persist(state); err != nil {
		m.opts.Logf("persist MCP reply for run %s: %v", state.scope.RunID, err)
	}
	m.emit(state, valueobject.RunEvent{Type: valueobject.EventMessage, Message: &item})
}

func (m *Manager) finish(state *runState, status valueobject.RunStatus, errorCode string, actualInputTokens *int) {
	state.finishOnce.Do(func() {
		state.mu.Lock()
		cancel := state.cancel
		state.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		state.setupMu.Lock()
		defer state.setupMu.Unlock()
		state.leaseMu.Lock()
		state.closing = true
		state.leaseMu.Unlock()

		state.mu.Lock()
		permissions := state.permissions
		state.mu.Unlock()
		if permissions != nil {
			permissions.Close()
		}

		state.leaseMu.Lock()
		for state.leases > 0 {
			state.leaseCond.Wait()
		}
		state.leaseMu.Unlock()

		state.mu.Lock()
		if state.timer != nil {
			state.timer.Stop()
		}
		session := state.session
		tool := state.tool
		permissions = state.permissions
		configReady := state.configReady
		state.mu.Unlock()
		if session != nil {
			if err := session.Close(); err != nil {
				m.opts.Logf("close Agent run %s: %v", state.scope.RunID, err)
			}
		}
		if permissions != nil {
			permissions.Close()
		}
		if tool != nil {
			stopCtx, stopCancel := context.WithTimeout(context.Background(), statusCleanupTimeout)
			if err := tool.Stop(stopCtx); err != nil {
				if status == valueobject.RunSucceeded {
					status = valueobject.RunFailed
					errorCode = "RUN_CLEANUP_FAILED"
				}
				m.opts.Logf("stop MCP server for run %s: %v", state.scope.RunID, err)
			}
			stopCancel()
		}
		if configReady {
			if err := m.opts.RunConfigs.Cleanup(context.Background(), state.scope.ProjectID, state.scope.RunID); err != nil {
				if status == valueobject.RunSucceeded {
					status = valueobject.RunFailed
					errorCode = "RUN_CLEANUP_FAILED"
				}
				m.opts.Logf("remove MCP config for run %s: %v", state.scope.RunID, err)
			}
		}
		if err := m.opts.Workflow.ReleaseRunLock(context.Background(), state.scope.RunID); err != nil {
			m.opts.Logf("release run lock %s: %v", state.scope.RunID, err)
			if status == valueobject.RunSucceeded {
				status = valueobject.RunFailed
				errorCode = "RUN_LOCK_RELEASE_FAILED"
			}
		}

		state.mu.Lock()
		state.record.Status = status
		state.record.Error = errorCode
		state.record.FinishedAt = m.now().UTC().Format(time.RFC3339Nano)
		if actualInputTokens != nil {
			if usage, err := (valueobject.ContextEstimate{
				EstimatorVersion: state.record.EstimatorVersion,
				EstimatedTokens:  state.record.EstimatedTokens,
				TokenBudget:      state.record.TokenBudget,
			}).WithActualInputTokens(*actualInputTokens); err == nil {
				state.record.ActualInputTokens = usage.ActualInputTokens
				state.record.UsageReported = true
				state.record.OverBudget = usage.OverBudget
			}
		}
		if len(state.record.Transcript) > valueobject.MaxRunTranscriptItems {
			state.record.Transcript = append([]valueobject.TranscriptItem(nil), state.record.Transcript[len(state.record.Transcript)-valueobject.MaxRunTranscriptItems:]...)
		}
		state.mu.Unlock()
		if err := m.persist(state); err != nil {
			m.opts.Logf("persist terminal run %s: %v", state.scope.RunID, err)
			state.mu.Lock()
			state.record.Status = valueobject.RunFailed
			state.record.Error = "RUN_STORE_FAILED"
			state.mu.Unlock()
			status = valueobject.RunFailed
			errorCode = "RUN_STORE_FAILED"
			if retryErr := m.persist(state); retryErr != nil {
				m.opts.Logf("retry terminal run store for %s: %v", state.scope.RunID, retryErr)
			}
		}
		m.mu.Lock()
		delete(m.active, state.scope.RunID)
		m.mu.Unlock()
		if err := m.gate.Release(state.scope.RunID); err != nil {
			m.opts.Logf("release turn gate for run %s: %v", state.scope.RunID, err)
		}
		var eventError *valueobject.AgentError
		if errorCode != "" {
			eventError = &valueobject.AgentError{Code: errorCode, Message: errorCode, Recoverable: false}
		}
		m.emit(state, valueobject.RunEvent{Type: valueobject.EventStatus, Status: status, Error: eventError, ActualInputTokens: actualInputTokens})
	})
}

func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	active := make([]*runState, 0, len(m.active))
	for _, state := range m.active {
		active = append(active, state)
	}
	m.mu.Unlock()
	for _, state := range active {
		m.finish(state, valueobject.RunInterrupted, "RUN_INTERRUPTED_BY_SHUTDOWN", nil)
	}
}

func (m *Manager) getActive(runID string) *runState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active[runID]
}

func (m *Manager) persist(state *runState) error {
	state.persistMu.Lock()
	defer state.persistMu.Unlock()
	return state.store.Save(context.Background(), state.snapshot())
}

func (m *Manager) resetWatchdog(state *runState) {
	state.mu.Lock()
	if state.timer != nil {
		state.timer.Reset(m.opts.RunTimeout)
	}
	state.mu.Unlock()
}

func (s *runState) acquireLease() bool {
	s.leaseMu.Lock()
	defer s.leaseMu.Unlock()
	if s.closing {
		return false
	}
	s.leases++
	return true
}

func (s *runState) releaseLease() {
	s.leaseMu.Lock()
	s.leases--
	if s.leases == 0 {
		s.leaseCond.Broadcast()
	}
	s.leaseMu.Unlock()
}

func (m *Manager) emitAgentEvent(state *runState, event valueobject.AgentEvent) {
	runEvent := valueobject.RunEvent{
		Type: event.Type, Message: event.Message, Tool: event.Tool,
		Permission: event.Permission, Question: event.Question,
		Error: event.Error, ActualInputTokens: event.ActualInputTokens,
	}
	m.emit(state, runEvent)
}

func (m *Manager) emitStatus(state *runState, status valueobject.RunStatus, eventError *valueobject.AgentError) {
	m.emit(state, valueobject.RunEvent{Type: valueobject.EventStatus, Status: status, Error: eventError})
}

func (m *Manager) emit(state *runState, event valueobject.RunEvent) {
	if m.opts.Publisher == nil {
		return
	}
	state.mu.Lock()
	state.seq++
	event.Version = 1
	event.Seq = state.seq
	event.RunID = state.scope.RunID
	event.TreeID = state.scope.TreeID
	event.ChainID = state.scope.ChainID
	event.NodeID = state.scope.NodeID
	event.ChainPath = append([]string(nil), state.scope.ChainPath...)
	state.mu.Unlock()
	m.opts.Publisher.PublishRun(event)
}

func (m *Manager) publishRecovered(record entity.RunRecord) {
	if m.opts.Publisher == nil {
		return
	}
	m.opts.Publisher.PublishRun(valueobject.RunEvent{
		Version: 1, Seq: 1, RunID: record.ID, TreeID: record.TreeID, ChainID: record.ChainID,
		NodeID: record.NodeID, ChainPath: append([]string(nil), record.ChainPath...),
		Type: valueobject.EventStatus, Status: valueobject.RunInterrupted,
		Error: &valueobject.AgentError{Code: "INTERRUPTED_BY_RESTART", Message: "INTERRUPTED_BY_RESTART", Recoverable: false},
	})
}

func (s *runState) snapshot() entity.RunRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	record := s.record
	record.ChainPath = append([]string(nil), s.record.ChainPath...)
	record.Transcript = append([]valueobject.TranscriptItem(nil), s.record.Transcript...)
	return record
}

func (m *Manager) sameScope(state *runState, scope port.RunScope) bool {
	return sameScope(state.scope, scope)
}

func sameScope(a, b port.RunScope) bool {
	return a.RunID == b.RunID && a.ProjectID == b.ProjectID && a.TreeID == b.TreeID &&
		a.NodeID == b.NodeID && a.ChainID == b.ChainID && slices.Equal(a.ChainPath, b.ChainPath) &&
		a.ContextHash == b.ContextHash && a.AncestorHash == b.AncestorHash && a.ParamsHash == b.ParamsHash
}

func applyMessage(transcript *[]valueobject.TranscriptItem, item valueobject.TranscriptItem) {
	for i := range *transcript {
		if (*transcript)[i].ID != item.ID {
			continue
		}
		if item.Streaming && (*transcript)[i].Streaming {
			(*transcript)[i].Text += item.Text
		} else {
			(*transcript)[i] = item
		}
		return
	}
	*transcript = append(*transcript, item)
	trimTranscript(transcript)
}

func applyTool(transcript *[]valueobject.TranscriptItem, item valueobject.TranscriptItem) {
	for i := range *transcript {
		if (*transcript)[i].ID != item.ID || (*transcript)[i].Kind != valueobject.TranscriptTool {
			continue
		}
		if item.ToolName != "" {
			(*transcript)[i].ToolName = item.ToolName
		}
		if item.ToolStatus != "" {
			(*transcript)[i].ToolStatus = item.ToolStatus
		}
		if item.ToolSummary != "" {
			(*transcript)[i].ToolSummary = item.ToolSummary
		}
		return
	}
	*transcript = append(*transcript, item)
	trimTranscript(transcript)
}

func trimTranscript(transcript *[]valueobject.TranscriptItem) {
	if len(*transcript) > valueobject.MaxRunTranscriptItems {
		*transcript = append([]valueobject.TranscriptItem(nil), (*transcript)[len(*transcript)-valueobject.MaxRunTranscriptItems:]...)
	}
}

func contextPreview(prompt string) (string, bool) {
	if len(prompt) <= contextPreviewMaxBytes {
		return prompt, false
	}
	end := contextPreviewMaxBytes
	for end > 0 && !utf8.ValidString(prompt[:end]) {
		end--
	}
	return prompt[:end], true
}

func errorCode(err error) string {
	if err == nil {
		return "RUN_FAILED"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "RUN_TIMEOUT"
	}
	message := strings.TrimSpace(err.Error())
	code, _, found := strings.Cut(message, ":")
	if found && code != "" && len(code) <= 64 {
		valid := true
		for _, char := range code {
			if (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '_' {
				valid = false
				break
			}
		}
		if valid {
			return code
		}
	}
	return "RUN_FAILED"
}

const runSystemPrompt = "You are AudioUI's one-shot node execution agent. Work only on the active node task and its provided workspace path. Follow the output contract in the initial context. Use ask_user when a decision is required, and use reply for chat or structured output. Never attempt to change the graph or apply an artifact."

var runBuiltinTools = []string{"Read", "Glob", "Grep", "Write", "Edit", "Bash"}

var runAllowedTools = []string{
	"mcp__audioui__get_chain", "mcp__audioui__get_graph", "mcp__audioui__get_node",
	"mcp__audioui__list_node_types", "mcp__audioui__validate", "mcp__audioui__read_artifact",
	"mcp__audioui__reply", "mcp__audioui__read_file", "mcp__audioui__list_dir",
	"mcp__audioui__grep", "mcp__audioui__stat", "mcp__audioui__ask_user",
}
