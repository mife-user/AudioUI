package audit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"audio-ui/internal/domain/port"
)

// Logger writes audit records directly to disk so write failures can be
// propagated to the permission broker (allow decisions fail closed).
type Logger struct {
	mu   sync.Mutex
	file *os.File
}

func New(path string) (*Logger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &Logger{file: file}, nil
}

func (l *Logger) Record(ctx context.Context, record port.AuditRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	payload := map[string]any{
		"ts":         time.Now().Format(time.RFC3339),
		"event":      record.Event,
		"chain_id":   record.ChainID,
		"session_id": record.SessionID,
		"request_id": record.RequestID,
		"tool":       record.Tool,
		"tier":       record.Tier,
		"decision":   record.Decision,
		"reason":     record.Reason,
		"paths":      record.Paths,
		"command":    record.Command,
		"input_hash": record.InputHash,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return os.ErrClosed
	}
	if _, err := l.file.Write(data); err != nil {
		return err
	}
	return l.file.Sync()
}

func (l *Logger) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}
