package persistence

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/valueobject"
)

// RunStore persists one project's runs.json index and runs/<runId>.json details.
type RunStore struct {
	dir string
	mu  sync.Mutex
}

func NewRunStore(projectDir string) *RunStore {
	return &RunStore{dir: projectDir}
}

func (s *RunStore) List(ctx context.Context) ([]entity.RunSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	index, err := s.loadIndexLocked()
	if err != nil {
		return nil, err
	}
	for _, run := range index.Runs {
		path := s.detailPath(run.ID)
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("%w: %s references missing %s", ErrCorruptRuns, s.indexPath(), path)
			}
			return nil, err
		}
	}
	result := make([]entity.RunSummary, len(index.Runs))
	for i, run := range index.Runs {
		run.ChainPath = append([]string{}, run.ChainPath...)
		result[i] = run
	}
	return result, nil
}

func (s *RunStore) Load(ctx context.Context, id string) (entity.RunRecord, error) {
	if err := ctx.Err(); err != nil {
		return entity.RunRecord{}, err
	}
	if err := valueobject.ValidateID(id); err != nil {
		return entity.RunRecord{}, fmt.Errorf("%w: %v", ErrRunNotFound, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var record entity.RunRecord
	found, err := readJSONFile(s.detailPath(id), &record)
	if err != nil {
		return entity.RunRecord{}, fmt.Errorf("%w: %v", ErrCorruptRuns, err)
	}
	if !found {
		return entity.RunRecord{}, fmt.Errorf("%w: %s", ErrRunNotFound, id)
	}
	record = normalizeRun(record, valueobject.MaxRunTranscriptItems)
	if record.ID != id {
		return entity.RunRecord{}, fmt.Errorf("%w: id %q does not match file %q", ErrCorruptRuns, record.ID, id)
	}
	if err := validateRun(record); err != nil {
		return entity.RunRecord{}, fmt.Errorf("%w: %v", ErrCorruptRuns, err)
	}
	record.Transcript = append([]valueobject.TranscriptItem{}, record.Transcript...)
	return record, nil
}

func (s *RunStore) Save(ctx context.Context, record entity.RunRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	record = normalizeRun(record, valueobject.MaxRunTranscriptItems)
	if err := validateRun(record); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	index, err := s.loadIndexLocked()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	if err := writeJSONAtomic(s.detailPath(record.ID), record); err != nil {
		return fmt.Errorf("写入运行记录: %w", err)
	}
	updated := false
	for i := range index.Runs {
		if index.Runs[i].ID == record.ID {
			index.Runs[i] = record.RunSummary
			updated = true
			break
		}
	}
	if !updated {
		index.Runs = append(index.Runs, record.RunSummary)
	}
	if err := writeJSONAtomic(s.indexPath(), index); err != nil {
		return fmt.Errorf("写入运行索引: %w", err)
	}
	return nil
}

func (s *RunStore) loadIndexLocked() (entity.RunsFile, error) {
	var index entity.RunsFile
	found, err := readJSONFile(s.indexPath(), &index)
	if err != nil {
		return entity.RunsFile{}, fmt.Errorf("%w: %v", ErrCorruptRuns, err)
	}
	if !found {
		return entity.RunsFile{Version: valueobject.RunsFileVersion, Runs: []entity.RunSummary{}}, nil
	}
	if index.Version != valueobject.RunsFileVersion {
		return entity.RunsFile{}, fmt.Errorf("%w: runs version %d", ErrUnsupportedVersion, index.Version)
	}
	if index.Runs == nil {
		index.Runs = []entity.RunSummary{}
	}
	seen := make(map[string]struct{}, len(index.Runs))
	for i := range index.Runs {
		index.Runs[i].ChainPath = append([]string{}, index.Runs[i].ChainPath...)
		if err := validateRun(entity.RunRecord{RunSummary: index.Runs[i]}); err != nil {
			return entity.RunsFile{}, fmt.Errorf("%w: %v", ErrCorruptRuns, err)
		}
		if _, ok := seen[index.Runs[i].ID]; ok {
			return entity.RunsFile{}, fmt.Errorf("%w: duplicate run id %q", ErrCorruptRuns, index.Runs[i].ID)
		}
		seen[index.Runs[i].ID] = struct{}{}
	}
	return index, nil
}

func (s *RunStore) indexPath() string           { return filepath.Join(s.dir, "runs.json") }
func (s *RunStore) detailPath(id string) string { return filepath.Join(s.dir, "runs", id+".json") }

func normalizeRun(record entity.RunRecord, limit int) entity.RunRecord {
	if record.ChainPath == nil {
		record.ChainPath = []string{}
	} else {
		record.ChainPath = append([]string{}, record.ChainPath...)
	}
	if record.Transcript == nil {
		record.Transcript = []valueobject.TranscriptItem{}
	}
	if limit > 0 && len(record.Transcript) > limit {
		record.Transcript = record.Transcript[len(record.Transcript)-limit:]
	}
	return record
}

func validateRun(record entity.RunRecord) error {
	if err := valueobject.ValidateID(record.ID); err != nil {
		return fmt.Errorf("%w: id: %v", ErrInvalidRun, err)
	}
	if record.NodeID == "" || valueobject.ValidateID(record.NodeID) != nil {
		return fmt.Errorf("%w: invalid nodeId %q", ErrInvalidRun, record.NodeID)
	}
	if record.TreeID != "" {
		if err := valueobject.ValidateID(record.TreeID); err != nil {
			return fmt.Errorf("%w: treeId: %v", ErrInvalidRun, err)
		}
	}
	if record.ChainID != "" {
		if err := valueobject.ValidateID(record.ChainID); err != nil {
			return fmt.Errorf("%w: chainId: %v", ErrInvalidRun, err)
		}
	}
	for _, id := range record.ChainPath {
		if err := valueobject.ValidateID(id); err != nil {
			return fmt.Errorf("%w: chainPath: %v", ErrInvalidRun, err)
		}
	}
	if !record.Status.Valid() {
		return fmt.Errorf("%w: status %q", ErrInvalidRun, record.Status)
	}
	if record.ContextVersion < 0 {
		return fmt.Errorf("%w: contextVersion is negative", ErrInvalidRun)
	}
	if record.EstimatorVersion < 0 || record.TokenBudget < 0 {
		return fmt.Errorf("%w: context estimate fields must not be negative", ErrInvalidRun)
	}
	if record.EstimatedTokens < 0 || record.ActualInputTokens < 0 {
		return fmt.Errorf("%w: token counts must not be negative", ErrInvalidRun)
	}
	if record.PendingID != "" {
		if err := valueobject.ValidateID(record.PendingID); err != nil {
			return fmt.Errorf("%w: pendingId: %v", ErrInvalidRun, err)
		}
	}
	return nil
}
