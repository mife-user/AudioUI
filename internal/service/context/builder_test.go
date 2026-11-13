package context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
)

type artifactItem struct {
	kind    valueobject.OutputKind
	payload []byte
}

type memoryArtifacts struct {
	items map[string]artifactItem
	gets  []string
}

func (s *memoryArtifacts) add(kind valueobject.OutputKind, payload string) string {
	id := hashBytes([]byte(payload))
	s.items[id] = artifactItem{kind: kind, payload: []byte(payload)}
	return id
}

func (s *memoryArtifacts) Put(context.Context, []byte, entity.ArtifactMeta) (entity.Artifact, error) {
	return entity.Artifact{}, errors.New("unexpected Put")
}
func (s *memoryArtifacts) Get(ctx context.Context, id string) (entity.Artifact, []byte, error) {
	if err := ctx.Err(); err != nil {
		return entity.Artifact{}, nil, err
	}
	s.gets = append(s.gets, id)
	item, ok := s.items[id]
	if !ok {
		return entity.Artifact{}, nil, errors.New("artifact missing")
	}
	return entity.Artifact{ID: id, Kind: item.kind}, append([]byte(nil), item.payload...), nil
}
func (s *memoryArtifacts) Exists(context.Context, string) (bool, error) {
	return false, errors.New("unexpected Exists")
}

type memoryProvider struct{ store *memoryArtifacts }

func (p *memoryProvider) ForProject(ctx context.Context, projectID string) (port.ArtifactStore, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if projectID != "p_test" {
		return nil, fmt.Errorf("unexpected project %s", projectID)
	}
	return p.store, nil
}

type memoryWorkspace struct {
	scope   port.WorkspaceScope
	dirs    map[string]port.WorkspaceList
	sources []port.WorkspaceSource
	reads   int
}

func (w *memoryWorkspace) Scope() port.WorkspaceScope { return w.scope }
func (w *memoryWorkspace) CWD() string                { return "" }
func (w *memoryWorkspace) RuntimeReadPath() string    { return "../../runtime" }
func (w *memoryWorkspace) InputSources() []port.WorkspaceSource {
	return append([]port.WorkspaceSource(nil), w.sources...)
}
func (w *memoryWorkspace) Resolver() port.ScopedPathResolver { return nil }
func (w *memoryWorkspace) ListDir(ctx context.Context, path string, maxEntries int) (port.WorkspaceList, error) {
	if err := ctx.Err(); err != nil {
		return port.WorkspaceList{}, err
	}
	listing, ok := w.dirs[path]
	if !ok {
		return port.WorkspaceList{}, fmt.Errorf("missing directory %s", path)
	}
	listing.Entries = append([]port.WorkspaceEntry(nil), listing.Entries...)
	if len(listing.Entries) > maxEntries {
		listing.Entries = listing.Entries[:maxEntries]
		listing.Truncated = true
	}
	return listing, nil
}
func (w *memoryWorkspace) ReadFile(context.Context, string, int64) (port.WorkspaceRead, error) {
	w.reads++
	return port.WorkspaceRead{}, errors.New("workspace file must not be read")
}
func (w *memoryWorkspace) Grep(context.Context, string, string, port.WorkspaceGrepOptions) (port.WorkspaceGrep, error) {
	w.reads++
	return port.WorkspaceGrep{}, errors.New("workspace contents must not be searched")
}
func (w *memoryWorkspace) Stat(context.Context, string) (port.WorkspaceStat, error) {
	w.reads++
	return port.WorkspaceStat{}, errors.New("workspace contents must not be inspected")
}

type fixture struct {
	builder   *Builder
	store     *memoryArtifacts
	workspace *memoryWorkspace
	input     Input
	topicID   string
	latestID  string
	targetID  string
	siblingID string
}

func newFixture() *fixture {
	store := &memoryArtifacts{items: make(map[string]artifactItem)}
	topicID := store.add(valueobject.OutputKindTopics, `{"topics":[{"title":"候选甲","angle":"角度甲","hook":"开头甲"},{"title":"其他候选乙","angle":"角度乙","hook":"开头乙"},{"title":"其他候选丙","angle":"角度丙","hook":"开头丙"}]}`)
	oldID := store.add(valueobject.OutputKindScript, `{"title":"旧祖先轮次","scenes":[{"narration":"旧正文","duration":5}]}`)
	latestID := store.add(valueobject.OutputKindScript, `{"title":"最新祖先轮次","scenes":[{"narration":"祖先正文","duration":8}]}`)
	targetID := store.add(valueobject.OutputKindScript, `{"title":"目标自己的历史","scenes":[{"narration":"目标历史正文","duration":9}]}`)
	siblingID := store.add(valueobject.OutputKindScript, `{"title":"兄弟分支正文","scenes":[{"narration":"兄弟秘密","duration":6}]}`)
	zero, one := 0, 1
	project := entity.Project{Version: 1, ID: "p_test", Name: "工程", Nodes: []entity.Node{
		{ID: "n_topic", Type: valueobject.NodeTypeTopic, Title: "选题", Params: entity.NodeParams{Idea: "节奏", CandidateCount: 3}, Rounds: []entity.Round{{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: topicID}}, Locked: true},
		{ID: "n_mid", Type: valueobject.NodeTypePrompt, Title: "上游脚本", Parent: "n_topic", ParentCandidate: &zero, Params: entity.NodeParams{Instruction: "撰写上游", OutputKind: valueobject.OutputKindScript}, Rounds: []entity.Round{{N: 1, Kind: valueobject.OutputKindScript, ArtifactID: oldID}, {N: 2, Kind: valueobject.OutputKindScript, ArtifactID: latestID}}, Locked: true},
		{ID: "n_target", Type: valueobject.NodeTypePrompt, Title: "目标标题", Parent: "n_mid", Params: entity.NodeParams{Instruction: "继续写中文脚本", OutputKind: valueobject.OutputKindScript, Constraints: &entity.NodeConstraints{Scenes: 3, Duration: 90}}, Rounds: []entity.Round{{N: 1, Kind: valueobject.OutputKindScript, ArtifactID: targetID}}},
		{ID: "n_child", Type: valueobject.NodeTypePrompt, Title: "子节点秘密", Parent: "n_target", Params: entity.NodeParams{Instruction: "子节点正文"}},
		{ID: "n_sibling", Type: valueobject.NodeTypePrompt, Title: "兄弟秘密", Parent: "n_topic", ParentCandidate: &one, Params: entity.NodeParams{Instruction: "兄弟指令"}, Rounds: []entity.Round{{N: 1, Kind: valueobject.OutputKindScript, ArtifactID: siblingID}}},
		{ID: "n_other", Type: valueobject.NodeTypeTopic, Title: "其他树秘密", Params: entity.NodeParams{Idea: "其他树正文"}},
		{ID: "n_draft", Type: valueobject.NodeTypePrompt, Title: "草稿"},
	}}
	workspace := &memoryWorkspace{
		scope: port.WorkspaceScope{ProjectID: "p_test", TreeID: "n_topic", NodeID: "n_target", ChainPath: []string{"n_topic", "n_mid", "n_target"}},
		dirs: map[string]port.WorkspaceList{
			".":    {Entries: []port.WorkspaceEntry{{Name: "z.bin", Size: 12}, {Name: "docs", Dir: true}, {Name: "a.txt", Size: 3}}},
			"docs": {Entries: []port.WorkspaceEntry{{Name: "note.md", Size: 8}}},
		},
	}
	return &fixture{builder: NewBuilder(&memoryProvider{store: store}), store: store, workspace: workspace,
		input: Input{Project: project, NodeID: "n_target", Workspace: workspace}, topicID: topicID, latestID: latestID, targetID: targetID, siblingID: siblingID}
}

func fixtureNode(t *testing.T, project *entity.Project, id string) *entity.Node {
	t.Helper()
	for i := range project.Nodes {
		if project.Nodes[i].ID == id {
			return &project.Nodes[i]
		}
	}
	t.Fatalf("missing fixture node %s", id)
	return nil
}

func decodeBody(t *testing.T, result Result) (contextBody, promptMetadata) {
	t.Helper()
	var envelope promptEnvelope
	if err := json.Unmarshal([]byte(result.InitialPrompt), &envelope); err != nil {
		t.Fatal(err)
	}
	var body contextBody
	if err := json.Unmarshal(envelope.Context, &body); err != nil {
		t.Fatal(err)
	}
	return body, envelope.Metadata
}

func snapshotFixture() (*Builder, *memoryArtifacts, entity.Project, entity.RunExecutionSnapshot, *memoryWorkspace, string, string) {
	store := &memoryArtifacts{items: make(map[string]artifactItem)}
	topicID := store.add(valueobject.OutputKindTopics, `{"topics":[{"title":"候选甲","angle":"甲角度","hook":"甲开头"},{"title":"候选乙","angle":"乙角度","hook":"乙开头"},{"title":"候选丙","angle":"丙角度","hook":"丙开头"}]}`)
	scriptID := store.add(valueobject.OutputKindScript, `{"title":"祖先脚本版本","scenes":[{"narration":"不可变祖先正文","duration":12}]}`)
	baselineID := store.add(valueobject.OutputKindScript, `{"title":"绑定脚本已有版本","scenes":[{"narration":"不应进入上下文","duration":15}]}`)
	project := entity.Project{
		Version: valueobject.ProjectFileVersion, ID: "p_test", Name: "快照工程", Revision: 9,
		Nodes: []entity.Node{
			{ID: "n_topic", Type: valueobject.NodeTypeTopic, Title: "选题", WorkspaceRootID: "n_topic", Params: entity.NodeParams{Idea: "快照主题"}, Rounds: []entity.Round{{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: topicID, RunID: "r_topic"}}},
			{ID: "n_candidate", Type: valueobject.NodeTypeCandidate, Title: "候选乙", WorkspaceRootID: "n_topic", CandidateSource: &entity.CandidateSource{TopicNodeID: "n_topic", Round: 1, ArtifactID: topicID, Index: 1, RunID: "r_topic"}},
			{ID: "n_prompt_old", Type: valueobject.NodeTypePrompt, Title: "旧任务", WorkspaceRootID: "n_topic", Params: entity.NodeParams{Instruction: "旧任务约束", OutputKind: valueobject.OutputKindScript}},
			{ID: "n_agent_old", Type: valueobject.NodeTypeAgent, Title: "旧 Agent", WorkspaceRootID: "n_topic"},
			{ID: "n_script", Type: valueobject.NodeTypeScript, Title: "祖先脚本", WorkspaceRootID: "n_topic", ScriptVersions: []entity.ScriptVersion{{N: 1, ArtifactID: scriptID, RunID: "r_previous"}}},
			{ID: "n_prompt", Type: valueobject.NodeTypePrompt, Title: "当前任务", WorkspaceRootID: "n_topic", Params: entity.NodeParams{Instruction: "完整的当前任务", OutputKind: valueobject.OutputKindScript, Constraints: &entity.NodeConstraints{Scenes: 4, Duration: 120}}},
			{ID: "n_agent", Type: valueobject.NodeTypeAgent, Title: "当前 Agent", WorkspaceRootID: "n_topic"},
			{ID: "n_output", Type: valueobject.NodeTypeScript, Title: "当前输出", WorkspaceRootID: "n_topic", ScriptVersions: []entity.ScriptVersion{{N: 1, ArtifactID: baselineID, RunID: "r_existing"}}},
		},
		Edges: []entity.Edge{
			{ID: "e_n_candidate", SourceNodeID: "n_topic", TargetNodeID: "n_candidate", Kind: entity.EdgeKindCandidateSource},
			{ID: "e_n_prompt_old", SourceNodeID: "n_candidate", TargetNodeID: "n_prompt_old", Kind: entity.EdgeKindConnection},
			{ID: "e_n_agent_old", SourceNodeID: "n_prompt_old", TargetNodeID: "n_agent_old", Kind: entity.EdgeKindConnection},
			{ID: "e_n_script", SourceNodeID: "n_agent_old", TargetNodeID: "n_script", Kind: entity.EdgeKindConnection},
			{ID: "e_n_prompt", SourceNodeID: "n_script", TargetNodeID: "n_prompt", Kind: entity.EdgeKindConnection, ScriptVersion: 1},
			{ID: "e_n_agent", SourceNodeID: "n_prompt", TargetNodeID: "n_agent", Kind: entity.EdgeKindConnection},
			{ID: "e_n_output", SourceNodeID: "n_agent", TargetNodeID: "n_output", Kind: entity.EdgeKindConnection},
		},
	}
	graph, _ := entity.NewGraph(&project)
	snapshot, _ := graph.SnapshotForRun("n_agent")
	workspace := &memoryWorkspace{
		scope: port.WorkspaceScope{
			ProjectID: "p_test", TreeID: snapshot.TreeID, NodeID: "n_agent",
			ChainPath: append([]string(nil), snapshot.ChainPath...), InputNodeIDs: snapshot.ScopeNodeIDs(),
		},
		dirs: map[string]port.WorkspaceList{".": {Entries: []port.WorkspaceEntry{{Name: "notes.md", Size: 5}}}},
		sources: []port.WorkspaceSource{
			{LogicalPath: "inputs/n_topic/n_candidate/", PhysicalPath: "C:/project/inputs/n_topic/n_candidate"},
			{LogicalPath: "inputs/n_topic/n_script/", PhysicalPath: "C:/project/inputs/n_topic/n_script"},
		},
	}
	return NewBuilder(&memoryProvider{store: store}), store, project, snapshot, workspace, scriptID, baselineID
}

func TestDAGContextUsesOrderedTasksAndIsolatedWorkspaceSources(t *testing.T) {
	project := entity.Project{
		Version: valueobject.ProjectFileVersion, ID: "p_test", Name: "DAG context", Revision: 1,
		Nodes: []entity.Node{
			{ID: "n_task_a", Type: valueobject.NodeTypePrompt, Title: "任务 A", Params: entity.NodeParams{Instruction: "先研究"}},
			{ID: "n_task_b", Type: valueobject.NodeTypePrompt, Title: "任务 B", Params: entity.NodeParams{Instruction: "再撰写"}},
			{ID: "n_agent", Type: valueobject.NodeTypeAgent, Title: "Agent"},
		},
		Edges: []entity.Edge{
			{ID: "e_task_a", SourceNodeID: "n_task_a", TargetNodeID: "n_agent", Kind: entity.EdgeKindConnection},
			{ID: "e_task_b", SourceNodeID: "n_task_b", TargetNodeID: "n_agent", Kind: entity.EdgeKindConnection},
		},
	}
	graph, err := entity.NewGraph(&project)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := graph.SnapshotForRunWithInputs("n_agent", []string{"e_task_b", "e_task_a"})
	if err != nil {
		t.Fatal(err)
	}
	sources := []port.WorkspaceSource{
		{LogicalPath: "inputs/project/n_task_b/", PhysicalPath: "C:/project/workspace/steps/n_task_b"},
		{LogicalPath: "inputs/project/n_task_a/", PhysicalPath: "C:/project/workspace/steps/n_task_a"},
	}
	workspace := &memoryWorkspace{
		scope: port.WorkspaceScope{
			ProjectID: project.ID, NodeID: snapshot.NodeID, InputNodeIDs: snapshot.ScopeNodeIDs(),
		},
		dirs:    map[string]port.WorkspaceList{".": {Path: ".", Entries: []port.WorkspaceEntry{}}},
		sources: sources,
	}
	builder := NewBuilder(nil)
	result, err := builder.Build(context.Background(), Input{
		Project: project, NodeID: snapshot.NodeID, Snapshot: snapshot, Workspace: workspace, Budget: 5000,
	})
	if err != nil {
		t.Fatal(err)
	}
	var envelope promptEnvelope
	if err := json.Unmarshal([]byte(result.InitialPrompt), &envelope); err != nil {
		t.Fatal(err)
	}
	var body contextBody
	if err := json.Unmarshal(envelope.Context, &body); err != nil {
		t.Fatal(err)
	}
	if result.ContextVersion != 2 || result.ChainPath != nil || !slices.Equal(result.InputNodeIDs, snapshot.ScopeNodeIDs()) ||
		body.Workspace.CWD != "workspace/steps/n_agent/" || len(body.Tasks) != 2 ||
		body.Tasks[0].NodeID != "n_task_b" || body.Tasks[1].NodeID != "n_task_a" ||
		!slices.Equal(body.Workspace.NativeReadSources, sources) || len(body.Contract) == 0 || !strings.Contains(body.Contract[0], "已选输入") {
		t.Fatalf("DAG context result=%+v body=%+v", result, body)
	}

	reordered, err := graph.SnapshotForRunWithInputs("n_agent", []string{"e_task_a", "e_task_b"})
	if err != nil {
		t.Fatal(err)
	}
	firstHashes, err := builder.InputHashesForSnapshot(context.Background(), project, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	secondHashes, err := builder.InputHashesForSnapshot(context.Background(), project, reordered)
	if err != nil {
		t.Fatal(err)
	}
	if firstHashes.AncestorHash == secondHashes.AncestorHash || firstHashes.ParamsHash == secondHashes.ParamsHash {
		t.Fatalf("input order did not affect hashes: first=%+v second=%+v", firstHashes, secondHashes)
	}
}

func TestSnapshotContextUsesFrozenCandidateTaskScriptAndOutputBinding(t *testing.T) {
	builder, store, project, snapshot, workspace, scriptID, baselineID := snapshotFixture()
	for _, node := range project.Nodes {
		if node.Type == valueobject.NodeTypePrompt && len(node.Rounds) != 0 {
			t.Fatalf("prompt %s unexpectedly requires applied rounds", node.ID)
		}
	}
	input := Input{Project: project, NodeID: "n_agent", Snapshot: snapshot, Workspace: workspace}
	result, err := builder.Build(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(store.gets, []string{snapshot.Artifacts[0].ArtifactID, scriptID}) {
		t.Fatalf("snapshot artifact reads = %v", store.gets)
	}
	body, _ := decodeBody(t, result)
	if body.Target.NodeType != valueobject.NodeTypeAgent || body.Target.OutputKind != valueobject.OutputKindScript || body.TaskNodeID != "n_prompt" {
		t.Fatalf("target/task contract = %+v task=%s", body.Target, body.TaskNodeID)
	}
	if body.Task.Instruction != "完整的当前任务" || body.Task.Constraints == nil || *body.Task.Constraints != (entity.NodeConstraints{Scenes: 4, Duration: 120}) {
		t.Fatalf("task params = %+v", body.Task)
	}
	if body.OutputBinding == nil || !body.OutputBinding.Bound || body.OutputBinding.NodeID != "n_output" || body.OutputBinding.BaselineVersion != 1 || body.OutputBinding.BaselineArtifactID != baselineID {
		t.Fatalf("output binding = %+v", body.OutputBinding)
	}
	if body.OutputContract == nil || body.OutputContract.Kind != valueobject.OutputKindScript || body.OutputContract.SchemaVersion != 1 || !strings.Contains(result.InitialPrompt, "script@1") || !strings.Contains(result.InitialPrompt, "创建待确认结果") || !strings.Contains(result.InitialPrompt, "追加新版本") {
		t.Fatalf("output contract = %+v", body.OutputContract)
	}
	if len(body.Sources) != 2 || !strings.Contains(string(body.Sources[0].Payload), "候选乙") || body.Sources[0].CandidateNumber == nil || *body.Sources[0].CandidateNumber != 2 ||
		!strings.Contains(string(body.Sources[1].Payload), "不可变祖先正文") || body.Sources[1].ArtifactVersion != 1 || body.Sources[1].Round != 0 {
		t.Fatalf("snapshot sources = %+v", body.Sources)
	}
	for _, forbidden := range []string{"候选甲", "候选丙", "不应进入上下文", "旧任务约束"} {
		if strings.Contains(result.InitialPrompt, forbidden) {
			t.Errorf("snapshot context leaked %q", forbidden)
		}
	}
	if body.Workspace.NativeWriteRoot != "steps/n_agent/" || len(body.Workspace.NativeReadSources) != 2 ||
		body.Workspace.NativeReadSources[0].LogicalPath != "inputs/n_topic/n_candidate/" || body.Workspace.NativeReadSources[1].LogicalPath != "inputs/n_topic/n_script/" ||
		!slices.Contains(body.Workspace.NativeReadRoots, workspace.RuntimeReadPath()) {
		t.Fatalf("native file scope = %+v", body.Workspace)
	}
	degraded := false
	for budget := result.Estimate.EstimatedTokens - 1; budget > 0; budget-- {
		budgetInput := input
		budgetInput.Budget = budget
		degradedResult, buildErr := builder.Build(context.Background(), budgetInput)
		if errors.Is(buildErr, ErrContextTooLarge) {
			break
		}
		if buildErr != nil {
			t.Fatal(buildErr)
		}
		degradedBody, _ := decodeBody(t, degradedResult)
		if degradedBody.Sources[0].Representation != "full" || degradedBody.Sources[1].Representation != "full" {
			if degradedBody.Task.Instruction != "完整的当前任务" || degradedBody.Task.Constraints == nil || degradedBody.Task.Constraints.Scenes != 4 {
				t.Fatalf("budget degradation changed current task: %+v", degradedBody.Task)
			}
			degraded = true
			break
		}
	}
	if !degraded {
		t.Fatal("ancestor artifacts were not degraded under a smaller budget")
	}

	candidateProject := project.Clone()
	candidate := fixtureNode(t, &candidateProject, "n_candidate")
	candidate.CandidateSource.Index = 0
	candidate.Title = "候选甲"
	candidateGraph, err := entity.NewGraph(&candidateProject)
	if err != nil {
		t.Fatal(err)
	}
	candidateSnapshot, err := candidateGraph.SnapshotForRun("n_agent")
	if err != nil {
		t.Fatal(err)
	}
	candidateResult, err := builder.Build(context.Background(), Input{
		Project: candidateProject, NodeID: "n_agent", Snapshot: candidateSnapshot, Workspace: workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	candidateBody, _ := decodeBody(t, candidateResult)
	if !strings.Contains(string(candidateBody.Sources[0].Payload), "候选甲") || strings.Contains(candidateResult.InitialPrompt, "候选乙") || candidateResult.Hashes.AncestorHash == result.Hashes.AncestorHash {
		t.Fatalf("new candidate snapshot was not isolated: %+v", candidateBody.Sources[0])
	}

	bindingProject := project.Clone()
	output := fixtureNode(t, &bindingProject, "n_output")
	output.ScriptVersions = append(output.ScriptVersions, entity.ScriptVersion{N: 2, ArtifactID: "sha256:baseline-v2", RunID: "r_new"})
	bindingGraph, err := entity.NewGraph(&bindingProject)
	if err != nil {
		t.Fatal(err)
	}
	bindingSnapshot, err := bindingGraph.SnapshotForRun("n_agent")
	if err != nil {
		t.Fatal(err)
	}
	bindingResult, err := builder.Build(context.Background(), Input{
		Project: bindingProject, NodeID: "n_agent", Snapshot: bindingSnapshot, Workspace: workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	bindingBody, _ := decodeBody(t, bindingResult)
	if bindingBody.OutputBinding == nil || bindingBody.OutputBinding.BaselineVersion != 2 || bindingResult.Hashes.ParamsHash == result.Hashes.ParamsHash {
		t.Fatalf("new output binding snapshot was not tracked: %+v", bindingBody.OutputBinding)
	}

	originalPrompt := result.InitialPrompt
	fixtureNode(t, &input.Project, "n_candidate").CandidateSource.Index = 0
	fixtureNode(t, &input.Project, "n_prompt").Params.Instruction = "实时图已变化"
	fixtureNode(t, &input.Project, "n_script").ScriptVersions[0].ArtifactID = baselineID
	changedProject, err := builder.Build(context.Background(), input)
	if err != nil || changedProject.InitialPrompt != originalPrompt {
		t.Fatalf("live graph changes altered frozen context: err=%v", err)
	}

	delete(store.items, scriptID)
	if _, err := builder.Build(context.Background(), input); !errors.Is(err, ErrAncestorNotReady) {
		t.Fatalf("missing snapshot script artifact error = %v, want ANCESTOR_NOT_READY", err)
	}
}

func TestSnapshotContextRejectsMismatchedScopeAndArtifactReferences(t *testing.T) {
	builder, _, project, snapshot, workspace, _, _ := snapshotFixture()
	input := Input{Project: project, NodeID: "n_agent", Snapshot: snapshot, Workspace: workspace}
	badScope := *workspace
	badScope.scope.NodeID = "n_other"
	input.Workspace = &badScope
	if _, err := builder.Build(context.Background(), input); !errors.Is(err, ErrWorkspaceScope) {
		t.Fatalf("mismatched workspace error = %v", err)
	}
	input.Workspace = workspace
	input.Snapshot.Artifacts[0].CandidateIndex = nil
	if _, err := builder.Build(context.Background(), input); !errors.Is(err, entity.ErrGraphInvalid) {
		t.Fatalf("invalid snapshot artifact reference error = %v", err)
	}
}

func TestContextGoldenAndPathIsolation(t *testing.T) {
	f := newFixture()
	got, err := f.builder.Build(context.Background(), f.input)
	if err != nil {
		t.Fatal(err)
	}
	if f.workspace.reads != 0 {
		t.Fatalf("read workspace contents %d times", f.workspace.reads)
	}
	if !reflect.DeepEqual(f.store.gets, []string{f.topicID, f.latestID}) {
		t.Fatalf("artifact reads = %v", f.store.gets)
	}
	for _, forbidden := range []string{"其他候选乙", "其他候选丙", "旧祖先轮次", "目标自己的历史", "兄弟秘密", "子节点秘密", "其他树秘密"} {
		if strings.Contains(got.InitialPrompt, forbidden) {
			t.Errorf("context leaked %q", forbidden)
		}
	}
	for _, required := range []string{"候选甲", "最新祖先轮次", "继续写中文脚本", "a.txt", "docs/note.md", "z.bin"} {
		if !strings.Contains(got.InitialPrompt, required) {
			t.Errorf("context missing %q", required)
		}
	}
	body, meta := decodeBody(t, got)
	if len(body.Path) != 2 || body.Path[0].CandidateNumber == nil || *body.Path[0].CandidateNumber != 1 || body.Path[1].Round != 2 {
		t.Fatalf("unexpected path: %+v", body.Path)
	}
	if got.Estimate.EstimatedTokens != EstimateTokens(got.InitialPrompt) || meta.EstimatedTokens != got.Estimate.EstimatedTokens || meta.ContextHash != got.Hashes.ContextHash {
		t.Fatalf("metadata/estimate mismatch: %+v %+v", meta, got.Estimate)
	}
	canonical, err := json.Marshal(body)
	if err != nil || hashBytes(canonical) != got.Hashes.ContextHash {
		t.Fatalf("context hash mismatch: %v", err)
	}
	golden := filepath.Join("testdata", "context.golden")
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if got.InitialPrompt+"\n" != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Fatalf("context differs from %s\n%s", golden, got.InitialPrompt)
	}
}

func TestCandidateSelectionAndSharedCandidate(t *testing.T) {
	f := newFixture()
	first, err := f.builder.Build(context.Background(), f.input)
	if err != nil {
		t.Fatal(err)
	}
	firstBody, _ := decodeBody(t, first)
	one := 1
	fixtureNode(t, &f.input.Project, "n_mid").ParentCandidate = &one
	second, err := f.builder.Build(context.Background(), f.input)
	if err != nil {
		t.Fatal(err)
	}
	secondBody, _ := decodeBody(t, second)
	if secondBody.Path[0].CandidateNumber == nil || *secondBody.Path[0].CandidateNumber != 2 || !strings.Contains(string(secondBody.Path[0].Payload), "其他候选乙") {
		t.Fatalf("wrong selected candidate: %+v", secondBody.Path[0])
	}
	if strings.Contains(second.InitialPrompt, "候选甲") || first.Hashes.AncestorHash == second.Hashes.AncestorHash || first.Hashes.ContextHash == second.Hashes.ContextHash || first.Hashes.ParamsHash != second.Hashes.ParamsHash {
		t.Fatal("candidate change was not isolated in context/hash")
	}
	zero := 0
	fixtureNode(t, &f.input.Project, "n_sibling").ParentCandidate = &zero
	f.input.NodeID = "n_sibling"
	f.workspace.scope.NodeID = "n_sibling"
	f.workspace.scope.ChainPath = []string{"n_topic", "n_sibling"}
	third, err := f.builder.Build(context.Background(), f.input)
	if err != nil {
		t.Fatal(err)
	}
	thirdBody, _ := decodeBody(t, third)
	if string(firstBody.Path[0].Payload) != string(thirdBody.Path[0].Payload) || *thirdBody.Path[0].CandidateNumber != 1 {
		t.Fatal("branches using the same candidate received different candidate content")
	}
}

func TestHashesIgnoreUnrelatedStateButTrackRelevantInputs(t *testing.T) {
	f := newFixture()
	original, err := f.builder.Build(context.Background(), f.input)
	if err != nil {
		t.Fatal(err)
	}
	f.input.Project.Revision++
	f.input.Project.ActiveNodeID = "n_sibling"
	fixtureNode(t, &f.input.Project, "n_mid").Position.X = 999
	fixtureNode(t, &f.input.Project, "n_mid").Size = entity.NodeSize{Width: 400, Height: 300}
	fixtureNode(t, &f.input.Project, "n_target").Title = "目标新标题"
	fixtureNode(t, &f.input.Project, "n_target").Rounds = nil
	fixtureNode(t, &f.input.Project, "n_sibling").Params.Instruction = "无关变化"
	same, err := f.builder.Build(context.Background(), f.input)
	if err != nil || same.Hashes != original.Hashes || same.InitialPrompt != original.InitialPrompt {
		t.Fatalf("unrelated state changed context: %+v %v", same.Hashes, err)
	}
	hashInput := f.input
	hashInput.Workspace = nil
	hashInput.Budget = -1
	recomputed, err := f.builder.InputHashes(context.Background(), hashInput)
	if err != nil || recomputed.AncestorHash != original.Hashes.AncestorHash || recomputed.ParamsHash != original.Hashes.ParamsHash || recomputed.ContextHash != "" {
		t.Fatalf("input hashes: %+v %v", recomputed, err)
	}
	files := f.workspace.dirs["."]
	files.Entries[0].Size++
	f.workspace.dirs["."] = files
	workspaceChanged, err := f.builder.Build(context.Background(), f.input)
	if err != nil || workspaceChanged.Hashes.ContextHash == original.Hashes.ContextHash || workspaceChanged.Hashes.AncestorHash != original.Hashes.AncestorHash || workspaceChanged.Hashes.ParamsHash != original.Hashes.ParamsHash {
		t.Fatalf("workspace hash behavior: %+v %v", workspaceChanged.Hashes, err)
	}
	fixtureNode(t, &f.input.Project, "n_target").Params.Instruction = "新任务"
	paramsChanged, err := f.builder.Build(context.Background(), f.input)
	if err != nil || paramsChanged.Hashes.ParamsHash == workspaceChanged.Hashes.ParamsHash || paramsChanged.Hashes.AncestorHash != workspaceChanged.Hashes.AncestorHash {
		t.Fatalf("parameter hash behavior: %+v %v", paramsChanged.Hashes, err)
	}
	fixtureNode(t, &f.input.Project, "n_mid").Title = "上游新标题"
	ancestorChanged, err := f.builder.Build(context.Background(), f.input)
	if err != nil || ancestorChanged.Hashes.AncestorHash == paramsChanged.Hashes.AncestorHash || ancestorChanged.Hashes.ParamsHash != paramsChanged.Hashes.ParamsHash {
		t.Fatalf("ancestor hash behavior: %+v %v", ancestorChanged.Hashes, err)
	}
}

func TestBudgetDegradesFarthestAncestorFirstAndKeepsTask(t *testing.T) {
	f := newFixture()
	topicBytes, _ := json.Marshal(map[string]any{"topics": []any{map[string]any{"title": strings.Repeat("甲", 7000), "angle": "角度", "hook": "开头"}}})
	scriptBytes, _ := json.Marshal(map[string]any{"title": "上游", "scenes": []any{map[string]any{"narration": strings.Repeat("乙", 7000), "duration": 6}}})
	topicID := f.store.add(valueobject.OutputKindTopics, string(topicBytes))
	scriptID := f.store.add(valueobject.OutputKindScript, string(scriptBytes))
	fixtureNode(t, &f.input.Project, "n_topic").Rounds[0].ArtifactID = topicID
	fixtureNode(t, &f.input.Project, "n_mid").Rounds[1].ArtifactID = scriptID
	build := func(budget int) Result {
		t.Helper()
		input := f.input
		input.Budget = budget
		result, err := f.builder.Build(context.Background(), input)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	full := build(0)
	rootExcerpt := build(full.Estimate.EstimatedTokens - 1)
	rootReference := build(rootExcerpt.Estimate.EstimatedTokens - 1)
	parentExcerpt := build(rootReference.Estimate.EstimatedTokens - 1)
	parentReference := build(parentExcerpt.Estimate.EstimatedTokens - 1)
	cases := []struct {
		result Result
		levels []string
	}{
		{full, []string{"full", "full"}},
		{rootExcerpt, []string{"excerpt", "full"}},
		{rootReference, []string{"reference", "full"}},
		{parentExcerpt, []string{"reference", "excerpt"}},
		{parentReference, []string{"reference", "reference"}},
	}
	for _, tc := range cases {
		body, _ := decodeBody(t, tc.result)
		if body.Path[0].Representation != tc.levels[0] || body.Path[1].Representation != tc.levels[1] {
			t.Fatalf("levels = %s/%s, want %v", body.Path[0].Representation, body.Path[1].Representation, tc.levels)
		}
		if body.Task.Instruction != "继续写中文脚本" || tc.result.Hashes.AncestorHash != full.Hashes.AncestorHash || tc.result.Hashes.ParamsHash != full.Hashes.ParamsHash {
			t.Fatal("degradation changed the task or its input hashes")
		}
		for _, section := range body.Path {
			if section.Representation == "excerpt" && (EstimateTokens(section.Excerpt) > ExcerptTokens || section.ExcerptTruncated == nil || !*section.ExcerptTruncated) {
				t.Fatalf("invalid excerpt: %+v", section)
			}
			if section.Representation == "reference" && (len(section.Payload) != 0 || section.Excerpt != "" || section.ArtifactID == "") {
				t.Fatalf("invalid reference: %+v", section)
			}
		}
	}
	input := f.input
	input.Budget = parentReference.Estimate.EstimatedTokens - 1
	_, err := f.builder.Build(context.Background(), input)
	var tooLarge *ContextTooLargeError
	if !errors.Is(err, ErrContextTooLarge) || !errors.As(err, &tooLarge) || tooLarge.EstimatedTokens != parentReference.Estimate.EstimatedTokens || tooLarge.TokenBudget != input.Budget {
		t.Fatalf("over-budget error = %#v", err)
	}
	fixtureNode(t, &f.input.Project, "n_target").Params.Instruction = strings.Repeat("目标指令", 3000)
	input = f.input
	input.Budget = 5000
	_, err = f.builder.Build(context.Background(), input)
	if !errors.Is(err, ErrContextTooLarge) {
		t.Fatalf("target instruction must not be truncated: %v", err)
	}
}

func TestBuildValidation(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*fixture)
		want   error
	}{
		{"unknown node", func(f *fixture) { f.input.NodeID = "n_missing" }, ErrNodeNotFound},
		{"draft", func(f *fixture) { f.input.NodeID = "n_draft" }, ErrNotInChain},
		{"negative budget", func(f *fixture) { f.input.Budget = -1 }, ErrInvalidBudget},
		{"missing workspace", func(f *fixture) { f.input.Workspace = nil }, ErrWorkspaceScope},
		{"wrong project scope", func(f *fixture) { f.workspace.scope.ProjectID = "p_other" }, ErrWorkspaceScope},
		{"wrong path scope", func(f *fixture) { f.workspace.scope.ChainPath = []string{"n_topic", "n_sibling", "n_target"} }, ErrWorkspaceScope},
		{"ancestor not applied", func(f *fixture) { fixtureNode(t, &f.input.Project, "n_mid").Rounds = nil }, ErrAncestorNotReady},
		{"missing artifact", func(f *fixture) { delete(f.store.items, f.latestID) }, ErrAncestorNotReady},
		{"invalid candidate", func(f *fixture) { value := 4; fixtureNode(t, &f.input.Project, "n_mid").ParentCandidate = &value }, ErrCandidateInvalid},
		{"scene dsl", func(f *fixture) {
			fixtureNode(t, &f.input.Project, "n_target").Params.OutputKind = valueobject.OutputKindSceneDSL
		}, ErrUnsupportedKind},
		{"wrong output kind", func(f *fixture) {
			fixtureNode(t, &f.input.Project, "n_target").Params.OutputKind = valueobject.OutputKindTopics
		}, ErrNodeKindMismatch},
		{"duplicate node", func(f *fixture) { f.input.Project.Nodes = append(f.input.Project.Nodes, f.input.Project.Nodes[0]) }, entity.ErrGraphInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture()
			tc.mutate(f)
			_, err := f.builder.Build(context.Background(), f.input)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
	f := newFixture()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.builder.Build(ctx, f.input); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled build: %v", err)
	}
}

func TestTopicTargetNeedsNoPriorArtifact(t *testing.T) {
	f := newFixture()
	f.input.NodeID = "n_topic"
	f.workspace.scope.NodeID = "n_topic"
	f.workspace.scope.ChainPath = []string{"n_topic"}
	f.builder = NewBuilder(nil)
	result, err := f.builder.Build(context.Background(), f.input)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := decodeBody(t, result)
	if len(body.Path) != 0 || result.OutputKind != valueobject.OutputKindTopics || strings.Contains(result.InitialPrompt, "候选甲") {
		t.Fatalf("unexpected topic target context: %s", result.InitialPrompt)
	}
}

func TestAncestorKindUsesAppliedRoundAndPreservesLegacySameKindPayloads(t *testing.T) {
	f := newFixture()
	f.store.items[f.topicID] = artifactItem{
		kind:    valueobject.OutputKindScript,
		payload: []byte(`{"topics":[{"title":"甲","angle":"角度","hook":"开头"},{"title":"乙","angle":"角度","hook":"开头"},{"title":"丙","angle":"角度","hook":"开头"}]}`),
	}
	if _, err := f.builder.Build(context.Background(), f.input); err != nil {
		t.Fatalf("valid payload with first-write metadata mismatch was rejected: %v", err)
	}
	f.store.items[f.topicID] = artifactItem{kind: valueobject.OutputKindScript, payload: []byte(`{"topics":[]}`)}
	if _, err := f.builder.Build(context.Background(), f.input); !errors.Is(err, ErrAncestorNotReady) {
		t.Fatalf("invalid cross-kind payload error = %v, want ANCESTOR_NOT_READY", err)
	}
	f.store.items[f.topicID] = artifactItem{kind: valueobject.OutputKindTopics, payload: []byte(`{"topics":[{"title":"甲","angle":"角度","hook":"开头"},{"title":"乙","angle":"角度","hook":"开头"},{"title":"丙","angle":"角度","hook":"开头"}]}`)}
	f.store.items[f.latestID] = artifactItem{kind: valueobject.OutputKindScript, payload: []byte(`{"legacy":true}`)}
	if _, err := f.builder.Build(context.Background(), f.input); err != nil {
		t.Fatalf("legacy same-kind artifact was retroactively schema-rejected: %v", err)
	}
}
