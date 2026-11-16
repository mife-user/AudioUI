package context

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"unicode/utf8"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
)

func (b *Builder) resolve(ctx context.Context, input Input, includeWorkspace bool) (resolvedInput, error) {
	if input.Snapshot.Version != 0 {
		return b.resolveSnapshot(ctx, input, includeWorkspace)
	}
	return b.resolveLegacy(ctx, input, includeWorkspace)
}

func (b *Builder) resolveLegacy(ctx context.Context, input Input, includeWorkspace bool) (resolvedInput, error) {
	if err := ctx.Err(); err != nil {
		return resolvedInput{}, err
	}
	project := input.Project.Clone()
	if err := valueobject.ValidateID(project.ID); err != nil {
		return resolvedInput{}, fmt.Errorf("invalid project: %w", err)
	}
	graph, err := entity.NewGraph(&project)
	if err != nil {
		return resolvedInput{}, err
	}
	if project.Version >= valueobject.ProjectFileVersion {
		if err := graph.Validate(); err != nil {
			return resolvedInput{}, err
		}
		snapshot, err := graph.SnapshotForRun(input.NodeID)
		if err != nil {
			return resolvedInput{}, err
		}
		input.Snapshot = snapshot
		return b.resolveSnapshot(ctx, input, includeWorkspace)
	}
	if err := graph.ValidateForest(); err != nil {
		return resolvedInput{}, err
	}
	target, ok := graph.Node(input.NodeID)
	if !ok {
		return resolvedInput{}, fmt.Errorf("%w: %s", ErrNodeNotFound, input.NodeID)
	}
	chainPath, err := graph.PathToRoot(target.ID)
	if err != nil {
		return resolvedInput{}, err
	}
	for _, id := range chainPath {
		if err := valueobject.ValidateID(id); err != nil {
			return resolvedInput{}, fmt.Errorf("invalid path node: %w", err)
		}
	}
	root, _ := graph.Node(chainPath[0])
	if root.Type != valueobject.NodeTypeTopic {
		return resolvedInput{}, fmt.Errorf("%w: %s", ErrNotInChain, target.ID)
	}
	kind, err := outputKind(target)
	if err != nil {
		return resolvedInput{}, err
	}
	if includeWorkspace {
		if isNil(input.Workspace) {
			return resolvedInput{}, fmt.Errorf("%w: run workspace is required", ErrWorkspaceScope)
		}
		scope := input.Workspace.Scope()
		if scope.ProjectID != project.ID || scope.TreeID != root.ID || scope.NodeID != target.ID || !slices.Equal(scope.ChainPath, chainPath) {
			return resolvedInput{}, fmt.Errorf("%w: workspace does not match project and graph path", ErrWorkspaceScope)
		}
	}

	ancestors := make([]resolvedAncestor, 0, len(chainPath)-1)
	if len(chainPath) > 1 {
		if b == nil || isNil(b.artifacts) {
			return resolvedInput{}, fmt.Errorf("%w: artifact store unavailable", ErrAncestorNotReady)
		}
		store, err := b.artifacts.ForProject(ctx, project.ID)
		if err != nil {
			return resolvedInput{}, fmt.Errorf("open artifact store: %w", err)
		}
		if isNil(store) {
			return resolvedInput{}, fmt.Errorf("%w: artifact store unavailable", ErrAncestorNotReady)
		}
		for i := 0; i < len(chainPath)-1; i++ {
			node, _ := graph.Node(chainPath[i])
			child, _ := graph.Node(chainPath[i+1])
			ancestor, err := loadAncestor(ctx, store, node, child)
			if err != nil {
				return resolvedInput{}, err
			}
			ancestors = append(ancestors, ancestor)
		}
	}
	workspace := workspaceSection{}
	if includeWorkspace {
		files, err := collectManifest(ctx, input.Workspace)
		if err != nil {
			return resolvedInput{}, err
		}
		overlay := make([]string, 0, len(chainPath)+1)
		for i := len(chainPath) - 1; i >= 0; i-- {
			overlay = append(overlay, "steps/"+chainPath[i]+"/")
		}
		overlay = append(overlay, "<exe>/runtime/")
		workspace = workspaceSection{CWD: "trees/" + root.ID + "/", Overlay: overlay, Files: files.Files, Truncated: files.Truncated}
	}
	return resolvedInput{
		target:       targetSection{TreeID: root.ID, ChainPath: append([]string(nil), chainPath...), NodeID: target.ID, NodeType: target.Type, OutputKind: kind, ProjectVersion: project.Version},
		params:       target.Params,
		targetParams: target.Params,
		ancestors:    ancestors,
		workspace:    workspace,
	}, nil
}

func (b *Builder) resolveSnapshot(ctx context.Context, input Input, includeWorkspace bool) (resolvedInput, error) {
	if err := ctx.Err(); err != nil {
		return resolvedInput{}, err
	}
	project := input.Project
	if err := valueobject.ValidateID(project.ID); err != nil {
		return resolvedInput{}, fmt.Errorf("invalid project: %w", err)
	}
	snapshot := input.Snapshot.Clone()
	if err := snapshot.Validate(); err != nil {
		return resolvedInput{}, fmt.Errorf("invalid execution snapshot: %w", err)
	}
	if snapshot.ProjectID != project.ID || snapshot.NodeID != input.NodeID {
		return resolvedInput{}, fmt.Errorf("%w: snapshot does not match project and target", ErrWorkspaceScope)
	}
	if snapshot.Version >= 3 {
		return b.resolveDAGSnapshot(ctx, input, snapshot, includeWorkspace)
	}
	if snapshot.PathNodes[0].Type != valueobject.NodeTypeTopic {
		return resolvedInput{}, fmt.Errorf("%w: run path must start at a topic", ErrNotInChain)
	}
	target := snapshot.PathNodes[len(snapshot.PathNodes)-1]
	kind, err := outputKind(&target)
	if err != nil {
		return resolvedInput{}, err
	}
	if kind != snapshot.OutputContract.Kind {
		return resolvedInput{}, fmt.Errorf("%w: snapshot output contract does not match target", ErrNodeKindMismatch)
	}
	if includeWorkspace {
		if isNil(input.Workspace) {
			return resolvedInput{}, fmt.Errorf("%w: run workspace is required", ErrWorkspaceScope)
		}
		scope := input.Workspace.Scope()
		if scope.ProjectID != snapshot.ProjectID || scope.TreeID != snapshot.TreeID || scope.NodeID != snapshot.NodeID || !slices.Equal(scope.ChainPath, snapshot.ChainPath) {
			return resolvedInput{}, fmt.Errorf("%w: workspace does not match execution snapshot", ErrWorkspaceScope)
		}
	}

	params := target.Params
	taskNodeID := ""
	if target.Type == valueobject.NodeTypeAgent {
		if len(snapshot.PathNodes) < 2 {
			return resolvedInput{}, fmt.Errorf("%w: agent path has no prompt task", ErrNotInChain)
		}
		task := snapshot.PathNodes[len(snapshot.PathNodes)-2]
		if task.Type != valueobject.NodeTypePrompt {
			return resolvedInput{}, fmt.Errorf("%w: agent path must end with a prompt task", ErrNotInChain)
		}
		taskKind, err := outputKind(&task)
		if err != nil {
			return resolvedInput{}, err
		}
		if taskKind != valueobject.OutputKindScript {
			return resolvedInput{}, fmt.Errorf("%w: prompt cannot output %s", ErrNodeKindMismatch, taskKind)
		}
		params = task.Params
		taskNodeID = task.ID
	}

	needsStore := false
	for _, node := range snapshot.PathNodes[:len(snapshot.PathNodes)-1] {
		if node.Type == valueobject.NodeTypeCandidate || node.Type == valueobject.NodeTypeScript {
			needsStore = true
			break
		}
	}
	var store port.ArtifactStore
	if needsStore {
		if b == nil || isNil(b.artifacts) {
			return resolvedInput{}, fmt.Errorf("%w: artifact store unavailable", ErrAncestorNotReady)
		}
		store, err = b.artifacts.ForProject(ctx, snapshot.ProjectID)
		if err != nil {
			return resolvedInput{}, fmt.Errorf("open artifact store: %w", err)
		}
		if isNil(store) {
			return resolvedInput{}, fmt.Errorf("%w: artifact store unavailable", ErrAncestorNotReady)
		}
	}
	ancestors := make([]resolvedAncestor, 0, len(snapshot.PathNodes)-1)
	for i := 0; i < len(snapshot.PathNodes)-1; i++ {
		node := snapshot.PathNodes[i]
		identity := ancestorIdentity{NodeID: node.ID, NodeType: node.Type, Title: node.Title, Params: node.Params}
		var ancestor resolvedAncestor
		switch node.Type {
		case valueobject.NodeTypeCandidate:
			if node.CandidateSource == nil || (snapshot.ProjectVersion < valueobject.DAGProjectVersion && node.CandidateSource.TopicNodeID != node.Parent) {
				return resolvedInput{}, fmt.Errorf("%w: candidate %s", ErrCandidateInvalid, node.ID)
			}
			reference, ok := snapshotArtifact(snapshot, node.ID)
			if !ok || reference.Kind != valueobject.OutputKindTopics || reference.ArtifactID != node.CandidateSource.ArtifactID || reference.CandidateIndex == nil || *reference.CandidateIndex != node.CandidateSource.Index {
				return resolvedInput{}, fmt.Errorf("%w: candidate %s snapshot source mismatch", ErrCandidateInvalid, node.ID)
			}
			ancestor, err = loadSnapshotAncestor(ctx, store, node, reference)
			if err == nil {
				number := node.CandidateSource.Index + 1
				ancestor.identity.CandidateNumber = &number
			}
		case valueobject.NodeTypeScript:
			if len(node.ScriptVersions) == 0 {
				return resolvedInput{}, fmt.Errorf("%w: script %s has no immutable version", ErrAncestorNotReady, node.ID)
			}
			latest := node.ScriptVersions[len(node.ScriptVersions)-1]
			reference, ok := snapshotArtifact(snapshot, node.ID)
			if !ok || reference.Kind != valueobject.OutputKindScript || reference.ArtifactID != latest.ArtifactID || reference.Version != latest.N {
				return resolvedInput{}, fmt.Errorf("%w: script %s snapshot version mismatch", ErrAncestorNotReady, node.ID)
			}
			ancestor, err = loadSnapshotAncestor(ctx, store, node, reference)
			if err == nil {
				ancestor.identity.ArtifactVersion = reference.Version
			}
		case valueobject.NodeTypePrompt:
			identity.Params = entity.NodeParams{}
			ancestor = resolvedAncestor{identity: identity}
		default:
			ancestor = resolvedAncestor{identity: identity}
		}
		if err != nil {
			return resolvedInput{}, err
		}
		if ancestor.identity.NodeID == "" {
			ancestor.identity = identity
		}
		ancestors = append(ancestors, ancestor)
	}

	workspace := workspaceSection{}
	if includeWorkspace {
		files, err := collectManifest(ctx, input.Workspace)
		if err != nil {
			return resolvedInput{}, err
		}
		overlay := make([]string, 0, len(snapshot.ChainPath)+1)
		readRoots := make([]string, 0, len(snapshot.ChainPath)+1)
		for i := len(snapshot.ChainPath) - 1; i >= 0; i-- {
			overlay = append(overlay, "steps/"+snapshot.ChainPath[i]+"/")
		}
		overlay = append(overlay, "<exe>/runtime/")
		for _, id := range snapshot.ChainPath {
			readRoots = append(readRoots, "steps/"+id+"/")
		}
		if runtimePath := input.Workspace.RuntimeReadPath(); runtimePath != "" {
			readRoots = append(readRoots, runtimePath)
		}
		workspace = workspaceSection{
			CWD: "trees/" + snapshot.TreeID + "/", Overlay: overlay,
			Files: files.Files, Truncated: files.Truncated,
			NativeReadRoots: readRoots, NativeWriteRoot: "steps/" + snapshot.NodeID + "/",
		}
	}
	binding := snapshot.OutputBinding
	contract := snapshot.OutputContract
	var outputBinding *entity.RunOutputBinding
	var outputContract *entity.RunOutputContract
	if target.Type == valueobject.NodeTypeAgent {
		outputBinding = &binding
		outputContract = &contract
	}
	return resolvedInput{
		target: targetSection{
			TreeID: snapshot.TreeID, ChainPath: append([]string(nil), snapshot.ChainPath...),
			NodeID: target.ID, NodeType: target.Type, OutputKind: kind, ProjectVersion: snapshot.ProjectVersion,
			TopicBatch: snapshot.TopicBatch,
		},
		params: params, targetParams: target.Params, taskNodeID: taskNodeID,
		outputBinding: outputBinding, outputContract: outputContract,
		ancestors: ancestors, workspace: workspace,
	}, nil
}

func (b *Builder) resolveDAGSnapshot(ctx context.Context, input Input, snapshot entity.RunExecutionSnapshot, includeWorkspace bool) (resolvedInput, error) {
	target := snapshot.InputNodes[len(snapshot.InputNodes)-1]
	kind, err := outputKind(&target)
	if err != nil {
		return resolvedInput{}, err
	}
	if kind != snapshot.OutputContract.Kind {
		return resolvedInput{}, fmt.Errorf("%w: snapshot output contract does not match target", ErrNodeKindMismatch)
	}
	if includeWorkspace {
		if isNil(input.Workspace) {
			return resolvedInput{}, fmt.Errorf("%w: run workspace is required", ErrWorkspaceScope)
		}
		scope := input.Workspace.Scope()
		if scope.ProjectID != snapshot.ProjectID || scope.TreeID != snapshot.TreeID || scope.NodeID != snapshot.NodeID ||
			len(scope.ChainPath) != 0 || !slices.Equal(scope.InputNodeIDs, snapshot.ScopeNodeIDs()) {
			return resolvedInput{}, fmt.Errorf("%w: workspace does not match DAG execution snapshot", ErrWorkspaceScope)
		}
	}

	nodeByID := make(map[string]entity.Node, len(snapshot.InputNodes))
	for _, node := range snapshot.InputNodes {
		nodeByID[node.ID] = node
	}
	tasks := make([]taskInputSection, 0)
	params := entity.NodeParams{}
	taskNodeID := ""
	for _, selected := range snapshot.Inputs {
		if selected.Role != "task" {
			continue
		}
		node := nodeByID[selected.SourceNodeID]
		tasks = append(tasks, taskInputSection{NodeID: node.ID, Title: node.Title, Order: selected.Order, Task: node.Params})
		if len(tasks) == 1 {
			params = node.Params
			taskNodeID = node.ID
		}
	}

	var store port.ArtifactStore
	if len(snapshot.Artifacts) > 0 {
		if b == nil || isNil(b.artifacts) {
			return resolvedInput{}, fmt.Errorf("%w: artifact store unavailable", ErrAncestorNotReady)
		}
		store, err = b.artifacts.ForProject(ctx, snapshot.ProjectID)
		if err != nil {
			return resolvedInput{}, fmt.Errorf("open artifact store: %w", err)
		}
		if isNil(store) {
			return resolvedInput{}, fmt.Errorf("%w: artifact store unavailable", ErrAncestorNotReady)
		}
	}
	ancestors := make([]resolvedAncestor, 0, len(snapshot.Artifacts))
	loaded := make(map[string]resolvedAncestor, len(snapshot.Artifacts))
	for _, reference := range snapshot.Artifacts {
		node := nodeByID[reference.NodeID]
		key := fmt.Sprintf("%s|%s|%d|%d", reference.Kind, reference.ArtifactID, reference.Version, -1)
		if reference.CandidateIndex != nil {
			key = fmt.Sprintf("%s|%s|%d|%d", reference.Kind, reference.ArtifactID, reference.Version, *reference.CandidateIndex)
		}
		if cached, ok := loaded[key]; ok {
			cached.identity.NodeID = node.ID
			cached.identity.NodeType = node.Type
			cached.identity.Title = node.Title
			cached.identity.Params = node.Params
			cached.identity.SourceEdgeID = reference.EdgeID
			cached.shared = true
			ancestors = append(ancestors, cached)
			continue
		}
		ancestor, err := loadSnapshotAncestor(ctx, store, node, reference)
		if err != nil {
			return resolvedInput{}, err
		}
		ancestor.identity.SourceEdgeID = reference.EdgeID
		if reference.CandidateIndex != nil {
			number := *reference.CandidateIndex + 1
			ancestor.identity.CandidateNumber = &number
		}
		if reference.Kind == valueobject.OutputKindScript {
			ancestor.identity.ArtifactVersion = reference.Version
		}
		loaded[key] = ancestor
		ancestors = append(ancestors, ancestor)
	}

	workspace := workspaceSection{}
	if includeWorkspace {
		files, err := collectManifest(ctx, input.Workspace)
		if err != nil {
			return resolvedInput{}, err
		}
		overlay := make([]string, 0, len(input.Workspace.InputSources())+1)
		for _, source := range input.Workspace.InputSources() {
			overlay = append(overlay, source.LogicalPath)
		}
		overlay = append(overlay, "<exe>/runtime/")
		writeRoot := "steps/" + snapshot.NodeID + "/"
		cwd := "trees/" + snapshot.TreeID + "/"
		if snapshot.TreeID == "" {
			writeRoot = "."
			cwd = "workspace/steps/" + snapshot.NodeID + "/"
		}
		workspace = workspaceSection{
			CWD: cwd, Overlay: overlay, Files: files.Files, Truncated: files.Truncated,
			NativeReadSources: input.Workspace.InputSources(),
			NativeReadRoots:   []string{input.Workspace.RuntimeReadPath()}, NativeWriteRoot: writeRoot,
		}
	}
	workspaceRoots := make([]workspaceRootIdentity, 0, len(snapshot.InputNodes)-1)
	for _, node := range snapshot.InputNodes {
		if node.ID == target.ID || node.Type == valueobject.NodeTypeTopic {
			continue
		}
		rootID := node.WorkspaceRootID
		if rootID == "" {
			rootID = "project"
		}
		workspaceRoots = append(workspaceRoots, workspaceRootIdentity{NodeID: node.ID, RootID: rootID})
	}
	binding := snapshot.OutputBinding
	contract := snapshot.OutputContract
	return resolvedInput{
		target: targetSection{
			TreeID: snapshot.TreeID, NodeID: target.ID, NodeType: target.Type,
			OutputKind: kind, ProjectVersion: snapshot.ProjectVersion,
		},
		params: params, targetParams: target.Params, taskNodeID: taskNodeID,
		outputBinding: &binding, outputContract: &contract,
		ancestors: ancestors, workspace: workspace,
		inputs:     append([]entity.RunInput(nil), snapshot.Inputs...),
		inputEdges: append([]entity.Edge(nil), snapshot.InputEdges...),
		tasks:      tasks, inputNodeIDs: snapshot.ScopeNodeIDs(), workspaceRoots: workspaceRoots,
		dag: true, contextVersion: 2,
	}, nil
}

func snapshotArtifact(snapshot entity.RunExecutionSnapshot, nodeID string) (entity.RunArtifactReference, bool) {
	var found entity.RunArtifactReference
	ok := false
	for _, reference := range snapshot.Artifacts {
		if reference.NodeID != nodeID {
			continue
		}
		if ok {
			return entity.RunArtifactReference{}, false
		}
		found, ok = reference, true
	}
	return found, ok
}

func loadSnapshotAncestor(ctx context.Context, store port.ArtifactStore, node entity.Node, reference entity.RunArtifactReference) (resolvedAncestor, error) {
	if err := ctx.Err(); err != nil {
		return resolvedAncestor{}, err
	}
	artifact, payload, err := store.Get(ctx, reference.ArtifactID)
	if err != nil {
		return resolvedAncestor{}, fmt.Errorf("%w: node %s: %w", ErrAncestorNotReady, node.ID, err)
	}
	if artifact.ID != reference.ArtifactID || len(payload) > valueobject.MaxPendingPayloadSize || !utf8.Valid(payload) || !json.Valid(payload) {
		return resolvedAncestor{}, fmt.Errorf("%w: node %s has an invalid artifact", ErrAncestorNotReady, node.ID)
	}
	if artifact.Kind != reference.Kind {
		if err := valueobject.ValidateStructuredPayload(reference.Kind, payload); err != nil {
			return resolvedAncestor{}, fmt.Errorf("%w: node %s artifact does not match its snapshot kind", ErrAncestorNotReady, node.ID)
		}
	}
	selected := payload
	identity := ancestorIdentity{
		NodeID: node.ID, NodeType: node.Type, Title: node.Title, Params: node.Params,
		ArtifactKind: reference.Kind, ArtifactID: reference.ArtifactID,
	}
	if reference.Kind == valueobject.OutputKindTopics {
		if reference.CandidateIndex == nil {
			return resolvedAncestor{}, fmt.Errorf("%w: candidate index missing", ErrCandidateInvalid)
		}
		var topics struct {
			Topics []json.RawMessage `json:"topics"`
		}
		if err := json.Unmarshal(payload, &topics); err != nil || *reference.CandidateIndex < 0 || *reference.CandidateIndex >= len(topics.Topics) {
			return resolvedAncestor{}, fmt.Errorf("%w: topic %s", ErrCandidateInvalid, node.ID)
		}
		selected = topics.Topics[*reference.CandidateIndex]
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(selected, &object); err != nil || object == nil {
		return resolvedAncestor{}, fmt.Errorf("%w: node %s artifact must be an object", ErrAncestorNotReady, node.ID)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, selected); err != nil {
		return resolvedAncestor{}, fmt.Errorf("%w: node %s: %v", ErrAncestorNotReady, node.ID, err)
	}
	return resolvedAncestor{identity: identity, payload: append(json.RawMessage(nil), compact.Bytes()...)}, nil
}

func outputKind(node *entity.Node) (valueobject.OutputKind, error) {
	if node.Type == valueobject.NodeTypeTopic {
		return valueobject.OutputKindTopics, nil
	}
	switch node.Params.OutputKind {
	case "", valueobject.OutputKindScript:
		return valueobject.OutputKindScript, nil
	case valueobject.OutputKindSceneDSL:
		return "", fmt.Errorf("%w: %s", ErrUnsupportedKind, node.Params.OutputKind)
	default:
		return "", fmt.Errorf("%w: prompt cannot output %s", ErrNodeKindMismatch, node.Params.OutputKind)
	}
}

func loadAncestor(ctx context.Context, store port.ArtifactStore, node, child *entity.Node) (resolvedAncestor, error) {
	if err := ctx.Err(); err != nil {
		return resolvedAncestor{}, err
	}
	if len(node.Rounds) == 0 {
		return resolvedAncestor{}, fmt.Errorf("%w: node %s has no applied round", ErrAncestorNotReady, node.ID)
	}
	latest := node.Rounds[0]
	for _, round := range node.Rounds[1:] {
		if round.N > latest.N {
			latest = round
		}
	}
	expected := valueobject.OutputKindScript
	if node.Type == valueobject.NodeTypeTopic {
		expected = valueobject.OutputKindTopics
	}
	if latest.N <= 0 || latest.ArtifactID == "" || latest.Kind != expected {
		return resolvedAncestor{}, fmt.Errorf("%w: node %s has an invalid applied round", ErrAncestorNotReady, node.ID)
	}
	artifact, payload, err := store.Get(ctx, latest.ArtifactID)
	if err != nil {
		return resolvedAncestor{}, fmt.Errorf("%w: node %s: %w", ErrAncestorNotReady, node.ID, err)
	}
	if artifact.ID != latest.ArtifactID || len(payload) > valueobject.MaxPendingPayloadSize || !utf8.Valid(payload) || !json.Valid(payload) {
		return resolvedAncestor{}, fmt.Errorf("%w: node %s has an invalid artifact", ErrAncestorNotReady, node.ID)
	}
	if artifact.Kind != expected {
		if err := valueobject.ValidateStructuredPayload(expected, payload); err != nil {
			return resolvedAncestor{}, fmt.Errorf("%w: node %s artifact does not match its round kind", ErrAncestorNotReady, node.ID)
		}
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(payload, &object); err != nil || object == nil {
		return resolvedAncestor{}, fmt.Errorf("%w: node %s artifact must be an object", ErrAncestorNotReady, node.ID)
	}
	identity := ancestorIdentity{
		NodeID: node.ID, NodeType: node.Type, Title: node.Title, Params: node.Params,
		Round: latest.N, ArtifactKind: latest.Kind, ArtifactID: latest.ArtifactID,
	}
	selected := payload
	if node.Type == valueobject.NodeTypeTopic {
		var topics struct {
			Topics []json.RawMessage `json:"topics"`
		}
		if err := json.Unmarshal(payload, &topics); err != nil || child.ParentCandidate == nil || *child.ParentCandidate < 0 || *child.ParentCandidate >= len(topics.Topics) {
			return resolvedAncestor{}, fmt.Errorf("%w: topic %s", ErrCandidateInvalid, node.ID)
		}
		selected = topics.Topics[*child.ParentCandidate]
		var candidate map[string]json.RawMessage
		if err := json.Unmarshal(selected, &candidate); err != nil || candidate == nil {
			return resolvedAncestor{}, fmt.Errorf("%w: topic %s candidate is not an object", ErrCandidateInvalid, node.ID)
		}
		number := *child.ParentCandidate + 1
		identity.CandidateNumber = &number
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, selected); err != nil {
		return resolvedAncestor{}, fmt.Errorf("%w: node %s: %v", ErrAncestorNotReady, node.ID, err)
	}
	return resolvedAncestor{identity: identity, payload: append(json.RawMessage(nil), compact.Bytes()...)}, nil
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
