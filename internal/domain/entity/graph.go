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
