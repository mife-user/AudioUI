package entity

import (
	"audio-ui/internal/domain/valueobject"
)

// RunSummary is the runs.json index entry. Transcript and context preview
// live in runs/<runId>.json so listing runs stays cheap.
type RunSummary struct {
	ID                string                `json:"id"`
	TreeID            string                `json:"treeId"`
	NodeID            string                `json:"nodeId"`
	ChainID           string                `json:"chainId"`
	ChainPath         []string              `json:"chainPath"`
	Status            valueobject.RunStatus `json:"status"`
	ContextVersion    int                   `json:"contextVersion"`
	EstimatorVersion  int                   `json:"estimatorVersion,omitempty"`
	ContextHash       string                `json:"contextHash,omitempty"`
	EstimatedTokens   int                   `json:"estimatedTokens,omitempty"`
	TokenBudget       int                   `json:"tokenBudget,omitempty"`
	ActualInputTokens int                   `json:"actualInputTokens,omitempty"`
	UsageReported     bool                  `json:"usageReported,omitempty"`
	OverBudget        bool                  `json:"overBudget,omitempty"`
	Error             string                `json:"error,omitempty"`
	PendingID         string                `json:"pendingId,omitempty"`
	StartedAt         string                `json:"startedAt,omitempty"`
	FinishedAt        string                `json:"finishedAt,omitempty"`
}

type RunArtifactReference struct {
	NodeID         string                 `json:"nodeId"`
	EdgeID         string                 `json:"edgeId,omitempty"`
	ArtifactID     string                 `json:"artifactId"`
	Kind           valueobject.OutputKind `json:"kind"`
	CandidateIndex *int                   `json:"candidateIndex,omitempty"`
	Version        int                    `json:"version,omitempty"`
}

type RunOutputBinding struct {
	Bound              bool   `json:"bound"`
	Mode               string `json:"mode,omitempty"`
	NodeID             string `json:"nodeId,omitempty"`
	ParentNodeID       string `json:"parentNodeId,omitempty"`
	BaselineVersion    int    `json:"baselineVersion"`
	BaselineArtifactID string `json:"baselineArtifactId,omitempty"`
}

const (
	RunOutputModeExisting = "existing"
	RunOutputModeCreate   = "create"
)

type RunStartOptions struct {
	InputEdgeIDs []string `json:"inputEdgeIds,omitempty"`
	OutputNodeID string   `json:"outputNodeId,omitempty"`
	CreateOutput bool     `json:"createOutput,omitempty"`
	TopicRerun   bool     `json:"topicRerun,omitempty"`
}

type RunTopicBatch struct {
	BaselineRound int `json:"baselineRound"`
}

type RunOutputContract struct {
	Kind          valueobject.OutputKind `json:"kind"`
	SchemaVersion int                    `json:"schemaVersion"`
}

type RunInput struct {
	EdgeID       string `json:"edgeId"`
	SourceNodeID string `json:"sourceNodeId"`
	Role         string `json:"role"`
	Order        int    `json:"order"`
}

type RunExecutionSnapshot struct {
	Version         int                    `json:"version"`
	ProjectVersion  int                    `json:"projectVersion"`
	ProjectID       string                 `json:"projectId"`
	ProjectRevision int64                  `json:"projectRevision"`
	TreeID          string                 `json:"treeId,omitempty"`
	NodeID          string                 `json:"nodeId"`
	ChainPath       []string               `json:"chainPath"`
	PathNodes       []Node                 `json:"pathNodes,omitempty"`
	PathEdges       []Edge                 `json:"pathEdges,omitempty"`
	InputNodes      []Node                 `json:"inputNodes,omitempty"`
	InputRoots      []Node                 `json:"inputRoots,omitempty"`
	InputEdges      []Edge                 `json:"inputEdges,omitempty"`
	Inputs          []RunInput             `json:"inputs,omitempty"`
	Artifacts       []RunArtifactReference `json:"artifacts"`
	OutputBinding   RunOutputBinding       `json:"outputBinding"`
	OutputNode      *Node                  `json:"outputNode,omitempty"`
	OutputEdge      *Edge                  `json:"outputEdge,omitempty"`
	TopicBatch      *RunTopicBatch         `json:"topicBatch,omitempty"`
	OutputContract  RunOutputContract      `json:"outputContract"`
}

func (s RunExecutionSnapshot) Clone() RunExecutionSnapshot {
	clone := s
	clone.ChainPath = append([]string(nil), s.ChainPath...)
	clone.PathEdges = append([]Edge(nil), s.PathEdges...)
	clone.InputEdges = append([]Edge(nil), s.InputEdges...)
	clone.Inputs = append([]RunInput(nil), s.Inputs...)
	clone.PathNodes = cloneNodes(s.PathNodes)
	clone.InputNodes = cloneNodes(s.InputNodes)
	clone.InputRoots = cloneNodes(s.InputRoots)
	if s.OutputNode != nil {
		outputNode := s.OutputNode.Clone()
		clone.OutputNode = &outputNode
	}
	if s.OutputEdge != nil {
		outputEdge := *s.OutputEdge
		clone.OutputEdge = &outputEdge
	}
	if s.TopicBatch != nil {
		topicBatch := *s.TopicBatch
		clone.TopicBatch = &topicBatch
	}
	clone.Artifacts = make([]RunArtifactReference, len(s.Artifacts))
	for i, reference := range s.Artifacts {
		clone.Artifacts[i] = reference
		if reference.CandidateIndex != nil {
			index := *reference.CandidateIndex
			clone.Artifacts[i].CandidateIndex = &index
		}
	}
	return clone
}

func cloneNodes(nodes []Node) []Node {
	clone := make([]Node, len(nodes))
	for i, node := range nodes {
		clone[i] = node.Clone()
	}
	return clone
}

func (s RunExecutionSnapshot) ScopeNodeIDs() []string {
	if s.Version == 3 || s.Version == 4 {
		ids := make([]string, len(s.InputNodes))
		for i, node := range s.InputNodes {
			ids[i] = node.ID
		}
		return ids
	}
	return append([]string(nil), s.ChainPath...)
}

// RunRecord is the per-run detail document.
type RunRecord struct {
	RunSummary
	ContextPreview          string                       `json:"contextPreview,omitempty"`
	ContextPreviewTruncated bool                         `json:"contextPreviewTruncated,omitempty"`
	ExecutionSnapshot       *RunExecutionSnapshot        `json:"executionSnapshot,omitempty"`
	Transcript              []valueobject.TranscriptItem `json:"transcript"`
}

// RunSnapshot is the in-memory recovery view used to resume an event stream.
// ProjectID and LastSeq are deliberately not persisted in runs.json: events
// and their sequence numbers are scoped to one process lifetime.
type RunSnapshot struct {
	RunID       string                          `json:"runId"`
	ProjectID   string                          `json:"projectId"`
	Run         RunRecord                       `json:"run"`
	LastSeq     uint64                          `json:"lastSeq"`
	Permissions []valueobject.PermissionRequest `json:"permissions"`
	Questions   []valueobject.QuestionRequest   `json:"questions"`
}

// RunsFile is the runs.json index document.
type RunsFile struct {
	Version int          `json:"version"`
	Runs    []RunSummary `json:"runs"`
}

// RunStartResult is the structured response to a UI start request. Expected
// business failures stay in the response so callers can render typed details
// such as a context-budget estimate without parsing error text.
type RunStartResult struct {
	Run   *RunRecord     `json:"run,omitempty"`
	Error *RunStartError `json:"error,omitempty"`
}

// RunStartError is a safe, read-only error view for a rejected start request.
type RunStartError struct {
	Code            string `json:"code"`
	Message         string `json:"message"`
	EstimatedTokens *int   `json:"estimatedTokens,omitempty"`
	TokenBudget     *int   `json:"tokenBudget,omitempty"`
}
