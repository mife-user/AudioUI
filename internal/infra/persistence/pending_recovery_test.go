package persistence

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/infra/paths"
)

func TestProjectLoadDiscardsUncommittedPendingTransaction(t *testing.T) {
	ctx := context.Background()
	layout := testLayout(t.TempDir())
	projects := NewProjectStore(layout)
	project := sampleProject("p_test")
	if err := projects.Create(ctx, project); err != nil {
		t.Fatal(err)
	}
	paths, err := paths.ResolveProject(layout, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	store := NewPendingStore(paths.Dir)
	pending := samplePending("pa_test")
	pending.ContextHash = "context"
	pending.AncestorHash = "ancestor"
	pending.ParamsHash = "params"
	payload := []byte(`{"unvalidated":true}`)
	transaction := entity.PendingTransaction{
		Version: valueobject.PendingTransactionFileVersion, BaseRevision: 1, TargetRevision: 2,
		Pending: pending, Payload: payload,
	}
	if err := store.SaveTransaction(ctx, transaction); err != nil {
		t.Fatal(err)
	}
	if err := store.WritePayload(ctx, pending.ID, payload); err != nil {
		t.Fatal(err)
	}

	loaded, err := projects.Load(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Revision != 1 {
		t.Fatalf("revision = %d, want uncommitted base revision", loaded.Revision)
	}
	if _, err := store.Load(ctx, pending.ID); !errors.Is(err, ErrPendingNotFound) {
		t.Fatalf("pending index error = %v, want PENDING_NOT_FOUND", err)
	}
	if _, found, err := store.LoadTransaction(ctx); err != nil || found {
		t.Fatalf("transaction remains: found=%v err=%v", found, err)
	}
	if _, err := store.GetPayload(ctx, pending.ID); err != nil {
		t.Fatalf("orphan raw payload should be harmless: %v", err)
	}
}

func TestProjectLoadCompletesCommittedPendingTransaction(t *testing.T) {
	ctx := context.Background()
	layout := testLayout(t.TempDir())
	projects := NewProjectStore(layout)
	project := sampleProject("p_test")
	project.Nodes[0].Rounds = []entity.Round{{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: "sha256:topic", RunID: "r_topic"}}
	project.ActiveNodeID = "n_script"
	project.Nodes = append(project.Nodes,
		entity.Node{
			ID: "n_candidate", Type: valueobject.NodeTypeCandidate, Title: "候选", Size: entity.DefaultNodeSize(valueobject.NodeTypeCandidate),
			CandidateSource: &entity.CandidateSource{TopicNodeID: "n_root", Round: 1, ArtifactID: "sha256:topic", Index: 0, RunID: "r_topic"}, Rounds: []entity.Round{},
		},
		entity.Node{ID: "n_prompt", Type: valueobject.NodeTypePrompt, Title: "提示词", Size: entity.DefaultNodeSize(valueobject.NodeTypePrompt), Rounds: []entity.Round{}},
		entity.Node{ID: "n_agent", Type: valueobject.NodeTypeAgent, Title: "Agent", Size: entity.DefaultNodeSize(valueobject.NodeTypeAgent), Rounds: []entity.Round{}},
		entity.Node{ID: "n_script", Type: valueobject.NodeTypeScript, Title: "脚本", Size: entity.DefaultNodeSize(valueobject.NodeTypeScript), Rounds: []entity.Round{}},
	)
	linkNodes(&project, "n_root", "n_candidate")
	linkNodes(&project, "n_candidate", "n_prompt")
	linkNodes(&project, "n_prompt", "n_agent")
	linkNodes(&project, "n_agent", "n_script")
	if err := projects.Create(ctx, project); err != nil {
		t.Fatal(err)
	}
	projectPaths, err := paths.ResolveProject(layout, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	pendingStore := NewPendingStore(projectPaths.Dir)
	artifactStore := NewArtifactStore(projectPaths.Dir)
	rawPayload := []byte(` { "title" : "脚本", "scenes" : [{"narration":"旁白","duration":0}] } `)
	artifactPayload := []byte(`{"scenes":[{"duration":0,"narration":"旁白"}],"title":"脚本"}`)
	pending := entity.Pending{
		ID: "pa_test", RunID: "r_test", ChainID: "c_n_script", ChainPath: []string{"n_root", "n_candidate", "n_prompt", "n_agent", "n_script"},
		NodeID: "n_script", Kind: valueobject.OutputKindScript, Status: valueobject.PendingApplied,
		ContextHash: "context", AncestorHash: "ancestor", ParamsHash: "params",
		ArtifactID: "sha256:" + hashPayload(artifactPayload), CreatedAt: "2026-11-01T10:00:00Z",
	}
	meta := entity.ArtifactMeta{
		Kind: valueobject.OutputKindScript, NodeID: pending.NodeID, CreatedAt: pending.CreatedAt,
		SourceRunID: pending.RunID, SourcePendingID: pending.ID,
	}
	transaction := entity.PendingTransaction{
		Version: valueobject.PendingTransactionFileVersion, BaseRevision: 1, TargetRevision: 2,
		Pending: pending, Payload: rawPayload, ArtifactPayload: artifactPayload, ArtifactMeta: &meta,
	}
	if err := pendingStore.SaveTransaction(ctx, transaction); err != nil {
		t.Fatal(err)
	}
	project.Revision = 2
	project.Nodes[4].Locked = true
	project.Nodes[4].ScriptVersions = []entity.ScriptVersion{{N: 1, ArtifactID: pending.ArtifactID, RunID: pending.RunID}}
	if err := projects.Save(ctx, project); err != nil {
		t.Fatal(err)
	}

	loaded, err := projects.Load(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Revision != 2 || !loaded.Nodes[4].Locked || len(loaded.Nodes[4].ScriptVersions) != 1 {
		t.Fatalf("project commit was not preserved: %+v", loaded)
	}
	stored, err := pendingStore.Load(ctx, pending.ID)
	if err != nil || stored.Status != valueobject.PendingApplied || stored.ArtifactID != pending.ArtifactID {
		t.Fatalf("pending recovery = %+v err=%v", stored, err)
	}
	storedPayload, err := pendingStore.GetPayload(ctx, pending.ID)
	if err != nil || string(storedPayload) != string(rawPayload) {
		t.Fatalf("raw payload = %q err=%v", storedPayload, err)
	}
	artifact, payload, err := artifactStore.Get(ctx, pending.ArtifactID)
	if err != nil || artifact.Kind != valueobject.OutputKindScript || string(payload) != string(artifactPayload) {
		t.Fatalf("artifact recovery = %+v payload=%s err=%v", artifact, payload, err)
	}
	if _, found, err := pendingStore.LoadTransaction(ctx); err != nil || found {
		t.Fatalf("transaction remains: found=%v err=%v", found, err)
	}
	if _, err := projects.Load(ctx, project.ID); err != nil {
		t.Fatalf("repeated recovery: %v", err)
	}
}

func TestCommittedRootlessDAGScriptReplyRecovery(t *testing.T) {
	ctx := context.Background()
	layout := testLayout(t.TempDir())
	projects := NewProjectStore(layout)
	project := entity.Project{
		Version: valueobject.ProjectFileVersion, ID: "p_test", Name: "无 Topic 恢复", Revision: 1,
		Nodes: []entity.Node{
			{ID: "n_source", Type: valueobject.NodeTypeScript, Title: "输入", Size: entity.DefaultNodeSize(valueobject.NodeTypeScript), ScriptVersions: []entity.ScriptVersion{{N: 1, ArtifactID: "sha256:source", RunID: "r_source"}}, Rounds: []entity.Round{}},
			{ID: "n_prompt", Type: valueobject.NodeTypePrompt, Title: "任务", Size: entity.DefaultNodeSize(valueobject.NodeTypePrompt), Params: entity.NodeParams{Instruction: "续写"}, Rounds: []entity.Round{}},
			{ID: "n_agent", Type: valueobject.NodeTypeAgent, Title: "Agent", Size: entity.DefaultNodeSize(valueobject.NodeTypeAgent), Rounds: []entity.Round{}},
			{ID: "n_output", Type: valueobject.NodeTypeScript, Title: "输出", Size: entity.DefaultNodeSize(valueobject.NodeTypeScript), ScriptVersions: []entity.ScriptVersion{{N: 1, ArtifactID: "sha256:output-v1", RunID: "r_old"}}, Rounds: []entity.Round{}},
		},
		Edges: []entity.Edge{
			{ID: "e_source_prompt", SourceNodeID: "n_source", TargetNodeID: "n_prompt", Kind: entity.EdgeKindConnection, ScriptVersion: 1},
			{ID: "e_prompt_agent", SourceNodeID: "n_prompt", TargetNodeID: "n_agent", Kind: entity.EdgeKindConnection},
			{ID: "e_agent_output", SourceNodeID: "n_agent", TargetNodeID: "n_output", Kind: entity.EdgeKindConnection},
		},
	}
	graph, err := entity.NewGraph(&project)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := graph.SnapshotForRunWithInputs("n_agent", []string{"e_prompt_agent"})
	if err != nil || snapshot.Version != 4 || snapshot.OutputBinding.NodeID != "n_output" {
		t.Fatalf("rootless DAG snapshot = %+v err=%v", snapshot, err)
	}
	if err := projects.Create(ctx, project); err != nil {
		t.Fatal(err)
	}
	projectPaths, err := paths.ResolveProject(layout, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	pendingStore := NewPendingStore(projectPaths.Dir)
	artifactPayload := []byte(`{"scenes":[{"duration":0,"narration":"旁白"}],"title":"续写"}`)
	pending := entity.Pending{
		ID: "pa_dag", RunID: "r_dag", ChainID: "c_n_agent", ChainPath: []string{}, NodeID: "n_agent",
		Kind: valueobject.OutputKindScript, Status: valueobject.PendingApplied, ArtifactID: "sha256:" + hashPayload(artifactPayload),
		OutputBinding: &snapshot.OutputBinding, OutputSchemaVersion: snapshot.OutputContract.SchemaVersion,
		ExecutionSnapshot: &snapshot, ResultNodeID: "n_output", CreatedAt: "2026-11-13T00:00:00Z",
	}
	meta := entity.ArtifactMeta{
		Kind: valueobject.OutputKindScript, NodeID: pending.ResultNodeID, CreatedAt: pending.CreatedAt,
		SourceRunID: pending.RunID, SourcePendingID: pending.ID,
	}
	if err := pendingStore.SaveTransaction(ctx, entity.PendingTransaction{
		Version: valueobject.PendingTransactionFileVersion, BaseRevision: 1, TargetRevision: 2,
		Pending: pending, Payload: artifactPayload, ArtifactPayload: artifactPayload, ArtifactMeta: &meta,
	}); err != nil {
		t.Fatal(err)
	}
	project.Revision = 2
	project.Nodes[3].ScriptVersions = append(project.Nodes[3].ScriptVersions, entity.ScriptVersion{
		N: 2, ArtifactID: pending.ArtifactID, RunID: pending.RunID,
	})
	if err := projects.Save(ctx, project); err != nil {
		t.Fatal(err)
	}

	if _, err := projects.Load(ctx, project.ID); err != nil {
		t.Fatalf("rootless DAG transaction recovery: %v", err)
	}
	stored, err := pendingStore.Load(ctx, pending.ID)
	if err != nil || stored.Status != valueobject.PendingApplied || stored.ExecutionSnapshot == nil || stored.ExecutionSnapshot.Version != 4 {
		t.Fatalf("recovered DAG pending = %+v err=%v", stored, err)
	}
}

func TestCommittedCreateOutputDAGReplyRecovery(t *testing.T) {
	ctx := context.Background()
	layout := testLayout(t.TempDir())
	projects := NewProjectStore(layout)
	project := entity.Project{
		Version: valueobject.ProjectFileVersion, ID: "p_dag_create", Name: "DAG 新建输出", Revision: 1, ActiveNodeID: "n_agent",
		Nodes: []entity.Node{
			{ID: "n_prompt", Type: valueobject.NodeTypePrompt, Title: "任务", Position: entity.Position{X: 10, Y: 20}, Size: entity.DefaultNodeSize(valueobject.NodeTypePrompt), Params: entity.NodeParams{Instruction: "续写"}, Rounds: []entity.Round{}},
			{ID: "n_agent", Type: valueobject.NodeTypeAgent, Title: "Agent", Position: entity.Position{X: 30, Y: 20}, Size: entity.DefaultNodeSize(valueobject.NodeTypeAgent), Rounds: []entity.Round{}},
		},
		Edges: []entity.Edge{{ID: "e_prompt_agent", SourceNodeID: "n_prompt", TargetNodeID: "n_agent", Kind: entity.EdgeKindConnection}},
	}
	graph, err := entity.NewGraph(&project)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := graph.SnapshotForRunWithOptions("n_agent", entity.RunStartOptions{CreateOutput: true})
	if err != nil || snapshot.Version != 4 || snapshot.OutputBinding.Mode != entity.RunOutputModeCreate || snapshot.OutputEdge == nil {
		t.Fatalf("create-output snapshot = %+v err=%v", snapshot, err)
	}
	if err := projects.Create(ctx, project); err != nil {
		t.Fatal(err)
	}
	projectPaths, err := paths.ResolveProject(layout, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	pendingStore := NewPendingStore(projectPaths.Dir)
	artifactPayload := []byte(`{"scenes":[{"duration":1,"narration":"新旁白"}],"title":"新脚本"}`)
	artifactID := "sha256:" + hashPayload(artifactPayload)
	pending := entity.Pending{
		ID: "pa_create", RunID: "r_create", ChainID: "c_n_agent", ChainPath: []string{}, NodeID: "n_agent",
		Kind: valueobject.OutputKindScript, Status: valueobject.PendingApplied, ArtifactID: artifactID,
		OutputBinding: &snapshot.OutputBinding, OutputSchemaVersion: snapshot.OutputContract.SchemaVersion,
		ExecutionSnapshot: &snapshot, ResultNodeID: snapshot.OutputBinding.NodeID,
		CreatedNodeIDs: []string{snapshot.OutputBinding.NodeID}, CreatedAt: "2026-11-13T00:00:00Z",
	}
	meta := entity.ArtifactMeta{
		Kind: valueobject.OutputKindScript, NodeID: pending.ResultNodeID, CreatedAt: pending.CreatedAt,
		SourceRunID: pending.RunID, SourcePendingID: pending.ID,
	}
	if err := pendingStore.SaveTransaction(ctx, entity.PendingTransaction{
		Version: valueobject.PendingTransactionFileVersion, BaseRevision: 1, TargetRevision: 2,
		Pending: pending, Payload: artifactPayload, ArtifactPayload: artifactPayload, ArtifactMeta: &meta,
	}); err != nil {
		t.Fatal(err)
	}
	project.Revision = 2
	project.Nodes = append(project.Nodes, entity.Node{
		ID: pending.ResultNodeID, Type: valueobject.NodeTypeScript, Title: "新脚本", Size: entity.DefaultNodeSize(valueobject.NodeTypeScript),
		ScriptVersions: []entity.ScriptVersion{{N: 1, ArtifactID: artifactID, RunID: pending.RunID}}, Rounds: []entity.Round{},
	})
	project.Edges = append(project.Edges, *snapshot.OutputEdge)
	if err := projects.Save(ctx, project); err != nil {
		t.Fatal(err)
	}

	loaded, err := projects.Load(ctx, project.ID)
	if err != nil || len(loaded.Nodes) != 3 || loaded.Nodes[2].ID != pending.ResultNodeID {
		t.Fatalf("create-output recovery nodes=%+v err=%v", loaded.Nodes, err)
	}
	stored, err := pendingStore.Load(ctx, pending.ID)
	if err != nil || stored.Status != valueobject.PendingApplied || stored.CreatedNodeIDs[0] != pending.ResultNodeID {
		t.Fatalf("recovered create-output pending = %+v err=%v", stored, err)
	}
	if _, _, err := NewArtifactStore(projectPaths.Dir).Get(ctx, artifactID); err != nil {
		t.Fatalf("create-output artifact recovery: %v", err)
	}
	if _, found, err := pendingStore.LoadTransaction(ctx); err != nil || found {
		t.Fatalf("create-output transaction remains: found=%v err=%v", found, err)
	}
}

func TestCommittedTopicBatchReplyRecovery(t *testing.T) {
	ctx := context.Background()
	layout := testLayout(t.TempDir())
	projects := NewProjectStore(layout)
	project := sampleProject("p_topic_batch")
	firstPayload := []byte(`{"topics":[{"title":"旧一"},{"title":"旧二"},{"title":"旧三"}]}`)
	firstArtifactID := "sha256:" + hashPayload(firstPayload)
	project.Nodes[0].Locked = true
	project.Nodes[0].Rounds = []entity.Round{{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: firstArtifactID, PendingID: "pa_first", RunID: "r_first"}}
	for index := 0; index < 3; index++ {
		id := fmt.Sprintf("n_candidate_old_%d", index)
		project.Nodes = append(project.Nodes, entity.Node{
			ID: id, Type: valueobject.NodeTypeCandidate, Title: fmt.Sprintf("旧候选 %d", index+1), Size: entity.DefaultNodeSize(valueobject.NodeTypeCandidate),
			CandidateSource: &entity.CandidateSource{TopicNodeID: "n_root", Round: 1, ArtifactID: firstArtifactID, Index: index, RunID: "r_first"},
			Rounds:          []entity.Round{},
		})
		linkNodes(&project, "n_root", id)
	}
	graph, err := entity.NewGraph(&project)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := graph.SnapshotForRunWithOptions("n_root", entity.RunStartOptions{TopicRerun: true})
	if err != nil || snapshot.TopicBatch == nil || snapshot.TopicBatch.BaselineRound != 1 {
		t.Fatalf("topic batch snapshot = %+v err=%v", snapshot, err)
	}
	if err := projects.Create(ctx, project); err != nil {
		t.Fatal(err)
	}
	projectPaths, err := paths.ResolveProject(layout, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	pendingStore := NewPendingStore(projectPaths.Dir)
	artifactPayload := []byte(`{"topics":[{"angle":"新方向一","hook":"新开场一","title":"新一"},{"angle":"新方向二","hook":"新开场二","title":"新二"},{"angle":"新方向三","hook":"新开场三","title":"新三"}]}`)
	artifactID := "sha256:" + hashPayload(artifactPayload)
	pending := entity.Pending{
		ID: "pa_topic_batch", RunID: "r_topic_batch", ChainID: "c_n_root", ChainPath: []string{"n_root"}, NodeID: "n_root",
		Kind: valueobject.OutputKindTopics, Status: valueobject.PendingApplied, ArtifactID: artifactID,
		OutputBinding: &snapshot.OutputBinding, OutputSchemaVersion: snapshot.OutputContract.SchemaVersion,
		ExecutionSnapshot: &snapshot, CreatedNodeIDs: []string{"n_candidate_new_0", "n_candidate_new_1", "n_candidate_new_2"},
		CreatedAt: "2026-11-13T00:00:00Z",
	}
	meta := entity.ArtifactMeta{
		Kind: valueobject.OutputKindTopics, NodeID: pending.NodeID, CreatedAt: pending.CreatedAt,
		SourceRunID: pending.RunID, SourcePendingID: pending.ID,
	}
	if err := pendingStore.SaveTransaction(ctx, entity.PendingTransaction{
		Version: valueobject.PendingTransactionFileVersion, BaseRevision: 1, TargetRevision: 2,
		Pending: pending, Payload: artifactPayload, ArtifactPayload: artifactPayload, ArtifactMeta: &meta,
	}); err != nil {
		t.Fatal(err)
	}
	project.Revision = 2
	project.Nodes[0].Rounds = append(project.Nodes[0].Rounds, entity.Round{
		N: 2, Kind: valueobject.OutputKindTopics, ArtifactID: artifactID, PendingID: pending.ID, RunID: pending.RunID,
	})
	for index, id := range pending.CreatedNodeIDs {
		project.Nodes = append(project.Nodes, entity.Node{
			ID: id, Type: valueobject.NodeTypeCandidate, Title: fmt.Sprintf("新候选 %d", index+1), Size: entity.DefaultNodeSize(valueobject.NodeTypeCandidate),
			CandidateSource: &entity.CandidateSource{TopicNodeID: "n_root", Round: 2, ArtifactID: artifactID, Index: index, RunID: pending.RunID},
			Rounds:          []entity.Round{},
		})
		linkNodes(&project, "n_root", id)
	}
	if err := projects.Save(ctx, project); err != nil {
		t.Fatal(err)
	}

	loaded, err := projects.Load(ctx, project.ID)
	if err != nil || len(loaded.Nodes) != 7 || len(loaded.Nodes[0].Rounds) != 2 {
		t.Fatalf("topic batch recovery nodes=%d rounds=%+v err=%v", len(loaded.Nodes), loaded.Nodes[0].Rounds, err)
	}
	stored, err := pendingStore.Load(ctx, pending.ID)
	if err != nil || stored.Status != valueobject.PendingApplied || stored.ExecutionSnapshot == nil || stored.ExecutionSnapshot.TopicBatch == nil {
		t.Fatalf("recovered topic batch pending = %+v err=%v", stored, err)
	}
	if _, _, err := NewArtifactStore(projectPaths.Dir).Get(ctx, artifactID); err != nil {
		t.Fatalf("topic batch artifact recovery: %v", err)
	}
	if _, found, err := pendingStore.LoadTransaction(ctx); err != nil || found {
		t.Fatalf("topic batch transaction remains: found=%v err=%v", found, err)
	}
}

func TestCommittedTopicReplyRecoveryValidatesCandidateGraphAssociation(t *testing.T) {
	ctx := context.Background()
	for _, validGraph := range []bool{true, false} {
		name := "mismatched graph"
		if validGraph {
			name = "matching graph"
		}
		t.Run(name, func(t *testing.T) {
			layout := testLayout(t.TempDir())
			projects := NewProjectStore(layout)
			project := sampleProject("p_test")
			if err := projects.Create(ctx, project); err != nil {
				t.Fatal(err)
			}
			projectPaths, err := paths.ResolveProject(layout, project.ID)
			if err != nil {
				t.Fatal(err)
			}
			artifactPayload := []byte(`{"topics":[{"angle":"夜间观察","hook":"凌晨四点","title":"夜班面包房"},{"angle":"旧物流转","hook":"三十年后","title":"唱片店"},{"angle":"城市切片","hook":"末班车后","title":"雨夜末班车"}]}`)
			artifactID := "sha256:" + hashPayload(artifactPayload)
			pending := entity.Pending{
				ID: "pa_test", RunID: "r_test", RequestID: "req_topics", ChainID: "c_n_root", ChainPath: []string{"n_root"},
				NodeID: "n_root", Kind: valueobject.OutputKindTopics, Status: valueobject.PendingApplied,
				Payload: "pending/pa_test.json", ArtifactID: artifactID,
				OutputBinding: &entity.RunOutputBinding{}, OutputSchemaVersion: 1,
				CreatedNodeIDs: []string{"n_candidate1", "n_candidate2", "n_candidate3"},
				CreatedAt:      "2026-11-01T10:00:00Z",
			}
			meta := entity.ArtifactMeta{
				Kind: valueobject.OutputKindTopics, NodeID: pending.NodeID, CreatedAt: pending.CreatedAt,
				SourceRunID: pending.RunID, SourcePendingID: pending.ID,
			}
			pendingStore := NewPendingStore(projectPaths.Dir)
			if err := pendingStore.SaveTransaction(ctx, entity.PendingTransaction{
				Version: valueobject.PendingTransactionFileVersion, BaseRevision: 1, TargetRevision: 2,
				Pending: pending, Payload: artifactPayload, ArtifactPayload: artifactPayload, ArtifactMeta: &meta,
			}); err != nil {
				t.Fatal(err)
			}
			project.Revision = 2
			if validGraph {
				project.Nodes[0].Locked = true
				project.Nodes[0].Rounds = []entity.Round{{
					N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: artifactID, PendingID: pending.ID, RunID: pending.RunID,
				}}
				for index, id := range pending.CreatedNodeIDs {
					project.Nodes = append(project.Nodes, entity.Node{
						ID: id, Type: valueobject.NodeTypeCandidate, Title: "候选", Size: entity.DefaultNodeSize(valueobject.NodeTypeCandidate),
						CandidateSource: &entity.CandidateSource{TopicNodeID: "n_root", Round: 1, ArtifactID: artifactID, Index: index, RunID: pending.RunID},
						Rounds:          []entity.Round{},
					})
					linkNodes(&project, "n_root", id)
				}
			}
			if err := projects.Save(ctx, project); err != nil {
				t.Fatal(err)
			}
			loaded, err := projects.Load(ctx, project.ID)
			if validGraph {
				if err != nil || len(loaded.Nodes) != 4 {
					t.Fatalf("recovered topic graph nodes=%d err=%v", len(loaded.Nodes), err)
				}
				stored, err := pendingStore.Load(ctx, pending.ID)
				if err != nil || stored.Status != valueobject.PendingApplied {
					t.Fatalf("recovered pending = %+v err=%v", stored, err)
				}
				artifact, data, err := NewArtifactStore(projectPaths.Dir).Get(ctx, artifactID)
				if err != nil || artifact.ID != artifactID || string(data) != string(artifactPayload) {
					t.Fatalf("recovered artifact = %+v payload=%s err=%v", artifact, data, err)
				}
				if _, err := projects.Load(ctx, project.ID); err != nil {
					t.Fatalf("repeated recovery = %v", err)
				}
				return
			}
			if !errors.Is(err, ErrPendingTransactionConflict) {
				t.Fatalf("mismatched graph recovery error = %v, want PENDING_TRANSACTION_CONFLICT", err)
			}
			if _, found, err := pendingStore.LoadTransaction(ctx); err != nil || !found {
				t.Fatalf("mismatched transaction lost: found=%v err=%v", found, err)
			}
			if _, err := pendingStore.Load(ctx, pending.ID); !errors.Is(err, ErrPendingNotFound) {
				t.Fatalf("mismatched pending index entry = %v, want PENDING_NOT_FOUND", err)
			}
			if _, _, err := NewArtifactStore(projectPaths.Dir).Get(ctx, artifactID); !errors.Is(err, ErrArtifactNotFound) {
				t.Fatalf("mismatched graph wrote artifact: %v", err)
			}
		})
	}
}

func TestCommittedScriptReplyRecoveryValidatesVersionAssociation(t *testing.T) {
	ctx := context.Background()
	for _, validGraph := range []bool{true, false} {
		name := "mismatched version"
		if validGraph {
			name = "matching version"
		}
		t.Run(name, func(t *testing.T) {
			layout := testLayout(t.TempDir())
			projects := NewProjectStore(layout)
			project := sampleProject("p_test")
			project.Nodes[0].Locked = true
			project.Nodes[0].Rounds = []entity.Round{{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: "sha256:topics", RunID: "r_topics"}}
			project.Nodes = append(project.Nodes,
				entity.Node{ID: "n_candidate", Type: valueobject.NodeTypeCandidate, Title: "候选", Size: entity.DefaultNodeSize(valueobject.NodeTypeCandidate), CandidateSource: &entity.CandidateSource{TopicNodeID: "n_root", Round: 1, ArtifactID: "sha256:topics", Index: 0, RunID: "r_topics"}, Rounds: []entity.Round{}},
				entity.Node{ID: "n_prompt", Type: valueobject.NodeTypePrompt, Title: "提示词", Size: entity.DefaultNodeSize(valueobject.NodeTypePrompt), Rounds: []entity.Round{}},
				entity.Node{ID: "n_agent", Type: valueobject.NodeTypeAgent, Title: "Agent", Size: entity.DefaultNodeSize(valueobject.NodeTypeAgent), Rounds: []entity.Round{}},
				entity.Node{ID: "n_script", Type: valueobject.NodeTypeScript, Title: "脚本", Size: entity.DefaultNodeSize(valueobject.NodeTypeScript), ScriptVersions: []entity.ScriptVersion{{N: 1, ArtifactID: "sha256:old-script", RunID: "r_old"}}, Rounds: []entity.Round{}},
			)
			linkNodes(&project, "n_root", "n_candidate")
			linkNodes(&project, "n_candidate", "n_prompt")
			linkNodes(&project, "n_prompt", "n_agent")
			linkNodes(&project, "n_agent", "n_script")
			if err := projects.Create(ctx, project); err != nil {
				t.Fatal(err)
			}
			projectPaths, err := paths.ResolveProject(layout, project.ID)
			if err != nil {
				t.Fatal(err)
			}
			artifactPayload := []byte(`{"scenes":[{"duration":0,"narration":"新旁白"}],"title":"新脚本"}`)
			artifactID := "sha256:" + hashPayload(artifactPayload)
			pending := entity.Pending{
				ID: "pa_test", RunID: "r_test", RequestID: "req_script",
				ChainID: "c_n_agent", ChainPath: []string{"n_root", "n_candidate", "n_prompt", "n_agent"},
				NodeID: "n_agent", Kind: valueobject.OutputKindScript, Status: valueobject.PendingApplied,
				Payload: "pending/pa_test.json", ArtifactID: artifactID,
				OutputBinding:       &entity.RunOutputBinding{Bound: true, NodeID: "n_script", ParentNodeID: "n_agent", BaselineVersion: 1, BaselineArtifactID: "sha256:old-script"},
				OutputSchemaVersion: 1, ResultNodeID: "n_script", CreatedAt: "2026-11-01T10:00:00Z",
			}
			meta := entity.ArtifactMeta{
				Kind: valueobject.OutputKindScript, NodeID: "n_script", CreatedAt: pending.CreatedAt,
				SourceRunID: pending.RunID, SourcePendingID: pending.ID,
			}
			pendingStore := NewPendingStore(projectPaths.Dir)
			if err := pendingStore.SaveTransaction(ctx, entity.PendingTransaction{
				Version: valueobject.PendingTransactionFileVersion, BaseRevision: 1, TargetRevision: 2,
				Pending: pending, Payload: artifactPayload, ArtifactPayload: artifactPayload, ArtifactMeta: &meta,
			}); err != nil {
				t.Fatal(err)
			}
			project.Revision = 2
			if validGraph {
				project.Nodes[4].ScriptVersions = append(project.Nodes[4].ScriptVersions, entity.ScriptVersion{N: 2, ArtifactID: artifactID, RunID: pending.RunID})
			}
			if err := projects.Save(ctx, project); err != nil {
				t.Fatal(err)
			}
			loaded, err := projects.Load(ctx, project.ID)
			if validGraph {
				if err != nil || len(loaded.Nodes[4].ScriptVersions) != 2 {
					t.Fatalf("recovered Script versions=%+v err=%v", loaded.Nodes[4].ScriptVersions, err)
				}
				stored, err := pendingStore.Load(ctx, pending.ID)
				if err != nil || stored.Status != valueobject.PendingApplied {
					t.Fatalf("recovered pending = %+v err=%v", stored, err)
				}
				if _, err := projects.Load(ctx, project.ID); err != nil {
					t.Fatalf("repeated recovery = %v", err)
				}
				return
			}
			if !errors.Is(err, ErrPendingTransactionConflict) {
				t.Fatalf("mismatched version recovery error = %v, want PENDING_TRANSACTION_CONFLICT", err)
			}
			if _, found, err := pendingStore.LoadTransaction(ctx); err != nil || !found {
				t.Fatalf("mismatched transaction lost: found=%v err=%v", found, err)
			}
			if _, _, err := NewArtifactStore(projectPaths.Dir).Get(ctx, artifactID); !errors.Is(err, ErrArtifactNotFound) {
				t.Fatalf("mismatched graph wrote artifact: %v", err)
			}
		})
	}
}

func TestProjectLoadRejectsAmbiguousOrCorruptPendingTransaction(t *testing.T) {
	ctx := context.Background()
	t.Run("revision conflict", func(t *testing.T) {
		layout := testLayout(t.TempDir())
		projects := NewProjectStore(layout)
		project := sampleProject("p_test")
		if err := projects.Create(ctx, project); err != nil {
			t.Fatal(err)
		}
		projectPaths, err := paths.ResolveProject(layout, project.ID)
		if err != nil {
			t.Fatal(err)
		}
		store := NewPendingStore(projectPaths.Dir)
		pending := samplePending("pa_test")
		pending.ContextHash, pending.AncestorHash, pending.ParamsHash = "context", "ancestor", "params"
		if err := store.SaveTransaction(ctx, entity.PendingTransaction{
			Version: valueobject.PendingTransactionFileVersion, BaseRevision: 1, TargetRevision: 2,
			Pending: pending, Payload: []byte(`{}`),
		}); err != nil {
			t.Fatal(err)
		}
		project.Revision = 3
		if err := projects.Save(ctx, project); err != nil {
			t.Fatal(err)
		}
		if _, err := projects.Load(ctx, project.ID); !errors.Is(err, ErrPendingTransactionConflict) {
			t.Fatalf("Load error = %v, want PENDING_TRANSACTION_CONFLICT", err)
		}
	})

	t.Run("corrupt journal", func(t *testing.T) {
		layout := testLayout(t.TempDir())
		projects := NewProjectStore(layout)
		project := sampleProject("p_test")
		if err := projects.Create(ctx, project); err != nil {
			t.Fatal(err)
		}
		projectPaths, err := paths.ResolveProject(layout, project.ID)
		if err != nil {
			t.Fatal(err)
		}
		journal := filepath.Join(projectPaths.Dir, "pending.txn.json")
		if err := os.WriteFile(journal, []byte("{"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := projects.Load(ctx, project.ID); !errors.Is(err, ErrCorruptPendingTransaction) {
			t.Fatalf("Load error = %v, want PENDING_TRANSACTION_CORRUPT", err)
		}
		contents, err := os.ReadFile(journal)
		if err != nil || string(contents) != "{" {
			t.Fatalf("corrupt journal was modified: %q err=%v", contents, err)
		}
	})
}
