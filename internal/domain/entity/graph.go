package entity

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"audio-ui/internal/domain/valueobject"
)

var (
	ErrGraphInvalid          = errors.New("GRAPH_INVALID")
	ErrGraphNodeNotFound     = errors.New("GRAPH_NODE_NOT_FOUND")
	ErrNodeConnectionInvalid = errors.New("NODE_CONNECTION_INVALID")
	ErrOutputBindingConflict = errors.New("OUTPUT_BINDING_CONFLICT")
	ErrScriptVersionUnbound  = errors.New("SCRIPT_VERSION_UNBOUND")
	ErrRunGraphUnsupported   = errors.New("RUN_GRAPH_UNSUPPORTED")
	ErrPromptTaskEmpty       = errors.New("PROMPT_TASK_EMPTY")
	ErrWorkspaceRootMismatch = errors.New("WORKSPACE_ROOT_MISMATCH")
)

type Graph struct {
	project  *Project
	nodes    map[string]*Node
	children map[string][]string
	parents  map[string][]string
	incoming map[string][]Edge
	outgoing map[string][]Edge
	edges    map[string]Edge
}

func NewGraph(project *Project) (*Graph, error) {
	if project == nil {
		return nil, fmt.Errorf("%w: nil project", ErrGraphInvalid)
	}
	graph := &Graph{
		project:  project,
		nodes:    make(map[string]*Node, len(project.Nodes)),
		children: make(map[string][]string, len(project.Nodes)),
		parents:  make(map[string][]string, len(project.Nodes)),
		incoming: make(map[string][]Edge, len(project.Nodes)),
		outgoing: make(map[string][]Edge, len(project.Nodes)),
		edges:    make(map[string]Edge, len(project.Edges)),
	}
	for i := range project.Nodes {
		node := &project.Nodes[i]
		if _, exists := graph.nodes[node.ID]; exists {
			return nil, fmt.Errorf("%w: duplicate node id %q", ErrGraphInvalid, node.ID)
		}
		graph.nodes[node.ID] = node
	}
	edges := project.Edges
	if project.Version < valueobject.DAGProjectVersion {
		edges = make([]Edge, 0, len(project.Nodes))
		for _, node := range project.Nodes {
			if node.Parent == "" {
				continue
			}
			kind := EdgeKindConnection
			if node.Type == valueobject.NodeTypeCandidate && node.CandidateSource != nil && node.CandidateSource.TopicNodeID == node.Parent {
				kind = EdgeKindCandidateSource
			}
			edges = append(edges, Edge{ID: "e_" + node.ID, SourceNodeID: node.Parent, TargetNodeID: node.ID, Kind: kind})
		}
	}
	for _, edge := range edges {
		if _, exists := graph.edges[edge.ID]; exists {
			return nil, fmt.Errorf("%w: duplicate edge id %q", ErrGraphInvalid, edge.ID)
		}
		graph.edges[edge.ID] = edge
		graph.children[edge.SourceNodeID] = append(graph.children[edge.SourceNodeID], edge.TargetNodeID)
		graph.parents[edge.TargetNodeID] = append(graph.parents[edge.TargetNodeID], edge.SourceNodeID)
		graph.incoming[edge.TargetNodeID] = append(graph.incoming[edge.TargetNodeID], edge)
		graph.outgoing[edge.SourceNodeID] = append(graph.outgoing[edge.SourceNodeID], edge)
	}
	return graph, nil
}

func (g *Graph) Node(id string) (*Node, bool) {
	node, ok := g.nodes[id]
	return node, ok
}

func (g *Graph) Parent(id string) (*Node, bool) {
	parents := g.parents[id]
	if len(parents) != 1 {
		return nil, false
	}
	parent, ok := g.nodes[parents[0]]
	return parent, ok
}

func (g *Graph) Children(id string) []string {
	return append([]string(nil), g.children[id]...)
}

func (g *Graph) Parents(id string) []string {
	return append([]string(nil), g.parents[id]...)
}

func (g *Graph) Incoming(id string) []Edge {
	return append([]Edge(nil), g.incoming[id]...)
}

func (g *Graph) Outgoing(id string) []Edge {
	return append([]Edge(nil), g.outgoing[id]...)
}

func (g *Graph) Edge(id string) (Edge, bool) {
	edge, ok := g.edges[id]
	return edge, ok
}

func (g *Graph) EdgeBetween(sourceID, targetID string) (Edge, bool) {
	for _, edge := range g.incoming[targetID] {
		if edge.SourceNodeID == sourceID {
			return edge, true
		}
	}
	return Edge{}, false
}

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

// Subtree returns root followed by every descendant in project node order.
func (g *Graph) Subtree(root string) ([]string, error) {
	if _, ok := g.nodes[root]; !ok {
		return nil, fmt.Errorf("%w: %s", ErrGraphNodeNotFound, root)
	}
	visited := make(map[string]bool, len(g.nodes))
	active := make(map[string]bool, len(g.nodes))
	result := make([]string, 0, len(g.nodes))
	var walk func(string) error
	walk = func(id string) error {
		if active[id] {
			return fmt.Errorf("%w: cycle at %s", ErrGraphInvalid, id)
		}
		if visited[id] {
			return nil
		}
		visited[id] = true
		active[id] = true
		result = append(result, id)
		for _, child := range g.children[id] {
			if err := walk(child); err != nil {
				return err
			}
		}
		delete(active, id)
		return nil
	}
	if err := walk(root); err != nil {
		return nil, err
	}
	return result, nil
}

// PathToRoot returns the path from the topic root (or draft node) to node,
// inclusive. It detects cycles.
func (g *Graph) PathToRoot(id string) ([]string, error) {
	path := make([]string, 0, 8)
	visited := make(map[string]bool)
	current := id
	for {
		if _, ok := g.nodes[current]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrGraphNodeNotFound, current)
		}
		if visited[current] {
			return nil, fmt.Errorf("%w: cycle at %s", ErrGraphInvalid, current)
		}
		visited[current] = true
		path = append(path, current)
		parents := g.parents[current]
		if len(parents) == 0 {
			break
		}
		if len(parents) > 1 {
			return nil, fmt.Errorf("%w: 节点 %s 存在多个上游，运行将在 U15 支持", ErrRunGraphUnsupported, current)
		}
		current = parents[0]
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path, nil
}

func (g *Graph) Root(id string) (string, error) {
	path, err := g.PathToRoot(id)
	if err != nil {
		return "", err
	}
	return path[0], nil
}

func (g *Graph) IsAncestor(ancestor, node string) bool {
	visited := make(map[string]bool)
	pending := append([]string(nil), g.parents[node]...)
	for len(pending) > 0 {
		id := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if id == ancestor {
			return true
		}
		if visited[id] {
			continue
		}
		visited[id] = true
		pending = append(pending, g.parents[id]...)
	}
	return false
}

// Validate checks typed node connection and provenance rules.
func (g *Graph) Validate() error {
	if g.project.Version >= valueobject.DAGProjectVersion {
		return g.validateDAG()
	}
	if err := g.ValidateForest(); err != nil {
		return err
	}
	outputs := make(map[string]int)
	for _, node := range g.project.Nodes {
		if node.ParentCandidate != nil {
			return fmt.Errorf("%w: parentCandidate is not supported", ErrGraphInvalid)
		}
		if node.Type == valueobject.NodeTypeCandidate {
			if node.CandidateSource == nil || node.Parent == "" ||
				node.CandidateSource.TopicNodeID != node.Parent ||
				node.CandidateSource.ArtifactID == "" || node.CandidateSource.Index < 0 {
				return fmt.Errorf("%w: candidate %s has invalid source", ErrGraphInvalid, node.ID)
			}
			if err := valueobject.ValidateID(node.CandidateSource.TopicNodeID); err != nil {
				return fmt.Errorf("%w: candidate %s source topic: %v", ErrGraphInvalid, node.ID, err)
			}
			topic, _ := g.Node(node.Parent)
			latest := Round{}
			for _, round := range topic.Rounds {
				if round.N > latest.N {
					latest = round
				}
			}
			if latest.Kind != valueobject.OutputKindTopics || latest.ArtifactID != node.CandidateSource.ArtifactID {
				return fmt.Errorf("%w: candidate %s artifact is not the latest topic result", ErrGraphInvalid, node.ID)
			}
		} else if node.CandidateSource != nil {
			return fmt.Errorf("%w: non-candidate node %s has a candidate source", ErrGraphInvalid, node.ID)
		}
		if node.Type != valueobject.NodeTypeScript && len(node.ScriptVersions) != 0 {
			return fmt.Errorf("%w: non-script node %s has script versions", ErrGraphInvalid, node.ID)
		}
		for i, version := range node.ScriptVersions {
			if version.N != i+1 || version.ArtifactID == "" || version.RunID == "" {
				return fmt.Errorf("%w: script node %s has invalid version %d", ErrGraphInvalid, node.ID, i+1)
			}
		}
		if node.Parent == "" {
			if node.Type == valueobject.NodeTypeCandidate {
				return fmt.Errorf("%w: candidate %s must have a topic parent", ErrGraphInvalid, node.ID)
			}
			continue
		}
		parent, _ := g.Node(node.Parent)
		if err := ValidateNodeConnection(*parent, node); err != nil {
			return err
		}
		if node.Type == valueobject.NodeTypeScript {
			outputs[parent.ID]++
			if outputs[parent.ID] > 1 {
				return fmt.Errorf("%w: agent %s has multiple script outputs", ErrOutputBindingConflict, parent.ID)
			}
		}
	}
	return nil
}

func (g *Graph) validateDAG() error {
	for _, node := range g.project.Nodes {
		if err := valueobject.ValidateID(node.ID); err != nil {
			return fmt.Errorf("%w: node id: %v", ErrGraphInvalid, err)
		}
		if !node.Type.Valid() {
			return fmt.Errorf("%w: node %s has invalid type %q", ErrGraphInvalid, node.ID, node.Type)
		}
		if node.Parent != "" || node.ParentCandidate != nil {
			return fmt.Errorf("%w: node %s contains a legacy parent", ErrGraphInvalid, node.ID)
		}
		if node.WorkspaceRootID != "" {
			root, ok := g.Node(node.WorkspaceRootID)
			if !ok || root.Type != valueobject.NodeTypeTopic {
				return fmt.Errorf("%w: node %s has invalid workspace root", ErrGraphInvalid, node.ID)
			}
		}
		if node.Type == valueobject.NodeTypeCandidate {
			source := node.CandidateSource
			if source == nil || source.TopicNodeID == "" || source.ArtifactID == "" || source.Index < 0 {
				return fmt.Errorf("%w: candidate %s has invalid source", ErrGraphInvalid, node.ID)
			}
			topic, ok := g.Node(source.TopicNodeID)
			if !ok || topic.Type != valueobject.NodeTypeTopic {
				return fmt.Errorf("%w: candidate %s has invalid source topic", ErrGraphInvalid, node.ID)
			}
			if g.project.Version >= valueobject.ProjectFileVersion {
				if source.Round <= 0 || source.RunID == "" {
					return fmt.Errorf("%w: candidate %s has incomplete batch source", ErrGraphInvalid, node.ID)
				}
				matched := false
				for _, round := range topic.Rounds {
					if round.N == source.Round && round.Kind == valueobject.OutputKindTopics &&
						round.ArtifactID == source.ArtifactID && round.RunID == source.RunID {
						matched = true
						break
					}
				}
				if !matched {
					return fmt.Errorf("%w: candidate %s source does not match a topic batch", ErrGraphInvalid, node.ID)
				}
			} else {
				latest := Round{}
				for _, round := range topic.Rounds {
					if round.N > latest.N {
						latest = round
					}
				}
				if latest.Kind != valueobject.OutputKindTopics || latest.ArtifactID != source.ArtifactID {
					return fmt.Errorf("%w: candidate %s artifact is not the topic result", ErrGraphInvalid, node.ID)
				}
			}
		} else if node.CandidateSource != nil {
			return fmt.Errorf("%w: non-candidate node %s has a candidate source", ErrGraphInvalid, node.ID)
		}
		if node.Type != valueobject.NodeTypeScript && len(node.ScriptVersions) != 0 {
			return fmt.Errorf("%w: non-script node %s has script versions", ErrGraphInvalid, node.ID)
		}
		for i, version := range node.ScriptVersions {
			if version.N != i+1 || version.ArtifactID == "" || version.RunID == "" {
				return fmt.Errorf("%w: script node %s has invalid version %d", ErrGraphInvalid, node.ID, i+1)
			}
		}
	}

	type endpoints struct{ source, target string }
	seenPairs := make(map[endpoints]bool, len(g.project.Edges))
	candidateSources := make(map[string]int)
	indegree := make(map[string]int, len(g.nodes))
	for _, edge := range g.project.Edges {
		if err := valueobject.ValidateID(edge.ID); err != nil {
			return fmt.Errorf("%w: edge id: %v", ErrGraphInvalid, err)
		}
		source, sourceOK := g.Node(edge.SourceNodeID)
		target, targetOK := g.Node(edge.TargetNodeID)
		if !sourceOK || !targetOK {
			return fmt.Errorf("%w: edge %s has a missing endpoint", ErrGraphInvalid, edge.ID)
		}
		if source.ID == target.ID {
			return fmt.Errorf("%w: edge %s is a self connection", ErrGraphInvalid, edge.ID)
		}
		pair := endpoints{source.ID, target.ID}
		if seenPairs[pair] {
			return fmt.Errorf("%w: duplicate edge %s → %s", ErrGraphInvalid, source.ID, target.ID)
		}
		seenPairs[pair] = true
		if g.project.Version >= valueobject.ProjectFileVersion {
			if source.Type == valueobject.NodeTypeScript {
				if edge.Kind != EdgeKindConnection || edge.ScriptVersion < 0 || edge.ScriptVersion > len(source.ScriptVersions) {
					return fmt.Errorf("%w: edge %s has an invalid Script version", ErrGraphInvalid, edge.ID)
				}
			} else if edge.ScriptVersion != 0 {
				return fmt.Errorf("%w: non-Script edge %s has a Script version", ErrGraphInvalid, edge.ID)
			}
		} else if edge.ScriptVersion != 0 {
			return fmt.Errorf("%w: legacy edge %s has a Script version", ErrGraphInvalid, edge.ID)
		}
		switch edge.Kind {
		case EdgeKindConnection:
		case EdgeKindCandidateSource:
			if source.Type != valueobject.NodeTypeTopic || target.Type != valueobject.NodeTypeCandidate ||
				target.CandidateSource == nil || target.CandidateSource.TopicNodeID != source.ID {
				return fmt.Errorf("%w: edge %s has invalid candidate source", ErrGraphInvalid, edge.ID)
			}
			candidateSources[target.ID]++
		default:
			return fmt.Errorf("%w: edge %s has invalid kind %q", ErrGraphInvalid, edge.ID, edge.Kind)
		}
		indegree[target.ID]++
	}
	for _, node := range g.project.Nodes {
		if node.Type == valueobject.NodeTypeCandidate && candidateSources[node.ID] != 1 {
			return fmt.Errorf("%w: candidate %s must have exactly one source edge", ErrGraphInvalid, node.ID)
		}
	}
	queue := make([]string, 0, len(g.nodes))
	for _, node := range g.project.Nodes {
		if indegree[node.ID] == 0 {
			queue = append(queue, node.ID)
		}
	}
	visited := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		visited++
		for _, childID := range g.children[id] {
			indegree[childID]--
			if indegree[childID] == 0 {
				queue = append(queue, childID)
			}
		}
	}
	if visited != len(g.nodes) {
		return fmt.Errorf("%w: graph contains a cycle", ErrGraphInvalid)
	}
	return nil
}

// ValidateForest checks that nodes form a single-parent forest rooted at topics
// or unconnected drafts.
func (g *Graph) ValidateForest() error {
	for _, node := range g.project.Nodes {
		if node.ID == "" {
			return fmt.Errorf("%w: empty node id", ErrGraphInvalid)
		}
		if !node.Type.Valid() {
			return fmt.Errorf("%w: node %s has invalid type %q", ErrGraphInvalid, node.ID, node.Type)
		}
		if node.Parent != "" {
			parent, ok := g.nodes[node.Parent]
			if !ok {
				return fmt.Errorf("%w: node %s has missing parent %s", ErrGraphInvalid, node.ID, node.Parent)
			}
			if node.Type == valueobject.NodeTypeTopic {
				return fmt.Errorf("%w: topic %s must not have a parent", ErrGraphInvalid, node.ID)
			}
			if parent.Type == valueobject.NodeTypeTopic && node.ParentCandidate != nil && *node.ParentCandidate < 0 {
				return fmt.Errorf("%w: node %s has negative candidate", ErrGraphInvalid, node.ID)
			}
		}
	}
	for _, node := range g.project.Nodes {
		path, err := g.PathToRoot(node.ID)
		if err != nil {
			return err
		}
		root, ok := g.nodes[path[0]]
		if !ok {
			return fmt.Errorf("%w: missing root", ErrGraphInvalid)
		}
		if node.Parent != "" && root.Type != valueobject.NodeTypeTopic {
			return fmt.Errorf("%w: node %s is not rooted at a topic", ErrGraphInvalid, node.ID)
		}
	}
	return nil
}

func ValidateNodeConnection(parent, child Node) error {
	valid := false
	switch {
	case parent.Type == valueobject.NodeTypeTopic && child.Type == valueobject.NodeTypeCandidate:
		valid = true
	case parent.Type == valueobject.NodeTypeCandidate && child.Type == valueobject.NodeTypePrompt:
		valid = true
	case parent.Type == valueobject.NodeTypePrompt && child.Type == valueobject.NodeTypeAgent:
		valid = true
	case parent.Type == valueobject.NodeTypeAgent && child.Type == valueobject.NodeTypeScript:
		valid = true
	case parent.Type == valueobject.NodeTypeScript && child.Type == valueobject.NodeTypePrompt:
		valid = true
	}
	if !valid {
		return fmt.Errorf("%w: %s cannot be parent of %s", ErrNodeConnectionInvalid, parent.Type, child.Type)
	}
	if child.Type == valueobject.NodeTypeCandidate &&
		(child.CandidateSource == nil || child.CandidateSource.TopicNodeID != parent.ID) {
		return fmt.Errorf("%w: candidate source does not match topic %s", ErrNodeConnectionInvalid, parent.ID)
	}
	return nil
}

// LeafChains returns derived leaf paths in deterministic node order. Chains
// are never persisted.
func (g *Graph) AncestorNodeIDs(id string) []string {
	visited := make(map[string]bool)
	pending := []string{id}
	for len(pending) > 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if visited[current] {
			continue
		}
		visited[current] = true
		pending = append(pending, g.parents[current]...)
	}
	result := make([]string, 0, len(visited))
	for _, node := range g.project.Nodes {
		if visited[node.ID] {
			result = append(result, node.ID)
		}
	}
	return result
}

func (g *Graph) LeafChains() []Chain {
	chains := make([]Chain, 0, len(g.nodes))
	for _, node := range g.project.Nodes {
		if len(g.children[node.ID]) != 0 {
			continue
		}
		path, err := g.PathToRoot(node.ID)
		if err != nil {
			if errors.Is(err, ErrRunGraphUnsupported) {
				chains = append(chains, Chain{
					ID: "c_" + node.ID, TailNodeID: node.ID,
					NodeIDs: g.AncestorNodeIDs(node.ID), Ambiguous: true,
				})
			}
			continue
		}
		root, ok := g.nodes[path[0]]
		if !ok || len(path) == 1 && root.Type != valueobject.NodeTypeTopic {
			continue
		}
		chains = append(chains, Chain{
			ID: "c_" + node.ID, TailNodeID: node.ID,
			Path: path, NodeIDs: append([]string(nil), path...),
		})
	}
	return chains
}

type Chain struct {
	ID         string   `json:"id"`
	TailNodeID string   `json:"tailNodeId"`
	Path       []string `json:"path"`
	NodeIDs    []string `json:"nodeIds"`
	Ambiguous  bool     `json:"ambiguous"`
}
