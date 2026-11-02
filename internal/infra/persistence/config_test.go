package persistence

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"audio-ui/internal/domain/valueobject"
)

func TestConfigStoreMissingReturnsDefaults(t *testing.T) {
	layout := testLayout(t.TempDir())
	config, err := NewConfigStore(layout.ConfigFile).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if config.LastProjectID != "" {
		t.Fatalf("LastProjectID = %q", config.LastProjectID)
	}
	if config.ContextTokenBudget != valueobject.DefaultContextTokenBudget {
		t.Fatalf("ContextTokenBudget = %d", config.ContextTokenBudget)
	}
}

func TestConfigStoreRoundTrip(t *testing.T) {
	layout := testLayout(t.TempDir())
	store := NewConfigStore(layout.ConfigFile)
	want := valueobject.AppConfig{LastProjectID: "p_test", ContextTokenBudget: 5000}
	if err := store.Save(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("config = %+v, want %+v", got, want)
	}
}

func TestConfigStoreNormalizesZeroBudget(t *testing.T) {
	layout := testLayout(t.TempDir())
	store := NewConfigStore(layout.ConfigFile)
	if err := store.Save(context.Background(), valueobject.AppConfig{LastProjectID: "p_test"}); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.ContextTokenBudget != valueobject.DefaultContextTokenBudget {
		t.Fatalf("budget = %d", got.ContextTokenBudget)
	}
}

func TestConfigStoreCorruptIsExplicit(t *testing.T) {
	layout := testLayout(t.TempDir())
	writeTestFile(t, layout.ConfigFile, "{")
	if _, err := NewConfigStore(layout.ConfigFile).Load(context.Background()); !errors.Is(err, ErrCorruptConfig) {
		t.Fatalf("error = %v, want ErrCorruptConfig", err)
	}
	data, err := os.ReadFile(layout.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{" {
		t.Fatalf("corrupt file was modified: %q", data)
	}
}

func TestConfigStoreRejectsNegativeBudget(t *testing.T) {
	layout := testLayout(t.TempDir())
	writeTestFile(t, layout.ConfigFile, `{"lastProjectId":"","contextTokenBudget":-1}`)
	if _, err := NewConfigStore(layout.ConfigFile).Load(context.Background()); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("error = %v, want ErrInvalidConfig", err)
	}
	if err := NewConfigStore(layout.ConfigFile).Save(context.Background(), valueobject.AppConfig{ContextTokenBudget: -1}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Save error = %v, want ErrInvalidConfig", err)
	}
}

func TestConfigStoreStripsBOM(t *testing.T) {
	layout := testLayout(t.TempDir())
	writeTestFile(t, layout.ConfigFile, string([]byte{0xEF, 0xBB, 0xBF})+`{"lastProjectId":"p_test","contextTokenBudget":12}`)
	config, err := NewConfigStore(layout.ConfigFile).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if config.LastProjectID != "p_test" || config.ContextTokenBudget != 12 {
		t.Fatalf("config = %+v", config)
	}
}

func TestConfigStoreAtomicWriteLeavesNoTempFiles(t *testing.T) {
	layout := testLayout(t.TempDir())
	store := NewConfigStore(layout.ConfigFile)
	for i := range 3 {
		if err := store.Save(context.Background(), valueobject.AppConfig{ContextTokenBudget: 100 + i}); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(filepath.Dir(layout.ConfigFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "config.json" {
			t.Fatalf("unexpected file %q", entry.Name())
		}
	}
}
