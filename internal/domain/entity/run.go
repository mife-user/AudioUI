package entity

import (
	"fmt"
	"strings"

	"audio-ui/internal/domain/valueobject"
)

// RunSummary is the runs.json index entry. Transcript and context preview
// live in runs/<runId>.json so listing runs stays cheap.
type RunSummary struct {
	ID                string                `json:"id"`
	TreeID            string                `json:"treeId"`
	NodeID            string                `json:"nodeId"`
	ChainID           string                `json:"chainId"`
	ChainPath         []string              `json:"chainPath"`
	Status            valueobject.RunStatus `json:"status"`
	ContextVersion    int                   `json:"contextVersion"`
	EstimatorVersion  int                   `json:"estimatorVersion,omitempty"`
	ContextHash       string                `json:"contextHash,omitempty"`
	EstimatedTokens   int                   `json:"estimatedTokens,omitempty"`
	TokenBudget       int                   `json:"tokenBudget,omitempty"`
	ActualInputTokens int                   `json:"actualInputTokens,omitempty"`
	UsageReported     bool                  `json:"usageReported,omitempty"`
	OverBudget        bool                  `json:"overBudget,omitempty"`
	Error             string                `json:"error,omitempty"`
	PendingID         string                `json:"pendingId,omitempty"`
	StartedAt         string                `json:"startedAt,omitempty"`
	FinishedAt        string                `json:"finishedAt,omitempty"`
}

type RunArtifactReference struct {
	NodeID         string                 `json:"nodeId"`
	ArtifactID     string                 `json:"artifactId"`
	Kind           valueobject.OutputKind `json:"kind"`
	CandidateIndex *int                   `json:"candidateIndex,omitempty"`
	Version        int                    `json:"version,omitempty"`
}

type RunOutputBinding struct {
	Bound              bool   `json:"bound"`
	NodeID             string `json:"nodeId,omitempty"`
	ParentNodeID       string `json:"parentNodeId,omitempty"`
	BaselineVersion    int    `json:"baselineVersion"`
	BaselineArtifactID string `json:"baselineArtifactId,omitempty"`
}

type RunOutputContract struct {
	Kind          valueobject.OutputKind `json:"kind"`
	SchemaVersion int                    `json:"schemaVersion"`
}

type RunInput struct {
	EdgeID       string `json:"edgeId"`
	SourceNodeID string `json:"sourceNodeId"`
	Role         string `json:"role"`
	Order        int    `json:"order"`
}

type RunExecutionSnapshot struct {
	Version         int                    `json:"version"`
	ProjectVersion  int                    `json:"projectVersion"`
	ProjectID       string                 `json:"projectId"`
	ProjectRevision int64                  `json:"projectRevision"`
	TreeID          string                 `json:"treeId,omitempty"`
	NodeID          string                 `json:"nodeId"`
	ChainPath       []string               `json:"chainPath"`
	PathNodes       []Node                 `json:"pathNodes,omitempty"`
	PathEdges       []Edge                 `json:"pathEdges,omitempty"`
	InputNodes      []Node                 `json:"inputNodes,omitempty"`
	InputRoots      []Node                 `json:"inputRoots,omitempty"`
	InputEdges      []Edge                 `json:"inputEdges,omitempty"`
	Inputs          []RunInput             `json:"inputs,omitempty"`
	Artifacts       []RunArtifactReference `json:"artifacts"`
	OutputBinding   RunOutputBinding       `json:"outputBinding"`
	OutputContract  RunOutputContract      `json:"outputContract"`
}

func (s RunExecutionSnapshot) Clone() RunExecutionSnapshot {
	clone := s
	clone.ChainPath = append([]string(nil), s.ChainPath...)
	clone.PathEdges = append([]Edge(nil), s.PathEdges...)
	clone.InputEdges = append([]Edge(nil), s.InputEdges...)
	clone.Inputs = append([]RunInput(nil), s.Inputs...)
	clone.PathNodes = cloneNodes(s.PathNodes)
	clone.InputNodes = cloneNodes(s.InputNodes)
	clone.InputRoots = cloneNodes(s.InputRoots)
	clone.Artifacts = make([]RunArtifactReference, len(s.Artifacts))
	for i, reference := range s.Artifacts {
		clone.Artifacts[i] = reference
		if reference.CandidateIndex != nil {
			index := *reference.CandidateIndex
			clone.Artifacts[i].CandidateIndex = &index
		}
	}
	return clone
}

func cloneNodes(nodes []Node) []Node {
	clone := make([]Node, len(nodes))
	for i, node := range nodes {
		clone[i] = node.Clone()
	}
	return clone
}

func (s RunExecutionSnapshot) ScopeNodeIDs() []string {
	if s.Version == 3 {
		ids := make([]string, len(s.InputNodes))
		for i, node := range s.InputNodes {
			ids[i] = node.ID
		}
		return ids
	}
	return append([]string(nil), s.ChainPath...)
}

func (s RunExecutionSnapshot) Validate() error {
	if s.Version == 3 {
		return s.validateDAGInputs()
	}
	if (s.Version != 1 && s.Version != 2) || s.ProjectID == "" || s.ProjectVersion <= 0 || s.TreeID == "" || s.NodeID == "" || len(s.ChainPath) == 0 || len(s.ChainPath) != len(s.PathNodes) {
		return fmt.Errorf("%w: incomplete execution snapshot", ErrGraphInvalid)
	}
	if s.ProjectVersion >= valueobject.DAGProjectVersion {
		if s.Version != 2 || len(s.PathEdges) != len(s.ChainPath)-1 {
			return fmt.Errorf("%w: invalid snapshot edges", ErrGraphInvalid)
		}
	} else if s.Version != 1 || len(s.PathEdges) != 0 {
		return fmt.Errorf("%w: invalid legacy snapshot", ErrGraphInvalid)
	}
	if err := valueobject.ValidateID(s.ProjectID); err != nil {
		return err
	}
	if s.ChainPath[0] != s.TreeID || s.ChainPath[len(s.ChainPath)-1] != s.NodeID || s.PathNodes[0].Type != valueobject.NodeTypeTopic {
		return fmt.Errorf("%w: invalid snapshot path", ErrGraphInvalid)
	}
	for i, node := range s.PathNodes {
		if node.ID != s.ChainPath[i] {
			return fmt.Errorf("%w: path node mismatch", ErrGraphInvalid)
		}
	}
	project := Project{Version: s.ProjectVersion, ID: s.ProjectID, Revision: s.ProjectRevision, Nodes: s.PathNodes, Edges: s.PathEdges}
	graph, err := NewGraph(&project)
	if err != nil {
		return err
	}
	if s.ProjectVersion >= 2 {
		err = graph.Validate()
	} else {
		err = graph.ValidateForest()
	}
	if err != nil {
		return err
	}
	path, err := graph.PathToRoot(s.NodeID)
	if err != nil || len(path) != len(s.ChainPath) {
		return fmt.Errorf("%w: invalid snapshot path", ErrGraphInvalid)
	}
	for i := range path {
		if path[i] != s.ChainPath[i] {
			return fmt.Errorf("%w: invalid snapshot path", ErrGraphInvalid)
		}
		if s.ProjectVersion >= valueobject.DAGProjectVersion {
			if s.PathNodes[i].WorkspaceRootID != "" && s.PathNodes[i].WorkspaceRootID != s.TreeID {
				return fmt.Errorf("%w: invalid snapshot workspace root", ErrGraphInvalid)
			}
			if i > 0 && ValidateNodeConnection(s.PathNodes[i-1], s.PathNodes[i]) != nil {
				return fmt.Errorf("%w: invalid snapshot connection", ErrGraphInvalid)
			}
		}
	}
	target, _ := graph.Node(s.NodeID)
	if target.Type == valueobject.NodeTypeTopic {
		if s.OutputContract.Kind != valueobject.OutputKindTopics || s.OutputBinding.Bound {
			return fmt.Errorf("%w: invalid topic output contract", ErrGraphInvalid)
		}
	} else if target.Type == valueobject.NodeTypeAgent {
		if s.OutputContract.Kind != valueobject.OutputKindScript {
			return fmt.Errorf("%w: invalid agent output contract", ErrGraphInvalid)
		}
		if s.OutputBinding.Bound {
			if valueobject.ValidateID(s.OutputBinding.NodeID) != nil || s.OutputBinding.NodeID == target.ID || s.OutputBinding.ParentNodeID != s.NodeID || s.OutputBinding.BaselineVersion < 0 {
				return fmt.Errorf("%w: invalid output binding", ErrGraphInvalid)
			}
			if (s.OutputBinding.BaselineVersion == 0) != (s.OutputBinding.BaselineArtifactID == "") {
				return fmt.Errorf("%w: invalid output binding baseline", ErrGraphInvalid)
			}
		} else if s.OutputBinding.NodeID != "" || s.OutputBinding.ParentNodeID != "" || s.OutputBinding.BaselineVersion != 0 || s.OutputBinding.BaselineArtifactID != "" {
			return fmt.Errorf("%w: unbound output contains a target", ErrGraphInvalid)
		}
	} else {
		return fmt.Errorf("%w: invalid run target", ErrGraphInvalid)
	}
	if s.OutputContract.SchemaVersion != 1 {
		return fmt.Errorf("%w: unsupported output schema version", ErrGraphInvalid)
	}
	pathNodes := make(map[string]Node, len(s.ChainPath)-1)
	for i, node := range s.PathNodes[:len(s.PathNodes)-1] {
		pathNodes[s.ChainPath[i]] = node
	}
	seenArtifacts := make(map[string]struct{}, len(s.Artifacts))
	for _, reference := range s.Artifacts {
		node, ok := pathNodes[reference.NodeID]
		if !ok || valueobject.ValidateID(reference.NodeID) != nil || reference.ArtifactID == "" {
			return fmt.Errorf("%w: invalid artifact reference", ErrGraphInvalid)
		}
		if _, exists := seenArtifacts[reference.NodeID]; exists {
			return fmt.Errorf("%w: duplicate artifact reference", ErrGraphInvalid)
		}
		seenArtifacts[reference.NodeID] = struct{}{}
		if s.ProjectVersion >= 2 {
			switch node.Type {
			case valueobject.NodeTypeCandidate:
				if node.CandidateSource == nil || reference.ArtifactID != node.CandidateSource.ArtifactID || reference.Kind != valueobject.OutputKindTopics || reference.CandidateIndex == nil || *reference.CandidateIndex != node.CandidateSource.Index || reference.Version != 0 {
					return fmt.Errorf("%w: candidate artifact does not match snapshot source", ErrGraphInvalid)
				}
			case valueobject.NodeTypeScript:
				if len(node.ScriptVersions) == 0 {
					return fmt.Errorf("%w: script artifact has no version", ErrGraphInvalid)
				}
				latest := node.ScriptVersions[len(node.ScriptVersions)-1]
				if reference.ArtifactID != latest.ArtifactID || reference.Kind != valueobject.OutputKindScript || reference.CandidateIndex != nil || reference.Version != latest.N {
					return fmt.Errorf("%w: script artifact does not match latest snapshot version", ErrGraphInvalid)
				}
			default:
				return fmt.Errorf("%w: artifact reference is not a candidate or script node", ErrGraphInvalid)
			}
		} else {
			matched := false
			for _, round := range node.Rounds {
				if round.ArtifactID == reference.ArtifactID && round.Kind == reference.Kind && round.N == reference.Version {
					matched = true
					break
				}
			}
			if !matched {
				return fmt.Errorf("%w: artifact reference does not match a round", ErrGraphInvalid)
			}
			if reference.Kind == valueobject.OutputKindTopics {
				if reference.CandidateIndex == nil || *reference.CandidateIndex < 0 {
					return fmt.Errorf("%w: invalid candidate reference", ErrGraphInvalid)
				}
			} else if reference.Kind != valueobject.OutputKindScript || reference.CandidateIndex != nil {
				return fmt.Errorf("%w: invalid artifact kind", ErrGraphInvalid)
			}
		}
	}
	if s.ProjectVersion >= 2 {
		for id, node := range pathNodes {
			if node.Type == valueobject.NodeTypeCandidate {
				if _, ok := seenArtifacts[id]; !ok {
					return fmt.Errorf("%w: candidate artifact reference missing", ErrGraphInvalid)
				}
			}
			if node.Type == valueobject.NodeTypeScript && len(node.ScriptVersions) > 0 {
				if _, ok := seenArtifacts[id]; !ok {
					return fmt.Errorf("%w: script artifact reference missing", ErrGraphInvalid)
				}
			}
		}
	}
	if s.OutputBinding.Bound {
		if _, exists := pathNodes[s.OutputBinding.NodeID]; exists {
			return fmt.Errorf("%w: output binding overlaps run path", ErrGraphInvalid)
		}
	}
	return nil
}

func (s RunExecutionSnapshot) validateDAGInputs() error {
	if s.ProjectVersion < valueobject.DAGProjectVersion || s.ProjectID == "" || s.NodeID == "" || len(s.ChainPath) != 0 || len(s.PathNodes) != 0 || len(s.PathEdges) != 0 || len(s.InputNodes) < 2 || len(s.Inputs) == 0 {
		return fmt.Errorf("%w: incomplete DAG execution snapshot", ErrGraphInvalid)
	}
	if err := valueobject.ValidateID(s.ProjectID); err != nil {
		return err
	}
	if s.TreeID != "" {
		if err := valueobject.ValidateID(s.TreeID); err != nil {
			return err
		}
	}
	inputNodeIDs := make(map[string]struct{}, len(s.InputNodes))
	allNodeIDs := make(map[string]struct{}, len(s.InputNodes)+len(s.InputRoots))
	allNodes := cloneNodes(s.InputNodes)
	for _, node := range s.InputNodes {
		inputNodeIDs[node.ID] = struct{}{}
		allNodeIDs[node.ID] = struct{}{}
	}
	rootRefs := make(map[string]struct{})
	for _, node := range s.InputNodes {
		if node.WorkspaceRootID != "" {
			rootRefs[node.WorkspaceRootID] = struct{}{}
		}
	}
	if s.TreeID != "" {
		rootRefs[s.TreeID] = struct{}{}
	}
	for _, root := range s.InputRoots {
		if _, duplicate := allNodeIDs[root.ID]; duplicate || root.Type != valueobject.NodeTypeTopic ||
			(root.WorkspaceRootID != "" && root.WorkspaceRootID != root.ID) {
			return fmt.Errorf("%w: invalid input workspace root", ErrGraphInvalid)
		}
		if _, referenced := rootRefs[root.ID]; !referenced {
			return fmt.Errorf("%w: unrelated input workspace root", ErrGraphInvalid)
		}
		allNodeIDs[root.ID] = struct{}{}
		delete(rootRefs, root.ID)
		allNodes = append(allNodes, root.Clone())
	}
	for id := range rootRefs {
		var root Node
		found := false
		for _, node := range s.InputNodes {
			if node.ID == id {
				root, found = node, true
				break
			}
		}
		if !found || root.Type != valueobject.NodeTypeTopic {
			return fmt.Errorf("%w: missing input workspace root", ErrGraphInvalid)
		}
	}
	project := Project{
		Version: s.ProjectVersion, ID: s.ProjectID, Revision: s.ProjectRevision,
		Nodes: allNodes, Edges: append([]Edge(nil), s.InputEdges...),
	}
	graph, err := NewGraph(&project)
	if err != nil {
		return err
	}
	if err := graph.Validate(); err != nil {
		return err
	}
	positions := make(map[string]int, len(s.InputNodes))
	for i, node := range s.InputNodes {
		positions[node.ID] = i
		if node.ID == s.NodeID && i != len(s.InputNodes)-1 {
			return fmt.Errorf("%w: target must be last in DAG snapshot", ErrGraphInvalid)
		}
	}
	target, ok := graph.Node(s.NodeID)
	if !ok || target.Type != valueobject.NodeTypeAgent || s.InputNodes[len(s.InputNodes)-1].ID != s.NodeID {
		return fmt.Errorf("%w: invalid DAG run target", ErrGraphInvalid)
	}
	if target.WorkspaceRootID != s.TreeID {
		return fmt.Errorf("%w: snapshot workspace root does not match target", ErrWorkspaceRootMismatch)
	}
	for _, edge := range s.InputEdges {
		sourcePosition, sourceExists := positions[edge.SourceNodeID]
		targetPosition, targetExists := positions[edge.TargetNodeID]
		if !sourceExists || !targetExists || sourcePosition >= targetPosition {
			return fmt.Errorf("%w: input nodes are not topologically ordered", ErrGraphInvalid)
		}
	}
	ancestors := graph.AncestorNodeIDs(s.NodeID)
	if len(ancestors) != len(s.InputNodes) {
		return fmt.Errorf("%w: DAG snapshot contains unrelated nodes", ErrGraphInvalid)
	}
	for _, id := range ancestors {
		if _, exists := positions[id]; !exists {
			return fmt.Errorf("%w: DAG snapshot is missing an input ancestor", ErrGraphInvalid)
		}
	}
	selectedEdges := make(map[string]struct{}, len(s.Inputs))
	taskCount := 0
	for i, input := range s.Inputs {
		if input.Order != i || input.EdgeID == "" || input.SourceNodeID == s.NodeID {
			return fmt.Errorf("%w: invalid run input order", ErrGraphInvalid)
		}
		source, ok := graph.Node(input.SourceNodeID)
		if !ok || InputRoleForNodeType(source.Type) == "" || input.Role != InputRoleForNodeType(source.Type) {
			return fmt.Errorf("%w: invalid run input role", ErrGraphInvalid)
		}
		edge, ok := graph.Edge(input.EdgeID)
		if !ok || edge.Kind != EdgeKindConnection || edge.SourceNodeID != input.SourceNodeID || edge.TargetNodeID != s.NodeID {
			return fmt.Errorf("%w: selected input edge is missing", ErrGraphInvalid)
		}
		if _, exists := selectedEdges[input.EdgeID]; exists {
			return fmt.Errorf("%w: duplicate selected input", ErrGraphInvalid)
		}
		selectedEdges[input.EdgeID] = struct{}{}
		if input.Role == "task" {
			if source.Type != valueobject.NodeTypePrompt || strings.TrimSpace(source.Params.Instruction) == "" {
				return fmt.Errorf("%w: prompt task is empty", ErrGraphInvalid)
			}
			taskCount++
		}
	}
	if taskCount == 0 || len(selectedEdges) != len(graph.Incoming(s.NodeID)) {
		return fmt.Errorf("%w: selected run inputs are incomplete", ErrGraphInvalid)
	}
	if s.OutputContract.Kind != valueobject.OutputKindScript || s.OutputContract.SchemaVersion != 1 {
		return fmt.Errorf("%w: invalid DAG output contract", ErrGraphInvalid)
	}
	if s.OutputBinding.Bound {
		if valueobject.ValidateID(s.OutputBinding.NodeID) != nil || s.OutputBinding.NodeID == s.NodeID || s.OutputBinding.ParentNodeID != s.NodeID || s.OutputBinding.BaselineVersion < 0 {
			return fmt.Errorf("%w: invalid output binding", ErrGraphInvalid)
		}
		if (s.OutputBinding.BaselineVersion == 0) != (s.OutputBinding.BaselineArtifactID == "") {
			return fmt.Errorf("%w: invalid output binding baseline", ErrGraphInvalid)
		}
		if _, exists := positions[s.OutputBinding.NodeID]; exists {
			return fmt.Errorf("%w: output binding overlaps run inputs", ErrGraphInvalid)
		}
	} else if s.OutputBinding.NodeID != "" || s.OutputBinding.ParentNodeID != "" || s.OutputBinding.BaselineVersion != 0 || s.OutputBinding.BaselineArtifactID != "" {
		return fmt.Errorf("%w: unbound output contains a target", ErrGraphInvalid)
	}

	artifactNodes := make(map[string]struct{}, len(s.Artifacts))
	for _, reference := range s.Artifacts {
		node, ok := graph.Node(reference.NodeID)
		if !ok || node.ID == s.NodeID || reference.ArtifactID == "" {
			return fmt.Errorf("%w: invalid DAG artifact reference", ErrGraphInvalid)
		}
		if _, exists := artifactNodes[node.ID]; exists {
			return fmt.Errorf("%w: duplicate DAG artifact reference", ErrGraphInvalid)
		}
		artifactNodes[node.ID] = struct{}{}
		switch node.Type {
		case valueobject.NodeTypeCandidate:
			source := node.CandidateSource
			if source == nil || reference.ArtifactID != source.ArtifactID || reference.Kind != valueobject.OutputKindTopics || reference.CandidateIndex == nil || *reference.CandidateIndex != source.Index || reference.Version != 0 {
				return fmt.Errorf("%w: candidate artifact does not match snapshot source", ErrGraphInvalid)
			}
		case valueobject.NodeTypeScript:
			if len(node.ScriptVersions) == 0 {
				return fmt.Errorf("%w: selected Script has no immutable version", ErrGraphInvalid)
			}
			latest := node.ScriptVersions[len(node.ScriptVersions)-1]
			if reference.ArtifactID != latest.ArtifactID || reference.Kind != valueobject.OutputKindScript || reference.CandidateIndex != nil || reference.Version != latest.N {
				return fmt.Errorf("%w: Script artifact does not match frozen version", ErrGraphInvalid)
			}
		default:
			return fmt.Errorf("%w: unsupported DAG artifact source", ErrGraphInvalid)
		}
	}
	for _, node := range s.InputNodes[:len(s.InputNodes)-1] {
		switch node.Type {
		case valueobject.NodeTypeCandidate, valueobject.NodeTypeScript:
			if _, exists := artifactNodes[node.ID]; !exists {
				return fmt.Errorf("%w: selected artifact is missing", ErrGraphInvalid)
			}
		}
	}
	return nil
}

func InputRoleForNodeType(nodeType valueobject.NodeType) string {
	switch nodeType {
	case valueobject.NodeTypePrompt:
		return "task"
	case valueobject.NodeTypeCandidate:
		return "candidate"
	case valueobject.NodeTypeScript:
		return "artifact"
	default:
		return ""
	}
}

// RunRecord is the per-run detail document.
type RunRecord struct {
	RunSummary
	ContextPreview          string                       `json:"contextPreview,omitempty"`
	ContextPreviewTruncated bool                         `json:"contextPreviewTruncated,omitempty"`
	ExecutionSnapshot       *RunExecutionSnapshot        `json:"executionSnapshot,omitempty"`
	Transcript              []valueobject.TranscriptItem `json:"transcript"`
}

// RunSnapshot is the in-memory recovery view used to resume an event stream.
// ProjectID and LastSeq are deliberately not persisted in runs.json: events
// and their sequence numbers are scoped to one process lifetime.
type RunSnapshot struct {
	RunID       string                          `json:"runId"`
	ProjectID   string                          `json:"projectId"`
	Run         RunRecord                       `json:"run"`
	LastSeq     uint64                          `json:"lastSeq"`
	Permissions []valueobject.PermissionRequest `json:"permissions"`
	Questions   []valueobject.QuestionRequest   `json:"questions"`
}

// RunsFile is the runs.json index document.
type RunsFile struct {
	Version int          `json:"version"`
	Runs    []RunSummary `json:"runs"`
}

// RunStartResult is the structured response to a UI start request. Expected
// business failures stay in the response so callers can render typed details
// such as a context-budget estimate without parsing error text.
type RunStartResult struct {
	Run   *RunRecord     `json:"run,omitempty"`
	Error *RunStartError `json:"error,omitempty"`
}

// RunStartError is a safe, read-only error view for a rejected start request.
type RunStartError struct {
	Code            string `json:"code"`
	Message         string `json:"message"`
	EstimatedTokens *int   `json:"estimatedTokens,omitempty"`
	TokenBudget     *int   `json:"tokenBudget,omitempty"`
}
