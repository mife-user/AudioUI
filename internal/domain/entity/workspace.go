package entity

import "audio-ui/internal/domain/valueobject"

// WorkspaceProject is the project identity in a workspace snapshot. Nodes are
// carried separately so the front end does not need to unpack project.json.
type WorkspaceProject struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Revision     int64  `json:"revision"`
	ActiveNodeID string `json:"activeNodeId"`
}

// PendingSummary is the payload-free view used by the workspace list.
type PendingSummary struct {
	ID      string                    `json:"id"`
	NodeID  string                    `json:"nodeId"`
	RunID   string                    `json:"runId"`
	ChainID string                    `json:"chainId"`
	Kind    valueobject.OutputKind    `json:"kind"`
	Status  valueobject.PendingStatus `json:"status"`
	Summary string                    `json:"summary,omitempty"`
	Error   string                    `json:"error,omitempty"`
}

// WorkspaceRunSummary is the payload-free view used by the workspace list.
type WorkspaceRunSummary struct {
	ID                string                `json:"id"`
	NodeID            string                `json:"nodeId"`
	Status            valueobject.RunStatus `json:"status"`
	ChainID           string                `json:"chainId"`
	ChainPath         []string              `json:"chainPath"`
	ContextHash       string                `json:"contextHash,omitempty"`
	EstimatedTokens   int                   `json:"estimatedTokens"`
	ActualInputTokens *int                  `json:"actualInputTokens,omitempty"`
	StartedAt         string                `json:"startedAt,omitempty"`
	FinishedAt        string                `json:"finishedAt,omitempty"`
}

// WorkspaceRunLock represents a run path locked while its starting record is
// being prepared. The persisted run summary replaces this placeholder once it
// becomes available.
type WorkspaceRunLock struct {
	RunID     string   `json:"runId"`
	TreeID    string   `json:"treeId"`
	NodeID    string   `json:"nodeId"`
	ChainPath []string `json:"chainPath"`
}

// WorkspaceChain is a leaf path derived from Graph.LeafChains.
type WorkspaceChain struct {
	ID         string   `json:"id"`
	TailNodeID string   `json:"tailNodeId"`
	Path       []string `json:"path"`
	Label      string   `json:"label"`
	Status     string   `json:"status"`
}

// WorkspaceSnapshot contains only the summary data required to draw a
// workspace. Run transcripts and pending/artifact payloads remain on demand.
type WorkspaceSnapshot struct {
	Project       *WorkspaceProject     `json:"project"`
	Nodes         []Node                `json:"nodes"`
	Pending       []PendingSummary      `json:"pending"`
	Runs          []WorkspaceRunSummary `json:"runs"`
	Chains        []WorkspaceChain      `json:"chains"`
	LockedNodeIDs []string              `json:"lockedNodeIds"`
}

// WorkspaceSnapshotResult pairs a snapshot with its process-local revision.
type WorkspaceSnapshotResult struct {
	Revision uint64            `json:"revision"`
	Snapshot WorkspaceSnapshot `json:"snapshot"`
}

// GraphMutationReceipt identifies the authoritative project revision produced
// by a graph command. NodeID is the created node for AddNode, otherwise the
// node directly affected by the command.
type GraphMutationReceipt struct {
	ProjectID       string `json:"projectId"`
	ProjectRevision int64  `json:"projectRevision"`
	NodeID          string `json:"nodeId"`
}
