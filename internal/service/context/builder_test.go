package context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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
	scope port.WorkspaceScope
	dirs  map[string]port.WorkspaceList
	reads int
}

func (w *memoryWorkspace) Scope() port.WorkspaceScope { return w.scope }
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
	if got.InitialPrompt+"\n" != string(want) {
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
