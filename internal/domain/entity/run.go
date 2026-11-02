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
	ContextHash       string                `json:"contextHash,omitempty"`
	EstimatedTokens   int                   `json:"estimatedTokens,omitempty"`
	ActualInputTokens int                   `json:"actualInputTokens,omitempty"`
	OverBudget        bool                  `json:"overBudget,omitempty"`
	Error             string                `json:"error,omitempty"`
	PendingID         string                `json:"pendingId,omitempty"`
	StartedAt         string                `json:"startedAt,omitempty"`
	FinishedAt        string                `json:"finishedAt,omitempty"`
}

// RunRecord is the per-run detail document.
type RunRecord struct {
	RunSummary
	ContextPreview string                       `json:"contextPreview,omitempty"`
	Transcript     []valueobject.TranscriptItem `json:"transcript"`
}

// RunsFile is the runs.json index document.
type RunsFile struct {
	Version int          `json:"version"`
	Runs    []RunSummary `json:"runs"`
}
