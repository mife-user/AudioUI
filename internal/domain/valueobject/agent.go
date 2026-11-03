package valueobject

import (
	"encoding/json"
	"time"
)

type SessionState string

const (
	SessionStarting     SessionState = "starting"
	SessionResuming     SessionState = "resuming"
	SessionIdle         SessionState = "idle"
	SessionRunning      SessionState = "running"
	SessionResumeFailed SessionState = "resume_failed"
	SessionError        SessionState = "error"
	SessionUnavailable  SessionState = "unavailable"
)

type TranscriptKind string

const (
	TranscriptMessage TranscriptKind = "message"
	TranscriptTool    TranscriptKind = "tool"
)

type TranscriptItem struct {
	ID          string         `json:"id"`
	Kind        TranscriptKind `json:"kind"`
	Role        string         `json:"role"`
	Text        string         `json:"text"`
	Streaming   bool           `json:"streaming"`
	ToolName    string         `json:"toolName,omitempty"`
	ToolStatus  string         `json:"toolStatus,omitempty"`
	ToolSummary string         `json:"toolSummary,omitempty"`
	CreatedAt   string         `json:"createdAt"`
}

type PermissionTier string

const (
	PermissionAutoAllow PermissionTier = "auto_allow"
	PermissionConfirm   PermissionTier = "confirm"
	PermissionHardDeny  PermissionTier = "hard_deny"
)

type PermissionRequest struct {
	ID           string         `json:"id"`
	Tool         string         `json:"tool"`
	Tier         PermissionTier `json:"tier"`
	Risk         string         `json:"risk"`
	Summary      string         `json:"summary"`
	Paths        []string       `json:"paths,omitempty"`
	Command      string         `json:"command,omitempty"`
	AllowSession bool           `json:"allowSession"`
	ExpiresAt    string         `json:"expiresAt"`
}

type PermissionDecision string

const (
	DecisionAllow        PermissionDecision = "allow"
	DecisionAllowSession PermissionDecision = "allow_session"
	DecisionDeny         PermissionDecision = "deny"
)

type QuestionRequest struct {
	ID         string   `json:"id"`
	Question   string   `json:"question"`
	Options    []string `json:"options"`
	AllowOther bool     `json:"allowOther"`
	ExpiresAt  string   `json:"expiresAt"`
}

type QuestionAnswer struct {
	Answer string `json:"answer"`
	Via    string `json:"via"`
}

type AgentError struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Recoverable bool   `json:"recoverable"`
}

type AgentCapabilities struct {
	MultiTurn          bool `json:"multiTurn"`
	Streaming          bool `json:"streaming"`
	ToolPermissions    bool `json:"toolPermissions"`
	Question           bool `json:"question"`
	ConversationRewind bool `json:"conversationRewind"`
	FileRewind         bool `json:"fileRewind"`
	ForkAtCheckpoint   bool `json:"forkAtCheckpoint"`
	Resume             bool `json:"resume"`
}

type AgentState struct {
	Version      int                 `json:"version"`
	Seq          uint64              `json:"seq"`
	ChainID      string              `json:"chainId"`
	SessionID    string              `json:"sessionId"`
	DisplayID    string              `json:"displayId"`
	Status       SessionState        `json:"status"`
	Adapter      string              `json:"adapter"`
	CLIAvailable bool                `json:"cliAvailable"`
	CLIError     string              `json:"cliError"`
	Permissions  []PermissionRequest `json:"permissions"`
	Questions    []QuestionRequest   `json:"questions"`
	Transcript   []TranscriptItem    `json:"transcript"`
	Notices      []string            `json:"notices"`
	ResumeError  string              `json:"resumeError"`
}

type SessionRecord struct {
	Adapter    string           `json:"adapter"`
	ResumeRef  json.RawMessage  `json:"resumeRef,omitempty"`
	DisplayID  string           `json:"displayId"`
	Workspace  string           `json:"workspace"`
	Status     SessionState     `json:"status"`
	UpdatedAt  string           `json:"updatedAt"`
	Transcript []TranscriptItem `json:"transcript"`
}

type SessionsFile struct {
	Version       int                      `json:"version"`
	ActiveChainID string                   `json:"activeChainId"`
	Chains        map[string]SessionRecord `json:"chains"`
}

type AgentEventType string

const (
	EventStatus          AgentEventType = "status"
	EventMessage         AgentEventType = "message"
	EventTool            AgentEventType = "tool"
	EventPermission      AgentEventType = "permission"
	EventQuestion        AgentEventType = "question"
	EventError           AgentEventType = "error"
	EventPending         AgentEventType = "pending"
	EventRequestResolved AgentEventType = "request_resolved"
)

type AgentEvent struct {
	Version           int                `json:"v"`
	Seq               uint64             `json:"seq,omitempty"`
	Type              AgentEventType     `json:"type"`
	State             *AgentState        `json:"state,omitempty"`
	Message           *TranscriptItem    `json:"message,omitempty"`
	Tool              *TranscriptItem    `json:"tool,omitempty"`
	Permission        *PermissionRequest `json:"permission,omitempty"`
	Question          *QuestionRequest   `json:"question,omitempty"`
	Error             *AgentError        `json:"error,omitempty"`
	ActualInputTokens *int               `json:"actualInputTokens,omitempty"`
}

// RunEvent is the run-scoped event envelope consumed by the M2 workspace.
// Every event carries enough identity to reject updates from another run.
type RunEvent struct {
	Version           int                `json:"v"`
	Seq               uint64             `json:"seq"`
	RunID             string             `json:"runId"`
	ProjectID         string             `json:"projectId"`
	TreeID            string             `json:"treeId"`
	ChainID           string             `json:"chainId"`
	NodeID            string             `json:"nodeId"`
	ChainPath         []string           `json:"chainPath"`
	Type              AgentEventType     `json:"type"`
	Status            RunStatus          `json:"status,omitempty"`
	Message           *TranscriptItem    `json:"message,omitempty"`
	Tool              *TranscriptItem    `json:"tool,omitempty"`
	Permission        *PermissionRequest `json:"permission,omitempty"`
	Question          *QuestionRequest   `json:"question,omitempty"`
	ResolvedKind      string             `json:"resolvedKind,omitempty"`
	ResolvedID        string             `json:"resolvedId,omitempty"`
	PendingID         string             `json:"pendingId,omitempty"`
	Error             *AgentError        `json:"error,omitempty"`
	ActualInputTokens *int               `json:"actualInputTokens,omitempty"`
}

// WorkspaceChanged is the process-local invalidation event for workspace
// snapshots. ProjectRevision is the durable graph/pending revision.
type WorkspaceChanged struct {
	ProjectID       string `json:"projectId"`
	Revision        uint64 `json:"revision"`
	Reason          string `json:"reason"`
	ProjectRevision int64  `json:"projectRevision"`
}

const (
	DefaultChainID      = "chain_default"
	DefaultWorkspaceDir = "workspace/chain_default"
	SessionFileVersion  = 1
	MaxTranscriptItems  = 200
	PermissionTimeout   = 120 * time.Second
	QuestionTimeout     = 120 * time.Second
)
