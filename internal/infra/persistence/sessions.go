package persistence

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"audio-ui/internal/domain/valueobject"
)

var ErrCorruptSessions = errors.New("sessions.json corrupt")

type SessionStore struct {
	path string
	mu   sync.Mutex
}

func NewSessionStore(path string) *SessionStore {
	return &SessionStore{path: path}
}

func (s *SessionStore) Load(ctx context.Context) (valueobject.SessionsFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked(ctx)
}

func (s *SessionStore) loadLocked(ctx context.Context) (valueobject.SessionsFile, error) {
	if err := ctx.Err(); err != nil {
		return valueobject.SessionsFile{}, err
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return newSessionsFile(), nil
		}
		return valueobject.SessionsFile{}, err
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	var file valueobject.SessionsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return valueobject.SessionsFile{}, fmt.Errorf("%w: %v", ErrCorruptSessions, err)
	}
	if file.Version != valueobject.SessionFileVersion {
		return valueobject.SessionsFile{}, fmt.Errorf("%w: unsupported version %d", ErrCorruptSessions, file.Version)
	}
	if file.ActiveChainID == "" {
		file.ActiveChainID = valueobject.DefaultChainID
	}
	if file.Chains == nil {
		file.Chains = map[string]valueobject.SessionRecord{}
	}
	return file, nil
}

func (s *SessionStore) Save(ctx context.Context, file valueobject.SessionsFile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if file.Version == 0 {
		file.Version = valueobject.SessionFileVersion
	}
	if file.ActiveChainID == "" {
		file.ActiveChainID = valueobject.DefaultChainID
	}
	if file.Chains == nil {
		file.Chains = map[string]valueobject.SessionRecord{}
	}
	for chainID, record := range file.Chains {
		if len(record.Transcript) > valueobject.MaxTranscriptItems {
			record.Transcript = record.Transcript[len(record.Transcript)-valueobject.MaxTranscriptItems:]
			file.Chains[chainID] = record
		}
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".sessions-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return err
	}
	return nil
}

func newSessionsFile() valueobject.SessionsFile {
	return valueobject.SessionsFile{
		Version:       valueobject.SessionFileVersion,
		ActiveChainID: valueobject.DefaultChainID,
		Chains:        map[string]valueobject.SessionRecord{},
	}
}
