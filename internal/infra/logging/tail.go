package logging

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"

	"audio-ui/internal/domain/valueobject"
)

const tailChunkSize int64 = 256 * 1024

// Tailer reads the newest entries from the current log file. It deliberately
// reads only the tail, so polling the UI does not scan a full log file.
type Tailer struct {
	path string
}

func (t *Tailer) Tail(ctx context.Context, limit int) ([]valueobject.LogEntry, error) {
	if limit <= 0 {
		limit = 150
	}
	if limit > 200 {
		limit = 200
	}

	file, err := os.Open(t.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []valueobject.LogEntry{}, nil
		}
		return nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() == 0 {
		return []valueobject.LogEntry{}, nil
	}

	offset := info.Size() - tailChunkSize
	if offset < 0 {
		offset = 0
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	chunk, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.ReplaceAll(string(chunk), "\r\n", "\n"), "\n")
	// The final segment is either the empty string after a complete newline or
	// a line that zap is still writing. Drop it either way.
	if len(lines) > 0 {
		lines = lines[:len(lines)-1]
	}
	if offset > 0 && len(lines) > 0 {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}

	start := len(lines) - limit
	if start < 0 {
		start = 0
	}
	entries := make([]valueobject.LogEntry, 0, len(lines)-start)
	for i := len(lines) - 1; i >= start; i-- {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entries = append(entries, parseLine(lines[i]))
	}
	return entries, nil
}

type entryPayload struct {
	Time    string `json:"ts"`
	Level   string `json:"level"`
	Message string `json:"msg"`
}

func parseLine(line string) valueobject.LogEntry {
	line = strings.TrimSpace(line)
	if line == "" {
		return valueobject.LogEntry{Level: "raw", Message: ""}
	}
	var payload entryPayload
	if err := json.Unmarshal([]byte(line), &payload); err != nil {
		return valueobject.LogEntry{Level: "raw", Message: line}
	}
	if payload.Level == "" {
		payload.Level = "info"
	}
	return valueobject.LogEntry{
		Time:    payload.Time,
		Level:   payload.Level,
		Message: payload.Message,
	}
}
