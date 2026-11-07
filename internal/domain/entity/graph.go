package entity

import (
	"errors"
	"fmt"

	"audio-ui/internal/domain/valueobject"
)

var (
	ErrGraphInvalid          = errors.New("GRAPH_INVALID")
	ErrGraphNodeNotFound     = errors.New("GRAPH_NODE_NOT_FOUND")
	ErrNodeConnectionInvalid = errors.New("NODE_CONNECTION_INVALID")
	ErrOutputBindingConflict = errors.New("OUTPUT_BINDING_CONFLICT")
)

// Graph is a read-only view over a project's nodes. Edges are derived from
// Parent; the graph never persists an edge list.
type Graph struct {
	project  *Project
	nodes    map[string]*Node
	children map[string][]string
}

func NewGraph(project *Project) (*Graph, error) {
	if project == nil {
		return nil, fmt.Errorf("%w: nil project", ErrGraphInvalid)
	}
	graph := &Graph{
		project:  project,
		nodes:    make(map[string]*Node, len(project.Nodes)),
		children: make(map[string][]string, len(project.Nodes)),
	}
	for i := range project.Nodes {
		node := &project.Nodes[i]
		if _, exists := graph.nodes[node.ID]; exists {
			return nil, fmt.Errorf("%w: duplicate node id %q", ErrGraphInvalid, node.ID)
		}
		graph.nodes[node.ID] = node
	}
	for i := range project.Nodes {
		node := &project.Nodes[i]
		if node.Parent != "" {
			graph.children[node.Parent] = append(graph.children[node.Parent], node.ID)
		}
	}
	return graph, nil
}

func (g *Graph) Node(id string) (*Node, bool) {
	node, ok := g.nodes[id]
	return node, ok
}

func (g *Graph) Parent(id string) (*Node, bool) {
	node, ok := g.nodes[id]
	if !ok || node.Parent == "" {
		return nil, false
	}
	parent, ok := g.nodes[node.Parent]
	return parent, ok
}

func (g *Graph) Children(id string) []string {
	return append([]string(nil), g.children[id]...)
}

func (g *Graph) SnapshotForRun(nodeID string) (RunExecutionSnapshot, error) {
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

// Subtree returns root followed by every descendant in project node order.
func (g *Graph) Subtree(root string) ([]string, error) {
	if _, ok := g.nodes[root]; !ok {
		return nil, fmt.Errorf("%w: %s", ErrGraphNodeNotFound, root)
	}
	visited := make(map[string]bool, len(g.nodes))
	result := make([]string, 0, len(g.nodes))
	var walk func(string) error
	walk = func(id string) error {
		if visited[id] {
			return fmt.Errorf("%w: cycle at %s", ErrGraphInvalid, id)
		}
		visited[id] = true
		result = append(result, id)
		for _, child := range g.children[id] {
			if err := walk(child); err != nil {
				return err
			}
		}
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
		parent := g.nodes[current].Parent
		if parent == "" {
			break
		}
		current = parent
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
	path, err := g.PathToRoot(node)
	if err != nil {
		return false
	}
	for _, id := range path[:len(path)-1] {
		if id == ancestor {
			return true
		}
	}
	return false
}

// Validate checks the version 2 node connection and provenance rules.
func (g *Graph) Validate() error {
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
func (g *Graph) LeafChains() []Chain {
	tails := make([]string, 0, len(g.nodes))
	for _, node := range g.project.Nodes {
		if len(g.children[node.ID]) == 0 {
			tails = append(tails, node.ID)
		}
	}
	chains := make([]Chain, 0, len(tails))
	for _, tail := range tails {
		path, err := g.PathToRoot(tail)
		if err != nil {
			continue
		}
		root, ok := g.nodes[path[0]]
		if !ok || root.Type != "topic" {
			continue
		}
		chains = append(chains, Chain{
			ID:         "c_" + tail,
			TailNodeID: tail,
			Path:       path,
		})
	}
	return chains
}

// Chain is a derived topic-root-to-leaf path.
type Chain struct {
	ID         string   `json:"id"`
	TailNodeID string   `json:"tailNodeId"`
	Path       []string `json:"path"`
}
