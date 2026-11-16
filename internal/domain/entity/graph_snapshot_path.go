package entity

import (
	"fmt"

	"audio-ui/internal/domain/valueobject"
)

func (g *Graph) snapshotLegacyForRun(nodeID string) (RunExecutionSnapshot, error) {
	target, ok := g.Node(nodeID)
	if !ok {
		return RunExecutionSnapshot{}, fmt.Errorf("%w: %s", ErrGraphNodeNotFound, nodeID)
	}
	path, err := g.PathToRoot(nodeID)
	if err != nil {
		return RunExecutionSnapshot{}, err
	}
	root, ok := g.Node(path[0])
	if !ok || root.Type != valueobject.NodeTypeTopic {
		return RunExecutionSnapshot{}, fmt.Errorf("%w: run path must start at a topic", ErrGraphInvalid)
	}
	kind := valueobject.OutputKindTopics
	switch target.Type {
	case valueobject.NodeTypeTopic:
	case valueobject.NodeTypeAgent:
		kind = valueobject.OutputKindScript
	default:
		return RunExecutionSnapshot{}, fmt.Errorf("%w: %s", ErrNodeConnectionInvalid, target.Type)
	}

	snapshot := RunExecutionSnapshot{
		Version: 1, ProjectVersion: g.project.Version, ProjectID: g.project.ID,
		ProjectRevision: g.project.Revision, TreeID: root.ID, NodeID: target.ID,
		ChainPath: append([]string(nil), path...), PathNodes: make([]Node, 0, len(path)),
		Artifacts:      []RunArtifactReference{},
		OutputContract: RunOutputContract{Kind: kind, SchemaVersion: 1},
	}
	for i, id := range path {
		node, _ := g.Node(id)
		snapshot.PathNodes = append(snapshot.PathNodes, node.Clone())
		if i == len(path)-1 {
			continue
		}
		child, _ := g.Node(path[i+1])
		switch {
		case node.Type == valueobject.NodeTypeCandidate && node.CandidateSource != nil:
			index := node.CandidateSource.Index
			snapshot.Artifacts = append(snapshot.Artifacts, RunArtifactReference{
				NodeID: node.ID, ArtifactID: node.CandidateSource.ArtifactID,
				Kind: valueobject.OutputKindTopics, CandidateIndex: &index,
			})
		case node.Type == valueobject.NodeTypeScript && len(node.ScriptVersions) > 0:
			latest := node.ScriptVersions[len(node.ScriptVersions)-1]
			snapshot.Artifacts = append(snapshot.Artifacts, RunArtifactReference{
				NodeID: node.ID, ArtifactID: latest.ArtifactID,
				Kind: valueobject.OutputKindScript, Version: latest.N,
			})
		case g.project.Version < 2:
			for _, round := range node.Rounds {
				reference := RunArtifactReference{NodeID: node.ID, ArtifactID: round.ArtifactID, Kind: round.Kind, Version: round.N}
				if round.Kind == valueobject.OutputKindTopics && child.ParentCandidate != nil {
					index := *child.ParentCandidate
					reference.CandidateIndex = &index
				}
				snapshot.Artifacts = append(snapshot.Artifacts, reference)
			}
		}
	}
	if target.Type == valueobject.NodeTypeAgent {
		for _, childID := range g.Children(target.ID) {
			child, _ := g.Node(childID)
			if child.Type != valueobject.NodeTypeScript {
				continue
			}
			snapshot.OutputBinding = RunOutputBinding{Bound: true, NodeID: child.ID, ParentNodeID: target.ID}
			if len(child.ScriptVersions) > 0 {
				latest := child.ScriptVersions[len(child.ScriptVersions)-1]
				snapshot.OutputBinding.BaselineVersion = latest.N
				snapshot.OutputBinding.BaselineArtifactID = latest.ArtifactID
			}
			break
		}
	}
	return snapshot, nil
}

func (g *Graph) snapshotDAGForRun(nodeID string) (RunExecutionSnapshot, error) {
	target, ok := g.Node(nodeID)
	if !ok {
		return RunExecutionSnapshot{}, fmt.Errorf("%w: %s", ErrGraphNodeNotFound, nodeID)
	}
	path, err := g.PathToRoot(nodeID)
	if err != nil {
		return RunExecutionSnapshot{}, err
	}
	root, ok := g.Node(path[0])
	if !ok || root.Type != valueobject.NodeTypeTopic {
		return RunExecutionSnapshot{}, fmt.Errorf("%w: 无 Topic 的运行将在 U15 支持", ErrRunGraphUnsupported)
	}
	kind := valueobject.OutputKindTopics
	switch target.Type {
	case valueobject.NodeTypeTopic:
	case valueobject.NodeTypeAgent:
		kind = valueobject.OutputKindScript
	default:
		return RunExecutionSnapshot{}, fmt.Errorf("%w: %s", ErrNodeConnectionInvalid, target.Type)
	}

	snapshot := RunExecutionSnapshot{
		Version: 2, ProjectVersion: g.project.Version, ProjectID: g.project.ID,
		ProjectRevision: g.project.Revision, TreeID: root.ID, NodeID: target.ID,
		ChainPath: append([]string(nil), path...), PathNodes: make([]Node, 0, len(path)),
		PathEdges: make([]Edge, 0, len(path)-1), Artifacts: []RunArtifactReference{},
		OutputContract: RunOutputContract{Kind: kind, SchemaVersion: 1},
	}
	for i, id := range path {
		node, _ := g.Node(id)
		if i > 0 {
			parent, _ := g.Node(path[i-1])
			if err := ValidateNodeConnection(*parent, *node); err != nil {
				return RunExecutionSnapshot{}, fmt.Errorf("%w: %s → %s 不符合现有运行契约", ErrRunGraphUnsupported, parent.Title, node.Title)
			}
			snapshot.PathEdges = append(snapshot.PathEdges, g.incoming[id][0])
		}
		if node.WorkspaceRootID != "" && node.WorkspaceRootID != root.ID {
			return RunExecutionSnapshot{}, fmt.Errorf("%w: 节点 %s 的工作区与当前路径不一致", ErrWorkspaceRootMismatch, id)
		}
		snapshot.PathNodes = append(snapshot.PathNodes, node.Clone())
		if i == len(path)-1 {
			continue
		}
		switch {
		case node.Type == valueobject.NodeTypeCandidate && node.CandidateSource != nil:
			index := node.CandidateSource.Index
			snapshot.Artifacts = append(snapshot.Artifacts, RunArtifactReference{
				NodeID: node.ID, ArtifactID: node.CandidateSource.ArtifactID,
				Kind: valueobject.OutputKindTopics, CandidateIndex: &index,
			})
		case node.Type == valueobject.NodeTypeScript && len(node.ScriptVersions) > 0:
			latest := node.ScriptVersions[len(node.ScriptVersions)-1]
			snapshot.Artifacts = append(snapshot.Artifacts, RunArtifactReference{
				NodeID: node.ID, ArtifactID: latest.ArtifactID,
				Kind: valueobject.OutputKindScript, Version: latest.N,
			})
		}
	}
	if target.Type == valueobject.NodeTypeAgent {
		var output *Node
		for _, childID := range g.Children(target.ID) {
			child, _ := g.Node(childID)
			if child.Type != valueobject.NodeTypeScript {
				continue
			}
			if output != nil {
				return RunExecutionSnapshot{}, fmt.Errorf("%w: 存在多个 Script 输出，暂不能选择运行目标", ErrOutputBindingConflict)
			}
			output = child
		}
		if output != nil {
			if len(g.incoming[output.ID]) != 1 || (output.WorkspaceRootID != "" && output.WorkspaceRootID != root.ID) {
				return RunExecutionSnapshot{}, fmt.Errorf("%w: Script 输出有其他输入或工作区不匹配", ErrRunGraphUnsupported)
			}
			snapshot.OutputBinding = RunOutputBinding{Bound: true, NodeID: output.ID, ParentNodeID: target.ID}
			if len(output.ScriptVersions) > 0 {
				latest := output.ScriptVersions[len(output.ScriptVersions)-1]
				snapshot.OutputBinding.BaselineVersion = latest.N
				snapshot.OutputBinding.BaselineArtifactID = latest.ArtifactID
			}
		}
	}
	return snapshot, nil
}
