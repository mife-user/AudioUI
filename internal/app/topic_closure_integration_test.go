package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/handler"
	"audio-ui/internal/infra/paths"
	"audio-ui/internal/infra/persistence"
	"audio-ui/internal/infra/workspace"
	contextservice "audio-ui/internal/service/context"
	projectservice "audio-ui/internal/service/project"
	runservice "audio-ui/internal/service/run"
	"audio-ui/internal/service/workflow"

	"go.uber.org/zap"
)

const validTopicsFixture = `{"topics":[{"title":"凌晨面包房","angle":"从夜班烘焙师看城市","hook":"凌晨四点，第一炉面包如何改变一条街？"},{"title":"旧唱片店","angle":"一张唱片的流转","hook":"这张唱片为什么被收藏了三十年？"},{"title":"雨夜末班车","angle":"末班车上的城市切片","hook":"最后一班车驶出后，城市还剩下什么？"}]}`
const invalidTopicsFixture = `{"topics":[{"title":"未达标候选","angle":"测试","hook":"测试"}]}`

func TestTopicRunAndCandidateProjectionWithControlledAgentDriver(t *testing.T) {
	layout := topicClosureTestLayout(t)
	projects := newProjectSetup(layout, zap.NewNop())
	workflowSetup := newWorkflowSetup(layout, projects.service)
	artifacts := persistence.NewArtifactStoreProvider(layout)
	builder := contextservice.NewBuilder(artifacts)
	toolFactory := &topicClosureToolFactory{}
	driver := &topicClosureDriver{
		workflow: workflowSetup.service, tools: toolFactory,
		blockNodeID: "", sendStarted: make(chan struct{}, 1), releaseSend: make(chan struct{}),
	}
	manager := runservice.New(runservice.Options{
		Projects: projects.service, Workflow: workflowSetup.service,
		Contexts: builder, Workspaces: workspace.NewRunProvider(layout),
		RunStores: persistence.NewRunStoreProvider(layout), RunConfigs: paths.NewRunConfigStore(layout),
		ToolServers: toolFactory, Permissions: topicClosurePermissionFactory{},
		DriverProvider: func(context.Context) (port.AgentDriver, error) { return driver, nil },
	})
	driver.manager = manager
	projects.service.SetRunActiveCheck(manager.IsBusy)
	defer manager.Close()
	defer driver.release()

	project, err := projects.handler.NewProject("M2-10C 隔离工程")
	if err != nil {
		t.Fatal(err)
	}
	params := entity.NodeParams{Idea: "城市夜间生活", Direction: "从普通人的日常观察", CandidateCount: 3}
	topicReceipt, err := workflowSetup.handler.AddNode(handler.AddNodeRequest{
		ProjectID: project.ID, Type: string(valueobject.NodeTypeTopic), Title: "夜间城市",
		Position: entity.Position{X: 120, Y: 80}, Params: params,
	})
	if err != nil {
		t.Fatal(err)
	}
	invalidTopicReceipt, err := workflowSetup.handler.AddNode(handler.AddNodeRequest{
		ProjectID: project.ID, Type: string(valueobject.NodeTypeTopic), Title: "无效结果验证",
		Position: entity.Position{X: 480, Y: 80}, Params: params,
	})
	if err != nil {
		t.Fatal(err)
	}
	params.Idea = "保存后的选题想法"
	if _, err := workflowSetup.handler.UpdateNodeParams(topicReceipt.NodeID, "夜间城市已保存", params, project.ID); err != nil {
		t.Fatal(err)
	}

	driver.blockNodeID = topicReceipt.NodeID
	startResult := make(chan entity.RunStartResult, 1)
	go func() { startResult <- handler.NewRunService(manager).Start(topicReceipt.NodeID, project.ID) }()
	select {
	case <-driver.sendStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("controlled AgentDriver did not reach Send")
	}
	if !manager.IsBusy() {
		t.Fatal("run did not hold the global turn gate")
	}
	busy := handler.NewRunService(manager).Start(invalidTopicReceipt.NodeID, project.ID)
	if busy.Error == nil || busy.Error.Code != "BUSY" {
		t.Fatalf("second run error = %+v, want BUSY", busy.Error)
	}
	if _, err := workflowSetup.handler.UpdateNodeParams(topicReceipt.NodeID, "运行中编辑", params, project.ID); !errors.Is(err, workflow.ErrRunningChainLocked) {
		t.Fatalf("editing the running path error = %v, want RUNNING_CHAIN_LOCKED", err)
	}
	if _, err := projects.handler.NewProject("运行中不应切换"); !errors.Is(err, projectservice.ErrRunInProgress) {
		t.Fatalf("project switch during a run error = %v, want BUSY", err)
	}

	driver.release()
	firstRun := receiveTopicClosureRun(t, startResult)
	if firstRun.Error != nil || firstRun.Run == nil {
		t.Fatalf("valid topic run result = %+v", firstRun)
	}
	waitForTopicClosure(t, func() bool { return !manager.IsBusy() })
	firstSnapshot, err := handler.NewRunService(manager).GetRunSnapshot(firstRun.Run.ID)
	if err != nil || firstSnapshot.Run.Status != valueobject.RunSucceeded || firstSnapshot.Run.PendingID != "" {
		t.Fatalf("completed topic run snapshot = %+v, err=%v", firstSnapshot, err)
	}

	pendingItems, err := workflowSetup.handler.ListPending(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	validPending := findTopicClosurePending(t, pendingItems, topicReceipt.NodeID)
	validDetail, err := workflowSetup.handler.GetPending(validPending.ID, project.ID)
	if err != nil || !bytes.Equal(validDetail.RawPayload, []byte(validTopicsFixture)) {
		t.Fatalf("valid pending detail payload mismatch: err=%v", err)
	}
	beforeApply, err := projects.service.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	appliedTopic := topicClosureNode(t, beforeApply, topicReceipt.NodeID)
	if !appliedTopic.Locked || len(appliedTopic.Rounds) != 1 {
		t.Fatalf("automatic topic result state = %+v", appliedTopic)
	}
	candidates, err := workflowSetup.handler.GetTopicCandidates(topicReceipt.NodeID, project.ID)
	if err != nil || len(candidates) != 3 {
		t.Fatalf("generated topic candidates = %+v, err=%v", candidates, err)
	}
	generatedCandidateIDs := make(map[string]bool, len(candidates))
	for index, candidate := range candidates {
		if candidate.Index != index || candidate.Title == "" || candidate.Angle == "" || candidate.Hook == "" {
			t.Fatalf("candidate[%d] = %+v", index, candidate)
		}
	}
	for _, node := range beforeApply.Nodes {
		if node.Type == valueobject.NodeTypeCandidate && node.Parent == topicReceipt.NodeID {
			generatedCandidateIDs[node.ID] = true
			if node.CandidateSource == nil || node.CandidateSource.Index < 0 || node.CandidateSource.Index >= len(candidates) ||
				node.CandidateSource.ArtifactID != appliedTopic.Rounds[0].ArtifactID {
				t.Fatalf("generated candidate provenance = %+v", node)
			}
		}
	}
	if len(generatedCandidateIDs) != len(candidates) {
		t.Fatalf("generated candidate node count = %d, want %d", len(generatedCandidateIDs), len(candidates))
	}
	if _, err := workflowSetup.handler.ApplyPending(validPending.ID, false, project.ID); !errors.Is(err, workflow.ErrPendingAlreadyApplied) {
		t.Fatalf("reapplying automatic topic result error = %v, want PENDING_ALREADY_APPLIED", err)
	}

	candidateIndex := 0
	if _, err := workflowSetup.handler.AddNode(handler.AddNodeRequest{
		ProjectID: project.ID, Type: string(valueobject.NodeTypeCandidate), Title: "手工候选",
		Parent: topicReceipt.NodeID, Position: entity.Position{X: 120, Y: 360},
	}); !errors.Is(err, workflow.ErrNodeConnectionInvalid) {
		t.Fatalf("manual candidate creation error = %v, want NODE_CONNECTION_INVALID", err)
	}
	if _, err := workflowSetup.handler.AddNode(handler.AddNodeRequest{
		ProjectID: project.ID, Type: string(valueobject.NodeTypePrompt), Title: "旧模型分支",
		Parent: topicReceipt.NodeID, ParentCandidate: &candidateIndex,
		Position: entity.Position{X: 120, Y: 360},
	}); !errors.Is(err, workflow.ErrNodeConnectionInvalid) {
		t.Fatalf("legacy topic port connection error = %v, want NODE_CONNECTION_INVALID", err)
	}

	invalidRun := handler.NewRunService(manager).Start(invalidTopicReceipt.NodeID, project.ID)
	if invalidRun.Error != nil || invalidRun.Run == nil {
		t.Fatalf("invalid topic run result = %+v", invalidRun)
	}
	waitForTopicClosure(t, func() bool { return !manager.IsBusy() })
	pendingItems, err = workflowSetup.handler.ListPending(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	invalidPending := findTopicClosurePending(t, pendingItems, invalidTopicReceipt.NodeID)
	if invalidPending.Status != valueobject.PendingInvalid {
		t.Fatalf("invalid pending status = %s", invalidPending.Status)
	}
	invalidDetail, err := workflowSetup.handler.GetPending(invalidPending.ID, project.ID)
	if err != nil || !bytes.Equal(invalidDetail.RawPayload, []byte(invalidTopicsFixture)) {
		t.Fatalf("invalid pending original payload mismatch: err=%v", err)
	}
	if _, err := workflowSetup.handler.ApplyPending(invalidPending.ID, false, project.ID); !errors.Is(err, workflow.ErrPendingInvalid) {
		t.Fatalf("applying invalid pending error = %v, want PENDING_INVALID", err)
	}
	if _, err := workflowSetup.handler.DiscardPending(invalidPending.ID, project.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := projects.handler.NewProject("候选工程切换"); err != nil {
		t.Fatal(err)
	}
	if _, err := workflowSetup.handler.GetTopicCandidates(topicReceipt.NodeID, project.ID); !errors.Is(err, workflow.ErrProjectChanged) {
		t.Fatalf("stale project candidate query error = %v, want PROJECT_CHANGED", err)
	}
}

func topicClosureTestLayout(t *testing.T) valueobject.Layout {
	t.Helper()
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	logsDir := filepath.Join(root, "logs")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return valueobject.Layout{
		Root: root, DataDir: dataDir, LogsDir: logsDir,
		WebView2Dir: filepath.Join(root, "webview2"), LogFile: filepath.Join(logsDir, "app.log"),
		AgentDir: filepath.Join(dataDir, "agent"), WorkspaceDir: filepath.Join(dataDir, "workspace", "chain_default"),
		SessionsFile: filepath.Join(dataDir, "sessions.json"), AuditFile: filepath.Join(logsDir, "audit.log"),
		ConfigFile: filepath.Join(dataDir, "config.json"), ProjectsDir: filepath.Join(dataDir, "projects"),
		RuntimeDir: filepath.Join(root, "runtime"), BackupDir: filepath.Join(dataDir, "backup", "m1"),
	}
}

func (d *topicClosureDriver) Start(_ context.Context, spec port.DriverSpec) (port.AgentSession, error) {
	return &topicClosureSession{
		driver: d, scope: d.tools.currentScope(), eventSink: spec.EventSink,
	}, nil
}

func (*topicClosureDriver) Resume(context.Context, port.DriverSpec, json.RawMessage) (port.AgentSession, error) {
	return nil, errors.New("resume is not used by the fixture")
}

func (d *topicClosureDriver) release() {
	d.releaseOnce.Do(func() { close(d.releaseSend) })
}

type topicClosureDriver struct {
	workflow    *workflow.Service
	manager     *runservice.Manager
	tools       *topicClosureToolFactory
	blockNodeID string
	sendStarted chan struct{}
	releaseSend chan struct{}
	releaseOnce sync.Once
}

type topicClosureSession struct {
	driver    *topicClosureDriver
	scope     port.RunScope
	eventSink func(valueobject.AgentEvent)
}

func (s *topicClosureSession) Send(ctx context.Context, _ string) error {
	if s.scope.NodeID == s.driver.blockNodeID {
		select {
		case s.driver.sendStarted <- struct{}{}:
		default:
		}
		select {
		case <-s.driver.releaseSend:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	payload := []byte(invalidTopicsFixture)
	if s.scope.NodeID == s.driver.blockNodeID {
		payload = []byte(validTopicsFixture)
	}
	pending, err := s.driver.workflow.SubmitRunReply(ctx, s.scope, valueobject.OutputKindTopics, payload, "fixture-"+s.scope.NodeID)
	if err != nil {
		return err
	}
	if pending.Status == valueobject.PendingPending || pending.Status == valueobject.PendingInvalid {
		if err := s.driver.manager.RecordRunPending(ctx, s.scope, pending); err != nil {
			return err
		}
	}
	s.eventSink(valueobject.AgentEvent{Type: valueobject.EventStatus, State: &valueobject.AgentState{Status: valueobject.SessionIdle}})
	return nil
}

func (s *topicClosureSession) Interrupt(context.Context) error { return nil }
func (s *topicClosureSession) Close() error                    { return nil }

type topicClosureToolFactory struct {
	mu    sync.Mutex
	scope port.RunScope
}

func (f *topicClosureToolFactory) New(string, string, port.QuestionGate, func(string)) (port.ToolServer, error) {
	return topicClosureToolServer{}, nil
}

func (f *topicClosureToolFactory) NewForRun(scope port.RunScope, _, _ string, _ port.QuestionGate, _ func(string), _ port.WorkspaceReader, _ port.RunLifecycle) (port.ToolServer, error) {
	f.mu.Lock()
	f.scope = scope
	f.mu.Unlock()
	return topicClosureToolServer{}, nil
}

func (f *topicClosureToolFactory) currentScope() port.RunScope {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.scope
}

type topicClosureToolServer struct{}

func (topicClosureToolServer) Start(context.Context) error { return nil }
func (topicClosureToolServer) Stop(context.Context) error  { return nil }
func (topicClosureToolServer) URL() string                 { return "http://127.0.0.1:1/mcp" }
func (topicClosureToolServer) Token() string               { return "test-token" }
func (topicClosureToolServer) WriteConfig(path string) error {
	return os.WriteFile(path, []byte(`{"mcpServers":{"audioui":{"type":"http","url":"http://127.0.0.1:1/mcp"}}}`), 0o600)
}

type topicClosurePermissionFactory struct{}

func (topicClosurePermissionFactory) NewRunPermissions(port.RunScope, port.ScopedPathResolver, func(valueobject.AgentEvent)) (port.RunPermissionController, error) {
	return topicClosurePermissions{}, nil
}

type topicClosurePermissions struct{}

func (topicClosurePermissions) Request(context.Context, valueobject.PermissionRequest) (valueobject.PermissionDecision, error) {
	return valueobject.DecisionDeny, nil
}
func (topicClosurePermissions) Check(context.Context, string, map[string]any) port.PolicyResult {
	return port.PolicyResult{Decision: port.PolicyAllow}
}
func (topicClosurePermissions) Ask(context.Context, valueobject.QuestionRequest) (valueobject.QuestionAnswer, error) {
	return valueobject.QuestionAnswer{}, nil
}
func (topicClosurePermissions) ResolvePermission(string, valueobject.PermissionDecision) error {
	return nil
}
func (topicClosurePermissions) ResolveQuestion(string, valueobject.QuestionAnswer) error { return nil }
func (topicClosurePermissions) SetResolvedCallback(func(string, string))                 {}
func (topicClosurePermissions) Close()                                                   {}

func receiveTopicClosureRun(t *testing.T, result <-chan entity.RunStartResult) entity.RunStartResult {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("controlled topic run did not return")
		return entity.RunStartResult{}
	}
}

func waitForTopicClosure(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if ready() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("topic closure condition did not settle")
}

func findTopicClosurePending(t *testing.T, pending []entity.Pending, nodeID string) entity.Pending {
	t.Helper()
	for _, item := range pending {
		if item.NodeID == nodeID {
			return item
		}
	}
	t.Fatalf("no pending item for node %s", nodeID)
	return entity.Pending{}
}

func topicClosureNode(t *testing.T, project entity.Project, nodeID string) entity.Node {
	t.Helper()
	for _, node := range project.Nodes {
		if node.ID == nodeID {
			return node
		}
	}
	t.Fatalf("no node %s in project %s", nodeID, project.ID)
	return entity.Node{}
}

const validScriptAClosureFixture = `{"title":"面包房脚本","scenes":[{"narration":"凌晨的第一炉面包","duration":5}]}`
const validScriptBClosureFixture = `{"title":"唱片店脚本","scenes":[{"narration":"旧唱片再次转动","duration":6}]}`
const validScriptEmptyClosureFixture = `{"title":"末班车脚本","scenes":[{"narration":"末班车驶出站台","duration":7}]}`
const invalidScriptClosureFixture = `{"title":17,"scenes":[]}`

type scriptClosureReply struct {
	kind      valueobject.OutputKind
	payload   []byte
	requestID string
}

type scriptClosureDriver struct {
	workflow *workflow.Service
	manager  *runservice.Manager
	tools    *topicClosureToolFactory

	mu      sync.Mutex
	replies map[string][]scriptClosureReply
	prompts map[string]string
}

func (d *scriptClosureDriver) Start(_ context.Context, spec port.DriverSpec) (port.AgentSession, error) {
	return &scriptClosureSession{driver: d, scope: d.tools.currentScope(), eventSink: spec.EventSink}, nil
}

func (*scriptClosureDriver) Resume(context.Context, port.DriverSpec, json.RawMessage) (port.AgentSession, error) {
	return nil, errors.New("resume is not used by the fixture")
}

func (d *scriptClosureDriver) queueReply(nodeID string, reply scriptClosureReply) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.replies[nodeID] = append(d.replies[nodeID], reply)
}

func (d *scriptClosureDriver) promptForRun(runID string) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.prompts[runID]
}

type scriptClosureSession struct {
	driver    *scriptClosureDriver
	scope     port.RunScope
	eventSink func(valueobject.AgentEvent)
}

func (s *scriptClosureSession) Send(ctx context.Context, prompt string) error {
	d := s.driver
	d.mu.Lock()
	d.prompts[s.scope.RunID] = prompt
	replies := d.replies[s.scope.NodeID]
	if len(replies) == 0 {
		d.mu.Unlock()
		return errors.New("fixture reply missing")
	}
	reply := replies[0]
	d.replies[s.scope.NodeID] = replies[1:]
	d.mu.Unlock()

	pending, err := d.workflow.SubmitRunReply(ctx, s.scope, reply.kind, reply.payload, reply.requestID)
	if err != nil {
		return err
	}
	if pending.Status == valueobject.PendingPending || pending.Status == valueobject.PendingInvalid {
		if err := d.manager.RecordRunPending(ctx, s.scope, pending); err != nil {
			return err
		}
	}
	s.eventSink(valueobject.AgentEvent{Type: valueobject.EventStatus, State: &valueobject.AgentState{Status: valueobject.SessionIdle}})
	return nil
}

func (*scriptClosureSession) Interrupt(context.Context) error { return nil }
func (*scriptClosureSession) Close() error                    { return nil }

func TestLocalScriptClosureWithControlledAgentDriver(t *testing.T) {
	layout := topicClosureTestLayout(t)
	projects := newProjectSetup(layout, zap.NewNop())
	workflowSetup := newWorkflowSetup(layout, projects.service)
	tools := &topicClosureToolFactory{}
	driver := &scriptClosureDriver{
		workflow: workflowSetup.service, tools: tools,
		replies: make(map[string][]scriptClosureReply), prompts: make(map[string]string),
	}
	manager := runservice.New(runservice.Options{
		Projects: projects.service, Workflow: workflowSetup.service,
		Contexts:   contextservice.NewBuilder(persistence.NewArtifactStoreProvider(layout)),
		Workspaces: workspace.NewRunProvider(layout), RunStores: persistence.NewRunStoreProvider(layout),
		RunConfigs: paths.NewRunConfigStore(layout), ToolServers: tools,
		Permissions:    topicClosurePermissionFactory{},
		DriverProvider: func(context.Context) (port.AgentDriver, error) { return driver, nil },
	})
	driver.manager = manager
	projects.service.SetRunActiveCheck(manager.IsBusy)
	defer manager.Close()

	project, err := projects.handler.NewProject("M2-13 本地脚本集成")
	if err != nil {
		t.Fatal(err)
	}
	runs := handler.NewRunService(manager)
	addNode := func(nodeType valueobject.NodeType, title, parent string, params entity.NodeParams) string {
		t.Helper()
		receipt, err := workflowSetup.handler.AddNode(handler.AddNodeRequest{
			ProjectID: project.ID, Type: string(nodeType), Title: title, Parent: parent, Params: params,
			Position: entity.Position{X: 120, Y: 120},
		})
		if err != nil {
			t.Fatalf("add %s node %q: %v", nodeType, title, err)
		}
		return receipt.NodeID
	}
	run := func(nodeID string) entity.RunRecord {
		t.Helper()
		result := runs.Start(nodeID, project.ID)
		if result.Error != nil || result.Run == nil {
			t.Fatalf("start %s: result=%+v", nodeID, result)
		}
		waitForTopicClosure(t, func() bool { return !manager.IsBusy() })
		record, err := runs.GetRun(result.Run.ID)
		if err != nil {
			t.Fatalf("load run %s: %v", result.Run.ID, err)
		}
		if record.Status != valueobject.RunSucceeded {
			t.Fatalf("run %s status = %s, want succeeded", record.ID, record.Status)
		}
		return record
	}

	topicID := addNode(valueobject.NodeTypeTopic, "城市夜间选题", "", entity.NodeParams{
		Idea: "观察城市夜间生活", Direction: "从普通人的日常切入", CandidateCount: 3,
	})
	driver.queueReply(topicID, scriptClosureReply{
		kind: valueobject.OutputKindTopics, payload: []byte(validTopicsFixture), requestID: "topics-m2-13",
	})
	topicRun := run(topicID)
	if topicRun.ExecutionSnapshot == nil || topicRun.ExecutionSnapshot.OutputContract.Kind != valueobject.OutputKindTopics {
		t.Fatalf("topic execution snapshot = %+v", topicRun.ExecutionSnapshot)
	}
	topicPrompt := driver.promptForRun(topicRun.ID)
	if !bytes.Contains([]byte(topicPrompt), []byte("自动生成全部 Candidate 节点")) || bytes.Contains([]byte(topicPrompt), []byte("产物须经用户应用")) {
		t.Fatalf("topic context does not describe automatic Candidate generation: %s", topicPrompt)
	}
	candidates, err := workflowSetup.handler.GetTopicCandidates(topicID, project.ID)
	if err != nil || len(candidates) != 3 {
		t.Fatalf("topic candidates = %+v, err=%v", candidates, err)
	}
	candidateIDs := make([]string, len(candidates))
	current, err := projects.service.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range current.Nodes {
		if node.Type == valueobject.NodeTypeCandidate && node.Parent == topicID && node.CandidateSource != nil {
			index := node.CandidateSource.Index
			if index < 0 || index >= len(candidateIDs) || node.CandidateSource.ArtifactID == "" {
				t.Fatalf("candidate provenance = %+v", node.CandidateSource)
			}
			candidateIDs[index] = node.ID
		}
	}
	for index, id := range candidateIDs {
		if id == "" || candidates[index].Index != index {
			t.Fatalf("candidate %d has no generated node: ids=%v candidates=%+v", index, candidateIDs, candidates)
		}
	}

	promptParams := entity.NodeParams{
		Instruction: "为候选写一段短脚本",
		OutputKind:  valueobject.OutputKindScript,
		Constraints: &entity.NodeConstraints{Scenes: 1, Duration: 30},
	}
	promptA := addNode(valueobject.NodeTypePrompt, "面包房任务", candidateIDs[0], promptParams)
	agentA := addNode(valueobject.NodeTypeAgent, "面包房 Agent", promptA, entity.NodeParams{})
	promptEmpty := addNode(valueobject.NodeTypePrompt, "空输出任务", candidateIDs[1], promptParams)
	agentEmpty := addNode(valueobject.NodeTypeAgent, "指定输出 Agent", promptEmpty, entity.NodeParams{})
	emptyScript := addNode(valueobject.NodeTypeScript, "预先指定脚本", agentEmpty, entity.NodeParams{})
	promptInvalid := addNode(valueobject.NodeTypePrompt, "非法结果任务", candidateIDs[2], promptParams)
	agentInvalid := addNode(valueobject.NodeTypeAgent, "非法结果 Agent", promptInvalid, entity.NodeParams{})
	if _, err := workflowSetup.handler.Connect(candidateIDs[0], agentA, nil, false, project.ID); !errors.Is(err, workflow.ErrNodeConnectionInvalid) {
		t.Fatalf("candidate-to-Agent connection error = %v, want NODE_CONNECTION_INVALID", err)
	}

	if result := runs.Start(promptA, project.ID); result.Error == nil || result.Error.Code != "RUN_ENTRY_UNAVAILABLE" || manager.IsBusy() {
		t.Fatalf("prompt run result = %+v, busy=%t", result, manager.IsBusy())
	}

	driver.queueReply(agentA, scriptClosureReply{
		kind: valueobject.OutputKindScript, payload: []byte(validScriptAClosureFixture), requestID: "script-a-first",
	})
	firstAgentRun := run(agentA)
	if firstAgentRun.ExecutionSnapshot == nil || firstAgentRun.ExecutionSnapshot.OutputBinding.Bound {
		t.Fatalf("unbound Agent snapshot = %+v", firstAgentRun.ExecutionSnapshot)
	}
	firstPrompt := driver.promptForRun(firstAgentRun.ID)
	if !bytes.Contains([]byte(firstPrompt), []byte("凌晨面包房")) || bytes.Contains([]byte(firstPrompt), []byte("旧唱片店")) ||
		!bytes.Contains([]byte(firstPrompt), []byte(promptParams.Instruction)) || !bytes.Contains([]byte(firstPrompt), []byte(`"duration": 30`)) ||
		!bytes.Contains([]byte(firstPrompt), []byte("自动创建直接 Script 节点")) {
		t.Fatalf("first Agent context did not isolate candidate A and preserve the prompt: %s", firstPrompt)
	}
	current, err = projects.service.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	firstOutputID := ""
	for _, node := range current.Nodes {
		if node.Type == valueobject.NodeTypeScript && node.Parent == agentA {
			firstOutputID = node.ID
		}
	}
	if firstOutputID == "" {
		t.Fatal("unbound Script reply did not create a direct output node")
	}
	firstOutput := topicClosureNode(t, current, firstOutputID)
	if len(firstOutput.ScriptVersions) != 1 || firstOutput.ScriptVersions[0].RunID != firstAgentRun.ID {
		t.Fatalf("automatically created Script versions = %+v", firstOutput.ScriptVersions)
	}
	firstArtifactID := firstOutput.ScriptVersions[0].ArtifactID

	driver.queueReply(agentEmpty, scriptClosureReply{
		kind: valueobject.OutputKindScript, payload: []byte(validScriptEmptyClosureFixture), requestID: "script-empty-first",
	})
	emptyRun := run(agentEmpty)
	if emptyRun.ExecutionSnapshot == nil || !emptyRun.ExecutionSnapshot.OutputBinding.Bound ||
		emptyRun.ExecutionSnapshot.OutputBinding.NodeID != emptyScript || emptyRun.ExecutionSnapshot.OutputBinding.BaselineVersion != 0 ||
		!bytes.Contains([]byte(driver.promptForRun(emptyRun.ID)), []byte("空 Script 节点")) ||
		!bytes.Contains([]byte(driver.promptForRun(emptyRun.ID)), []byte("首个不可变版本")) {
		t.Fatalf("empty Script binding snapshot = %+v", emptyRun.ExecutionSnapshot)
	}
	current, err = projects.service.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	emptyOutput := topicClosureNode(t, current, emptyScript)
	if len(emptyOutput.ScriptVersions) != 1 || emptyOutput.ScriptVersions[0].RunID != emptyRun.ID {
		t.Fatalf("specified empty Script versions = %+v", emptyOutput.ScriptVersions)
	}

	if _, err := workflowSetup.handler.Connect(candidateIDs[1], promptA, nil, true, project.ID); err != nil {
		t.Fatalf("reparent Prompt from candidate A to B: %v", err)
	}
	driver.queueReply(agentA, scriptClosureReply{
		kind: valueobject.OutputKindScript, payload: []byte(validScriptBClosureFixture), requestID: "script-a-second",
	})
	secondAgentRun := run(agentA)
	if secondAgentRun.ExecutionSnapshot == nil || secondAgentRun.ExecutionSnapshot.OutputBinding.NodeID != firstOutputID ||
		secondAgentRun.ExecutionSnapshot.OutputBinding.BaselineVersion != 1 {
		t.Fatalf("existing Script binding snapshot = %+v", secondAgentRun.ExecutionSnapshot)
	}
	secondPrompt := driver.promptForRun(secondAgentRun.ID)
	if !bytes.Contains([]byte(secondPrompt), []byte("旧唱片店")) || bytes.Contains([]byte(secondPrompt), []byte("凌晨面包房")) ||
		!bytes.Contains([]byte(secondPrompt), []byte(promptParams.Instruction)) || !bytes.Contains([]byte(secondPrompt), []byte("待确认结果")) ||
		!bytes.Contains([]byte(secondPrompt), []byte("追加新版本")) {
		t.Fatalf("reparented Agent context did not use candidate B and preserve the prompt: %s", secondPrompt)
	}
	if firstAgentRun.ExecutionSnapshot.ChainPath[1] != candidateIDs[0] || secondAgentRun.ExecutionSnapshot.ChainPath[1] != candidateIDs[1] {
		t.Fatalf("run snapshots changed across reparent: first=%v second=%v", firstAgentRun.ExecutionSnapshot.ChainPath, secondAgentRun.ExecutionSnapshot.ChainPath)
	}

	pendingItems, err := workflowSetup.handler.ListPending(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	var appendPending entity.Pending
	for _, item := range pendingItems {
		if item.RunID == secondAgentRun.ID {
			appendPending = item
			break
		}
	}
	if appendPending.ID == "" || appendPending.Status != valueobject.PendingPending || appendPending.ResultNodeID != firstOutputID ||
		appendPending.OutputBinding == nil || appendPending.OutputBinding.BaselineVersion != 1 ||
		appendPending.OutputBinding.BaselineArtifactID != firstArtifactID {
		t.Fatalf("existing Script pending = %+v", appendPending)
	}
	if _, err := workflowSetup.handler.ApplyPending(appendPending.ID, false, project.ID); err != nil {
		t.Fatalf("confirm Script version append: %v", err)
	}
	current, err = projects.service.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	appendedOutput := topicClosureNode(t, current, firstOutputID)
	if len(appendedOutput.ScriptVersions) != 2 || appendedOutput.ScriptVersions[0].ArtifactID != firstArtifactID ||
		appendedOutput.ScriptVersions[0].RunID != firstAgentRun.ID || appendedOutput.ScriptVersions[1].N != 2 ||
		appendedOutput.ScriptVersions[1].RunID != secondAgentRun.ID || appendedOutput.ScriptVersions[1].ArtifactID == firstArtifactID {
		t.Fatalf("confirmed Script versions = %+v", appendedOutput.ScriptVersions)
	}

	branchA := addNode(valueobject.NodeTypePrompt, "脚本后续分支 A", firstOutputID, entity.NodeParams{Instruction: "后续方向 A"})
	branchB := addNode(valueobject.NodeTypePrompt, "脚本后续分支 B", firstOutputID, entity.NodeParams{Instruction: "后续方向 B"})
	if branchA == branchB {
		t.Fatalf("script branches share node ID %q", branchA)
	}

	driver.queueReply(agentInvalid, scriptClosureReply{
		kind: valueobject.OutputKindScript, payload: []byte(invalidScriptClosureFixture), requestID: "script-invalid",
	})
	invalidRun := run(agentInvalid)
	pendingItems, err = workflowSetup.handler.ListPending(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	var invalidPending entity.Pending
	for _, item := range pendingItems {
		if item.RunID == invalidRun.ID {
			invalidPending = item
			break
		}
	}
	invalidDetail, detailErr := workflowSetup.handler.GetPending(invalidPending.ID, project.ID)
	if invalidPending.ID == "" || invalidPending.Status != valueobject.PendingInvalid || detailErr != nil ||
		!bytes.Equal(invalidDetail.RawPayload, []byte(invalidScriptClosureFixture)) {
		t.Fatalf("invalid Script reply evidence = %+v detail=%+v err=%v", invalidPending, invalidDetail, detailErr)
	}
	current, err = projects.service.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range current.Nodes {
		if node.Type == valueobject.NodeTypeScript && node.Parent == agentInvalid {
			t.Fatalf("invalid Script reply changed graph: %+v", node)
		}
	}
	if len(current.Nodes) == 0 || current.Version != valueobject.ProjectFileVersion {
		t.Fatalf("project state = version %d with %d nodes", current.Version, len(current.Nodes))
	}

	if manager.IsBusy() {
		t.Fatal("controlled runs left the global turn gate occupied")
	}
	runHistory, err := runs.ListRuns()
	if err != nil || len(runHistory) != 5 {
		t.Fatalf("run history after Script outputs = %d, err=%v; outputs must not run downstream automatically", len(runHistory), err)
	}

	restarted := newProjectSetup(layout, zap.NewNop())
	persisted, err := restarted.service.Current(context.Background())
	if err != nil {
		t.Fatalf("reload v2 project after controlled runs: %v", err)
	}
	persistedOutput := topicClosureNode(t, persisted, firstOutputID)
	if len(persistedOutput.ScriptVersions) != 2 || persistedOutput.ScriptVersions[0].ArtifactID != firstArtifactID ||
		persistedOutput.ScriptVersions[0].RunID != firstAgentRun.ID || persistedOutput.ScriptVersions[1].RunID != secondAgentRun.ID ||
		topicClosureNode(t, persisted, emptyScript).ScriptVersions[0].RunID != emptyRun.ID {
		t.Fatalf("reloaded Script versions = %+v", persistedOutput.ScriptVersions)
	}
	persistedGraph, err := entity.NewGraph(&persisted)
	if err != nil || persistedGraph.Validate() != nil {
		t.Fatalf("reloaded graph validation: graph=%v err=%v", persistedGraph, err)
	}
	artifactStore, err := persistence.NewArtifactStoreProvider(layout).ForProject(context.Background(), project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, payload, err := artifactStore.Get(context.Background(), firstArtifactID); err != nil || !bytes.Contains(payload, []byte("面包房脚本")) {
		t.Fatalf("original immutable Script artifact missing after reload: err=%v payload=%s", err, payload)
	}
	runStore, err := persistence.NewRunStoreProvider(layout).ForProject(context.Background(), project.ID)
	if err != nil {
		t.Fatal(err)
	}
	persistedFirstRun, err := runStore.Load(context.Background(), firstAgentRun.ID)
	if err != nil || persistedFirstRun.ExecutionSnapshot == nil || persistedFirstRun.ExecutionSnapshot.ChainPath[1] != candidateIDs[0] {
		t.Fatalf("original run snapshot after reparent = %+v err=%v", persistedFirstRun.ExecutionSnapshot, err)
	}
	persistedSecondRun, err := runStore.Load(context.Background(), secondAgentRun.ID)
	if err != nil || persistedSecondRun.ExecutionSnapshot == nil || persistedSecondRun.ExecutionSnapshot.ChainPath[1] != candidateIDs[1] {
		t.Fatalf("reparented run snapshot = %+v err=%v", persistedSecondRun.ExecutionSnapshot, err)
	}
}
