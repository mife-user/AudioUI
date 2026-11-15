package handler

import (
	"context"
	"errors"
	"strings"
	"time"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/service/workflow"
)

type WorkflowService struct {
	service *workflow.Service
}

func NewWorkflowService(service *workflow.Service) *WorkflowService {
	return &WorkflowService{service: service}
}

type AddNodeRequest struct {
	ProjectID       string            `json:"projectId"`
	Type            string            `json:"type"`
	Title           string            `json:"title"`
	Parent          string            `json:"parent"`
	ParentCandidate *int              `json:"parentCandidate"`
	Position        entity.Position   `json:"position"`
	Params          entity.NodeParams `json:"params"`
}

type ReconnectTargetRequest struct {
	ProjectID    string      `json:"projectId"`
	ExpectedEdge entity.Edge `json:"expectedEdge"`
	TargetNodeID string      `json:"targetNodeId"`
}

func (h *WorkflowService) AddNode(request AddNodeRequest) (entity.GraphMutationReceipt, error) {
	request.ProjectID = strings.TrimSpace(request.ProjectID)
	if request.ProjectID == "" {
		return entity.GraphMutationReceipt{}, errors.New("工程 ID 不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	projectValue, err := h.service.AddNode(ctx, workflow.AddNodeInput{
		ProjectID:       strings.TrimSpace(request.ProjectID),
		Type:            valueobject.NodeType(strings.TrimSpace(request.Type)),
		Title:           strings.TrimSpace(request.Title),
		Parent:          strings.TrimSpace(request.Parent),
		ParentCandidate: request.ParentCandidate,
		Position:        request.Position,
		Params:          request.Params,
	})
	if err != nil {
		return entity.GraphMutationReceipt{}, err
	}
	if len(projectValue.Nodes) == 0 {
		return entity.GraphMutationReceipt{}, errors.New("ADD_NODE_RECEIPT_MISSING_NODE")
	}
	return mutationReceipt(projectValue, projectValue.Nodes[len(projectValue.Nodes)-1].ID), nil
}

func (h *WorkflowService) Connect(sourceID, targetID, projectID string) (entity.GraphMutationReceipt, error) {
	sourceID = strings.TrimSpace(sourceID)
	targetID = strings.TrimSpace(targetID)
	projectID = strings.TrimSpace(projectID)
	if sourceID == "" || targetID == "" {
		return entity.GraphMutationReceipt{}, errors.New("连接起点与目标节点 ID 不能为空")
	}
	if projectID == "" {
		return entity.GraphMutationReceipt{}, errors.New("工程 ID 不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	projectValue, err := h.service.Connect(ctx, sourceID, targetID, projectID)
	if err != nil {
		return entity.GraphMutationReceipt{}, err
	}
	return mutationReceipt(projectValue, targetID), nil
}

func (h *WorkflowService) Disconnect(edgeID, projectID string) (entity.GraphMutationReceipt, error) {
	edgeID = strings.TrimSpace(edgeID)
	projectID = strings.TrimSpace(projectID)
	if edgeID == "" || projectID == "" {
		return entity.GraphMutationReceipt{}, errors.New("连接 ID 与工程 ID 不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	projectValue, targetID, err := h.service.Disconnect(ctx, edgeID, projectID)
	if err != nil {
		return entity.GraphMutationReceipt{}, err
	}
	return mutationReceipt(projectValue, targetID), nil
}

func (h *WorkflowService) ReplaceConnection(edgeID, sourceID, projectID string) (entity.GraphMutationReceipt, error) {
	edgeID = strings.TrimSpace(edgeID)
	sourceID = strings.TrimSpace(sourceID)
	projectID = strings.TrimSpace(projectID)
	if edgeID == "" || sourceID == "" || projectID == "" {
		return entity.GraphMutationReceipt{}, errors.New("连接 ID、起点与工程 ID 不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	projectValue, err := h.service.ReplaceConnection(ctx, edgeID, sourceID, projectID)
	if err != nil {
		return entity.GraphMutationReceipt{}, err
	}
	for _, edge := range projectValue.Edges {
		if edge.ID == edgeID {
			return mutationReceipt(projectValue, edge.TargetNodeID), nil
		}
	}
	return entity.GraphMutationReceipt{}, errors.New("EDGE_RECEIPT_MISSING_EDGE")
}

func (h *WorkflowService) ReconnectTarget(request ReconnectTargetRequest) (entity.GraphMutationReceipt, error) {
	request.ProjectID = strings.TrimSpace(request.ProjectID)
	request.TargetNodeID = strings.TrimSpace(request.TargetNodeID)
	request.ExpectedEdge.ID = strings.TrimSpace(request.ExpectedEdge.ID)
	request.ExpectedEdge.SourceNodeID = strings.TrimSpace(request.ExpectedEdge.SourceNodeID)
	request.ExpectedEdge.TargetNodeID = strings.TrimSpace(request.ExpectedEdge.TargetNodeID)
	request.ExpectedEdge.Kind = strings.TrimSpace(request.ExpectedEdge.Kind)
	if request.ProjectID == "" || request.TargetNodeID == "" || request.ExpectedEdge.ID == "" ||
		request.ExpectedEdge.SourceNodeID == "" || request.ExpectedEdge.TargetNodeID == "" || request.ExpectedEdge.Kind == "" {
		return entity.GraphMutationReceipt{}, errors.New("连接身份、目标与工程 ID 不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	projectValue, err := h.service.ReconnectTarget(ctx, request.ExpectedEdge, request.TargetNodeID, request.ProjectID)
	if err != nil {
		return entity.GraphMutationReceipt{}, err
	}
	return mutationReceipt(projectValue, request.TargetNodeID), nil
}

func (h *WorkflowService) SetEdgeScriptVersion(edgeID string, version int, projectID string) (entity.GraphMutationReceipt, error) {
	edgeID = strings.TrimSpace(edgeID)
	projectID = strings.TrimSpace(projectID)
	if edgeID == "" || projectID == "" {
		return entity.GraphMutationReceipt{}, errors.New("连接 ID 与工程 ID 不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	projectValue, err := h.service.SetEdgeScriptVersion(ctx, edgeID, version, projectID)
	if err != nil {
		return entity.GraphMutationReceipt{}, err
	}
	for _, edge := range projectValue.Edges {
		if edge.ID == edgeID {
			return mutationReceipt(projectValue, edge.TargetNodeID), nil
		}
	}
	return entity.GraphMutationReceipt{}, errors.New("EDGE_RECEIPT_MISSING_EDGE")
}

func (h *WorkflowService) MoveNode(id string, x, y float64, projectID string) (entity.GraphMutationReceipt, error) {
	id = strings.TrimSpace(id)
	projectID = strings.TrimSpace(projectID)
	if id == "" {
		return entity.GraphMutationReceipt{}, errors.New("节点 ID 不能为空")
	}
	if projectID == "" {
		return entity.GraphMutationReceipt{}, errors.New("工程 ID 不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	projectValue, err := h.service.MoveNode(ctx, id, x, y, projectID)
	if err != nil {
		return entity.GraphMutationReceipt{}, err
	}
	return mutationReceipt(projectValue, id), nil
}

func (h *WorkflowService) SetNodeLayout(id string, x, y, width, height float64, projectID string) (entity.GraphMutationReceipt, error) {
	id = strings.TrimSpace(id)
	projectID = strings.TrimSpace(projectID)
	if id == "" {
		return entity.GraphMutationReceipt{}, errors.New("节点 ID 不能为空")
	}
	if projectID == "" {
		return entity.GraphMutationReceipt{}, errors.New("工程 ID 不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	projectValue, err := h.service.SetNodeLayout(ctx, id, x, y, width, height, projectID)
	if err != nil {
		return entity.GraphMutationReceipt{}, err
	}
	return mutationReceipt(projectValue, id), nil
}

func (h *WorkflowService) SetNodeCollapsed(id string, collapsed bool, projectID string) (entity.GraphMutationReceipt, error) {
	id = strings.TrimSpace(id)
	projectID = strings.TrimSpace(projectID)
	if id == "" {
		return entity.GraphMutationReceipt{}, errors.New("节点 ID 不能为空")
	}
	if projectID == "" {
		return entity.GraphMutationReceipt{}, errors.New("工程 ID 不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	projectValue, err := h.service.SetNodeCollapsed(ctx, id, collapsed, projectID)
	if err != nil {
		return entity.GraphMutationReceipt{}, err
	}
	return mutationReceipt(projectValue, id), nil
}

func (h *WorkflowService) UpdateNodeParams(id, title string, params entity.NodeParams, projectID string) (entity.GraphMutationReceipt, error) {
	id = strings.TrimSpace(id)
	projectID = strings.TrimSpace(projectID)
	if id == "" {
		return entity.GraphMutationReceipt{}, errors.New("节点 ID 不能为空")
	}
	if projectID == "" {
		return entity.GraphMutationReceipt{}, errors.New("工程 ID 不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	projectValue, err := h.service.UpdateNodeParams(ctx, id, strings.TrimSpace(title), params, projectID)
	if err != nil {
		return entity.GraphMutationReceipt{}, err
	}
	return mutationReceipt(projectValue, id), nil
}

func (h *WorkflowService) SelectNode(id, projectID string) (entity.GraphMutationReceipt, error) {
	id = strings.TrimSpace(id)
	projectID = strings.TrimSpace(projectID)
	if id == "" {
		return entity.GraphMutationReceipt{}, errors.New("节点 ID 不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	projectValue, err := h.service.SelectNode(ctx, id, projectID)
	if err != nil {
		return entity.GraphMutationReceipt{}, err
	}
	return mutationReceipt(projectValue, id), nil
}

func mutationReceipt(projectValue entity.Project, nodeID string) entity.GraphMutationReceipt {
	return entity.GraphMutationReceipt{
		ProjectID: projectValue.ID, ProjectRevision: projectValue.Revision, NodeID: nodeID,
	}
}

func (h *WorkflowService) ListPending(projectID string) ([]entity.Pending, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, errors.New("工程 ID 不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return h.service.ListPending(ctx, projectID)
}

func (h *WorkflowService) GetPending(id, projectID string) (entity.PendingDetail, error) {
	id = strings.TrimSpace(id)
	projectID = strings.TrimSpace(projectID)
	if id == "" {
		return entity.PendingDetail{}, errors.New("待应用 ID 不能为空")
	}
	if projectID == "" {
		return entity.PendingDetail{}, errors.New("工程 ID 不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return h.service.GetPending(ctx, id, projectID)
}

func (h *WorkflowService) ApplyPending(id string, force bool, projectID string) (entity.Project, error) {
	id = strings.TrimSpace(id)
	projectID = strings.TrimSpace(projectID)
	if id == "" {
		return entity.Project{}, errors.New("待应用 ID 不能为空")
	}
	if projectID == "" {
		return entity.Project{}, errors.New("工程 ID 不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return h.service.ApplyPending(ctx, id, force, projectID)
}

func (h *WorkflowService) DiscardPending(id, projectID string) (entity.Project, error) {
	id = strings.TrimSpace(id)
	projectID = strings.TrimSpace(projectID)
	if id == "" {
		return entity.Project{}, errors.New("待应用 ID 不能为空")
	}
	if projectID == "" {
		return entity.Project{}, errors.New("工程 ID 不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return h.service.DiscardPending(ctx, id, projectID)
}

func (h *WorkflowService) GetTopicCandidates(nodeID, projectID string) ([]entity.TopicCandidate, error) {
	nodeID = strings.TrimSpace(nodeID)
	projectID = strings.TrimSpace(projectID)
	if nodeID == "" {
		return nil, errors.New("节点 ID 不能为空")
	}
	if projectID == "" {
		return nil, errors.New("工程 ID 不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return h.service.GetTopicCandidates(ctx, nodeID, projectID)
}

func (h *WorkflowService) GetScriptVersion(nodeID string, version int, projectID string) (entity.ScriptVersionDetail, error) {
	nodeID = strings.TrimSpace(nodeID)
	projectID = strings.TrimSpace(projectID)
	if nodeID == "" {
		return entity.ScriptVersionDetail{}, errors.New("节点 ID 不能为空")
	}
	if projectID == "" {
		return entity.ScriptVersionDetail{}, errors.New("工程 ID 不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return h.service.GetScriptVersion(ctx, nodeID, version, projectID)
}
