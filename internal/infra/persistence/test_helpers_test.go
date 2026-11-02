package persistence

import (
	"os"
	"path/filepath"
	"testing"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/valueobject"
)

func testLayout(root string) valueobject.Layout {
	dataDir := filepath.Join(root, "data")
	return valueobject.Layout{
		Root:         root,
		DataDir:      dataDir,
		LogsDir:      filepath.Join(root, "logs"),
		WebView2Dir:  filepath.Join(root, "webview2"),
		LogFile:      filepath.Join(root, "logs", "app.log"),
		RootSource:   "exe",
		Portable:     true,
		AgentDir:     filepath.Join(dataDir, "agent"),
		WorkspaceDir: filepath.Join(dataDir, "workspace", "chain_default"),
		SessionsFile: filepath.Join(dataDir, "sessions.json"),
		AuditFile:    filepath.Join(root, "logs", "audit.log"),
		ConfigFile:   filepath.Join(dataDir, "config.json"),
		ProjectsDir:  filepath.Join(dataDir, "projects"),
		BackupDir:    filepath.Join(dataDir, "backup", "m1"),
	}
}

func sampleProject(id string) entity.Project {
	return entity.Project{
		Version:      valueobject.ProjectFileVersion,
		ID:           id,
		Name:         "测试工程",
		Revision:     1,
		ActiveNodeID: "n_root",
		Nodes: []entity.Node{{
			ID:       "n_root",
			Type:     valueobject.NodeTypeTopic,
			Title:    "选题",
			Position: entity.Position{X: 10, Y: 20},
			Params:   entity.NodeParams{Idea: "想法", CandidateCount: 3},
			Rounds:   []entity.Round{},
		}},
	}
}

func sampleRun(id string) entity.RunRecord {
	return entity.RunRecord{
		RunSummary: entity.RunSummary{
			ID:             id,
			TreeID:         "n_root",
			NodeID:         "n_root",
			ChainID:        "c_n_root",
			ChainPath:      []string{"n_root"},
			Status:         valueobject.RunSucceeded,
			ContextVersion: valueobject.DefaultContextVersion,
			StartedAt:      "2026-11-01T10:00:00Z",
			FinishedAt:     "2026-11-01T10:01:00Z",
		},
		Transcript: []valueobject.TranscriptItem{{
			ID: "m1", Kind: valueobject.TranscriptMessage, Role: "assistant", Text: "完成",
		}},
	}
}

func samplePending(id string) entity.Pending {
	return entity.Pending{
		ID:        id,
		RunID:     "r_test",
		ChainID:   "c_n_root",
		ChainPath: []string{"n_root"},
		NodeID:    "n_root",
		Kind:      valueobject.OutputKindScript,
		Status:    valueobject.PendingPending,
		Payload:   "pending/" + id + ".json",
		CreatedAt: "2026-11-01T10:00:00Z",
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
