package entity

import (
	"errors"
	"testing"

	"audio-ui/internal/domain/valueobject"
)

func graphProject() Project {
	return Project{
		Version: 3, ID: "p_test", Name: "测试", Revision: 1,
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

func TestDefaultNodeSizes(t *testing.T) {
	cases := []struct {
		nodeType valueobject.NodeType
		want     NodeSize
	}{
		{valueobject.NodeTypeTopic, NodeSize{Width: 280, Height: 250}},
		{valueobject.NodeTypeCandidate, NodeSize{Width: 235, Height: 160}},
		{valueobject.NodeTypePrompt, NodeSize{Width: 280, Height: 230}},
		{valueobject.NodeTypeAgent, NodeSize{Width: 235, Height: 165}},
		{valueobject.NodeTypeScript, NodeSize{Width: 270, Height: 205}},
	}
	for _, tc := range cases {
		if got := DefaultNodeSize(tc.nodeType); got != tc.want || !got.Valid() {
			t.Errorf("DefaultNodeSize(%s) = %+v, want %+v", tc.nodeType, got, tc.want)
		}
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

func dagProject() Project {
	project := graphProject()
	project.Version = valueobject.ProjectFileVersion
	for i := range project.Nodes {
		node := &project.Nodes[i]
		if node.ID != "n_draft" {
			node.WorkspaceRootID = "n_topic"
		}
		if node.Parent == "" {
			continue
		}
		kind := EdgeKindConnection
		if node.Type == valueobject.NodeTypeCandidate {
			kind = EdgeKindCandidateSource
		}
		project.Edges = append(project.Edges, Edge{
			ID: "e_" + node.ID, SourceNodeID: node.Parent, TargetNodeID: node.ID, Kind: kind,
		})
		node.Parent = ""
	}
	return project
}

func TestDAGAcceptsMultipleInputsAndArbitraryNodeTypes(t *testing.T) {
	project := dagProject()
	project.Edges = append(project.Edges,
		Edge{ID: "e_merge", SourceNodeID: "n_candidate2", TargetNodeID: "n_b", Kind: EdgeKindConnection},
		Edge{ID: "e_same_type", SourceNodeID: "n_a", TargetNodeID: "n_c", Kind: EdgeKindConnection},
		Edge{ID: "e_topic_input", SourceNodeID: "n_draft", TargetNodeID: "n_topic", Kind: EdgeKindConnection},
	)
	graph, err := NewGraph(&project)
	if err != nil {
		t.Fatal(err)
	}
	if err := graph.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if parents := graph.Parents("n_b"); len(parents) != 2 || parents[0] != "n_script" || parents[1] != "n_candidate2" {
		t.Fatalf("parents = %v", parents)
	}
	if !graph.IsAncestor("n_candidate2", "n_b") || !graph.IsAncestor("n_a", "n_c") {
		t.Fatal("ancestor relationships missing")
	}
	descendants, err := graph.Subtree("n_topic")
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	for _, id := range descendants {
		if seen[id] {
			t.Fatalf("shared descendant %s visited twice", id)
		}
		seen[id] = true
	}
	if len(descendants) != 8 {
		t.Fatalf("descendants = %v", descendants)
	}
	chains := graph.LeafChains()
	if len(chains) != 2 || !chains[0].Ambiguous || len(chains[0].Path) != 0 || len(chains[0].NodeIDs) == 0 {
		t.Fatalf("chains = %+v", chains)
	}
}

func TestDAGRejectsInvalidEdgesAndCandidateSources(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Project)
	}{
		{"missing endpoint", func(p *Project) { p.Edges[0].SourceNodeID = "n_missing" }},
		{"self edge", func(p *Project) { p.Edges[1].SourceNodeID = p.Edges[1].TargetNodeID }},
		{"long cycle", func(p *Project) {
			p.Edges = append(p.Edges, Edge{ID: "e_cycle", SourceNodeID: "n_b", TargetNodeID: "n_topic", Kind: EdgeKindConnection})
		}},
		{"duplicate endpoints", func(p *Project) {
			duplicate := p.Edges[1]
			duplicate.ID = "e_duplicate"
			p.Edges = append(p.Edges, duplicate)
		}},
		{"duplicate edge id", func(p *Project) { p.Edges[1].ID = p.Edges[0].ID }},
		{"invalid edge kind", func(p *Project) { p.Edges[1].Kind = "unknown" }},
		{"missing source edge", func(p *Project) { p.Edges[0].Kind = EdgeKindConnection }},
		{"forged candidate source", func(p *Project) { p.Nodes[1].CandidateSource.TopicNodeID = "n_candidate2" }},
		{"legacy parent in v4", func(p *Project) { p.Nodes[2].Parent = "n_topic" }},
		{"invalid workspace root", func(p *Project) { p.Nodes[2].WorkspaceRootID = "n_a" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			project := dagProject()
			tc.mutate(&project)
			graph, err := NewGraph(&project)
			if err == nil {
				err = graph.Validate()
			}
			if err == nil {
				t.Fatal("expected graph rejection")
			}
		})
	}
}

func TestDAGRunSnapshotRejectsAmbiguousInputsAndOutputs(t *testing.T) {
	project := dagProject()
	graph, err := NewGraph(&project)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := graph.SnapshotForRun("n_agent")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Version != 2 || len(snapshot.PathEdges) != len(snapshot.ChainPath)-1 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("snapshot.Validate() = %v", err)
	}
	project.Edges = append(project.Edges, Edge{ID: "e_extra_input", SourceNodeID: "n_draft", TargetNodeID: "n_agent", Kind: EdgeKindConnection})
	project.Nodes[2].Params.Instruction = "主任务"
	project.Nodes[8].Params.Instruction = "附加任务"
	graph, _ = NewGraph(&project)
	if err := graph.Validate(); err != nil {
		t.Fatal(err)
	}
	snapshot, err = graph.SnapshotForRun("n_agent")
	if err != nil || snapshot.Version != 3 || len(snapshot.Inputs) != 2 {
		t.Fatalf("multi-input snapshot=%+v error=%v", snapshot, err)
	}
	project = dagProject()
	project.Nodes[2].Params.Instruction = "主任务"
	project.Nodes = append(project.Nodes, Node{ID: "n_script2", Type: valueobject.NodeTypeScript, WorkspaceRootID: "n_topic"})
	project.Edges = append(project.Edges, Edge{ID: "e_script2", SourceNodeID: "n_agent", TargetNodeID: "n_script2", Kind: EdgeKindConnection})
	graph, _ = NewGraph(&project)
	if err := graph.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := graph.SnapshotForRun("n_agent"); !errors.Is(err, ErrOutputBindingConflict) {
		t.Fatalf("multiple outputs error = %v", err)
	}
}

func TestDAGRunSnapshotFreezesSelectedInputOrderAndRoles(t *testing.T) {
	project := dagProject()
	project.Nodes[2].Params.Instruction = "主任务"
	project.Nodes = append(project.Nodes, Node{
		ID: "n_input_script", Type: valueobject.NodeTypeScript, WorkspaceRootID: "n_topic",
		ScriptVersions: []ScriptVersion{{N: 1, ArtifactID: "sha256:input", RunID: "r_seed"}},
	})
	project.Edges = append(project.Edges,
		Edge{ID: "e_script_input", SourceNodeID: "n_input_script", TargetNodeID: "n_agent", Kind: EdgeKindConnection},
	)
	graph, err := NewGraph(&project)
	if err != nil {
		t.Fatal(err)
	}
	if err := graph.Validate(); err != nil {
		t.Fatal(err)
	}

	selected, err := graph.SnapshotForRunWithInputs("n_agent", []string{"e_script_input", "e_n_agent"})
	if err != nil {
		t.Fatal(err)
	}
	if selected.Version != 3 || len(selected.ChainPath) != 0 || len(selected.Inputs) != 2 ||
		selected.Inputs[0] != (RunInput{EdgeID: "e_script_input", SourceNodeID: "n_input_script", Role: "artifact", Order: 0}) ||
		selected.Inputs[1] != (RunInput{EdgeID: "e_n_agent", SourceNodeID: "n_a", Role: "task", Order: 1}) {
		t.Fatalf("selected inputs = %+v", selected.Inputs)
	}
	if err := selected.Validate(); err != nil {
		t.Fatalf("selected.Validate() = %v", err)
	}

	withoutScript, err := graph.SnapshotForRunWithInputs("n_agent", []string{"e_n_agent"})
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range withoutScript.InputNodes {
		if node.ID == "n_input_script" {
			t.Fatal("deselected Script was included in the frozen input subgraph")
		}
	}
	if _, err := graph.SnapshotForRunWithInputs("n_agent", []string{"e_script_input"}); !errors.Is(err, ErrRunGraphUnsupported) {
		t.Fatalf("snapshot without a Prompt task error = %v", err)
	}
}

func TestDAGRunSnapshotIsolatesDifferentWorkspaceRoot(t *testing.T) {
	project := dagProject()
	project.Nodes = append(project.Nodes, Node{ID: "n_topic2", Type: valueobject.NodeTypeTopic, WorkspaceRootID: "n_topic2"})
	project.Nodes[2].Params.Instruction = "延续已选输入"
	project.Nodes[2].WorkspaceRootID = "n_topic2"
	graph, _ := NewGraph(&project)
	if err := graph.Validate(); err != nil {
		t.Fatal(err)
	}
	snapshot, err := graph.SnapshotForRun("n_agent")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Version != 3 || len(snapshot.ChainPath) != 0 || len(snapshot.InputRoots) != 1 || snapshot.InputRoots[0].ID != "n_topic2" {
		t.Fatalf("DAG snapshot did not preserve the isolated source root: %+v", snapshot)
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("snapshot.Validate() = %v", err)
	}
}
