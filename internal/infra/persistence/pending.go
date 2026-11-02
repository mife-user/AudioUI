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

// PendingStore persists one project's pending.json index and raw payloads.
type PendingStore struct {
	dir string
	mu  sync.Mutex
}

func NewPendingStore(projectDir string) *PendingStore {
	return &PendingStore{dir: projectDir}
}

func (s *PendingStore) List(ctx context.Context) ([]entity.Pending, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	index, err := s.loadIndexLocked()
	if err != nil {
		return nil, err
	}
	result := make([]entity.Pending, len(index.Pending))
	for i, pending := range index.Pending {
		pending.ChainPath = append([]string{}, pending.ChainPath...)
		result[i] = pending
	}
	return result, nil
}

func (s *PendingStore) Load(ctx context.Context, id string) (entity.Pending, error) {
	if err := ctx.Err(); err != nil {
		return entity.Pending{}, err
	}
	if err := valueobject.ValidateID(id); err != nil {
		return entity.Pending{}, fmt.Errorf("%w: %v", ErrPendingNotFound, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	index, err := s.loadIndexLocked()
	if err != nil {
		return entity.Pending{}, err
	}
	for _, pending := range index.Pending {
		if pending.ID == id {
			return pending, nil
		}
	}
	return entity.Pending{}, fmt.Errorf("%w: %s", ErrPendingNotFound, id)
}

func (s *PendingStore) GetPayload(ctx context.Context, id string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := valueobject.ValidateID(id); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPendingNotFound, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.payloadPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrPendingNotFound, id)
		}
		return nil, err
	}
	return data, nil
}

func (s *PendingStore) Save(ctx context.Context, pending entity.Pending, payload []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(payload) > valueobject.MaxPendingPayloadSize {
		return fmt.Errorf("%w: %d > %d", ErrPendingTooLarge, len(payload), valueobject.MaxPendingPayloadSize)
	}
	pending = normalizePending(pending)
	if err := validatePending(pending); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	index, err := s.loadIndexLocked()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.payloadPath(pending.ID)), 0o755); err != nil {
		return err
	}
	if err := writeFileAtomic(s.payloadPath(pending.ID), payload, 0o644); err != nil {
		return fmt.Errorf("写入待应用原文: %w", err)
	}
	pending.Payload = s.payloadRelative(pending.ID)
	updated := false
	for i := range index.Pending {
		if index.Pending[i].ID == pending.ID {
			index.Pending[i] = pending
			updated = true
			break
		}
	}
	if !updated {
		index.Pending = append(index.Pending, pending)
	}
	if err := writeJSONAtomic(s.indexPath(), index); err != nil {
		return fmt.Errorf("写入待应用索引: %w", err)
	}
	return nil
}

func (s *PendingStore) loadIndexLocked() (entity.PendingFile, error) {
	var index entity.PendingFile
	found, err := readJSONFile(s.indexPath(), &index)
	if err != nil {
		return entity.PendingFile{}, fmt.Errorf("%w: %v", ErrCorruptPending, err)
	}
	if !found {
		return entity.PendingFile{Version: valueobject.PendingFileVersion, Pending: []entity.Pending{}}, nil
	}
	if index.Version != valueobject.PendingFileVersion {
		return entity.PendingFile{}, fmt.Errorf("%w: pending version %d", ErrUnsupportedVersion, index.Version)
	}
	if index.Pending == nil {
		index.Pending = []entity.Pending{}
	}
	seen := make(map[string]struct{}, len(index.Pending))
	for i := range index.Pending {
		index.Pending[i] = normalizePending(index.Pending[i])
		if err := validatePending(index.Pending[i]); err != nil {
			return entity.PendingFile{}, fmt.Errorf("%w: %v", ErrCorruptPending, err)
		}
		index.Pending[i].Payload = s.payloadRelative(index.Pending[i].ID)
		if _, ok := seen[index.Pending[i].ID]; ok {
			return entity.PendingFile{}, fmt.Errorf("%w: duplicate pending id %q", ErrCorruptPending, index.Pending[i].ID)
		}
		seen[index.Pending[i].ID] = struct{}{}
	}
	return index, nil
}

func (s *PendingStore) indexPath() string { return filepath.Join(s.dir, "pending.json") }

func (s *PendingStore) payloadPath(id string) string {
	return filepath.Join(s.dir, "pending", id+".json")
}

func (s *PendingStore) payloadRelative(id string) string {
	return "pending/" + id + ".json"
}

func normalizePending(pending entity.Pending) entity.Pending {
	if pending.ChainPath == nil {
		pending.ChainPath = []string{}
	} else {
		pending.ChainPath = append([]string{}, pending.ChainPath...)
	}
	return pending
}

func validatePending(pending entity.Pending) error {
	if err := valueobject.ValidateID(pending.ID); err != nil {
		return fmt.Errorf("%w: id: %v", ErrInvalidPending, err)
	}
	if err := valueobject.ValidateID(pending.RunID); err != nil {
		return fmt.Errorf("%w: runId: %v", ErrInvalidPending, err)
	}
	if pending.ChainID != "" {
		if err := valueobject.ValidateID(pending.ChainID); err != nil {
			return fmt.Errorf("%w: chainId: %v", ErrInvalidPending, err)
		}
	}
	for _, id := range pending.ChainPath {
		if err := valueobject.ValidateID(id); err != nil {
			return fmt.Errorf("%w: chainPath: %v", ErrInvalidPending, err)
		}
	}
	if err := valueobject.ValidateID(pending.NodeID); err != nil {
		return fmt.Errorf("%w: nodeId: %v", ErrInvalidPending, err)
	}
	if !pending.Kind.Valid() {
		return fmt.Errorf("%w: kind %q", ErrInvalidPending, pending.Kind)
	}
	if !pending.Status.Valid() {
		return fmt.Errorf("%w: status %q", ErrInvalidPending, pending.Status)
	}
	if pending.CreatedAt == "" {
		return fmt.Errorf("%w: createdAt is empty", ErrInvalidPending)
	}
	return nil
}
