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
	Type            string            `json:"type"`
	Title           string            `json:"title"`
	Parent          string            `json:"parent"`
	ParentCandidate *int              `json:"parentCandidate"`
	Position        entity.Position   `json:"position"`
	Params          entity.NodeParams `json:"params"`
}

func (h *WorkflowService) AddNode(request AddNodeRequest) (entity.Project, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return h.service.AddNode(ctx, workflow.AddNodeInput{
		Type:            valueobject.NodeType(strings.TrimSpace(request.Type)),
		Title:           strings.TrimSpace(request.Title),
		Parent:          strings.TrimSpace(request.Parent),
		ParentCandidate: request.ParentCandidate,
		Position:        request.Position,
		Params:          request.Params,
	})
}

func (h *WorkflowService) Connect(parentID, childID string, parentCandidate *int, replace bool) (entity.Project, error) {
	parentID = strings.TrimSpace(parentID)
	childID = strings.TrimSpace(childID)
	if parentID == "" || childID == "" {
		return entity.Project{}, errors.New("父节点与子节点 ID 不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return h.service.Connect(ctx, parentID, childID, parentCandidate, replace)
}

func (h *WorkflowService) MoveNode(id string, x, y float64) (entity.Project, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return entity.Project{}, errors.New("节点 ID 不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return h.service.MoveNode(ctx, id, x, y)
}

func (h *WorkflowService) UpdateNodeParams(id, title string, params entity.NodeParams) (entity.Project, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return entity.Project{}, errors.New("节点 ID 不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return h.service.UpdateNodeParams(ctx, id, strings.TrimSpace(title), params)
}

func (h *WorkflowService) SelectNode(id string) (entity.Project, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return entity.Project{}, errors.New("节点 ID 不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return h.service.SelectNode(ctx, id)
}
