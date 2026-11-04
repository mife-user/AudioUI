package entity

import "audio-ui/internal/domain/valueobject"

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

// RunRecord is the per-run detail document.
type RunRecord struct {
	RunSummary
	ContextPreview          string                       `json:"contextPreview,omitempty"`
	ContextPreviewTruncated bool                         `json:"contextPreviewTruncated,omitempty"`
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
