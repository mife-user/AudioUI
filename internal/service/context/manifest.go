package context

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"audio-ui/internal/domain/port"
)

const (
	// The cap covers both files and directories, preventing an unbounded walk.
	MaxManifestEntries = 5_000
	MaxManifestDepth   = 64
)

// WorkspaceFile is a logical reference, never file contents or a physical path.
type WorkspaceFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type manifest struct {
	Files     []WorkspaceFile `json:"files"`
	Truncated bool            `json:"truncated"`
}

func collectManifest(ctx context.Context, reader port.RunWorkspaceReader) (manifest, error) {
	type directory struct {
		path  string
		depth int
	}
	result := manifest{Files: []WorkspaceFile{}}
	queue := []directory{{path: "."}}
	entriesSeen := 0
	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return manifest{}, err
		}
		if entriesSeen >= MaxManifestEntries {
			result.Truncated = true
			break
		}
		dir := queue[0]
		queue = queue[1:]
		listing, err := reader.ListDir(ctx, dir.path, MaxManifestEntries)
		if err != nil {
			return manifest{}, fmt.Errorf("list workspace %s: %w", dir.path, err)
		}
		entries := append([]port.WorkspaceEntry(nil), listing.Entries...)
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
		if listing.Truncated {
			result.Truncated = true
		}
		for _, entry := range entries {
			if entriesSeen >= MaxManifestEntries {
				result.Truncated = true
				break
			}
			if !validEntryName(entry.Name) || entry.Size < 0 {
				return manifest{}, fmt.Errorf("%w: invalid workspace entry", ErrWorkspaceScope)
			}
			entriesSeen++
			logicalPath := path.Join(dir.path, entry.Name)
			if entry.Dir {
				if dir.depth >= MaxManifestDepth {
					result.Truncated = true
					continue
				}
				queue = append(queue, directory{path: logicalPath, depth: dir.depth + 1})
				continue
			}
			result.Files = append(result.Files, WorkspaceFile{Path: logicalPath, Size: entry.Size})
		}
	}
	sort.Slice(result.Files, func(i, j int) bool { return result.Files[i].Path < result.Files[j].Path })
	return result, nil
}

func validEntryName(name string) bool {
	if name == "" || name == "." || name == ".." || !utf8.ValidString(name) || strings.ContainsAny(name, "/\\\x00") {
		return false
	}
	for _, r := range name {
		if r < 0x20 {
			return false
		}
	}
	return true
}
