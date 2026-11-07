package port

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/valueobject"
)

var (
	ErrTurnNotActive    = errors.New("TURN_NOT_ACTIVE")
	ErrChainScopeDenied = errors.New("CHAIN_SCOPE_DENIED")
	ErrArtifactNotFound = errors.New("ARTIFACT_NOT_FOUND")
	ErrArtifactCorrupt  = errors.New("ARTIFACT_CORRUPT")
)

type RunScope struct {
	RunID             string
	ProjectID         string
	TreeID            string
	NodeID            string
	ChainID           string
	ChainPath         []string
	ContextHash       string
	AncestorHash      string
	ParamsHash        string
	ExecutionSnapshot entity.RunExecutionSnapshot
}

func (s RunScope) Clone() RunScope {
	clone := s
	clone.ChainPath = append([]string(nil), s.ChainPath...)
	clone.ExecutionSnapshot = s.ExecutionSnapshot.Clone()
	return clone
}

type RunGraphNode struct {
	ID              string                  `json:"id"`
	Type            valueobject.NodeType    `json:"type"`
	Title           string                  `json:"title"`
	Parent          string                  `json:"parent,omitempty"`
	ParentCandidate *int                    `json:"parentCandidate,omitempty"`
	CandidateSource *entity.CandidateSource `json:"candidateSource,omitempty"`
	Locked          bool                    `json:"locked"`
	Stale           bool                    `json:"stale"`
}

type RunRound struct {
	N          int                    `json:"n"`
	Kind       valueobject.OutputKind `json:"kind"`
	ArtifactID string                 `json:"artifactId"`
	PendingID  string                 `json:"pendingId"`
	RunID      string                 `json:"runId"`
}

type RunNode struct {
	RunGraphNode
	Params              entity.NodeParams     `json:"params"`
	Rounds              []RunRound            `json:"rounds"`
	LatestScriptVersion *entity.ScriptVersion `json:"latestScriptVersion,omitempty"`
}

type RunGraph struct {
	Nodes         []RunGraphNode           `json:"nodes"`
	OutputBinding *entity.RunOutputBinding `json:"outputBinding,omitempty"`
}

type RunArtifact struct {
	Kind valueobject.OutputKind
	Data []byte
}

type GraphReader interface {
	ReadRunGraph(ctx context.Context, scope RunScope) (RunGraph, error)
	ReadRunNode(ctx context.Context, scope RunScope, nodeID string) (RunNode, error)
}

type ArtifactReader interface {
	ReadRunArtifact(ctx context.Context, scope RunScope, artifactID string) (RunArtifact, error)
}

type ReplySink interface {
	SubmitRunReply(ctx context.Context, scope RunScope, kind valueobject.OutputKind, payload []byte, requestID string) (entity.Pending, error)
}

type RunToolService interface {
	GraphReader
	ArtifactReader
	ReplySink
}

type RunLifecycle interface {
	// WithActiveRun holds a run lease until operation returns so termination cannot race writes.
	WithActiveRun(ctx context.Context, scope RunScope, operation func() error) error
}

type RunPendingObserver interface {
	RecordRunPending(ctx context.Context, scope RunScope, pending entity.Pending) error
}

// AgentDriver starts and resumes a concrete agent implementation.
type AgentDriver interface {
	Start(ctx context.Context, spec DriverSpec) (AgentSession, error)
	Resume(ctx context.Context, spec DriverSpec, ref json.RawMessage) (AgentSession, error)
}

type AgentSession interface {
	Send(ctx context.Context, text string) error
	Interrupt(ctx context.Context) error
	Close() error
}

type DriverSpec struct {
	ChainID        string
	Workspace      string
	SystemPrompt   string
	BuiltinTools   []string
	AllowedTools   []string
	MCPConfigPath  string
	MCPURL         string
	MCPToken       string
	EventSink      func(valueobject.AgentEvent)
	OnResumeRef    func(json.RawMessage)
	PermissionGate PermissionGate
	QuestionGate   QuestionGate
	ToolPolicy     ToolPolicy
}

type PermissionGate interface {
	Request(ctx context.Context, request valueobject.PermissionRequest) (valueobject.PermissionDecision, error)
}

type PolicyDecision string

const (
	PolicyAllow   PolicyDecision = "allow"
	PolicyConfirm PolicyDecision = "confirm"
	PolicyDeny    PolicyDecision = "deny"
)

type PolicyResult struct {
	Decision PolicyDecision
	Tier     valueobject.PermissionTier
	Reason   string
	Risk     string
	Paths    []string
	Command  string
}

type ToolPolicy interface {
	Check(ctx context.Context, tool string, input map[string]any) PolicyResult
}

type PathResolver interface {
	Resolve(path string) (string, error)
}

// ScopedPathResolver separates built-in read and write access. Resolve keeps
// the broader tree boundary used by cooperative Bash checks.
type ScopedPathResolver interface {
	PathResolver
	ResolveRead(path string) (string, error)
	ResolveWrite(path string) (string, error)
}

type ToolServer interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	URL() string
	Token() string
	WriteConfig(path string) error
}

type ToolServerFactory interface {
	New(chainID, displayID string, questionGate QuestionGate, onReply func(string)) (ToolServer, error)
}

// RunToolServerFactory creates the run-scoped MCP server with its logical
// workspace overlay. M1's legacy factory method remains available for migration
// regression coverage.
type RunToolServerFactory interface {
	ToolServerFactory
	NewForRun(scope RunScope, workspace, displayID string, questionGate QuestionGate, onReply func(string), reader WorkspaceReader, lifecycle RunLifecycle) (ToolServer, error)
}

// RunPermissionController is a fresh permission/question broker for one run.
type RunPermissionController interface {
	PermissionGate
	ToolPolicy
	QuestionGate
	ResolvePermission(id string, decision valueobject.PermissionDecision) error
	ResolveQuestion(id string, answer valueobject.QuestionAnswer) error
	SetResolvedCallback(callback func(kind, id string))
	Close()
}

type RunPermissionFactory interface {
	NewRunPermissions(scope RunScope, paths ScopedPathResolver, publish func(valueobject.AgentEvent)) (RunPermissionController, error)
}

// RunConfigPaths owns the short-lived per-run MCP credential file.
type RunConfigPaths interface {
	Prepare(ctx context.Context, projectID, runID string) (string, error)
	Cleanup(ctx context.Context, projectID, runID string) error
}

type AgentPermissionController interface {
	PermissionGate
	ToolPolicy
	ResolvePermission(id string, decision valueobject.PermissionDecision) error
}

type AgentQuestionController interface {
	QuestionGate
	ResolveQuestion(id string, answer valueobject.QuestionAnswer) error
}

type QuestionGate interface {
	Ask(ctx context.Context, request valueobject.QuestionRequest) (valueobject.QuestionAnswer, error)
}

type SessionStore interface {
	Load(ctx context.Context) (valueobject.SessionsFile, error)
	Save(ctx context.Context, file valueobject.SessionsFile) error
}

type EventPublisher interface {
	Publish(event valueobject.AgentEvent)
}

type RunEventPublisher interface {
	PublishRun(event valueobject.RunEvent)
}

type AuditSink interface {
	Record(ctx context.Context, record AuditRecord) error
}

type AuditRecord struct {
	Event     string
	RunID     string
	TreeID    string
	NodeID    string
	ChainID   string
	SessionID string
	RequestID string
	Tool      string
	Tier      string
	Decision  string
	Reason    string
	Paths     []string
	Command   string
	InputHash string
}

type Clock interface {
	Now() time.Time
}
