package logging

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoggerWritesAndTailsNewestFirst(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	logger, err := New(path)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer logger.Close()

	logger.Zap.Info("first")
	logger.Zap.Warn("second")
	logger.Zap.Error("third")
	_ = logger.Zap.Sync()

	entries, err := logger.Tailer.Tail(context.Background(), 10)
	if err != nil {
		t.Fatalf("Tail() error = %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("len(entries) = %d, want 3: %+v", len(entries), entries)
	}
	if entries[0].Message != "third" || entries[1].Message != "second" || entries[2].Message != "first" {
		t.Fatalf("unexpected order: %+v", entries)
	}
	if entries[0].Level != "error" {
		t.Fatalf("level = %q", entries[0].Level)
	}
	if entries[0].Time == "" {
		t.Fatal("time should be parsed from JSON log")
	}
}

func TestTailMissingFileReturnsEmpty(t *testing.T) {
	tailer := &Tailer{path: filepath.Join(t.TempDir(), "missing.log")}
	entries, err := tailer.Tail(context.Background(), 10)
	if err != nil {
		t.Fatalf("Tail() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries = %+v, want empty", entries)
	}
}

func TestTailLimitAndRawLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	content := strings.Join([]string{
		"raw one",
		`{"ts":"2026-10-31T10:00:00+08:00","level":"info","msg":"raw two"}`,
		"raw three",
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	tailer := &Tailer{path: path}
	entries, err := tailer.Tail(context.Background(), 2)
	if err != nil {
		t.Fatalf("Tail() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if entries[0].Message != "raw three" || entries[0].Level != "raw" {
		t.Fatalf("first = %+v", entries[0])
	}
	if entries[1].Message != "raw two" || entries[1].Level != "info" {
		t.Fatalf("second = %+v", entries[1])
	}
}

func TestTailDropsPartialTrailingLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	content := "{\"ts\":\"2026-10-31T10:00:00+08:00\",\"level\":\"info\",\"msg\":\"complete\"}\n" +
		`{"ts":"2026-10-31T10:00:01+08:00","msg":"in pro`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	tailer := &Tailer{path: path}
	entries, err := tailer.Tail(context.Background(), 10)
	if err != nil {
		t.Fatalf("Tail() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Message != "complete" {
		t.Fatalf("entries = %+v, want only complete line", entries)
	}
}

func TestTailRespectsCancelledContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(path, []byte("line one\nline two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tailer := &Tailer{path: path}
	if _, err := tailer.Tail(ctx, 10); err == nil {
		t.Fatal("Tail() error = nil, want context cancellation")
	}
}
