package entity

import (
	"errors"
	"testing"

	"audio-ui/internal/domain/valueobject"
)

func graphProject() Project {
	zero := 0
	return Project{
		Version: valueobject.ProjectFileVersion, ID: "p_test", Name: "测试", Revision: 1,
		ActiveNodeID: "n_a",
		Nodes: []Node{
			{ID: "n_topic", Type: valueobject.NodeTypeTopic, Title: "选题", Rounds: []Round{}},
			{ID: "n_a", Type: valueobject.NodeTypePrompt, Title: "A", Parent: "n_topic", ParentCandidate: &zero, Rounds: []Round{}},
			{ID: "n_b", Type: valueobject.NodeTypePrompt, Title: "B", Parent: "n_a", Rounds: []Round{}},
			{ID: "n_c", Type: valueobject.NodeTypePrompt, Title: "C", Parent: "n_topic", ParentCandidate: &zero, Rounds: []Round{}},
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
		{"missing parent", func(p *Project) { p.Nodes[1].Parent = "n_missing" }},
		{"topic parent candidate missing", func(p *Project) { p.Nodes[1].ParentCandidate = nil }},
		{"prompt parent candidate present", func(p *Project) { p.Nodes[2].ParentCandidate = intPtr(0) }},
		{"prompt root has child", func(p *Project) { p.Nodes[3].Parent = "n_draft"; p.Nodes[3].ParentCandidate = nil }},
		{"cycle", func(p *Project) { p.Nodes[1].Parent = "n_b"; p.Nodes[1].ParentCandidate = nil }},
		{"self parent", func(p *Project) { p.Nodes[1].Parent = "n_a"; p.Nodes[1].ParentCandidate = nil }},
		{"invalid type", func(p *Project) { p.Nodes[1].Type = "render" }},
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
	if len(path) != 3 || path[0] != "n_topic" || path[1] != "n_a" || path[2] != "n_b" {
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
	if len(subtree) != 4 || subtree[0] != "n_topic" || subtree[1] != "n_a" || subtree[2] != "n_b" || subtree[3] != "n_c" {
		t.Fatalf("subtree = %+v", subtree)
	}
	chains := graph.LeafChains()
	if len(chains) != 2 || chains[0].ID != "c_n_b" || chains[1].ID != "c_n_c" {
		t.Fatalf("chains = %+v", chains)
	}
	if len(chains[0].Path) != 3 || chains[0].TailNodeID != "n_b" {
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
