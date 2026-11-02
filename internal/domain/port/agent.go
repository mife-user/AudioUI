package port

import (
	"context"
	"encoding/json"
	"time"

	"audio-ui/internal/domain/valueobject"
)

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
	NewForRun(chainID, displayID string, questionGate QuestionGate, onReply func(string), reader WorkspaceReader) (ToolServer, error)
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

type AuditSink interface {
	Record(ctx context.Context, record AuditRecord) error
}

type AuditRecord struct {
	Event     string
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
