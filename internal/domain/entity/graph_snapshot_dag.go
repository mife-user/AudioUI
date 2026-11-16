package entity

import (
	"fmt"
	"sort"
	"strings"

	"audio-ui/internal/domain/valueobject"
)

func (g *Graph) snapshotDAGInputsForRun(nodeID string, inputEdgeIDs []string) (RunExecutionSnapshot, error) {
	return g.snapshotDAGInputsForRunWithOptions(nodeID, RunStartOptions{InputEdgeIDs: inputEdgeIDs})
}

func (g *Graph) snapshotDAGInputsForRunWithOptions(nodeID string, options RunStartOptions) (RunExecutionSnapshot, error) {
	inputEdgeIDs := options.InputEdgeIDs
	target, ok := g.Node(nodeID)
	if !ok {
		return RunExecutionSnapshot{}, fmt.Errorf("%w: %s", ErrGraphNodeNotFound, nodeID)
	}
	if target.Type != valueobject.NodeTypeAgent {
		return RunExecutionSnapshot{}, fmt.Errorf("%w: DAG input runs require an Agent target", ErrRunGraphUnsupported)
	}
	if options.TopicRerun {
		return RunExecutionSnapshot{}, fmt.Errorf("%w: Topic rerun option requires a Topic target", ErrRunGraphUnsupported)
	}
	if inputEdgeIDs == nil {
		inputEdgeIDs = make([]string, 0, len(g.incoming[nodeID]))
		for _, edge := range g.incoming[nodeID] {
			if edge.Kind == EdgeKindConnection {
				inputEdgeIDs = append(inputEdgeIDs, edge.ID)
			}
		}
	}
	inputs := make([]RunInput, 0, len(inputEdgeIDs))
	selectedEdges := make(map[string]Edge, len(inputEdgeIDs))
	tasks := 0
	for order, edgeID := range inputEdgeIDs {
		edge, exists := g.Edge(edgeID)
		if !exists || edge.Kind != EdgeKindConnection || edge.TargetNodeID != nodeID {
			return RunExecutionSnapshot{}, fmt.Errorf("%w: selected input edge %s does not target Agent", ErrRunGraphUnsupported, edgeID)
		}
		if _, duplicate := selectedEdges[edgeID]; duplicate {
			return RunExecutionSnapshot{}, fmt.Errorf("%w: duplicate selected input %s", ErrRunGraphUnsupported, edgeID)
		}
		source, exists := g.Node(edge.SourceNodeID)
		if !exists {
			return RunExecutionSnapshot{}, fmt.Errorf("%w: input source not found", ErrGraphInvalid)
		}
		role := InputRoleForNodeType(source.Type)
		if role == "" {
			return RunExecutionSnapshot{}, fmt.Errorf("%w: %s cannot be an Agent input", ErrRunGraphUnsupported, source.Type)
		}
		if role == "task" {
			if strings.TrimSpace(source.Params.Instruction) == "" {
				return RunExecutionSnapshot{}, fmt.Errorf("%w: Prompt %s has no task", ErrPromptTaskEmpty, source.ID)
			}
			tasks++
		}
		selectedEdges[edgeID] = edge
		inputs = append(inputs, RunInput{EdgeID: edge.ID, SourceNodeID: source.ID, Role: role, Order: order})
	}
	if tasks == 0 {
		return RunExecutionSnapshot{}, fmt.Errorf("%w: Agent run requires a connected Prompt task", ErrRunGraphUnsupported)
	}

	included := map[string]struct{}{nodeID: {}}
	inputEdgeSet := make(map[string]Edge)
	for _, edge := range selectedEdges {
		inputEdgeSet[edge.ID] = edge
	}
	processed := make(map[string]struct{})
	var includeAncestors func(string, bool) error
	includeAncestors = func(id string, includeIncoming bool) error {
		included[id] = struct{}{}
		if !includeIncoming {
			return nil
		}
		if _, exists := processed[id]; exists {
			return nil
		}
		processed[id] = struct{}{}
		for _, edge := range g.incoming[id] {
			if id == nodeID {
				if _, selected := selectedEdges[edge.ID]; !selected {
					continue
				}
			} else if edge.Kind == EdgeKindConnection {
				source, _ := g.Node(edge.SourceNodeID)
				if source.Type == valueobject.NodeTypeTopic {
					return fmt.Errorf("%w: Topic inputs must use a generated Candidate", ErrRunGraphUnsupported)
				}
			}
			inputEdgeSet[edge.ID] = edge
			if edge.Kind == EdgeKindCandidateSource {
				included[edge.SourceNodeID] = struct{}{}
				continue
			}
			if err := includeAncestors(edge.SourceNodeID, true); err != nil {
				return err
			}
		}
		return nil
	}
	for _, input := range inputs {
		if err := includeAncestors(input.SourceNodeID, true); err != nil {
			return RunExecutionSnapshot{}, err
		}
	}
	workspaceRoots := make(map[string]struct{})
	for id := range included {
		node, _ := g.Node(id)
		if node.WorkspaceRootID == "" {
			continue
		}
		if _, exists := included[node.WorkspaceRootID]; !exists {
			workspaceRoots[node.WorkspaceRootID] = struct{}{}
		}
	}
	inputRoots := make([]Node, 0, len(workspaceRoots))
	for _, projectNode := range g.project.Nodes {
		if _, needed := workspaceRoots[projectNode.ID]; needed {
			if projectNode.Type != valueobject.NodeTypeTopic {
				return RunExecutionSnapshot{}, fmt.Errorf("%w: invalid workspace root %s", ErrWorkspaceRootMismatch, projectNode.ID)
			}
			inputRoots = append(inputRoots, projectNode.Clone())
		}
	}
	if len(inputRoots) != len(workspaceRoots) {
		return RunExecutionSnapshot{}, fmt.Errorf("%w: missing input workspace root", ErrWorkspaceRootMismatch)
	}
	inputNodes, err := g.topologicalSubset(included, inputEdgeSet)
	if err != nil {
		return RunExecutionSnapshot{}, err
	}
	inputEdges := make([]Edge, 0, len(inputEdgeSet))
	for _, edge := range g.project.Edges {
		if _, exists := inputEdgeSet[edge.ID]; exists {
			inputEdges = append(inputEdges, edge)
		}
	}
	if g.project.Version >= valueobject.ProjectFileVersion {
		for _, edge := range inputEdges {
			source, _ := g.Node(edge.SourceNodeID)
			if source.Type != valueobject.NodeTypeScript {
				continue
			}
			if edge.ScriptVersion <= 0 {
				return RunExecutionSnapshot{}, fmt.Errorf("%w: Script %s 的输入连接尚未绑定版本", ErrScriptVersionUnbound, source.ID)
			}
			if edge.ScriptVersion > len(source.ScriptVersions) {
				return RunExecutionSnapshot{}, fmt.Errorf("%w: Script %s 不存在 v%d", ErrRunGraphUnsupported, source.ID, edge.ScriptVersion)
			}
		}
	}

	kind := valueobject.OutputKindScript
	outputBinding := RunOutputBinding{}
	var output *Node
	var outputNode *Node
	var outputEdge *Edge
	if g.project.Version >= valueobject.ProjectFileVersion {
		var scriptChildren []*Node
		for _, childID := range g.children[nodeID] {
			child, _ := g.Node(childID)
			if child.Type == valueobject.NodeTypeScript {
				scriptChildren = append(scriptChildren, child)
			}
		}
		if options.CreateOutput && options.OutputNodeID != "" {
			return RunExecutionSnapshot{}, fmt.Errorf("%w: output selection is ambiguous", ErrOutputBindingConflict)
		}
		mode := ""
		switch {
		case options.CreateOutput:
			mode = RunOutputModeCreate
		case options.OutputNodeID != "":
			mode = RunOutputModeExisting
		case len(scriptChildren) == 0:
			mode = RunOutputModeCreate
		case len(scriptChildren) == 1:
			mode = RunOutputModeExisting
		default:
			return RunExecutionSnapshot{}, fmt.Errorf("%w: 多个 Script 输出必须选择运行目标", ErrOutputBindingConflict)
		}
		if mode == RunOutputModeCreate {
			outputID, err := valueobject.NewID(valueobject.NodeIDPrefix)
			if err != nil {
				return RunExecutionSnapshot{}, err
			}
			edgeID, err := valueobject.NewID(valueobject.EdgeIDPrefix)
			if err != nil {
				return RunExecutionSnapshot{}, err
			}
			outputBinding = RunOutputBinding{Bound: true, Mode: mode, NodeID: outputID, ParentNodeID: target.ID}
			outputEdge = &Edge{ID: edgeID, SourceNodeID: target.ID, TargetNodeID: outputID, Kind: EdgeKindConnection}
		} else {
			for _, child := range scriptChildren {
				if options.OutputNodeID == "" || child.ID == options.OutputNodeID {
					output = child
					break
				}
			}
			if output == nil || len(g.incoming[output.ID]) != 1 || output.WorkspaceRootID != target.WorkspaceRootID {
				return RunExecutionSnapshot{}, fmt.Errorf("%w: Script 输出不存在、存在其他输入或工作区不匹配", ErrRunGraphUnsupported)
			}
			edge, ok := g.EdgeBetween(target.ID, output.ID)
			if !ok {
				return RunExecutionSnapshot{}, fmt.Errorf("%w: Script 输出连接不存在", ErrGraphInvalid)
			}
			outputBinding = RunOutputBinding{Bound: true, Mode: mode, NodeID: output.ID, ParentNodeID: target.ID}
			if len(output.ScriptVersions) > 0 {
				latest := output.ScriptVersions[len(output.ScriptVersions)-1]
				outputBinding.BaselineVersion = latest.N
				outputBinding.BaselineArtifactID = latest.ArtifactID
			}
			frozen := output.Clone()
			outputNode = &frozen
			outputEdge = &edge
		}
	} else {
		for _, childID := range g.children[nodeID] {
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
			if len(g.incoming[output.ID]) != 1 || output.WorkspaceRootID != target.WorkspaceRootID {
				return RunExecutionSnapshot{}, fmt.Errorf("%w: Script 输出有其他输入或工作区不匹配", ErrRunGraphUnsupported)
			}
			outputBinding = RunOutputBinding{Bound: true, NodeID: output.ID, ParentNodeID: target.ID}
			if len(output.ScriptVersions) > 0 {
				latest := output.ScriptVersions[len(output.ScriptVersions)-1]
				outputBinding.BaselineVersion = latest.N
				outputBinding.BaselineArtifactID = latest.ArtifactID
			}
		}
	}

	snapshotVersion := 3
	if g.project.Version >= valueobject.ProjectFileVersion {
		snapshotVersion = 4
	}
	snapshot := RunExecutionSnapshot{
		Version: snapshotVersion, ProjectVersion: g.project.Version, ProjectID: g.project.ID,
		ProjectRevision: g.project.Revision, TreeID: target.WorkspaceRootID, NodeID: target.ID,
		InputNodes: inputNodes, InputRoots: inputRoots, InputEdges: inputEdges, Inputs: inputs,
		Artifacts: []RunArtifactReference{}, OutputBinding: outputBinding,
		OutputNode: outputNode, OutputEdge: outputEdge,
		OutputContract: RunOutputContract{Kind: kind, SchemaVersion: 1},
	}
	for _, node := range inputNodes[:len(inputNodes)-1] {
		switch {
		case node.Type == valueobject.NodeTypeCandidate && node.CandidateSource != nil:
			index := node.CandidateSource.Index
			snapshot.Artifacts = append(snapshot.Artifacts, RunArtifactReference{
				NodeID: node.ID, ArtifactID: node.CandidateSource.ArtifactID,
				Kind: valueobject.OutputKindTopics, CandidateIndex: &index,
			})
		case node.Type == valueobject.NodeTypeScript && len(node.ScriptVersions) > 0:
			if snapshot.Version == 4 {
				for _, edge := range inputEdges {
					if edge.SourceNodeID != node.ID {
						continue
					}
					version := node.ScriptVersions[edge.ScriptVersion-1]
					snapshot.Artifacts = append(snapshot.Artifacts, RunArtifactReference{
						NodeID: node.ID, EdgeID: edge.ID, ArtifactID: version.ArtifactID,
						Kind: valueobject.OutputKindScript, Version: version.N,
					})
				}
			} else {
				latest := node.ScriptVersions[len(node.ScriptVersions)-1]
				snapshot.Artifacts = append(snapshot.Artifacts, RunArtifactReference{
					NodeID: node.ID, ArtifactID: latest.ArtifactID,
					Kind: valueobject.OutputKindScript, Version: latest.N,
				})
			}
		}
	}
	if err := snapshot.Validate(); err != nil {
		return RunExecutionSnapshot{}, err
	}
	return snapshot, nil
}

func (g *Graph) topologicalSubset(nodes map[string]struct{}, edges map[string]Edge) ([]Node, error) {
	order := make(map[string]int, len(g.project.Nodes))
	for i, node := range g.project.Nodes {
		order[node.ID] = i
	}
	indegree := make(map[string]int, len(nodes))
	children := make(map[string][]string, len(nodes))
	for _, edge := range edges {
		indegree[edge.TargetNodeID]++
		children[edge.SourceNodeID] = append(children[edge.SourceNodeID], edge.TargetNodeID)
	}
	ready := make([]string, 0, len(nodes))
	for _, node := range g.project.Nodes {
		if _, exists := nodes[node.ID]; exists && indegree[node.ID] == 0 {
			ready = append(ready, node.ID)
		}
	}
	result := make([]Node, 0, len(nodes))
	for len(ready) > 0 {
		sort.Slice(ready, func(i, j int) bool { return order[ready[i]] < order[ready[j]] })
		id := ready[0]
		ready = ready[1:]
		node, _ := g.Node(id)
		result = append(result, node.Clone())
		for _, childID := range children[id] {
			indegree[childID]--
			if indegree[childID] == 0 {
				ready = append(ready, childID)
			}
		}
	}
	if len(result) != len(nodes) {
		return nil, fmt.Errorf("%w: input graph contains a cycle", ErrGraphInvalid)
	}
	return result, nil
}
