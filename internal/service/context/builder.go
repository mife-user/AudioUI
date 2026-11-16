// Package context constructs the deterministic, path-scoped initial prompt for
// an M2 node run. It does not start a run or read workspace file contents.
package context

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

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
	InputNodeIDs   []string
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
	TopicBatch     *entity.RunTopicBatch  `json:"topicBatch,omitempty"`
}

type workspaceSection struct {
	CWD               string                 `json:"cwd"`
	Overlay           []string               `json:"overlay"`
	Files             []WorkspaceFile        `json:"files"`
	Truncated         bool                   `json:"truncated"`
	NativeReadRoots   []string               `json:"nativeReadRoots,omitempty"`
	NativeReadSources []port.WorkspaceSource `json:"nativeReadSources,omitempty"`
	NativeWriteRoot   string                 `json:"nativeWriteRoot,omitempty"`
}

type taskInputSection struct {
	NodeID string            `json:"nodeId"`
	Title  string            `json:"title"`
	Order  int               `json:"order"`
	Task   entity.NodeParams `json:"task"`
}

type workspaceRootIdentity struct {
	NodeID string `json:"nodeId"`
	RootID string `json:"rootId,omitempty"`
}

type ancestorIdentity struct {
	NodeID          string                 `json:"nodeId"`
	SourceEdgeID    string                 `json:"sourceEdgeId,omitempty"`
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
	Path           []ancestorSection         `json:"path,omitempty"`
	Inputs         []entity.RunInput         `json:"inputs,omitempty"`
	InputEdges     []entity.Edge             `json:"inputEdges,omitempty"`
	Tasks          []taskInputSection        `json:"tasks,omitempty"`
	Sources        []ancestorSection         `json:"sources,omitempty"`
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
	shared   bool
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
	inputs         []entity.RunInput
	inputEdges     []entity.Edge
	tasks          []taskInputSection
	inputNodeIDs   []string
	workspaceRoots []workspaceRootIdentity
	dag            bool
	contextVersion int
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
	contextVersion := resolved.contextVersion
	if contextVersion == 0 {
		contextVersion = valueobject.DefaultContextVersion
	}
	body := contextBody{
		Version: contextVersion, Target: resolved.target,
		TaskNodeID: resolved.taskNodeID, Task: resolved.params,
		Inputs:        append([]entity.RunInput(nil), resolved.inputs...),
		InputEdges:    append([]entity.Edge(nil), resolved.inputEdges...),
		Tasks:         append([]taskInputSection(nil), resolved.tasks...),
		Workspace:     resolved.workspace,
		Contract:      outputContract(resolved.target, resolved.outputBinding),
		OutputBinding: resolved.outputBinding, OutputContract: resolved.outputContract,
	}
	prompt, contextHash, estimated, err := renderWithinBudget(ctx, body, resolved.ancestors, resolved.dag, budget)
	if err != nil {
		return Result{}, err
	}
	hashes.ContextHash = contextHash
	return Result{
		TreeID: resolved.target.TreeID, NodeID: resolved.target.NodeID,
		ChainPath:    append([]string(nil), resolved.target.ChainPath...),
		InputNodeIDs: append([]string(nil), resolved.inputNodeIDs...),
		OutputKind:   resolved.target.OutputKind, ContextVersion: contextVersion,
		InitialPrompt: prompt, Hashes: hashes,
		Estimate: valueobject.ContextEstimate{EstimatorVersion: EstimatorVersion, EstimatedTokens: estimated, TokenBudget: budget},
	}, nil
}

func (b *Builder) BuildRunContext(ctx context.Context, project entity.Project, nodeID string, snapshot entity.RunExecutionSnapshot, workspace port.RunWorkspace, budget int) (port.RunContext, error) {
	result, err := b.Build(ctx, Input{Project: project, NodeID: nodeID, Snapshot: snapshot, Workspace: workspace, Budget: budget})
	if err != nil {
		return port.RunContext{}, err
	}
	return port.RunContext{
		TreeID: result.TreeID, NodeID: result.NodeID, ChainPath: append([]string(nil), result.ChainPath...),
		InputNodeIDs: append([]string(nil), result.InputNodeIDs...),
		OutputKind:   result.OutputKind, ContextVersion: result.ContextVersion,
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

func (b *Builder) InputHashesForSnapshot(ctx context.Context, project entity.Project, snapshot entity.RunExecutionSnapshot) (valueobject.ContextHashes, error) {
	return b.InputHashes(ctx, Input{Project: project, NodeID: snapshot.NodeID, Snapshot: snapshot})
}

func inputHashes(input resolvedInput) (valueobject.ContextHashes, error) {
	ancestors := make([]ancestorIdentity, 0, len(input.ancestors))
	for _, ancestor := range input.ancestors {
		ancestors = append(ancestors, ancestor.identity)
	}
	version := input.contextVersion
	if version == 0 {
		version = valueobject.DefaultContextVersion
	}
	material := struct {
		Version        int                     `json:"version"`
		TreeID         string                  `json:"treeId"`
		Ancestors      []ancestorIdentity      `json:"ancestors"`
		Inputs         []entity.RunInput       `json:"inputs,omitempty"`
		InputEdges     []entity.Edge           `json:"inputEdges,omitempty"`
		WorkspaceRoots []workspaceRootIdentity `json:"workspaceRoots,omitempty"`
	}{version, input.target.TreeID, ancestors, input.inputs, input.inputEdges, input.workspaceRoots}
	ancestorHash, err := hashJSON(material)
	if err != nil {
		return valueobject.ContextHashes{}, err
	}
	var paramsMaterial any = input.params
	if input.target.TopicBatch != nil {
		paramsMaterial = struct {
			Params     entity.NodeParams     `json:"params"`
			TopicBatch *entity.RunTopicBatch `json:"topicBatch"`
		}{input.targetParams, input.target.TopicBatch}
	}
	if input.outputContract != nil && input.dag {
		paramsMaterial = struct {
			Tasks          []taskInputSection        `json:"tasks"`
			Target         entity.NodeParams         `json:"target"`
			OutputBinding  *entity.RunOutputBinding  `json:"outputBinding"`
			OutputContract *entity.RunOutputContract `json:"outputContract"`
		}{input.tasks, input.targetParams, input.outputBinding, input.outputContract}
	} else if input.outputContract != nil {
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
			fmt.Sprintf("reply 的 node_id 必须是 %s，kind 必须是 %s。", target.NodeID, target.OutputKind),
		}
		if target.TopicBatch == nil {
			contract = append(contract, "合法 topics reply 由应用自动生成首批 Candidate 节点并锁定 Topic；不需要人工应用。")
		} else {
			contract = append(contract, fmt.Sprintf("这是 Topic 重跑，沿用锁定参数并基于第 %d 批次生成新结果；合法 reply 进入待确认预览，确认后创建独立 Candidate 批次。", target.TopicBatch.BaselineRound))
		}
		return append(contract, `topics payload: {"topics":[{"title":"...","angle":"...","hook":"..."}]}；候选为 3–10 条。`)
	}
	if target.NodeType == valueobject.NodeTypeAgent {
		var contract []string
		if len(target.ChainPath) == 0 {
			contract = []string{
				"只根据本次已选输入及其冻结祖先工作，不推断或补入其他连接；禁止直接修改工程图。",
				"多个 Prompt 按输入顺序分别作为任务，不自动合并或裁决冲突；Candidate 与 Script 按冻结来源作为上下文。",
				"输入工作区按来源根和节点隔离，只能读取授权输入目录及 exe 同级只读 runtime；工作区文件正文不自动注入。",
				"祖先产物的摘录或引用可用 read_artifact 按块读取（每次不超过 16 KiB）；候选及脚本版本均按发送快照读取。",
				"有歧义时调用 ask_user；结构化脚本结果必须通过 reply 提交。",
				"输出契约固定为 script@1。",
				fmt.Sprintf("reply 的 node_id 必须是 %s，kind 必须是 %s；输出 Script 节点以发送时绑定为准，不能自行指定。", target.NodeID, target.OutputKind),
			}
		} else {
			contract = []string{
				"只根据当前路径与任务工作；禁止直接修改工程图。",
				"工作区文件仅提供逻辑路径与大小，不自动注入文件内容；二进制资源仅通过路径引用。",
				"祖先产物的摘录或引用可用 read_artifact 按块读取（每次不超过 16 KiB）；候选、提示词任务与脚本版本均按发送快照读取。",
				"有歧义时调用 ask_user；结构化脚本结果必须通过 reply 提交。",
				"输出契约固定为 script@1。",
				fmt.Sprintf("reply 的 node_id 必须是 %s，kind 必须是 %s；输出 Script 节点以发送时绑定为准，不能自行指定。", target.NodeID, target.OutputKind),
			}
		}
		switch {
		case binding == nil || !binding.Bound:
			contract = append(contract, "当前没有绑定 Script 输出节点；合法 reply 会自动创建直接 Script 节点并写入首个不可变版本，不需要人工确认。")
		case binding.Mode == entity.RunOutputModeCreate:
			contract = append(contract, fmt.Sprintf("本次输出目标是新建 Script 节点 %s；该节点和版本仅在合法 reply 原子提交时创建。", binding.NodeID))
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
		Metadata: promptMetadata{ContextVersion: body.Version, ContextHash: contextHash, EstimatorVersion: EstimatorVersion},
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
