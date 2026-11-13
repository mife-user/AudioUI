package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/valueobject"
)

func recoverPendingTransaction(ctx context.Context, projectDir string, project entity.Project) error {
	store := NewPendingStore(projectDir)
	transaction, found, err := store.LoadTransaction(ctx)
	if err != nil || !found {
		return err
	}
	if project.Revision == transaction.BaseRevision {
		if err := store.DeleteTransaction(ctx); err != nil {
			return fmt.Errorf("discard uncommitted pending transaction: %w", err)
		}
		return nil
	}
	if project.Revision != transaction.TargetRevision {
		return fmt.Errorf("%w: project revision %d is neither %d nor %d", ErrPendingTransactionConflict, project.Revision, transaction.BaseRevision, transaction.TargetRevision)
	}
	if err := validateAppliedReplyGraph(project, transaction.Pending, transaction.ArtifactPayload); err != nil {
		return fmt.Errorf("%w: persisted result does not match pending: %v", ErrPendingTransactionConflict, err)
	}
	if transaction.ArtifactMeta != nil {
		artifact, err := NewArtifactStore(projectDir).Put(ctx, transaction.ArtifactPayload, *transaction.ArtifactMeta)
		if err != nil {
			return fmt.Errorf("recover pending artifact: %w", err)
		}
		if artifact.ID != transaction.Pending.ArtifactID {
			return fmt.Errorf("%w: recovered artifact id %s does not match %s", ErrPendingTransactionConflict, artifact.ID, transaction.Pending.ArtifactID)
		}
	}
	if err := store.Save(ctx, transaction.Pending, transaction.Payload); err != nil {
		return fmt.Errorf("recover pending index: %w", err)
	}
	if err := store.DeleteTransaction(ctx); err != nil {
		return fmt.Errorf("finish pending recovery: %w", err)
	}
	return nil
}

func validateAppliedReplyGraph(project entity.Project, pending entity.Pending, artifactPayload []byte) error {
	if pending.Status != valueobject.PendingApplied || pending.OutputBinding == nil {
		return nil
	}
	dagSnapshot := pending.ExecutionSnapshot != nil && pending.ExecutionSnapshot.Version == 3
	if pending.OutputSchemaVersion != 1 || pending.ChainID != "c_"+pending.NodeID ||
		(!dagSnapshot && (len(pending.ChainPath) == 0 || pending.ChainPath[len(pending.ChainPath)-1] != pending.NodeID)) ||
		(dagSnapshot && (pending.ExecutionSnapshot.Validate() != nil || pending.ExecutionSnapshot.NodeID != pending.NodeID ||
			pending.ExecutionSnapshot.OutputContract.Kind != pending.Kind || pending.ExecutionSnapshot.OutputContract.SchemaVersion != pending.OutputSchemaVersion ||
			pending.ExecutionSnapshot.OutputBinding != *pending.OutputBinding || len(pending.ChainPath) != 0)) {
		return fmt.Errorf("invalid reply identity")
	}
	graph, err := entity.NewGraph(&project)
	if err != nil {
		return err
	}
	if err := graph.Validate(); err != nil {
		return err
	}
	if dagSnapshot {
		snapshot := pending.ExecutionSnapshot
		for _, frozen := range snapshot.InputNodes {
			live, ok := graph.Node(frozen.ID)
			if !ok || !samePendingSnapshotNode(live, frozen) {
				return fmt.Errorf("DAG input node does not match persisted graph")
			}
		}
		for _, root := range snapshot.InputRoots {
			live, ok := graph.Node(root.ID)
			if !ok || live.Type != valueobject.NodeTypeTopic || live.WorkspaceRootID != root.WorkspaceRootID {
				return fmt.Errorf("DAG input root does not match persisted graph")
			}
		}
		for _, frozen := range snapshot.InputEdges {
			live, ok := graph.Edge(frozen.ID)
			if !ok || live != frozen {
				return fmt.Errorf("DAG input edge does not match persisted graph")
			}
		}
	} else {
		path, err := graph.PathToRoot(pending.NodeID)
		if err != nil || !slices.Equal(path, pending.ChainPath) {
			return fmt.Errorf("reply path does not match persisted graph")
		}
	}
	target, ok := graph.Node(pending.NodeID)
	if !ok {
		return fmt.Errorf("reply target is missing")
	}
	binding := pending.OutputBinding
	if pending.Kind == valueobject.OutputKindTopics {
		if target.Type != valueobject.NodeTypeTopic || binding.Bound || pending.ResultNodeID != "" || len(pending.CreatedNodeIDs) == 0 || !target.Locked || target.Stale || len(target.Rounds) != 1 {
			return fmt.Errorf("topic result identity is invalid")
		}
		round := target.Rounds[0]
		if round.N != 1 || round.PendingID != pending.ID || round.RunID != pending.RunID || round.Kind != pending.Kind || round.ArtifactID != pending.ArtifactID {
			return fmt.Errorf("topic round does not reference pending result")
		}
		var output struct {
			Topics []json.RawMessage `json:"topics"`
		}
		if err := json.Unmarshal(artifactPayload, &output); err != nil || len(output.Topics) != len(pending.CreatedNodeIDs) {
			return fmt.Errorf("topic candidates do not match artifact")
		}
		created := make(map[string]struct{}, len(pending.CreatedNodeIDs))
		indices := make(map[int]struct{}, len(pending.CreatedNodeIDs))
		for _, id := range pending.CreatedNodeIDs {
			node, ok := graph.Node(id)
			sourceEdge, hasSourceEdge := graph.EdgeBetween(target.ID, id)
			if !ok || node.Type != valueobject.NodeTypeCandidate || !hasSourceEdge || sourceEdge.Kind != entity.EdgeKindCandidateSource || node.CandidateSource == nil ||
				node.CandidateSource.TopicNodeID != target.ID || node.CandidateSource.ArtifactID != pending.ArtifactID ||
				node.CandidateSource.Index < 0 || node.CandidateSource.Index >= len(output.Topics) {
				return fmt.Errorf("candidate %s does not match topic result", id)
			}
			if _, exists := indices[node.CandidateSource.Index]; exists {
				return fmt.Errorf("duplicate candidate index %d", node.CandidateSource.Index)
			}
			created[id] = struct{}{}
			indices[node.CandidateSource.Index] = struct{}{}
		}
		sourceCount := 0
		for _, edge := range graph.Outgoing(target.ID) {
			if edge.Kind == entity.EdgeKindCandidateSource {
				sourceCount++
			}
		}
		if sourceCount != len(created) || len(indices) != len(output.Topics) {
			return fmt.Errorf("topic candidate set is incomplete")
		}
		for index := range output.Topics {
			if _, ok := indices[index]; !ok {
				return fmt.Errorf("topic candidate index %d is missing", index)
			}
		}
		return nil
	}
	if pending.Kind != valueobject.OutputKindScript || target.Type != valueobject.NodeTypeAgent {
		return fmt.Errorf("script result target is invalid")
	}
	var output *entity.Node
	for _, childID := range graph.Children(target.ID) {
		child, ok := graph.Node(childID)
		if ok && child.Type == valueobject.NodeTypeScript {
			if output != nil {
				return fmt.Errorf("Agent has multiple Script outputs")
			}
			output = child
		}
	}
	if output == nil || output.Stale {
		return fmt.Errorf("Script output is missing or stale")
	}
	if project.Version >= valueobject.DAGProjectVersion &&
		(len(graph.Incoming(output.ID)) != 1 || output.WorkspaceRootID != target.WorkspaceRootID) {
		return fmt.Errorf("Script output scope is invalid")
	}
	if binding.Bound {
		if binding.ParentNodeID != target.ID || binding.NodeID != pending.ResultNodeID || output.ID != binding.NodeID || len(pending.CreatedNodeIDs) != 0 {
			return fmt.Errorf("bound Script identity is invalid")
		}
		if len(output.ScriptVersions) != binding.BaselineVersion+1 {
			return fmt.Errorf("bound Script version count does not match pending")
		}
		if binding.BaselineVersion > 0 {
			baseline := output.ScriptVersions[binding.BaselineVersion-1]
			if baseline.N != binding.BaselineVersion || baseline.ArtifactID != binding.BaselineArtifactID {
				return fmt.Errorf("bound Script baseline does not match pending")
			}
		}
		latest := output.ScriptVersions[len(output.ScriptVersions)-1]
		if latest.N != binding.BaselineVersion+1 || latest.ArtifactID != pending.ArtifactID || latest.RunID != pending.RunID {
			return fmt.Errorf("appended Script version does not match pending")
		}
		return nil
	}
	if binding.NodeID != "" || binding.ParentNodeID != "" || binding.BaselineVersion != 0 || binding.BaselineArtifactID != "" ||
		len(pending.CreatedNodeIDs) != 1 || pending.ResultNodeID != pending.CreatedNodeIDs[0] || output.ID != pending.ResultNodeID ||
		len(output.ScriptVersions) != 1 {
		return fmt.Errorf("generated Script identity is invalid")
	}
	version := output.ScriptVersions[0]
	if version.N != 1 || version.ArtifactID != pending.ArtifactID || version.RunID != pending.RunID {
		return fmt.Errorf("generated Script version does not match pending")
	}
	return nil
}

func samePendingSnapshotNode(live *entity.Node, frozen entity.Node) bool {
	return live.Type == frozen.Type && live.Title == frozen.Title && live.Parent == frozen.Parent &&
		live.WorkspaceRootID == frozen.WorkspaceRootID && live.Locked == frozen.Locked && live.Stale == frozen.Stale &&
		reflect.DeepEqual(live.ParentCandidate, frozen.ParentCandidate) && reflect.DeepEqual(live.CandidateSource, frozen.CandidateSource) &&
		reflect.DeepEqual(live.Params, frozen.Params) && slices.Equal(live.Rounds, frozen.Rounds) &&
		slices.Equal(live.ScriptVersions, frozen.ScriptVersions)
}
