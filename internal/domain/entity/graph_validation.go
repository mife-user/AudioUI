package entity

import (
	"fmt"

	"audio-ui/internal/domain/valueobject"
)

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
