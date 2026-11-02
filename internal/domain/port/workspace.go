package port

import (
	"context"
	"time"
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
