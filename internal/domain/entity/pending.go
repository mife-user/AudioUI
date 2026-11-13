package entity

import "audio-ui/internal/domain/valueobject"

// Pending binds one structured Agent output to the run and path that
// produced it. The raw payload is stored separately so invalid output is
// preserved.
type Pending struct {
	ID                  string                    `json:"id"`
	RunID               string                    `json:"runId"`
	RequestID           string                    `json:"requestId,omitempty"`
	ChainID             string                    `json:"chainId"`
	ChainPath           []string                  `json:"chainPath"`
	NodeID              string                    `json:"nodeId"`
	Kind                valueobject.OutputKind    `json:"kind"`
	Status              valueobject.PendingStatus `json:"status"`
	ContextHash         string                    `json:"contextHash,omitempty"`
	AncestorHash        string                    `json:"ancestorHash,omitempty"`
	ParamsHash          string                    `json:"paramsHash,omitempty"`
	Payload             string                    `json:"payload"`
	Summary             string                    `json:"summary,omitempty"`
	Error               string                    `json:"error,omitempty"`
	ArtifactID          string                    `json:"artifactId,omitempty"`
	OutputBinding       *RunOutputBinding         `json:"outputBinding,omitempty"`
	OutputSchemaVersion int                       `json:"outputSchemaVersion,omitempty"`
	ExecutionSnapshot   *RunExecutionSnapshot     `json:"executionSnapshot,omitempty"`
	ResultNodeID        string                    `json:"resultNodeId,omitempty"`
	CreatedNodeIDs      []string                  `json:"createdNodeIds,omitempty"`
	CreatedAt           string                    `json:"createdAt"`
}

func (p Pending) Clone() Pending {
	clone := p
	clone.ChainPath = append([]string(nil), p.ChainPath...)
	clone.CreatedNodeIDs = append([]string(nil), p.CreatedNodeIDs...)
	if p.OutputBinding != nil {
		binding := *p.OutputBinding
		clone.OutputBinding = &binding
	}
	if p.ExecutionSnapshot != nil {
		snapshot := p.ExecutionSnapshot.Clone()
		clone.ExecutionSnapshot = &snapshot
	}
	return clone
}

// PendingFile is the pending.json index document.
type PendingFile struct {
	Version int       `json:"version"`
	Pending []Pending `json:"pending"`
}

type PendingDetail struct {
	Pending    Pending `json:"pending"`
	RawPayload []byte  `json:"rawPayload"`
}

// Revision is the commit point used to distinguish rollback from replay.
type PendingTransaction struct {
	Version         int           `json:"version"`
	BaseRevision    int64         `json:"baseRevision"`
	TargetRevision  int64         `json:"targetRevision"`
	Pending         Pending       `json:"pending"`
	Payload         []byte        `json:"payload"`
	ArtifactPayload []byte        `json:"artifactPayload,omitempty"`
	ArtifactMeta    *ArtifactMeta `json:"artifactMeta,omitempty"`
}
