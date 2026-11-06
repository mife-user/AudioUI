package entity

import (
	"errors"
	"testing"

	"audio-ui/internal/domain/valueobject"
)

func graphProject() Project {
	return Project{
		Version: valueobject.ProjectFileVersion, ID: "p_test", Name: "测试", Revision: 1,
		ActiveNodeID: "n_a",
		Nodes: []Node{
			{ID: "n_topic", Type: valueobject.NodeTypeTopic, Title: "选题", Rounds: []Round{{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: "sha256:a"}}},
			{ID: "n_candidate", Type: valueobject.NodeTypeCandidate, Title: "候选一", Parent: "n_topic", CandidateSource: &CandidateSource{TopicNodeID: "n_topic", ArtifactID: "sha256:a", Index: 0}, Rounds: []Round{}},
			{ID: "n_a", Type: valueobject.NodeTypePrompt, Title: "A", Parent: "n_candidate", Rounds: []Round{}},
			{ID: "n_agent", Type: valueobject.NodeTypeAgent, Title: "Agent", Parent: "n_a", Rounds: []Round{}},
			{ID: "n_script", Type: valueobject.NodeTypeScript, Title: "脚本", Parent: "n_agent", Rounds: []Round{}},
			{ID: "n_b", Type: valueobject.NodeTypePrompt, Title: "B", Parent: "n_script", Rounds: []Round{}},
			{ID: "n_candidate2", Type: valueobject.NodeTypeCandidate, Title: "候选二", Parent: "n_topic", CandidateSource: &CandidateSource{TopicNodeID: "n_topic", ArtifactID: "sha256:a", Index: 1}, Rounds: []Round{}},
			{ID: "n_c", Type: valueobject.NodeTypePrompt, Title: "C", Parent: "n_candidate2", Rounds: []Round{}},
			{ID: "n_draft", Type: valueobject.NodeTypePrompt, Title: "草稿", Rounds: []Round{}},
		},
	}
}

func TestGraphValidate(t *testing.T) {
	project := graphProject()
	graph, err := NewGraph(&project)
	if err != nil {
		t.Fatal(err)
	}
	if err := graph.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestGraphRejectsDuplicateIDs(t *testing.T) {
	project := graphProject()
	project.Nodes = append(project.Nodes, project.Nodes[0])
	if _, err := NewGraph(&project); !errors.Is(err, ErrGraphInvalid) {
		t.Fatalf("error = %v, want ErrGraphInvalid", err)
	}
}

func TestGraphValidateInvariants(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Project)
	}{
		{"topic parent", func(p *Project) { p.Nodes[0].Parent = "n_a" }},
		{"missing parent", func(p *Project) { p.Nodes[2].Parent = "n_missing" }},
		{"invalid topic connection", func(p *Project) { p.Nodes[2].Parent = "n_topic" }},
		{"candidate source missing", func(p *Project) { p.Nodes[1].CandidateSource = nil }},
		{"candidate source mismatch", func(p *Project) { p.Nodes[1].CandidateSource.TopicNodeID = "n_candidate2" }},
		{"candidate artifact mismatch", func(p *Project) { p.Nodes[1].CandidateSource.ArtifactID = "sha256:b" }},
		{"candidate index negative", func(p *Project) { p.Nodes[1].CandidateSource.Index = -1 }},
		{"legacy candidate port", func(p *Project) { p.Nodes[2].ParentCandidate = intPtr(0) }},
		{"cycle", func(p *Project) { p.Nodes[1].Parent = "n_b" }},
		{"self parent", func(p *Project) { p.Nodes[2].Parent = "n_a" }},
		{"invalid type", func(p *Project) { p.Nodes[2].Type = "render" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			project := graphProject()
			tc.mutate(&project)
			graph, err := NewGraph(&project)
			if err != nil {
				t.Fatal(err)
			}
			if err := graph.Validate(); err == nil {
				t.Fatal("Validate() error = nil")
			}
		})
	}
}

func TestGraphRejectsMultipleScriptOutputs(t *testing.T) {
	project := graphProject()
	project.Nodes = append(project.Nodes, Node{
		ID: "n_script2", Type: valueobject.NodeTypeScript, Title: "第二脚本", Parent: "n_agent",
	})
	graph, err := NewGraph(&project)
	if err != nil {
		t.Fatal(err)
	}
	if err := graph.Validate(); !errors.Is(err, ErrOutputBindingConflict) {
		t.Fatalf("Validate() error = %v, want OUTPUT_BINDING_CONFLICT", err)
	}
}

func TestGraphPathsSubtreeAndChains(t *testing.T) {
	project := graphProject()
	graph, err := NewGraph(&project)
	if err != nil {
		t.Fatal(err)
	}
	path, err := graph.PathToRoot("n_b")
	if err != nil {
		t.Fatal(err)
	}
	if len(path) != 6 || path[0] != "n_topic" || path[1] != "n_candidate" || path[2] != "n_a" || path[3] != "n_agent" || path[4] != "n_script" || path[5] != "n_b" {
		t.Fatalf("path = %+v", path)
	}
	root, err := graph.Root("n_c")
	if err != nil || root != "n_topic" {
		t.Fatalf("root = %q err = %v", root, err)
	}
	if !graph.IsAncestor("n_topic", "n_b") || graph.IsAncestor("n_b", "n_topic") {
		t.Fatal("IsAncestor result wrong")
	}
	subtree, err := graph.Subtree("n_topic")
	if err != nil {
		t.Fatal(err)
	}
	if len(subtree) != 8 || subtree[0] != "n_topic" || subtree[1] != "n_candidate" || subtree[2] != "n_a" || subtree[3] != "n_agent" || subtree[4] != "n_script" || subtree[5] != "n_b" || subtree[6] != "n_candidate2" || subtree[7] != "n_c" {
		t.Fatalf("subtree = %+v", subtree)
	}
	chains := graph.LeafChains()
	if len(chains) != 2 || chains[0].ID != "c_n_b" || chains[1].ID != "c_n_c" {
		t.Fatalf("chains = %+v", chains)
	}
	if len(chains[0].Path) != 6 || chains[0].TailNodeID != "n_b" {
		t.Fatalf("chain = %+v", chains[0])
	}
}

func TestGraphMissingNode(t *testing.T) {
	project := graphProject()
	graph, err := NewGraph(&project)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := graph.Subtree("n_missing"); !errors.Is(err, ErrGraphNodeNotFound) {
		t.Fatalf("error = %v, want ErrGraphNodeNotFound", err)
	}
	if _, err := graph.PathToRoot("n_missing"); !errors.Is(err, ErrGraphNodeNotFound) {
		t.Fatalf("error = %v, want ErrGraphNodeNotFound", err)
	}
}

func intPtr(value int) *int { return &value }
