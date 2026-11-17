package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type scopedRunTools struct {
	mu           sync.Mutex
	scope        port.RunScope
	graph        port.RunGraph
	node         port.RunNode
	artifact     port.RunArtifact
	payload      []byte
	requestID    string
	pending      entity.Pending
	pendingErr   error
	replyEntered chan struct{}
	replyRelease chan struct{}
}

func (f *scopedRunTools) ReadRunGraph(_ context.Context, scope port.RunScope) (port.RunGraph, error) {
	if scope.RunID != f.scope.RunID || scope.ProjectID != f.scope.ProjectID {
		return port.RunGraph{}, port.ErrChainScopeDenied
	}
	return f.graph, nil
}

func (f *scopedRunTools) ReadRunNode(_ context.Context, scope port.RunScope, nodeID string) (port.RunNode, error) {
	if !slices.Contains(scope.ChainPath, nodeID) {
		return port.RunNode{}, port.ErrChainScopeDenied
	}
	return f.node, nil
}

func (f *scopedRunTools) ReadRunArtifact(_ context.Context, scope port.RunScope, artifactID string) (port.RunArtifact, error) {
	if !slices.Contains(scope.ChainPath, f.scope.NodeID) || artifactID != "sha256:allowed" {
		return port.RunArtifact{}, port.ErrChainScopeDenied
	}
	return f.artifact, nil
}

func (f *scopedRunTools) SubmitRunReply(_ context.Context, _ port.RunScope, kind valueobject.OutputKind, payload []byte, requestID string) (entity.Pending, error) {
	if f.replyEntered != nil {
		close(f.replyEntered)
		<-f.replyRelease
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.payload = append([]byte(nil), payload...)
	f.requestID = requestID
	if f.pendingErr != nil {
		return entity.Pending{}, f.pendingErr
	}
	pending := f.pending
	pending.Kind = kind
	return pending, nil
}

type activeRunLease struct {
	mu     sync.RWMutex
	active bool
}

func (l *activeRunLease) WithActiveRun(_ context.Context, _ port.RunScope, operation func() error) error {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if !l.active {
		return port.ErrTurnNotActive
	}
	return operation()
}

func (l *activeRunLease) End() {
	l.mu.Lock()
	l.active = false
	l.mu.Unlock()
}

type observingRunLease struct {
	*activeRunLease
	pending []entity.Pending
	err     error
}

func (l *observingRunLease) RecordRunPending(_ context.Context, _ port.RunScope, pending entity.Pending) error {
	l.pending = append(l.pending, pending.Clone())
	return l.err
}

func scopedMCPOptions(root string) Options {
	workspaceRoot := filepath.Join(root, "workspace")
	_ = os.MkdirAll(workspaceRoot, 0o755)
	scope := port.RunScope{
		RunID: "r_scoped", ProjectID: "p_scoped", TreeID: "n_root", NodeID: "n_target",
		ChainID: "c_n_target", ChainPath: []string{"n_root", "n_target"},
		ContextHash: "context", AncestorHash: "ancestor", ParamsHash: "params",
	}
	tools := &scopedRunTools{
		scope: scope,
		graph: port.RunGraph{Nodes: []port.RunGraphNode{
			{ID: "n_root", Type: valueobject.NodeTypeTopic, Title: "Root"},
			{ID: "n_target", Type: valueobject.NodeTypePrompt, Title: "Target", Parent: "n_root"},
		}},
		node:     port.RunNode{RunGraphNode: port.RunGraphNode{ID: "n_target", Type: valueobject.NodeTypePrompt, Title: "Target"}, Params: entity.NodeParams{Instruction: "write"}},
		artifact: port.RunArtifact{Kind: valueobject.OutputKindScript, Data: []byte(`{"title":"中文","scenes":[{"narration":"旁白","duration":0}]}`)},
		pending:  entity.Pending{ID: "p_1", Status: valueobject.PendingInvalid, Error: "SCHEMA_INVALID"},
	}
	return Options{
		Root: root, Workspace: workspaceRoot, ChainID: scope.ChainID, DisplayID: "d_scoped", Token: "scope-token",
		QuestionGate: &fakeQuestions{}, OnReply: func(string) {}, RunScope: &scope, RunTools: tools,
		RunLifecycle: &activeRunLease{active: true},
	}
}

func TestRunMCPOptionsExposeRootlessDAGScope(t *testing.T) {
	project := entity.Project{
		Version: valueobject.ProjectFileVersion, ID: "p_rootless", Name: "无 Topic",
		Nodes: []entity.Node{
			{ID: "n_prompt", Type: valueobject.NodeTypePrompt, Title: "任务", Params: entity.NodeParams{Instruction: "写一段脚本"}},
			{ID: "n_agent", Type: valueobject.NodeTypeAgent, Title: "Agent"},
		},
		Edges: []entity.Edge{{ID: "e_prompt_agent", SourceNodeID: "n_prompt", TargetNodeID: "n_agent", Kind: entity.EdgeKindConnection}},
	}
	graph, err := entity.NewGraph(&project)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := graph.SnapshotForRun("n_agent")
	if err != nil || snapshot.Version != 4 {
		t.Fatalf("rootless DAG snapshot = %+v err=%v", snapshot, err)
	}
	options := scopedMCPOptions(t.TempDir())
	scope := port.RunScope{
		RunID: "r_rootless", ProjectID: project.ID, NodeID: snapshot.NodeID, ChainID: "c_" + snapshot.NodeID,
		InputNodeIDs: snapshot.ScopeNodeIDs(), ContextHash: "context", AncestorHash: "ancestor", ParamsHash: "params",
		ExecutionSnapshot: snapshot,
	}
	options.RunScope = &scope
	if err := validateRunToolOptions(options); err != nil {
		t.Fatalf("rootless DAG MCP scope rejected: %v", err)
	}
	server, err := New(options)
	if err != nil {
		t.Fatalf("construct scoped MCP server: %v", err)
	}
	scope.InputNodeIDs = nil
	scope.ExecutionSnapshot.InputNodes[0].Params.Instruction = "changed after construction"
	_, output, err := server.getRunChain(context.Background(), nil, GetChainInput{})
	if err != nil || output.TreeID != "" || len(output.ChainPath) != 0 || !slices.Equal(output.InputNodeIDs, snapshot.ScopeNodeIDs()) {
		t.Fatalf("rootless DAG chain output = %+v err=%v", output, err)
	}
	if server.options.RunScope.ExecutionSnapshot.InputNodes[0].Params.Instruction != "写一段脚本" {
		t.Fatal("scoped MCP server did not retain its frozen execution snapshot")
	}

	invalid := scope.Clone()
	invalid.InputNodeIDs = nil
	options.RunScope = &invalid
	if err := validateRunToolOptions(options); err == nil {
		t.Fatal("MCP scope accepted DAG snapshot without its authorized input nodes")
	}
}

func TestSnapshotReplyRejectsKindOutsideOutputContract(t *testing.T) {
	options := scopedMCPOptions(t.TempDir())
	project := entity.Project{Version: valueobject.ProjectFileVersion, ID: "p_scoped", Nodes: []entity.Node{
		{ID: "n_topic", Type: valueobject.NodeTypeTopic, Rounds: []entity.Round{{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: "sha256:topics", RunID: "r_topics"}}},
		{ID: "n_candidate", Type: valueobject.NodeTypeCandidate, CandidateSource: &entity.CandidateSource{TopicNodeID: "n_topic", Round: 1, ArtifactID: "sha256:topics", Index: 0, RunID: "r_topics"}},
		{ID: "n_prompt", Type: valueobject.NodeTypePrompt, Params: entity.NodeParams{Instruction: "任务"}},
		{ID: "n_agent", Type: valueobject.NodeTypeAgent},
	}, Edges: []entity.Edge{
		{ID: "e_n_candidate", SourceNodeID: "n_topic", TargetNodeID: "n_candidate", Kind: entity.EdgeKindCandidateSource},
		{ID: "e_n_prompt", SourceNodeID: "n_candidate", TargetNodeID: "n_prompt", Kind: entity.EdgeKindConnection},
		{ID: "e_n_agent", SourceNodeID: "n_prompt", TargetNodeID: "n_agent", Kind: entity.EdgeKindConnection},
	}}
	graph, err := entity.NewGraph(&project)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := graph.SnapshotForRun("n_agent")
	if err != nil {
		t.Fatal(err)
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
	scope := options.RunScope.Clone()
	scope.TreeID = snapshot.TreeID
	scope.NodeID = snapshot.NodeID
	scope.ChainID = "c_" + snapshot.NodeID
	scope.ChainPath = append([]string(nil), snapshot.ChainPath...)
	scope.ExecutionSnapshot = snapshot
	options.RunScope = &scope
	options.RunTools.(*scopedRunTools).scope = scope
	server := &Server{options: options}
	request := &sdkmcp.CallToolRequest{Params: &sdkmcp.CallToolParamsRaw{
		Arguments: json.RawMessage(`{"node_id":"n_agent","kind":"topics","payload":{"topics":[]}}`),
	}}
	result, err := server.replyRun(context.Background(), request)
	if err != nil || result == nil || !result.IsError || !strings.Contains(toolResultText(result), "OUTPUT_KIND_MISMATCH") {
		t.Fatalf("reply result=%+v err=%v, want OUTPUT_KIND_MISMATCH", result, err)
	}
	if len(options.RunTools.(*scopedRunTools).payload) != 0 {
		t.Fatal("mismatched reply reached the pending sink")
	}
	agentScript := &sdkmcp.CallToolRequest{Params: &sdkmcp.CallToolParamsRaw{
		Arguments: json.RawMessage(`{"node_id":"n_agent","kind":"script","payload":{"title":"x","scenes":[{"narration":"y","duration":1}]}}`),
	}}
	result, err = server.replyRun(context.Background(), agentScript)
	if err != nil || result == nil || result.IsError {
		t.Fatalf("Agent script reply result=%+v err=%v, want accepted structured result", result, err)
	}
	if len(options.RunTools.(*scopedRunTools).payload) == 0 {
		t.Fatal("valid Agent script reply did not reach the result sink")
	}
}

func TestReplyRunRecordsOnlyPendingResults(t *testing.T) {
	options := scopedMCPOptions(t.TempDir())
	tools := options.RunTools.(*scopedRunTools)
	lifecycle := &observingRunLease{activeRunLease: &activeRunLease{active: true}}
	options.RunLifecycle = lifecycle
	server, err := New(options)
	if err != nil {
		t.Fatal(err)
	}
	request := &sdkmcp.CallToolRequest{Params: &sdkmcp.CallToolParamsRaw{
		Arguments: json.RawMessage(`{"node_id":"n_target","kind":"script","payload":{"title":"x","scenes":[]}}`),
	}}

	tools.pending.Status = valueobject.PendingApplied
	if result, err := server.replyRun(context.Background(), request); err != nil || result == nil || result.IsError {
		t.Fatalf("automatic result reply = %+v err=%v", result, err)
	}
	if len(lifecycle.pending) != 0 {
		t.Fatalf("automatic result recorded as pending: %+v", lifecycle.pending)
	}

	tools.pending.Status = valueobject.PendingPending
	if result, err := server.replyRun(context.Background(), request); err != nil || result == nil || result.IsError {
		t.Fatalf("pending result reply = %+v err=%v", result, err)
	}
	if len(lifecycle.pending) != 1 || lifecycle.pending[0].Status != valueobject.PendingPending {
		t.Fatalf("pending result observer calls = %+v", lifecycle.pending)
	}
}

func TestScopedMCPToolSurfaceAndRawReply(t *testing.T) {
	root := t.TempDir()
	options := scopedMCPOptions(root)
	tools := options.RunTools.(*scopedRunTools)
	var message string
	options.OnReply = func(text string) { message = text }
	server, err := New(options)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer server.Stop(context.Background())

	unauthorized := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test", Version: "1"}, nil)
	if _, err := unauthorized.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: server.URL()}, nil); err == nil {
		t.Fatal("unauthorized scoped MCP connect succeeded")
	}
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test", Version: "1"}, nil)
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req = req.Clone(req.Context())
		req.Header.Set("Authorization", "Bearer "+server.Token())
		return http.DefaultTransport.RoundTrip(req)
	})}
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: server.URL(), HTTPClient: httpClient}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range listed.Tools {
		names[tool.Name] = true
	}
	for _, name := range []string{"get_chain", "get_graph", "get_node", "list_node_types", "validate", "read_artifact", "reply", "ask_user"} {
		if !names[name] {
			t.Fatalf("missing tool %q: %v", name, names)
		}
	}
	for _, forbiddenFileTool := range []string{"read_file", "list_dir", "grep", "stat"} {
		if names[forbiddenFileTool] {
			t.Fatalf("run-scoped MCP exposed native file tool %q", forbiddenFileTool)
		}
	}
	for _, forbidden := range []string{"create_node", "connect", "update_params", "delete_node", "apply_artifact"} {
		if names[forbidden] {
			t.Fatalf("forbidden tool %q was registered", forbidden)
		}
	}

	chain, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "get_chain"})
	if err != nil || chain.IsError {
		t.Fatalf("get_chain = %+v err=%v", chain, err)
	}
	chainJSON, _ := json.Marshal(chain.StructuredContent)
	for _, expected := range []string{"r_scoped", "n_root", "n_target", "workspace"} {
		if !strings.Contains(string(chainJSON), expected) {
			t.Fatalf("get_chain omitted %q: %s", expected, chainJSON)
		}
	}

	graph, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "get_graph"})
	if err != nil || graph.IsError {
		t.Fatalf("get_graph = %+v err=%v", graph, err)
	}
	graphJSON, _ := json.Marshal(graph.StructuredContent)
	if strings.Contains(string(graphJSON), "sibling") || !strings.Contains(string(graphJSON), "n_target") {
		t.Fatalf("get_graph scope = %s", graphJSON)
	}
	node, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "get_node", Arguments: map[string]any{"nodeId": "n_target"}})
	if err != nil || node.IsError || !strings.Contains(fmt.Sprint(node.StructuredContent), "write") {
		t.Fatalf("get_node = %+v err=%v", node, err)
	}
	sibling, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "get_node", Arguments: map[string]any{"nodeId": "n_sibling"}})
	if err != nil || !sibling.IsError || !strings.Contains(toolResultText(sibling), "CHAIN_SCOPE_DENIED") {
		t.Fatalf("sibling get_node = %+v err=%v", sibling, err)
	}
	nodeTypes, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "list_node_types"})
	if err != nil || nodeTypes.IsError || !strings.Contains(fmt.Sprint(nodeTypes.StructuredContent), "topics") || !strings.Contains(fmt.Sprint(nodeTypes.StructuredContent), "script") {
		t.Fatalf("list_node_types = %+v err=%v", nodeTypes, err)
	}

	invalidPayload := json.RawMessage(`{"title":"无效","scenes":[]}`)
	validation, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "validate", Arguments: map[string]any{
		"kind": "script", "payload": invalidPayload,
	}})
	if err != nil || validation.IsError {
		t.Fatalf("validate = %+v err=%v", validation, err)
	}
	validationJSON, _ := json.Marshal(validation.StructuredContent)
	if !strings.Contains(string(validationJSON), `"valid":false`) || !strings.Contains(string(validationJSON), "SCHEMA_INVALID") {
		t.Fatalf("validate result = %s", validationJSON)
	}

	payload := json.RawMessage("{ \"title\" : \"无效\", \"scenes\" : [] }")
	reply, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "reply", Arguments: map[string]any{
		"node_id": "n_target", "kind": "script", "payload": payload, "request_id": "req_1",
	}})
	if err != nil || reply.IsError {
		t.Fatalf("reply = %+v err=%v", reply, err)
	}
	if string(tools.payload) != strings.ReplaceAll(string(payload), " ", "") || tools.requestID != "req_1" {
		t.Fatalf("reply payload=%s requestID=%s, want equivalent JSON and req_1", tools.payload, tools.requestID)
	}
	if !strings.Contains(fmt.Sprint(reply.StructuredContent), "p_1") || !strings.Contains(fmt.Sprint(reply.StructuredContent), "invalid") {
		t.Fatalf("reply output = %+v", reply.StructuredContent)
	}

	messageResult, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "reply", Arguments: map[string]any{
		"node_id": "n_target", "kind": "message", "payload": map[string]any{"text": "完成"},
	}})
	if err != nil || messageResult.IsError || message != "完成" {
		t.Fatalf("message reply=%+v message=%q err=%v", messageResult, message, err)
	}

	artifact, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "read_artifact", Arguments: map[string]any{
		"artifactId": "sha256:allowed", "offset": 0, "limit": 8,
	}})
	if err != nil || artifact.IsError {
		t.Fatalf("read_artifact = %+v err=%v", artifact, err)
	}
	artifactJSON, _ := json.Marshal(artifact.StructuredContent)
	if !strings.Contains(string(artifactJSON), "nextOffset") || strings.Contains(string(artifactJSON), "旁白") {
		t.Fatalf("read_artifact output = %s", artifactJSON)
	}

	wrongTarget, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "reply", Arguments: map[string]any{
		"node_id": "n_root", "kind": "script", "payload": payload,
	}})
	if err != nil || !wrongTarget.IsError || !strings.Contains(toolResultText(wrongTarget), "TURN_NOT_ACTIVE") {
		t.Fatalf("in-path non-target reply = %+v err=%v", wrongTarget, err)
	}
	outside, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "reply", Arguments: map[string]any{
		"node_id": "n_sibling", "kind": "script", "payload": payload,
	}})
	if err != nil || !outside.IsError || !strings.Contains(toolResultText(outside), "CHAIN_SCOPE_DENIED") {
		t.Fatalf("out-of-path reply = %+v err=%v", outside, err)
	}
}

func TestArtifactChunkUTF8BoundariesAndLimits(t *testing.T) {
	data := []byte(`{"title":"中文"}`)
	chineseStart := int64(strings.Index(string(data), "中"))
	artifact := port.RunArtifact{Kind: valueobject.OutputKindScript, Data: data}
	for offset := int64(0); offset < int64(len(data)); {
		chunk, err := artifactChunk("sha256:test", artifact, offset, 4)
		if err != nil {
			t.Fatal(err)
		}
		if chunk.NextOffset <= offset || len([]byte(chunk.Content)) > 4 {
			t.Fatalf("chunk did not advance within limit: %+v", chunk)
		}
		offset = chunk.NextOffset
	}
	end, err := artifactChunk("sha256:test", artifact, int64(len(data)), 4)
	if err != nil || !end.EOF || end.Content != "" {
		t.Fatalf("EOF chunk = %+v err=%v", end, err)
	}
	if _, err := artifactChunk("sha256:test", artifact, chineseStart+1, 4); err == nil || !strings.Contains(err.Error(), "ARTIFACT_OFFSET_INVALID") {
		t.Fatalf("invalid offset error = %v", err)
	}
	if _, err := artifactChunk("sha256:test", artifact, chineseStart, 1); err == nil || !strings.Contains(err.Error(), "ARTIFACT_LIMIT_INVALID") {
		t.Fatalf("too-small limit error = %v", err)
	}
	large := port.RunArtifact{Kind: valueobject.OutputKindScript, Data: []byte(strings.Repeat("x", maxArtifactChunkBytes+20))}
	clamped, err := artifactChunk("sha256:test", large, 0, maxArtifactChunkBytes+100)
	if err != nil || len(clamped.Content) != maxArtifactChunkBytes || clamped.NextOffset != maxArtifactChunkBytes {
		t.Fatalf("limit clamp = %+v err=%v", clamped, err)
	}
}

func TestReplyRunPassesRawPayloadBytesToPendingSink(t *testing.T) {
	options := scopedMCPOptions(t.TempDir())
	tools := options.RunTools.(*scopedRunTools)
	server, err := New(options)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{ "title" : "x", "scenes" : [ { "narration" : "中", "duration" : 9007199254740993 } ] }`)
	arguments := []byte(fmt.Sprintf(`{"node_id":"n_target","kind":"script","payload":%s,"request_id":"req_raw"}`, payload))
	result, err := server.replyRun(context.Background(), &sdkmcp.CallToolRequest{Params: &sdkmcp.CallToolParamsRaw{Arguments: arguments}})
	if err != nil || result == nil || result.IsError {
		t.Fatalf("reply result=%+v err=%v", result, err)
	}
	if string(tools.payload) != string(payload) || tools.requestID != "req_raw" {
		t.Fatalf("stored payload=%s requestID=%s", tools.payload, tools.requestID)
	}
}

func TestReplyAndRunTerminationShareLifecycleLease(t *testing.T) {
	options := scopedMCPOptions(t.TempDir())
	tools := options.RunTools.(*scopedRunTools)
	tools.replyEntered = make(chan struct{})
	tools.replyRelease = make(chan struct{})
	lifecycle := options.RunLifecycle.(*activeRunLease)
	request := &sdkmcp.CallToolRequest{Params: &sdkmcp.CallToolParamsRaw{
		Arguments: json.RawMessage(`{"node_id":"n_target","kind":"script","payload":{"title":"x","scenes":[]}}`),
	}}

	replyDone := make(chan *sdkmcp.CallToolResult, 1)
	go func() {
		result, _ := newScopedHandlerForTest(options).replyRun(context.Background(), request)
		replyDone <- result
	}()
	<-tools.replyEntered
	ended := make(chan struct{})
	go func() {
		lifecycle.End()
		close(ended)
	}()
	select {
	case <-ended:
		t.Fatal("run ended while a reply was being committed")
	case <-time.After(30 * time.Millisecond):
	}
	close(tools.replyRelease)
	if result := <-replyDone; result == nil || result.IsError {
		t.Fatalf("reply result = %+v", result)
	}
	select {
	case <-ended:
	case <-time.After(time.Second):
		t.Fatal("run did not end after reply completed")
	}

	result, err := newScopedHandlerForTest(options).replyRun(context.Background(), request)
	if err != nil || result == nil || !result.IsError || !strings.Contains(toolResultText(result), "TURN_NOT_ACTIVE") {
		t.Fatalf("reply after end = %+v err=%v", result, err)
	}
}

func newScopedHandlerForTest(options Options) *Server {
	server, err := New(options)
	if err != nil {
		panic(err)
	}
	return server
}

func toolResultText(result *sdkmcp.CallToolResult) string {
	var parts []string
	for _, content := range result.Content {
		if text, ok := content.(*sdkmcp.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n")
}
