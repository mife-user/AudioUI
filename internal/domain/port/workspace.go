package port

import (
	"context"
	"time"

	"audio-ui/internal/domain/entity"
)

// WorkspaceMove represents a completed subtree step move that can be undone
// if the graph save fails.
type WorkspaceMove interface {
	Rollback(ctx context.Context) error
}

// WorkspaceMover moves a prompt subtree's steps between two topic trees.
type WorkspaceMover interface {
	MoveSubtree(ctx context.Context, projectID, sourceRootID, targetRootID string, nodeIDs []string) (WorkspaceMove, error)
}

// WorkspaceRead is a bounded file read from the logical path overlay.
type WorkspaceRead struct {
	Path      string
	Data      []byte
	Size      int64
	Truncated bool
}

// WorkspaceEntry is one visible entry after overlay shadowing is applied.
type WorkspaceEntry struct {
	Name string
	Dir  bool
	Size int64
}

// WorkspaceList is a bounded, merged directory listing.
type WorkspaceList struct {
	Path      string
	Entries   []WorkspaceEntry
	Truncated bool
}

// WorkspaceGrepOptions bounds one logical grep operation.
type WorkspaceGrepOptions struct {
	MaxHits      int
	MaxFiles     int
	MaxFileBytes int64
}

// WorkspaceGrepHit points at one visible logical file and line.
type WorkspaceGrepHit struct {
	Path string
	Line int
	Text string
}

// WorkspaceGrep is a bounded regex search result.
type WorkspaceGrep struct {
	Hits      []WorkspaceGrepHit
	Truncated bool
}

// WorkspaceStat is metadata for the highest-priority visible path.
type WorkspaceStat struct {
	Path    string
	Dir     bool
	Size    int64
	ModTime time.Time
}

// WorkspaceReader exposes the merged read-only view for one run path. Paths
// are logical and never expose physical steps or runtime directories.
type WorkspaceReader interface {
	ReadFile(ctx context.Context, path string, maxBytes int64) (WorkspaceRead, error)
	ListDir(ctx context.Context, path string, maxEntries int) (WorkspaceList, error)
	Grep(ctx context.Context, pattern, path string, options WorkspaceGrepOptions) (WorkspaceGrep, error)
	Stat(ctx context.Context, path string) (WorkspaceStat, error)
}

type WorkspaceScope struct {
	ProjectID    string
	TreeID       string
	NodeID       string
	ChainPath    []string
	InputNodeIDs []string
}

// RunWorkspaceReader lets consumers verify that a manifest belongs to the
// same project and graph path as the run for which it is being constructed.
type RunWorkspaceReader interface {
	WorkspaceReader
	Scope() WorkspaceScope
}

type WorkspaceSource struct {
	LogicalPath  string
	PhysicalPath string
}

// RunWorkspace combines the path-scoped MCP overlay with the physical cwd and
// the stricter resolver used by native Agent tools.
type RunWorkspace interface {
	RunWorkspaceReader
	CWD() string
	RuntimeReadPath() string
	InputSources() []WorkspaceSource
	Resolver() ScopedPathResolver
}

type RunWorkspaceProvider interface {
	OpenRun(ctx context.Context, projectID string, snapshot entity.RunExecutionSnapshot) (RunWorkspace, error)
}
