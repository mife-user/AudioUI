package run

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
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
	ErrAgentUnavailable   = errors.New("AGENT_UNAVAILABLE")
	ErrRunNotFound        = errors.New("RUN_NOT_FOUND")
	ErrRunPendingConflict = errors.New("RUN_PENDING_CONFLICT")
)

type ProjectService interface {
	Current(ctx context.Context) (entity.Project, error)
	Config(ctx context.Context) (valueobject.AppConfig, error)
	List(ctx context.Context) ([]entity.ProjectSummary, error)
}

type WorkflowService interface {
	BeginRun(ctx context.Context, runID, nodeID string, expectedProjectID ...string) (entity.Project, entity.RunExecutionSnapshot, error)
	BeginRunWithInputs(ctx context.Context, runID, nodeID string, inputEdgeIDs []string, expectedProjectID ...string) (entity.Project, entity.RunExecutionSnapshot, error)
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
	Projects       ProjectService
	Workflow       WorkflowService
	Contexts       port.RunContextBuilder
	Workspaces     port.RunWorkspaceProvider
	RunStores      port.RunStoreProvider
	RunConfigs     port.RunConfigPaths
	ToolServers    port.RunToolServerFactory
	Permissions    port.RunPermissionFactory
	Driver         port.AgentDriver
	DriverProvider func(context.Context) (port.AgentDriver, error)
	Publisher      port.RunEventPublisher
	Gate           *TurnGate
	RunTimeout     time.Duration
	NewTimer       func(time.Duration, func()) RunTimer
	Now            func() time.Time
	Logf           func(format string, args ...any)
}

type Manager struct {
	opts        Options
	gate        *TurnGate
	now         func() time.Time
	mu          sync.Mutex
	active      map[string]*runState
	terminalSeq map[string]uint64
	closed      bool
	seq         atomic.Uint64
}

type runState struct {
	eventMu         sync.Mutex
	eventQueue      []valueobject.RunEvent
	publishing      bool
	setupMu         sync.Mutex
	mu              sync.Mutex
	persistMu       sync.Mutex
	leaseMu         sync.Mutex
	leaseCond       *sync.Cond
	leases          int
	closing         bool
	interrupting    bool
	finishOnce      sync.Once
	seq             uint64
	scope           port.RunScope
	spec            RunSpec
	record          entity.RunRecord
	permissionsByID map[string]valueobject.PermissionRequest
	questionsByID   map[string]valueobject.QuestionRequest
	store           port.RunStore
	session         port.AgentSession
	tool            port.ToolServer
	permissions     port.RunPermissionController
	runContext      context.Context
	cancel          context.CancelFunc
	timer           RunTimer
	configReady     bool
	messageSeq      atomic.Uint64
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
	return &Manager{
		opts: options, gate: options.Gate, now: options.Now,
		active: make(map[string]*runState), terminalSeq: make(map[string]uint64),
	}
}

func (m *Manager) Start(ctx context.Context, nodeID string, expectedProjectID ...string) (entity.RunRecord, error) {
	return m.StartWithInputs(ctx, nodeID, nil, expectedProjectID...)
}

func (m *Manager) StartWithInputs(ctx context.Context, nodeID string, inputEdgeIDs []string, expectedProjectID ...string) (entity.RunRecord, error) {
	if err := ctx.Err(); err != nil {
		return entity.RunRecord{}, err
	}
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed {
		return entity.RunRecord{}, ErrManagerClosed
	}
	driver := m.opts.Driver
	if m.opts.DriverProvider != nil {
		resolved, err := m.opts.DriverProvider(ctx)
		if err != nil {
			m.opts.Logf("resolve agent driver: %v", err)
			return entity.RunRecord{}, ErrAgentUnavailable
		}
		driver = resolved
	}
	if driver == nil {
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

	project, executionSnapshot, err := m.opts.Workflow.BeginRunWithInputs(ctx, runID, nodeID, inputEdgeIDs, expectedProjectID...)
	if err != nil {
		return entity.RunRecord{}, err
	}
	lockAcquired = true
	executionSnapshot = executionSnapshot.Clone()
	chainPath := executionSnapshot.ChainPath
	if executionSnapshot.Version != 3 && len(chainPath) < 1 {
		return entity.RunRecord{}, errors.New("NOT_IN_CHAIN: empty run path")
	}
	treeID := executionSnapshot.TreeID
	workspace, err := m.opts.Workspaces.OpenRun(ctx, project.ID, executionSnapshot)
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
	runContext, err := m.opts.Contexts.BuildRunContext(ctx, project, nodeID, executionSnapshot, workspace, budget)
	if err != nil {
		return entity.RunRecord{}, err
	}
	if runContext.TreeID != treeID || runContext.NodeID != nodeID || !slices.Equal(runContext.ChainPath, chainPath) ||
		executionSnapshot.Version == 3 && !slices.Equal(runContext.InputNodeIDs, executionSnapshot.ScopeNodeIDs()) ||
		runContext.Hashes.ContextHash == "" || runContext.Hashes.AncestorHash == "" || runContext.Hashes.ParamsHash == "" {
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
		InputNodeIDs: append([]string(nil), executionSnapshot.ScopeNodeIDs()...),
		ContextHash:  runContext.Hashes.ContextHash, AncestorHash: runContext.Hashes.AncestorHash,
		ParamsHash: runContext.Hashes.ParamsHash, ExecutionSnapshot: executionSnapshot.Clone(),
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
		ExecutionSnapshot: &executionSnapshot,
		Transcript:        []valueobject.TranscriptItem{},
	}
	if err := store.Save(ctx, record); err != nil {
		return entity.RunRecord{}, err
	}
	runCtx, cancel := context.WithCancel(context.Background())
	state := &runState{
		scope: scope.Clone(), store: store, record: record, runContext: runCtx, cancel: cancel,
		permissionsByID: make(map[string]valueobject.PermissionRequest),
		questionsByID:   make(map[string]valueobject.QuestionRequest),
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
		Scope: scope.Clone(), Workspace: workspace, Context: runContext, MCPConfigPath: mcpConfigPath,
		SystemPrompt: runSystemPrompt,
		BuiltinTools: append([]string(nil), runBuiltinTools...),
		AllowedTools: append([]string(nil), runAllowedTools...),
	}
	state.mu.Lock()
	state.spec = spec
	state.mu.Unlock()

	permissions, err := m.opts.Permissions.NewRunPermissions(scope.Clone(), workspace.Resolver(), func(event valueobject.AgentEvent) {
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

	tool, err := m.opts.ToolServers.NewForRun(scope.Clone(), workspace.CWD(), runID, permissions, func(message string) {
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
	session, err := driver.Start(setupCtx, driverSpec)
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
	if err := permissions.ResolvePermission(id, decision); err != nil {
		return err
	}
	m.resolveRequest(state, "permission", id)
	return nil
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
	if err := permissions.ResolveQuestion(id, answer); err != nil {
		return err
	}
	m.resolveRequest(state, "question", id)
	return nil
}

func (m *Manager) List(ctx context.Context) ([]entity.RunSummary, error) {
	if m.opts.Projects == nil || m.opts.RunStores == nil {
		return nil, errors.New("RUN_MANAGER_DEPENDENCY_MISSING")
	}
	project, err := m.opts.Projects.Current(ctx)
	if err != nil {
		return nil, err
	}
	return m.ListForProject(ctx, project.ID)
}

// ListForProject reads persisted run summaries and overlays active in-memory
// state for one explicit project. It never guesses project ownership from a
// run's node or tree identifiers.
func (m *Manager) ListForProject(ctx context.Context, projectID string) ([]entity.RunSummary, error) {
	if err := valueobject.ValidateID(projectID); err != nil || m.opts.RunStores == nil {
		return nil, errors.New("RUN_MANAGER_DEPENDENCY_MISSING")
	}
	store, err := m.opts.RunStores.ForProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	runs, err := store.List(ctx)
	if err != nil {
		return nil, err
	}
	positions := make(map[string]int, len(runs))
	for i := range runs {
		positions[runs[i].ID] = i
	}
	m.mu.Lock()
	active := make([]*runState, 0, len(m.active))
	for _, state := range m.active {
		if state.scope.ProjectID == projectID {
			active = append(active, state)
		}
	}
	m.mu.Unlock()
	for _, state := range active {
		record := state.snapshot()
		if index, ok := positions[record.ID]; ok {
			runs[index] = record.RunSummary
			continue
		}
		positions[record.ID] = len(runs)
		runs = append(runs, record.RunSummary)
	}
	sort.SliceStable(runs, func(i, j int) bool { return runs[i].StartedAt > runs[j].StartedAt })
	for i := range runs {
		runs[i].ChainPath = append([]string(nil), runs[i].ChainPath...)
	}
	return runs, nil
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

// GetSnapshot returns a run view and the event sequence from one in-memory
// synchronization point. Historical runs have no process-local requests and
// use the last sequence retained by this process, or zero after a restart.
func (m *Manager) GetSnapshot(ctx context.Context, runID string) (entity.RunSnapshot, error) {
	if err := valueobject.ValidateID(runID); err != nil {
		return entity.RunSnapshot{}, ErrRunNotFound
	}
	if m.opts.Projects == nil || m.opts.RunStores == nil {
		return entity.RunSnapshot{}, errors.New("RUN_MANAGER_DEPENDENCY_MISSING")
	}
	project, err := m.opts.Projects.Current(ctx)
	if err != nil {
		return entity.RunSnapshot{}, err
	}
	if state := m.getActive(runID); state != nil {
		if state.scope.ProjectID != project.ID {
			return entity.RunSnapshot{}, ErrRunNotFound
		}
		return state.runSnapshot(), nil
	}
	store, err := m.opts.RunStores.ForProject(ctx, project.ID)
	if err != nil {
		return entity.RunSnapshot{}, err
	}
	record, err := store.Load(ctx, runID)
	if err != nil {
		return entity.RunSnapshot{}, err
	}
	m.mu.Lock()
	lastSeq := m.terminalSeq[runID]
	m.mu.Unlock()
	return entity.RunSnapshot{
		RunID: runID, ProjectID: project.ID, Run: record, LastSeq: lastSeq,
		Permissions: []valueobject.PermissionRequest{}, Questions: []valueobject.QuestionRequest{},
	}, nil
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
			if m.opts.Publisher != nil {
				m.mu.Lock()
				m.terminalSeq[record.ID] = 1
				m.mu.Unlock()
			}
			m.publishRecovered(project.ID, record)
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
	if scope.ExecutionSnapshot.Version == 3 && (pending.ExecutionSnapshot == nil || !reflect.DeepEqual(*pending.ExecutionSnapshot, scope.ExecutionSnapshot)) {
		return ErrRunPendingConflict
	}
	// The reply callback already holds a lease if finish has begun closing the run.
	if !state.acquireLease() {
		state.leaseMu.Lock()
		alreadyLeased := state.leases > 0
		state.leaseMu.Unlock()
		if !alreadyLeased {
			return port.ErrTurnNotActive
		}
	} else {
		defer state.releaseLease()
		m.resetWatchdog(state)
	}
	state.eventMu.Lock()
	state.mu.Lock()
	recordedPendingID := state.record.PendingID
	if recordedPendingID != "" && recordedPendingID != pending.ID {
		state.mu.Unlock()
		state.eventMu.Unlock()
		return ErrRunPendingConflict
	}
	shouldDrain := false
	if recordedPendingID == "" {
		state.record.PendingID = pending.ID
		event := nextRunEventLocked(state, valueobject.RunEvent{Type: valueobject.EventPending, PendingID: pending.ID})
		state.mu.Unlock()
		shouldDrain = m.queueRunEventLocked(state, event)
	} else {
		state.mu.Unlock()
	}
	state.eventMu.Unlock()
	if shouldDrain {
		m.drainRunEvents(state)
	}
	if err := m.persist(state); err != nil {
		return err
	}
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
			status := valueobject.RunStarting
			if event.State.Status == valueobject.SessionRunning {
				status = valueobject.RunRunning
			}
			m.emitWith(state, valueobject.RunEvent{Type: valueobject.EventStatus, Status: status}, func() {
				state.record.Status = status
			})
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
			item := *event.Message
			m.emitWith(state, valueobject.RunEvent{Type: valueobject.EventMessage, Message: &item}, func() {
				applyMessage(&state.record.Transcript, item)
			})
			if !event.Message.Streaming {
				if err := m.persist(state); err != nil {
					m.opts.Logf("persist transcript for run %s: %v", state.scope.RunID, err)
				}
			}
			return
		}
		m.emitAgentEvent(state, event)
	case valueobject.EventTool:
		if event.Tool != nil {
			item := *event.Tool
			m.emitWith(state, valueobject.RunEvent{Type: valueobject.EventTool, Tool: &item}, func() {
				applyTool(&state.record.Transcript, item)
			})
			if err := m.persist(state); err != nil {
				m.opts.Logf("persist tool event for run %s: %v", state.scope.RunID, err)
			}
			return
		}
		m.emitAgentEvent(state, event)
	case valueobject.EventPermission:
		if event.Permission == nil {
			return
		}
		request := clonePermissionRequest(*event.Permission)
		m.emitWith(state, valueobject.RunEvent{Type: valueobject.EventPermission, Permission: &request}, func() {
			state.permissionsByID[request.ID] = request
		})
	case valueobject.EventQuestion:
		if event.Question == nil {
			return
		}
		request := cloneQuestionRequest(*event.Question)
		m.emitWith(state, valueobject.RunEvent{Type: valueobject.EventQuestion, Question: &request}, func() {
			state.questionsByID[request.ID] = request
		})
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
	m.emitWith(state, valueobject.RunEvent{Type: valueobject.EventMessage, Message: &item}, func() {
		applyMessage(&state.record.Transcript, item)
	})
	if err := m.persist(state); err != nil {
		m.opts.Logf("persist MCP reply for run %s: %v", state.scope.RunID, err)
	}
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
		record := state.snapshot()
		record.Status = status
		record.Error = errorCode
		record.FinishedAt = m.now().UTC().Format(time.RFC3339Nano)
		if actualInputTokens != nil {
			if usage, err := (valueobject.ContextEstimate{
				EstimatorVersion: record.EstimatorVersion,
				EstimatedTokens:  record.EstimatedTokens,
				TokenBudget:      record.TokenBudget,
			}).WithActualInputTokens(*actualInputTokens); err == nil {
				record.ActualInputTokens = usage.ActualInputTokens
				record.UsageReported = true
				record.OverBudget = usage.OverBudget
			}
		}
		if len(record.Transcript) > valueobject.MaxRunTranscriptItems {
			record.Transcript = append([]valueobject.TranscriptItem(nil), record.Transcript[len(record.Transcript)-valueobject.MaxRunTranscriptItems:]...)
		}
		if err := m.persistRecord(state, record); err != nil {
			m.opts.Logf("persist terminal run %s: %v", state.scope.RunID, err)
			record.Status = valueobject.RunFailed
			record.Error = "RUN_STORE_FAILED"
			status = valueobject.RunFailed
			errorCode = "RUN_STORE_FAILED"
			if retryErr := m.persistRecord(state, record); retryErr != nil {
				m.opts.Logf("retry terminal run store for %s: %v", state.scope.RunID, retryErr)
			}
		}
		var eventError *valueobject.AgentError
		if errorCode != "" {
			eventError = &valueobject.AgentError{Code: errorCode, Message: errorCode, Recoverable: false}
		}
		m.emitWith(state, valueobject.RunEvent{Type: valueobject.EventStatus, Status: status, Error: eventError, ActualInputTokens: actualInputTokens}, func() {
			state.record = cloneRunRecord(record)
			state.permissionsByID = make(map[string]valueobject.PermissionRequest)
			state.questionsByID = make(map[string]valueobject.QuestionRequest)
		})
		if err := m.opts.Workflow.ReleaseRunLock(context.Background(), state.scope.RunID); err != nil {
			m.opts.Logf("release run lock %s: %v", state.scope.RunID, err)
			if status == valueobject.RunSucceeded {
				status = valueobject.RunFailed
				errorCode = "RUN_LOCK_RELEASE_FAILED"
				record.Status = status
				record.Error = errorCode
				if persistErr := m.persistRecord(state, record); persistErr != nil {
					m.opts.Logf("persist run lock failure for %s: %v", state.scope.RunID, persistErr)
				}
				lockError := &valueobject.AgentError{Code: errorCode, Message: errorCode, Recoverable: false}
				m.emitWith(state, valueobject.RunEvent{Type: valueobject.EventStatus, Status: status, Error: lockError, ActualInputTokens: actualInputTokens}, func() {
					state.record = cloneRunRecord(record)
				})
			}
		}
		if err := m.gate.Release(state.scope.RunID); err != nil {
			m.opts.Logf("release turn gate for run %s: %v", state.scope.RunID, err)
		}
		m.mu.Lock()
		delete(m.active, state.scope.RunID)
		state.mu.Lock()
		m.terminalSeq[state.scope.RunID] = state.seq
		state.mu.Unlock()
		m.mu.Unlock()
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

func (m *Manager) persistRecord(state *runState, record entity.RunRecord) error {
	state.persistMu.Lock()
	defer state.persistMu.Unlock()
	return state.store.Save(context.Background(), cloneRunRecord(record))
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
	m.emitWith(state, event, nil)
}

func (m *Manager) emitWith(state *runState, event valueobject.RunEvent, mutate func()) {
	state.eventMu.Lock()
	state.mu.Lock()
	if mutate != nil {
		mutate()
	}
	event = nextRunEventLocked(state, event)
	state.mu.Unlock()
	drain := m.queueRunEventLocked(state, event)
	state.eventMu.Unlock()
	if drain {
		m.drainRunEvents(state)
	}
}

func (m *Manager) resolveRequest(state *runState, kind, id string) {
	state.eventMu.Lock()
	state.mu.Lock()
	switch kind {
	case "permission":
		if _, exists := state.permissionsByID[id]; !exists {
			state.mu.Unlock()
			state.eventMu.Unlock()
			return
		}
		delete(state.permissionsByID, id)
	case "question":
		if _, exists := state.questionsByID[id]; !exists {
			state.mu.Unlock()
			state.eventMu.Unlock()
			return
		}
		delete(state.questionsByID, id)
	default:
		state.mu.Unlock()
		state.eventMu.Unlock()
		return
	}
	event := nextRunEventLocked(state, valueobject.RunEvent{
		Type: valueobject.EventRequestResolved, ResolvedKind: kind, ResolvedID: id,
	})
	state.mu.Unlock()
	drain := m.queueRunEventLocked(state, event)
	state.eventMu.Unlock()
	if drain {
		m.drainRunEvents(state)
	}
}

// queueRunEventLocked appends in sequence order. Callers publish only after
// releasing eventMu and the run state mutex.
func (m *Manager) queueRunEventLocked(state *runState, event valueobject.RunEvent) bool {
	if m.opts.Publisher == nil {
		return false
	}
	state.eventQueue = append(state.eventQueue, event)
	if state.publishing {
		return false
	}
	state.publishing = true
	return true
}

func (m *Manager) drainRunEvents(state *runState) {
	for {
		state.eventMu.Lock()
		if len(state.eventQueue) == 0 {
			state.publishing = false
			state.eventMu.Unlock()
			return
		}
		event := state.eventQueue[0]
		state.eventQueue[0] = valueobject.RunEvent{}
		state.eventQueue = state.eventQueue[1:]
		state.eventMu.Unlock()
		m.opts.Publisher.PublishRun(event)
	}
}

func nextRunEventLocked(state *runState, event valueobject.RunEvent) valueobject.RunEvent {
	state.seq++
	event.Version = 1
	event.Seq = state.seq
	event.RunID = state.scope.RunID
	event.ProjectID = state.scope.ProjectID
	event.TreeID = state.scope.TreeID
	event.ChainID = state.scope.ChainID
	event.NodeID = state.scope.NodeID
	event.ChainPath = append([]string(nil), state.scope.ChainPath...)
	return event
}

func (m *Manager) publishRecovered(projectID string, record entity.RunRecord) {
	if m.opts.Publisher == nil {
		return
	}
	m.opts.Publisher.PublishRun(valueobject.RunEvent{
		Version: 1, Seq: 1, RunID: record.ID, ProjectID: projectID, TreeID: record.TreeID, ChainID: record.ChainID,
		NodeID: record.NodeID, ChainPath: append([]string(nil), record.ChainPath...),
		Type: valueobject.EventStatus, Status: valueobject.RunInterrupted,
		Error: &valueobject.AgentError{Code: "INTERRUPTED_BY_RESTART", Message: "INTERRUPTED_BY_RESTART", Recoverable: false},
	})
}

func (s *runState) snapshot() entity.RunRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneRunRecord(s.record)
}

func (s *runState) runSnapshot() entity.RunSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	permissions := make([]valueobject.PermissionRequest, 0, len(s.permissionsByID))
	for _, request := range s.permissionsByID {
		permissions = append(permissions, clonePermissionRequest(request))
	}
	sort.Slice(permissions, func(i, j int) bool { return permissions[i].ID < permissions[j].ID })
	questions := make([]valueobject.QuestionRequest, 0, len(s.questionsByID))
	for _, request := range s.questionsByID {
		questions = append(questions, cloneQuestionRequest(request))
	}
	sort.Slice(questions, func(i, j int) bool { return questions[i].ID < questions[j].ID })
	return entity.RunSnapshot{
		RunID: s.scope.RunID, ProjectID: s.scope.ProjectID,
		Run: cloneRunRecord(s.record), LastSeq: s.seq,
		Permissions: permissions, Questions: questions,
	}
}

func cloneRunRecord(record entity.RunRecord) entity.RunRecord {
	clone := record
	clone.ChainPath = append([]string(nil), record.ChainPath...)
	if record.ExecutionSnapshot != nil {
		snapshot := record.ExecutionSnapshot.Clone()
		clone.ExecutionSnapshot = &snapshot
	}
	clone.Transcript = append([]valueobject.TranscriptItem(nil), record.Transcript...)
	return clone
}

func clonePermissionRequest(request valueobject.PermissionRequest) valueobject.PermissionRequest {
	clone := request
	clone.Paths = append([]string(nil), request.Paths...)
	return clone
}

func cloneQuestionRequest(request valueobject.QuestionRequest) valueobject.QuestionRequest {
	clone := request
	clone.Options = append([]string(nil), request.Options...)
	return clone
}

func (m *Manager) sameScope(state *runState, scope port.RunScope) bool {
	return sameScope(state.scope, scope)
}

func sameScope(a, b port.RunScope) bool {
	return a.RunID == b.RunID && a.ProjectID == b.ProjectID && a.TreeID == b.TreeID &&
		a.NodeID == b.NodeID && a.ChainID == b.ChainID && slices.Equal(a.ChainPath, b.ChainPath) &&
		slices.Equal(a.InputNodeIDs, b.InputNodeIDs) && reflect.DeepEqual(a.ExecutionSnapshot, b.ExecutionSnapshot) &&
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
	if !found {
		code = message
	}
	if code != "" && len(code) <= 64 {
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

// ErrorCode returns the stable code used for run lifecycle failures.
func ErrorCode(err error) string { return errorCode(err) }

const runSystemPrompt = "You are AudioUI's one-shot node execution agent. Work only on the active node task and its provided workspace path. Follow the output contract in the initial context. Use ask_user when a decision is required, and use reply for chat messages; submit structured output only when the output contract explicitly permits it. Never attempt to change the graph or apply an artifact."

var runBuiltinTools = []string{"Read", "Glob", "Grep", "Write", "Edit", "Bash"}

var runAllowedTools = []string{
	"mcp__audioui__get_chain", "mcp__audioui__get_graph", "mcp__audioui__get_node",
	"mcp__audioui__list_node_types", "mcp__audioui__validate", "mcp__audioui__read_artifact",
	"mcp__audioui__reply", "mcp__audioui__ask_user",
}
