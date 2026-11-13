package entity

import "audio-ui/internal/domain/valueobject"

// ArtifactMeta is persisted beside an immutable artifact payload.
type ArtifactMeta struct {
	Kind            valueobject.OutputKind `json:"kind"`
	NodeID          string                 `json:"nodeId"`
	CreatedAt       string                 `json:"createdAt"`
	SourceRunID     string                 `json:"sourceRunId,omitempty"`
	SourcePendingID string                 `json:"sourcePendingId,omitempty"`
}

// Artifact is an applied, content-addressed result.
type Artifact struct {
	ID        string                 `json:"id"`
	Kind      valueobject.OutputKind `json:"kind"`
	NodeID    string                 `json:"nodeId"`
	Path      string                 `json:"path"`
	CreatedAt string                 `json:"createdAt"`
}

// TopicCandidate is one output port from an applied topic artifact.
type TopicCandidate struct {
	NodeID string `json:"nodeId,omitempty"`
	Round  int    `json:"round,omitempty"`
	RunID  string `json:"runId,omitempty"`
	Index  int    `json:"index"`
	Title  string `json:"title"`
	Angle  string `json:"angle"`
	Hook   string `json:"hook"`
}
