package entity

import (
	"math"

	"audio-ui/internal/domain/valueobject"
)

const (
	MinNodeWidth  = 200
	MinNodeHeight = 140
	MaxNodeWidth  = 1200
	MaxNodeHeight = 900
)

type NodeSize struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

func (s NodeSize) Valid() bool {
	return !math.IsNaN(s.Width) && !math.IsInf(s.Width, 0) &&
		!math.IsNaN(s.Height) && !math.IsInf(s.Height, 0) &&
		s.Width >= MinNodeWidth && s.Width <= MaxNodeWidth &&
		s.Height >= MinNodeHeight && s.Height <= MaxNodeHeight
}

func DefaultNodeSize(nodeType valueobject.NodeType) NodeSize {
	switch nodeType {
	case valueobject.NodeTypeTopic:
		return NodeSize{Width: 280, Height: 250}
	case valueobject.NodeTypeCandidate:
		return NodeSize{Width: 235, Height: 160}
	case valueobject.NodeTypePrompt:
		return NodeSize{Width: 280, Height: 230}
	case valueobject.NodeTypeAgent:
		return NodeSize{Width: 235, Height: 165}
	case valueobject.NodeTypeScript:
		return NodeSize{Width: 270, Height: 205}
	default:
		return NodeSize{}
	}
}

// Position is the persisted canvas position of a node.
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// NodeConstraints holds the prompt constraints used by v1 nodes.
type NodeConstraints struct {
	Scenes   int `json:"scenes,omitempty"`
	Duration int `json:"duration,omitempty"`
}

// NodeParams is the type-specific parameter bag persisted with a node.
// Topic nodes use Idea/Direction/CandidateCount; prompt nodes use
// Instruction/OutputKind/Constraints.
type NodeParams struct {
	Idea           string                 `json:"idea,omitempty"`
	Direction      string                 `json:"direction,omitempty"`
	CandidateCount int                    `json:"candidateCount,omitempty"`
	Instruction    string                 `json:"instruction,omitempty"`
	OutputKind     valueobject.OutputKind `json:"outputKind,omitempty"`
	Constraints    *NodeConstraints       `json:"constraints,omitempty"`
}

// Round is one applied output of a node. Rounds reference immutable
// content-addressed artifacts and the pending record that produced them.
type Round struct {
	N          int                    `json:"n"`
	Kind       valueobject.OutputKind `json:"kind"`
	ArtifactID string                 `json:"artifactId"`
	PendingID  string                 `json:"pendingId"`
	RunID      string                 `json:"runId"`
}

type CandidateSource struct {
	TopicNodeID string `json:"topicNodeId"`
	ArtifactID  string `json:"artifactId"`
	Index       int    `json:"index"`
}

type ScriptVersion struct {
	N          int    `json:"n"`
	ArtifactID string `json:"artifactId"`
	RunID      string `json:"runId"`
}

type ScriptVersionDetail struct {
	NodeID  string        `json:"nodeId"`
	Version ScriptVersion `json:"version"`
	Payload string        `json:"payload"`
}

// Node is one persisted graph node. Edges are derived from Parent.
type Node struct {
	ID              string               `json:"id"`
	Type            valueobject.NodeType `json:"type"`
	Title           string               `json:"title"`
	Parent          string               `json:"parent,omitempty"`
	ParentCandidate *int                 `json:"parentCandidate,omitempty"`
	CandidateSource *CandidateSource     `json:"candidateSource,omitempty"`
	ScriptVersions  []ScriptVersion      `json:"scriptVersions,omitempty"`
	Position        Position             `json:"position"`
	Size            NodeSize             `json:"size"`
	Params          NodeParams           `json:"params"`
	Rounds          []Round              `json:"rounds"`
	Locked          bool                 `json:"locked"`
	Stale           bool                 `json:"stale"`
}

// Project is the persisted project.json document. Chains are derived views
// and are intentionally absent.
type Project struct {
	Version      int    `json:"version"`
	ID           string `json:"id"`
	Name         string `json:"name"`
	Revision     int64  `json:"revision"`
	ActiveNodeID string `json:"activeNodeId"`
	Nodes        []Node `json:"nodes"`
}

// ProjectSummary is the lightweight project list entry.
type ProjectSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Revision  int64  `json:"revision"`
	Version   int    `json:"version,omitempty"`
	ErrorCode string `json:"errorCode,omitempty"`
}

// Clone returns a deep copy safe to hand to callers outside the service lock.
func (p Project) Clone() Project {
	clone := p
	clone.Nodes = make([]Node, len(p.Nodes))
	for i, node := range p.Nodes {
		clone.Nodes[i] = node.Clone()
	}
	return clone
}

// Clone returns a deep copy of a node.
func (n Node) Clone() Node {
	clone := n
	if n.ParentCandidate != nil {
		value := *n.ParentCandidate
		clone.ParentCandidate = &value
	}
	if n.CandidateSource != nil {
		source := *n.CandidateSource
		clone.CandidateSource = &source
	}
	clone.ScriptVersions = append([]ScriptVersion(nil), n.ScriptVersions...)
	if n.Params.Constraints != nil {
		constraints := *n.Params.Constraints
		clone.Params.Constraints = &constraints
	}
	clone.Rounds = append([]Round(nil), n.Rounds...)
	return clone
}
