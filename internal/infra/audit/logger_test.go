package audit

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"audio-ui/internal/domain/port"
)

func TestLoggerWritesRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	logger, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Record(context.Background(), port.AuditRecord{Event: "permission", Tool: "Write", Decision: "deny"}); err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		t.Fatalf("audit data = %q, err=%v", data, err)
	}
}

func TestLoggerReportsClosedWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	logger, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = logger.Close()
	if err := logger.Record(context.Background(), port.AuditRecord{Event: "permission"}); err == nil {
		t.Fatal("Record after Close returned nil")
	}
}

func TestLoggerReportsWriteFailure(t *testing.T) {
	if _, err := New(t.TempDir()); err == nil {
		t.Fatal("New(directory) error = nil, want failure")
	}
}
