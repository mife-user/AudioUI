package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/infra/paths"
)

// Mover moves node step directories between topic trees under one project.
type Mover struct {
	layout valueobject.Layout
}

func NewMover(layout valueobject.Layout) *Mover {
	return &Mover{layout: layout}
}

type movePair struct {
	source string
	target string
}

type workspaceMove struct {
	pairs []movePair
	done  bool
}

func (m *workspaceMove) Rollback(_ context.Context) error {
	if m.done || len(m.pairs) == 0 {
		return nil
	}
	var rollbackErr error
	for i := len(m.pairs) - 1; i >= 0; i-- {
		pair := m.pairs[i]
		if err := os.MkdirAll(filepath.Dir(pair.source), 0o755); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
			continue
		}
		if err := os.Rename(pair.target, pair.source); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	if rollbackErr == nil {
		m.done = true
	}
	return rollbackErr
}

func (m *Mover) MoveSubtree(ctx context.Context, projectID, sourceRootID, targetRootID string, nodeIDs []string) (port.WorkspaceMove, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if sourceRootID == targetRootID || len(nodeIDs) == 0 {
		return &workspaceMove{}, nil
	}
	projectPaths, err := paths.ResolveProject(m.layout, projectID)
	if err != nil {
		return nil, err
	}
	if err := rejectReparseIfExists(filepath.Join(projectPaths.TreesDir, targetRootID)); err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(nodeIDs))
	pending := make([]movePair, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		if _, ok := seen[nodeID]; ok {
			continue
		}
		seen[nodeID] = struct{}{}
		source, err := projectPaths.StepDir(sourceRootID, nodeID)
		if err != nil {
			return nil, err
		}
		target, err := projectPaths.StepDir(targetRootID, nodeID)
		if err != nil {
			return nil, err
		}
		info, err := os.Lstat(source)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("节点步骤不是目录: %s", source)
		}
		reparse, err := paths.IsReparsePoint(source)
		if err != nil {
			return nil, err
		}
		if reparse {
			return nil, fmt.Errorf("节点步骤是符号链接或 junction: %s", source)
		}
		if _, err := os.Lstat(target); err == nil {
			return nil, fmt.Errorf("目标节点步骤已存在: %s", target)
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		pending = append(pending, movePair{source: source, target: target})
	}
	if len(pending) == 0 {
		return &workspaceMove{}, nil
	}
	targetSteps := filepath.Join(projectPaths.TreesDir, targetRootID, "steps")
	if err := os.MkdirAll(targetSteps, 0o755); err != nil {
		return nil, err
	}
	moved := make([]movePair, 0, len(pending))
	for _, pair := range pending {
		if err := ctx.Err(); err != nil {
			rollbackErr := (&workspaceMove{pairs: moved}).Rollback(context.Background())
			return nil, errors.Join(err, rollbackErr)
		}
		if err := os.Rename(pair.source, pair.target); err != nil {
			rollbackErr := (&workspaceMove{pairs: moved}).Rollback(context.Background())
			return nil, errors.Join(fmt.Errorf("移动节点步骤失败: %w", err), rollbackErr)
		}
		moved = append(moved, pair)
	}
	return &workspaceMove{pairs: moved}, nil
}

func rejectReparseIfExists(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("路径不是目录: %s", path)
	}
	reparse, err := paths.IsReparsePoint(path)
	if err != nil {
		return err
	}
	if reparse {
		return fmt.Errorf("路径是符号链接或 junction: %s", path)
	}
	return nil
}
