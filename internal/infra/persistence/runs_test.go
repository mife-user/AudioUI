package persistence

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"audio-ui/internal/domain/valueobject"
)

func TestRunStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewRunStore(dir)
	record := sampleRun("r_test")
	if err := store.Save(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(dir, "runs.json"), filepath.Join(dir, "runs", "r_test.json")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing %s: %v", path, err)
		}
	}
	list, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Status != valueobject.RunSucceeded {
		t.Fatalf("list = %+v", list)
	}
	loaded, err := store.Load(context.Background(), "r_test")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Transcript) != 1 || loaded.Transcript[0].Text != "完成" {
		t.Fatalf("loaded = %+v", loaded)
	}
}

func TestRunStoreUpdateDoesNotDuplicateIndex(t *testing.T) {
	dir := t.TempDir()
	store := NewRunStore(dir)
	record := sampleRun("r_test")
	if err := store.Save(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	record.Status = valueobject.RunFailed
	record.Error = "boom"
	if err := store.Save(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	list, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Status != valueobject.RunFailed || list[0].Error != "boom" {
		t.Fatalf("list = %+v", list)
	}
}

func TestRunStoreTrimsTranscript(t *testing.T) {
	dir := t.TempDir()
	store := NewRunStore(dir)
	record := sampleRun("r_test")
	record.Transcript = make([]valueobject.TranscriptItem, 0, valueobject.MaxRunTranscriptItems+10)
	for i := range valueobject.MaxRunTranscriptItems + 10 {
		record.Transcript = append(record.Transcript, valueobject.TranscriptItem{
			ID: fmt.Sprintf("m%d", i), Kind: valueobject.TranscriptMessage, Text: "x",
		})
	}
	if err := store.Save(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(context.Background(), "r_test")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Transcript) != valueobject.MaxRunTranscriptItems {
		t.Fatalf("transcript length = %d", len(loaded.Transcript))
	}
	if loaded.Transcript[0].ID != "m10" {
		t.Fatalf("first item = %q, want m10", loaded.Transcript[0].ID)
	}
}

func TestRunStoreMissingDetailForIndexIsCorrupt(t *testing.T) {
	dir := t.TempDir()
	store := NewRunStore(dir)
	if err := store.Save(context.Background(), sampleRun("r_test")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "runs", "r_test.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.List(context.Background()); !errors.Is(err, ErrCorruptRuns) {
		t.Fatalf("error = %v, want ErrCorruptRuns", err)
	}
}

func TestRunStoreOrphanDetailIsIgnoredByList(t *testing.T) {
	dir := t.TempDir()
	store := NewRunStore(dir)
	record := sampleRun("r_orphan")
	writeJSONFileForTest(t, filepath.Join(dir, "runs", "r_orphan.json"), record)
	list, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("list = %+v", list)
	}
	loaded, err := store.Load(context.Background(), "r_orphan")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != "r_orphan" {
		t.Fatalf("loaded = %+v", loaded)
	}
}

func TestRunStoreCorruptIndex(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "runs.json"), "{")
	store := NewRunStore(dir)
	if _, err := store.List(context.Background()); !errors.Is(err, ErrCorruptRuns) {
		t.Fatalf("error = %v, want ErrCorruptRuns", err)
	}
	if err := store.Save(context.Background(), sampleRun("r_test")); !errors.Is(err, ErrCorruptRuns) {
		t.Fatalf("Save error = %v, want ErrCorruptRuns", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "runs", "r_test.json")); !os.IsNotExist(err) {
		t.Fatalf("detail was written despite corrupt index: %v", err)
	}
}

func TestRunStoreUnsupportedVersion(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "runs.json"), `{"version":2,"runs":[]}`)
	if _, err := NewRunStore(dir).List(context.Background()); !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("error = %v, want ErrUnsupportedVersion", err)
	}
}

func TestRunStoreCorruptDetail(t *testing.T) {
	dir := t.TempDir()
	store := NewRunStore(dir)
	if err := store.Save(context.Background(), sampleRun("r_test")); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(dir, "runs", "r_test.json"), "{")
	if _, err := store.Load(context.Background(), "r_test"); !errors.Is(err, ErrCorruptRuns) {
		t.Fatalf("error = %v, want ErrCorruptRuns", err)
	}
}

func TestRunStoreNotFoundAndInvalidID(t *testing.T) {
	store := NewRunStore(t.TempDir())
	if _, err := store.Load(context.Background(), "r_missing"); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("error = %v, want ErrRunNotFound", err)
	}
	if _, err := store.Load(context.Background(), "../escape"); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("error = %v, want ErrRunNotFound", err)
	}
}

func TestRunStoreRejectsInvalidRecord(t *testing.T) {
	store := NewRunStore(t.TempDir())
	record := sampleRun("r_test")
	record.Status = "unknown"
	if err := store.Save(context.Background(), record); !errors.Is(err, ErrInvalidRun) {
		t.Fatalf("error = %v, want ErrInvalidRun", err)
	}
}

func TestRunIndexAndDetailStayConsistent(t *testing.T) {
	dir := t.TempDir()
	store := NewRunStore(dir)
	first := sampleRun("r_one")
	second := sampleRun("r_two")
	if err := store.Save(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	list, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != "r_one" || list[1].ID != "r_two" {
		t.Fatalf("list = %+v", list)
	}
	detail, err := store.Load(context.Background(), "r_two")
	if err != nil {
		t.Fatal(err)
	}
	if detail.ID != second.ID || detail.ContextVersion != second.ContextVersion {
		t.Fatalf("detail = %+v", detail)
	}
}
