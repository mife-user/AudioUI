package persistence

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"audio-ui/internal/domain/valueobject"
)

func TestSessionStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "sessions.json")
	store := NewSessionStore(path)
	file := valueobject.SessionsFile{
		Version: valueobject.SessionFileVersion, ActiveChainID: valueobject.DefaultChainID,
		Chains: map[string]valueobject.SessionRecord{
			valueobject.DefaultChainID: {Adapter: "claude", DisplayID: "abc", Status: valueobject.SessionIdle, Transcript: []valueobject.TranscriptItem{{ID: "m", Kind: valueobject.TranscriptMessage, Text: "hi"}}},
		},
	}
	if err := store.Save(context.Background(), file); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ActiveChainID != valueobject.DefaultChainID || loaded.Chains[valueobject.DefaultChainID].DisplayID != "abc" {
		t.Fatalf("loaded = %+v", loaded)
	}
}

func TestSessionStoreCorruptDoesNotPanic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSessionStore(path).Load(context.Background()); err == nil {
		t.Fatal("expected corrupt error")
	}
}

func TestSessionStoreMissingReturnsDefault(t *testing.T) {
	file, err := NewSessionStore(filepath.Join(t.TempDir(), "sessions.json")).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if file.Version != valueobject.SessionFileVersion || file.Chains == nil {
		t.Fatalf("file = %+v", file)
	}
}
