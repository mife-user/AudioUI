package valueobject

// NodeType is the closed set of node types supported by M2.
type NodeType string

const (
	NodeTypeTopic     NodeType = "topic"
	NodeTypeCandidate NodeType = "candidate"
	NodeTypePrompt    NodeType = "prompt"
	NodeTypeAgent     NodeType = "agent"
	NodeTypeScript    NodeType = "script"
)

func (t NodeType) Valid() bool {
	switch t {
	case NodeTypeTopic, NodeTypeCandidate, NodeTypePrompt, NodeTypeAgent, NodeTypeScript:
		return true
	default:
		return false
	}
}

// OutputKind identifies the structured output produced by a node.
type OutputKind string

const (
	OutputKindTopics   OutputKind = "topics"
	OutputKindScript   OutputKind = "script"
	OutputKindSceneDSL OutputKind = "scene_dsl"
)

func (k OutputKind) Valid() bool {
	switch k {
	case OutputKindTopics, OutputKindScript, OutputKindSceneDSL:
		return true
	default:
		return false
	}
}

// RunStatus follows the run lifecycle in agent-protocol.md.
type RunStatus string

const (
	RunStarting    RunStatus = "starting"
	RunRunning     RunStatus = "running"
	RunSucceeded   RunStatus = "succeeded"
	RunFailed      RunStatus = "failed"
	RunInterrupted RunStatus = "interrupted"
	RunImported    RunStatus = "imported"
)

func (s RunStatus) Valid() bool {
	switch s {
	case RunStarting, RunRunning, RunSucceeded, RunFailed, RunInterrupted, RunImported:
		return true
	default:
		return false
	}
}

// PendingStatus is the lifecycle state of a pending artifact.
type PendingStatus string

const (
	PendingPending   PendingStatus = "pending"
	PendingInvalid   PendingStatus = "invalid"
	PendingApplied   PendingStatus = "applied"
	PendingDiscarded PendingStatus = "discarded"
)

func (s PendingStatus) Valid() bool {
	switch s {
	case PendingPending, PendingInvalid, PendingApplied, PendingDiscarded:
		return true
	default:
		return false
	}
}

const (
	DAGProjectVersion             = 4
	ProjectFileVersion            = 5
	RunsFileVersion               = 1
	PendingFileVersion            = 1
	PendingTransactionFileVersion = 1

	DefaultContextTokenBudget = 1_000_000
	DefaultContextVersion     = 1

	MaxRunTranscriptItems     = 500
	MaxProjectNameLength      = 120
	MaxNodeTitleLength        = 200
	MaxPendingPayloadSize     = 256 * 1024
	MaxPendingPerTree         = 50
	MaxPendingRequestIDLength = 128
)
