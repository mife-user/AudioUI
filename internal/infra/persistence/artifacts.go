package persistence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/valueobject"
)

const artifactIDPrefix = "sha256:"

// ArtifactStore writes immutable content-addressed artifacts under
// artifacts/<sha256>/ for one project.
type ArtifactStore struct {
	dir string
	mu  sync.Mutex
}

func NewArtifactStore(projectDir string) *ArtifactStore {
	return &ArtifactStore{dir: projectDir}
}

func (s *ArtifactStore) Put(ctx context.Context, payload []byte, meta entity.ArtifactMeta) (entity.Artifact, error) {
	if err := ctx.Err(); err != nil {
		return entity.Artifact{}, err
	}
	if err := validateArtifactMeta(meta); err != nil {
		return entity.Artifact{}, err
	}
	hexHash := hashPayload(payload)
	artifact := artifactFromMeta(hexHash, meta)

	s.mu.Lock()
	defer s.mu.Unlock()
	finalDir := s.artifactDir(hexHash)
	if info, err := os.Stat(finalDir); err == nil {
		if !info.IsDir() {
			return entity.Artifact{}, fmt.Errorf("%w: %s is not a directory", ErrArtifactCorrupt, finalDir)
		}
		return s.existingLocked(hexHash)
	} else if !os.IsNotExist(err) {
		return entity.Artifact{}, err
	}

	artifactsDir := filepath.Join(s.dir, "artifacts")
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		return entity.Artifact{}, err
	}
	staging, err := os.MkdirTemp(artifactsDir, ".artifact-*")
	if err != nil {
		return entity.Artifact{}, err
	}
	defer os.RemoveAll(staging)
	if err := writeFileAtomic(filepath.Join(staging, "payload.json"), payload, 0o644); err != nil {
		return entity.Artifact{}, err
	}
	if err := writeJSONAtomic(filepath.Join(staging, "meta.json"), meta); err != nil {
		return entity.Artifact{}, err
	}
	if err := os.Rename(staging, finalDir); err != nil {
		if _, statErr := os.Stat(finalDir); statErr == nil {
			return s.existingLocked(hexHash)
		}
		return entity.Artifact{}, err
	}
	return artifact, nil
}

func (s *ArtifactStore) Get(ctx context.Context, id string) (entity.Artifact, []byte, error) {
	if err := ctx.Err(); err != nil {
		return entity.Artifact{}, nil, err
	}
	hexHash, err := parseArtifactID(id)
	if err != nil {
		return entity.Artifact{}, nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	payload, err := os.ReadFile(s.payloadPath(hexHash))
	if err != nil {
		if os.IsNotExist(err) {
			return entity.Artifact{}, nil, fmt.Errorf("%w: %s", ErrArtifactNotFound, id)
		}
		return entity.Artifact{}, nil, err
	}
	artifact, err := s.loadLocked(hexHash)
	if err != nil {
		return entity.Artifact{}, nil, err
	}
	if artifactIDFromPayload(payload) != id {
		return entity.Artifact{}, nil, fmt.Errorf("%w: payload hash mismatch for %s", ErrArtifactCorrupt, id)
	}
	return artifact, payload, nil
}

func (s *ArtifactStore) Exists(ctx context.Context, id string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	hexHash, err := parseArtifactID(id)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(s.payloadPath(hexHash)); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *ArtifactStore) existingLocked(hexHash string) (entity.Artifact, error) {
	existing, err := s.loadLocked(hexHash)
	if err != nil {
		return entity.Artifact{}, err
	}
	payload, err := os.ReadFile(s.payloadPath(hexHash))
	if err != nil {
		return entity.Artifact{}, fmt.Errorf("%w: %v", ErrArtifactCorrupt, err)
	}
	if hashPayload(payload) != hexHash {
		return entity.Artifact{}, fmt.Errorf("%w: payload mismatch for %s", ErrArtifactCorrupt, existing.ID)
	}
	return existing, nil
}

func (s *ArtifactStore) loadLocked(hexHash string) (entity.Artifact, error) {
	var meta entity.ArtifactMeta
	found, err := readJSONFile(filepath.Join(s.artifactDir(hexHash), "meta.json"), &meta)
	if err != nil {
		return entity.Artifact{}, fmt.Errorf("%w: %v", ErrArtifactCorrupt, err)
	}
	if !found {
		return entity.Artifact{}, fmt.Errorf("%w: missing meta.json for %s", ErrArtifactCorrupt, hexHash)
	}
	if err := validateArtifactMeta(meta); err != nil {
		return entity.Artifact{}, fmt.Errorf("%w: %v", ErrArtifactCorrupt, err)
	}
	return artifactFromMeta(hexHash, meta), nil
}

func (s *ArtifactStore) artifactDir(hexHash string) string {
	return filepath.Join(s.dir, "artifacts", hexHash)
}

func (s *ArtifactStore) payloadPath(hexHash string) string {
	return filepath.Join(s.artifactDir(hexHash), "payload.json")
}

func hashPayload(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func artifactIDFromPayload(payload []byte) string {
	return artifactIDPrefix + hashPayload(payload)
}

func artifactFromMeta(hexHash string, meta entity.ArtifactMeta) entity.Artifact {
	return entity.Artifact{
		ID:        artifactIDPrefix + hexHash,
		Kind:      meta.Kind,
		NodeID:    meta.NodeID,
		Path:      "artifacts/" + hexHash + "/payload.json",
		CreatedAt: meta.CreatedAt,
	}
}

func parseArtifactID(id string) (string, error) {
	if !strings.HasPrefix(id, artifactIDPrefix) {
		return "", fmt.Errorf("%w: %q", ErrArtifactInvalid, id)
	}
	hexHash := strings.TrimPrefix(id, artifactIDPrefix)
	if len(hexHash) != sha256.Size*2 {
		return "", fmt.Errorf("%w: %q", ErrArtifactInvalid, id)
	}
	if strings.ToLower(hexHash) != hexHash {
		return "", fmt.Errorf("%w: %q", ErrArtifactInvalid, id)
	}
	if _, err := hex.DecodeString(hexHash); err != nil {
		return "", fmt.Errorf("%w: %q", ErrArtifactInvalid, id)
	}
	return hexHash, nil
}

func validateArtifactMeta(meta entity.ArtifactMeta) error {
	if !meta.Kind.Valid() {
		return fmt.Errorf("%w: kind %q", ErrArtifactInvalid, meta.Kind)
	}
	if err := valueobject.ValidateID(meta.NodeID); err != nil {
		return fmt.Errorf("%w: nodeId: %v", ErrArtifactInvalid, err)
	}
	if meta.CreatedAt == "" {
		return fmt.Errorf("%w: createdAt is empty", ErrArtifactInvalid)
	}
	if meta.SourceRunID != "" {
		if err := valueobject.ValidateID(meta.SourceRunID); err != nil {
			return fmt.Errorf("%w: sourceRunId: %v", ErrArtifactInvalid, err)
		}
	}
	if meta.SourcePendingID != "" {
		if err := valueobject.ValidateID(meta.SourcePendingID); err != nil {
			return fmt.Errorf("%w: sourcePendingId: %v", ErrArtifactInvalid, err)
		}
	}
	return nil
}
