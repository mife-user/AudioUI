package entity

import (
	"fmt"

	"audio-ui/internal/domain/valueobject"
)

func (g *Graph) SnapshotForRun(nodeID string) (RunExecutionSnapshot, error) {
	return g.SnapshotForRunWithOptions(nodeID, RunStartOptions{})
}

func (g *Graph) SnapshotForRunWithInputs(nodeID string, inputEdgeIDs []string) (RunExecutionSnapshot, error) {
	return g.SnapshotForRunWithOptions(nodeID, RunStartOptions{InputEdgeIDs: inputEdgeIDs})
}

func (g *Graph) SnapshotForRunWithOptions(nodeID string, options RunStartOptions) (RunExecutionSnapshot, error) {
	if g.project.Version >= valueobject.ProjectFileVersion {
		target, ok := g.Node(nodeID)
		if !ok {
			return RunExecutionSnapshot{}, fmt.Errorf("%w: %s", ErrGraphNodeNotFound, nodeID)
		}
		switch target.Type {
		case valueobject.NodeTypeTopic:
			if len(options.InputEdgeIDs) != 0 || options.OutputNodeID != "" || options.CreateOutput {
				return RunExecutionSnapshot{}, fmt.Errorf("%w: topic run does not accept Agent output options", ErrRunGraphUnsupported)
			}
			snapshot, err := g.snapshotDAGForRun(nodeID)
			if err != nil {
				return RunExecutionSnapshot{}, err
			}
			if options.TopicRerun {
				if !target.Locked || len(target.Rounds) == 0 {
					return RunExecutionSnapshot{}, fmt.Errorf("%w: Topic has no applied batch to rerun", ErrNodeConnectionInvalid)
				}
				latest := target.Rounds[len(target.Rounds)-1]
				snapshot.TopicBatch = &RunTopicBatch{BaselineRound: latest.N}
			}
			if err := snapshot.Validate(); err != nil {
				return RunExecutionSnapshot{}, err
			}
			return snapshot, nil
		case valueobject.NodeTypeAgent:
			return g.snapshotDAGInputsForRunWithOptions(nodeID, options)
		default:
			return RunExecutionSnapshot{}, fmt.Errorf("%w: %s", ErrNodeConnectionInvalid, target.Type)
		}
	}
	if g.project.Version >= valueobject.DAGProjectVersion {
		if options.OutputNodeID != "" || options.CreateOutput || options.TopicRerun {
			return RunExecutionSnapshot{}, fmt.Errorf("%w: output selection requires project v5", ErrRunGraphUnsupported)
		}
		if options.InputEdgeIDs != nil {
			return g.snapshotDAGInputsForRun(nodeID, options.InputEdgeIDs)
		}
		snapshot, legacyErr := g.snapshotDAGForRun(nodeID)
		if legacyErr == nil {
			return snapshot, nil
		}
		if target, ok := g.Node(nodeID); ok && target.Type == valueobject.NodeTypeAgent {
			return g.snapshotDAGInputsForRun(nodeID, nil)
		}
		return RunExecutionSnapshot{}, legacyErr
	}
	if options.InputEdgeIDs != nil || options.OutputNodeID != "" || options.CreateOutput || options.TopicRerun {
		return RunExecutionSnapshot{}, fmt.Errorf("%w: explicit run options require a DAG project", ErrRunGraphUnsupported)
	}
	return g.snapshotLegacyForRun(nodeID)
}
