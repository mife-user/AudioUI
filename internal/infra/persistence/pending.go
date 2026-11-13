package persistence

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
		result[i] = pending
		result[i].Payload = s.payloadRelative(pending.ID)
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

func (s *PendingStore) WritePayload(ctx context.Context, id string, payload []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := valueobject.ValidateID(id); err != nil {
		return fmt.Errorf("%w: id: %v", ErrInvalidPending, err)
	}
	if len(payload) > valueobject.MaxPendingPayloadSize {
		return fmt.Errorf("%w: %d > %d", ErrPendingTooLarge, len(payload), valueobject.MaxPendingPayloadSize)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writePayloadLocked(id, payload)
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
	pending.Payload = s.payloadRelative(pending.ID)
	updated := false
	for i := range index.Pending {
		if index.Pending[i].ID == pending.ID {
			if !samePendingIdentity(index.Pending[i], pending) {
				return fmt.Errorf("%w: conflicting pending id %q", ErrPendingConflict, pending.ID)
			}
			updated = true
			break
		}
	}
	if err := s.writePayloadLocked(pending.ID, payload); err != nil {
		return fmt.Errorf("写入待应用原文: %w", err)
	}
	if updated {
		for i := range index.Pending {
			if index.Pending[i].ID == pending.ID {
				index.Pending[i] = pending
				break
			}
		}
	} else {
		index.Pending = append(index.Pending, pending)
	}
	if err := writeJSONAtomic(s.indexPath(), index); err != nil {
		return fmt.Errorf("写入待应用索引: %w", err)
	}
	return nil
}

func (s *PendingStore) SaveTransaction(ctx context.Context, transaction entity.PendingTransaction) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	transaction.Pending = normalizePending(transaction.Pending)
	transaction.Pending.Payload = s.payloadRelative(transaction.Pending.ID)
	if err := validatePendingTransaction(transaction); err != nil {
		return fmt.Errorf("%w: %v", ErrCorruptPendingTransaction, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := os.Stat(s.transactionPath()); err == nil {
		return ErrPendingTransactionConflict
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := writeJSONAtomic(s.transactionPath(), transaction); err != nil {
		return err
	}
	return nil
}

func (s *PendingStore) LoadTransaction(ctx context.Context) (entity.PendingTransaction, bool, error) {
	if err := ctx.Err(); err != nil {
		return entity.PendingTransaction{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var transaction entity.PendingTransaction
	found, err := readJSONFile(s.transactionPath(), &transaction)
	if err != nil {
		return entity.PendingTransaction{}, false, fmt.Errorf("%w: %v", ErrCorruptPendingTransaction, err)
	}
	if !found {
		return entity.PendingTransaction{}, false, nil
	}
	transaction.Pending = normalizePending(transaction.Pending)
	transaction.Pending.Payload = s.payloadRelative(transaction.Pending.ID)
	if err := validatePendingTransaction(transaction); err != nil {
		return entity.PendingTransaction{}, false, fmt.Errorf("%w: %v", ErrCorruptPendingTransaction, err)
	}
	return transaction, true, nil
}

func (s *PendingStore) DeleteTransaction(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(s.transactionPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *PendingStore) writePayloadLocked(id string, payload []byte) error {
	if err := os.MkdirAll(filepath.Dir(s.payloadPath(id)), 0o755); err != nil {
		return err
	}
	return writeFileAtomic(s.payloadPath(id), payload, 0o644)
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

func (s *PendingStore) transactionPath() string {
	return filepath.Join(s.dir, "pending.txn.json")
}

func samePendingIdentity(left, right entity.Pending) bool {
	return left.ID == right.ID && left.RunID == right.RunID && left.RequestID == right.RequestID &&
		left.ChainID == right.ChainID && slices.Equal(left.ChainPath, right.ChainPath) &&
		left.NodeID == right.NodeID && left.Kind == right.Kind &&
		left.ContextHash == right.ContextHash && left.AncestorHash == right.AncestorHash &&
		left.ParamsHash == right.ParamsHash && left.CreatedAt == right.CreatedAt &&
		left.OutputSchemaVersion == right.OutputSchemaVersion &&
		sameOutputBinding(left.OutputBinding, right.OutputBinding)
}

func sameOutputBinding(left, right *entity.RunOutputBinding) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func validatePendingTransaction(transaction entity.PendingTransaction) error {
	if transaction.Version != valueobject.PendingTransactionFileVersion {
		return fmt.Errorf("unsupported transaction version %d", transaction.Version)
	}
	if transaction.BaseRevision < 0 || transaction.TargetRevision != transaction.BaseRevision+1 {
		return fmt.Errorf("invalid revision range %d -> %d", transaction.BaseRevision, transaction.TargetRevision)
	}
	if len(transaction.Payload) > valueobject.MaxPendingPayloadSize {
		return fmt.Errorf("pending payload too large: %d", len(transaction.Payload))
	}
	if err := validatePending(transaction.Pending); err != nil {
		return err
	}
	if transaction.ArtifactMeta == nil {
		if len(transaction.ArtifactPayload) != 0 || transaction.Pending.Status == valueobject.PendingApplied || transaction.Pending.ArtifactID != "" {
			return fmt.Errorf("applied pending is missing its artifact")
		}
		return nil
	}
	meta := *transaction.ArtifactMeta
	artifactNodeID := transaction.Pending.NodeID
	if transaction.Pending.ResultNodeID != "" {
		artifactNodeID = transaction.Pending.ResultNodeID
	}
	if transaction.Pending.Status != valueobject.PendingApplied || transaction.Pending.ArtifactID == "" ||
		meta.Kind != transaction.Pending.Kind || meta.NodeID != artifactNodeID ||
		meta.CreatedAt != transaction.Pending.CreatedAt || meta.SourceRunID != transaction.Pending.RunID ||
		meta.SourcePendingID != transaction.Pending.ID {
		return fmt.Errorf("artifact metadata does not match pending")
	}
	if len(transaction.ArtifactPayload) == 0 || len(transaction.ArtifactPayload) > valueobject.MaxPendingPayloadSize {
		return fmt.Errorf("invalid artifact payload size %d", len(transaction.ArtifactPayload))
	}
	canonical, err := valueobject.CanonicalJSON(transaction.Payload)
	if err != nil || !bytes.Equal(canonical, transaction.ArtifactPayload) {
		return fmt.Errorf("artifact payload does not match pending payload")
	}
	return valueobject.ValidateStructuredPayload(meta.Kind, transaction.ArtifactPayload)
}

func normalizePending(pending entity.Pending) entity.Pending {
	pending = pending.Clone()
	if pending.ChainPath == nil {
		pending.ChainPath = []string{}
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
	if len(pending.RequestID) > valueobject.MaxPendingRequestIDLength {
		return fmt.Errorf("%w: requestId is too long", ErrInvalidPending)
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
	if pending.OutputBinding != nil {
		if pending.OutputSchemaVersion != 1 || pending.ChainID != "c_"+pending.NodeID || len(pending.ChainPath) == 0 && (pending.ExecutionSnapshot == nil || pending.ExecutionSnapshot.Version < 3 || pending.ExecutionSnapshot.Version > 4) || (pending.ExecutionSnapshot == nil || pending.ExecutionSnapshot.Version < 3 || pending.ExecutionSnapshot.Version > 4) && pending.ChainPath[0] == "" || (pending.ExecutionSnapshot == nil || pending.ExecutionSnapshot.Version < 3 || pending.ExecutionSnapshot.Version > 4) && pending.ChainPath[len(pending.ChainPath)-1] != pending.NodeID {
			return fmt.Errorf("%w: invalid output contract identity", ErrInvalidPending)
		}
		if snapshot := pending.ExecutionSnapshot; snapshot != nil && (snapshot.Validate() != nil || snapshot.NodeID != pending.NodeID ||
			snapshot.OutputBinding != *pending.OutputBinding || snapshot.OutputContract.Kind != pending.Kind ||
			snapshot.OutputContract.SchemaVersion != pending.OutputSchemaVersion || !slices.Equal(snapshot.ChainPath, pending.ChainPath)) {
			return fmt.Errorf("%w: invalid execution snapshot", ErrInvalidPending)
		}
		binding := pending.OutputBinding
		if binding.Bound {
			if err := valueobject.ValidateID(binding.NodeID); err != nil {
				return fmt.Errorf("%w: output node id: %v", ErrInvalidPending, err)
			}
			if err := valueobject.ValidateID(binding.ParentNodeID); err != nil {
				return fmt.Errorf("%w: output parent node id: %v", ErrInvalidPending, err)
			}
			versionedCreate := pending.ExecutionSnapshot != nil && pending.ExecutionSnapshot.Version == 4 && binding.Mode == entity.RunOutputModeCreate
			if pending.Kind != valueobject.OutputKindScript || binding.ParentNodeID != pending.NodeID || binding.NodeID == binding.ParentNodeID ||
				binding.BaselineVersion < 0 || (binding.BaselineVersion == 0) != (binding.BaselineArtifactID == "") ||
				pending.ResultNodeID != binding.NodeID || !versionedCreate && len(pending.CreatedNodeIDs) != 0 ||
				versionedCreate && pending.Status == valueobject.PendingApplied && (len(pending.CreatedNodeIDs) != 1 || pending.CreatedNodeIDs[0] != binding.NodeID) ||
				versionedCreate && pending.Status != valueobject.PendingApplied && len(pending.CreatedNodeIDs) != 0 {
				return fmt.Errorf("%w: invalid output binding", ErrInvalidPending)
			}
		} else {
			if binding.NodeID != "" || binding.ParentNodeID != "" || binding.BaselineVersion != 0 || binding.BaselineArtifactID != "" {
				return fmt.Errorf("%w: unbound output contains a target", ErrInvalidPending)
			}
			switch pending.Kind {
			case valueobject.OutputKindTopics:
				if pending.ResultNodeID != "" || (pending.Status == valueobject.PendingApplied) != (len(pending.CreatedNodeIDs) > 0) {
					return fmt.Errorf("%w: invalid topic result identity", ErrInvalidPending)
				}
			case valueobject.OutputKindScript:
				if pending.Status == valueobject.PendingApplied {
					if len(pending.CreatedNodeIDs) != 1 || pending.ResultNodeID != pending.CreatedNodeIDs[0] {
						return fmt.Errorf("%w: invalid generated Script identity", ErrInvalidPending)
					}
				} else if pending.ResultNodeID != "" || len(pending.CreatedNodeIDs) != 0 {
					return fmt.Errorf("%w: unexpected generated Script identity", ErrInvalidPending)
				}
			default:
				return fmt.Errorf("%w: invalid output kind", ErrInvalidPending)
			}
		}
	} else if pending.OutputSchemaVersion != 0 {
		return fmt.Errorf("%w: output schema version has no binding", ErrInvalidPending)
	}
	if pending.ResultNodeID != "" {
		if err := valueobject.ValidateID(pending.ResultNodeID); err != nil {
			return fmt.Errorf("%w: result node id: %v", ErrInvalidPending, err)
		}
	}
	seenCreated := make(map[string]struct{}, len(pending.CreatedNodeIDs))
	for _, id := range pending.CreatedNodeIDs {
		if err := valueobject.ValidateID(id); err != nil {
			return fmt.Errorf("%w: created node id: %v", ErrInvalidPending, err)
		}
		if _, exists := seenCreated[id]; exists {
			return fmt.Errorf("%w: duplicate created node id %q", ErrInvalidPending, id)
		}
		seenCreated[id] = struct{}{}
	}
	if pending.CreatedAt == "" {
		return fmt.Errorf("%w: createdAt is empty", ErrInvalidPending)
	}
	return nil
}
