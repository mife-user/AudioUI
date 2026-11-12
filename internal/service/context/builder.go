// Package context constructs the deterministic, path-scoped initial prompt for
// an M2 node run. It does not start a run or read workspace file contents.
package context

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"unicode/utf8"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
)

var (
	ErrNodeNotFound     = errors.New("NODE_NOT_FOUND")
	ErrNotInChain       = errors.New("NOT_IN_CHAIN")
	ErrAncestorNotReady = errors.New("ANCESTOR_NOT_READY")
	ErrCandidateInvalid = errors.New("CANDIDATE_INVALID")
	ErrNodeKindMismatch = errors.New("NODE_KIND_MISMATCH")
	ErrUnsupportedKind  = errors.New("UNSUPPORTED_UNTIL_M3")
	ErrWorkspaceScope   = errors.New("WORKSPACE_SCOPE_INVALID")
	ErrInvalidBudget    = errors.New("INVALID_CONTEXT_BUDGET")
	ErrContextTooLarge  = errors.New("CONTEXT_TOO_LARGE")
)

// ContextTooLargeError reports the final estimate after all ancestor outputs
// have been reduced to references. Target parameters are never truncated.
type ContextTooLargeError struct {
	EstimatedTokens int
	TokenBudget     int
}

func (e *ContextTooLargeError) Error() string {
	return fmt.Sprintf("%s: estimated %d tokens exceeds budget %d", ErrContextTooLarge, e.EstimatedTokens, e.TokenBudget)
}

func (e *ContextTooLargeError) Unwrap() error { return ErrContextTooLarge }

// Input uses a project snapshot and an overlay created for that exact graph
// path. A zero budget selects the configured default; negative budgets fail.
type Input struct {
	Project   entity.Project
	NodeID    string
	Snapshot  entity.RunExecutionSnapshot
	Workspace port.RunWorkspace
	Budget    int
}

// Result is ready for the later run service to inject as its first message.
type Result struct {
	TreeID         string
	NodeID         string
	ChainPath      []string
	OutputKind     valueobject.OutputKind
	ContextVersion int
	InitialPrompt  string
	Hashes         valueobject.ContextHashes
	Estimate       valueobject.ContextEstimate
}

type Builder struct {
	artifacts port.ArtifactStoreProvider
}

func NewBuilder(artifacts port.ArtifactStoreProvider) *Builder {
	return &Builder{artifacts: artifacts}
}

type targetSection struct {
	TreeID         string                 `json:"treeId"`
	ChainPath      []string               `json:"chainPath"`
	NodeID         string                 `json:"nodeId"`
	NodeType       valueobject.NodeType   `json:"nodeType"`
	OutputKind     valueobject.OutputKind `json:"outputKind"`
	ProjectVersion int                    `json:"-"`
}

type workspaceSection struct {
	CWD             string          `json:"cwd"`
	Overlay         []string        `json:"overlay"`
	Files           []WorkspaceFile `json:"files"`
	Truncated       bool            `json:"truncated"`
	NativeReadRoots []string        `json:"nativeReadRoots,omitempty"`
	NativeWriteRoot string          `json:"nativeWriteRoot,omitempty"`
}

type ancestorIdentity struct {
	NodeID          string                 `json:"nodeId"`
	NodeType        valueobject.NodeType   `json:"nodeType"`
	Title           string                 `json:"title"`
	Params          entity.NodeParams      `json:"params"`
	Round           int                    `json:"round"`
	ArtifactVersion int                    `json:"artifactVersion,omitempty"`
	ArtifactKind    valueobject.OutputKind `json:"artifactKind"`
	ArtifactID      string                 `json:"artifactId"`
	CandidateNumber *int                   `json:"candidateNumber,omitempty"`
}

type ancestorSection struct {
	ancestorIdentity
	Representation   string          `json:"representation"`
	Payload          json.RawMessage `json:"payload,omitempty"`
	Excerpt          string          `json:"excerpt,omitempty"`
	ExcerptTruncated *bool           `json:"excerptTruncated,omitempty"`
}

type contextBody struct {
	Version        int                       `json:"version"`
	Target         targetSection             `json:"target"`
	TaskNodeID     string                    `json:"taskNodeId,omitempty"`
	Task           entity.NodeParams         `json:"task"`
	Path           []ancestorSection         `json:"path"`
	Workspace      workspaceSection          `json:"workspace"`
	Contract       []string                  `json:"contract"`
	OutputBinding  *entity.RunOutputBinding  `json:"outputBinding,omitempty"`
	OutputContract *entity.RunOutputContract `json:"outputContract,omitempty"`
}

type promptMetadata struct {
	ContextVersion   int    `json:"contextVersion"`
	ContextHash      string `json:"contextHash"`
	EstimatorVersion int    `json:"estimatorVersion"`
	EstimatedTokens  int    `json:"estimatedTokens"`
}

type promptEnvelope struct {
	Context  json.RawMessage `json:"context"`
	Metadata promptMetadata  `json:"metadata"`
}

type resolvedAncestor struct {
	identity ancestorIdentity
	payload  json.RawMessage
}

type resolvedInput struct {
	target         targetSection
	params         entity.NodeParams
	targetParams   entity.NodeParams
	taskNodeID     string
	outputBinding  *entity.RunOutputBinding
	outputContract *entity.RunOutputContract
	ancestors      []resolvedAncestor
	workspace      workspaceSection
}

// Build creates the prompt, its hashes and a preflight estimate. If necessary,
// each ancestor is reduced in root-to-parent order: full, excerpt, reference.
// The immediate parent is therefore reduced last; target parameters stay whole.
func (b *Builder) Build(ctx context.Context, input Input) (Result, error) {
	budget := input.Budget
	if budget == 0 {
		budget = valueobject.DefaultContextTokenBudget
	}
	if budget < 0 {
		return Result{}, fmt.Errorf("%w: %d", ErrInvalidBudget, budget)
	}
	resolved, err := b.resolve(ctx, input, true)
	if err != nil {
		return Result{}, err
	}
	hashes, err := inputHashes(resolved)
	if err != nil {
		return Result{}, err
	}
	body := contextBody{
		Version: valueobject.DefaultContextVersion, Target: resolved.target,
		TaskNodeID: resolved.taskNodeID, Task: resolved.params,
		Workspace:     resolved.workspace,
		Contract:      outputContract(resolved.target, resolved.outputBinding),
		OutputBinding: resolved.outputBinding, OutputContract: resolved.outputContract,
	}
	levels := make([]int, len(resolved.ancestors))
	nextAncestor := 0
	for {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		body.Path = renderAncestors(resolved.ancestors, levels)
		prompt, contextHash, estimated, err := renderPrompt(body)
		if err != nil {
			return Result{}, err
		}
		if estimated <= budget {
			hashes.ContextHash = contextHash
			return Result{
				TreeID: resolved.target.TreeID, NodeID: resolved.target.NodeID,
				ChainPath:  append([]string(nil), resolved.target.ChainPath...),
				OutputKind: resolved.target.OutputKind, ContextVersion: valueobject.DefaultContextVersion,
				InitialPrompt: prompt, Hashes: hashes,
				Estimate: valueobject.ContextEstimate{EstimatorVersion: EstimatorVersion, EstimatedTokens: estimated, TokenBudget: budget},
			}, nil
		}
		if nextAncestor == len(levels) {
			return Result{}, &ContextTooLargeError{EstimatedTokens: estimated, TokenBudget: budget}
		}
		levels[nextAncestor]++
		if levels[nextAncestor] == 2 {
			nextAncestor++
		}
	}
}

func (b *Builder) BuildRunContext(ctx context.Context, project entity.Project, nodeID string, snapshot entity.RunExecutionSnapshot, workspace port.RunWorkspace, budget int) (port.RunContext, error) {
	result, err := b.Build(ctx, Input{Project: project, NodeID: nodeID, Snapshot: snapshot, Workspace: workspace, Budget: budget})
	if err != nil {
		return port.RunContext{}, err
	}
	return port.RunContext{
		TreeID: result.TreeID, NodeID: result.NodeID, ChainPath: append([]string(nil), result.ChainPath...),
		OutputKind: result.OutputKind, ContextVersion: result.ContextVersion,
		InitialPrompt: result.InitialPrompt, Hashes: result.Hashes, Estimate: result.Estimate,
	}, nil
}

// InputHashes recomputes the graph-ancestor and target-parameter fingerprints
// independently of budget, representation, or mutable workspace files. It is
// suitable for a future pending-application check; ContextHash is empty.
func (b *Builder) InputHashes(ctx context.Context, input Input) (valueobject.ContextHashes, error) {
	resolved, err := b.resolve(ctx, input, false)
	if err != nil {
		return valueobject.ContextHashes{}, err
	}
	return inputHashes(resolved)
}

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
		},
		params: params, targetParams: target.Params, taskNodeID: taskNodeID,
		outputBinding: outputBinding, outputContract: outputContract,
		ancestors: ancestors, workspace: workspace,
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

func renderAncestors(ancestors []resolvedAncestor, levels []int) []ancestorSection {
	sections := make([]ancestorSection, 0, len(ancestors))
	for i, ancestor := range ancestors {
		section := ancestorSection{ancestorIdentity: ancestor.identity}
		switch levels[i] {
		case 0:
			section.Representation = "full"
			section.Payload = ancestor.payload
		case 1:
			section.Representation = "excerpt"
			text, truncated := excerpt(string(ancestor.payload), ExcerptTokens)
			section.Excerpt = text
			section.ExcerptTruncated = &truncated
		default:
			section.Representation = "reference"
		}
		sections = append(sections, section)
	}
	return sections
}

func inputHashes(input resolvedInput) (valueobject.ContextHashes, error) {
	ancestors := make([]ancestorIdentity, 0, len(input.ancestors))
	for _, ancestor := range input.ancestors {
		ancestors = append(ancestors, ancestor.identity)
	}
	material := struct {
		Version   int                `json:"version"`
		TreeID    string             `json:"treeId"`
		Ancestors []ancestorIdentity `json:"ancestors"`
	}{valueobject.DefaultContextVersion, input.target.TreeID, ancestors}
	ancestorHash, err := hashJSON(material)
	if err != nil {
		return valueobject.ContextHashes{}, err
	}
	var paramsMaterial any = input.params
	if input.outputContract != nil {
		paramsMaterial = struct {
			TaskNodeID     string                    `json:"taskNodeId"`
			Task           entity.NodeParams         `json:"task"`
			Target         entity.NodeParams         `json:"target"`
			OutputBinding  *entity.RunOutputBinding  `json:"outputBinding"`
			OutputContract *entity.RunOutputContract `json:"outputContract"`
		}{input.taskNodeID, input.params, input.targetParams, input.outputBinding, input.outputContract}
	}
	paramsHash, err := hashJSON(paramsMaterial)
	if err != nil {
		return valueobject.ContextHashes{}, err
	}
	return valueobject.ContextHashes{AncestorHash: ancestorHash, ParamsHash: paramsHash}, nil
}

func outputContract(target targetSection, binding *entity.RunOutputBinding) []string {
	if target.NodeType == valueobject.NodeTypeTopic && target.ProjectVersion >= valueobject.ProjectFileVersion {
		contract := []string{
			"只根据当前路径与任务工作；禁止直接修改工程图。",
			"工作区文件仅提供逻辑路径与大小，不自动注入文件内容；二进制资源仅通过路径引用。",
			"有歧义时调用 ask_user；使用 reply(kind=topics)提交选题结果。",
			"合法 topics reply 由应用自动生成全部 Candidate 节点并锁定 Topic；不需要人工应用。",
			fmt.Sprintf("reply 的 node_id 必须是 %s，kind 必须是 %s。", target.NodeID, target.OutputKind),
		}
		return append(contract, `topics payload: {"topics":[{"title":"...","angle":"...","hook":"..."}]}；候选为 3–10 条。`)
	}
	if target.NodeType == valueobject.NodeTypeAgent {
		contract := []string{
			"只根据当前路径与任务工作；禁止直接修改工程图。",
			"工作区文件仅提供逻辑路径与大小，不自动注入文件内容；二进制资源仅通过路径引用。",
			"祖先产物的摘录或引用可用 read_artifact 按块读取（每次不超过 16 KiB）；候选、提示词任务与脚本版本均按发送快照读取。",
			"有歧义时调用 ask_user；结构化脚本结果必须通过 reply 提交。",
			"输出契约固定为 script@1。",
			fmt.Sprintf("reply 的 node_id 必须是 %s，kind 必须是 %s；输出 Script 节点以发送时绑定为准，不能自行指定。", target.NodeID, target.OutputKind),
		}
		switch {
		case binding == nil || !binding.Bound:
			contract = append(contract, "当前没有绑定 Script 输出节点；合法 reply 会自动创建直接 Script 节点并写入首个不可变版本，不需要人工确认。")
		case binding.BaselineVersion == 0:
			contract = append(contract, fmt.Sprintf("输出绑定到空 Script 节点 %s；合法 reply 会自动写入首个不可变版本，不需要人工确认。", binding.NodeID))
		default:
			contract = append(contract, fmt.Sprintf("输出绑定到已有版本的 Script 节点 %s，基准版本为 %d；reply 会创建待确认结果，用户确认后追加新版本，不覆盖旧版本；取消不改变旧版本。", binding.NodeID, binding.BaselineVersion))
		}
		return append(contract, `script payload: {"title":"...","scenes":[{"narration":"...","duration":0}]}；至少一个场景。`)
	}

	contract := []string{
		"只根据当前路径与任务工作；禁止直接修改工程图。",
		"工作区文件仅提供逻辑路径与大小，不自动注入文件内容；二进制资源仅通过路径引用。",
		"祖先产物的摘录或引用可用 read_artifact 按块读取（每次不超过 16 KiB）；topic 只返回当前路径所选候选。",
		"有歧义时调用 ask_user；结构化输出使用 reply，payload 不超过 256 KiB。",
		fmt.Sprintf("reply 的 node_id 必须是 %s，kind 必须是 %s；产物须经用户应用。", target.NodeID, target.OutputKind),
	}
	if target.OutputKind == valueobject.OutputKindTopics {
		return append(contract, `topics payload: {"topics":[{"title":"...","angle":"...","hook":"..."}]}；候选为 3–10 条。`)
	}
	return append(contract, `script payload: {"title":"...","scenes":[{"narration":"...","duration":0}]}；至少一个场景。`)
}

func renderPrompt(body contextBody) (string, string, int, error) {
	canonical, err := json.Marshal(body)
	if err != nil {
		return "", "", 0, err
	}
	contextHash := hashBytes(canonical)
	envelope := promptEnvelope{
		Context:  canonical,
		Metadata: promptMetadata{ContextVersion: valueobject.DefaultContextVersion, ContextHash: contextHash, EstimatorVersion: EstimatorVersion},
	}
	for i := 0; i < 16; i++ {
		encoded, err := json.MarshalIndent(envelope, "", "  ")
		if err != nil {
			return "", "", 0, err
		}
		estimated := EstimateTokens(string(encoded))
		if estimated == envelope.Metadata.EstimatedTokens {
			return string(encoded), contextHash, estimated, nil
		}
		envelope.Metadata.EstimatedTokens = estimated
	}
	return "", "", 0, errors.New("context estimate did not converge")
}

func hashJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return hashBytes(data), nil
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
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
