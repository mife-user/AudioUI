package handler

import (
	"context"
	"errors"
	"strings"
	"time"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/service/project"
)

type ProjectService struct {
	service *project.Service
}

func NewProjectService(service *project.Service) *ProjectService {
	return &ProjectService{service: service}
}

func (h *ProjectService) ListProjects() ([]entity.ProjectSummary, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return h.service.List(ctx)
}

func (h *ProjectService) NewProject(name string) (entity.Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return entity.Project{}, errors.New("工程名称不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return h.service.New(ctx, name)
}

func (h *ProjectService) OpenProject(id string) (entity.Project, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return entity.Project{}, errors.New("工程 ID 不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return h.service.Open(ctx, id)
}

func (h *ProjectService) GetActiveProject() (entity.Project, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	projectValue, err := h.service.Current(ctx)
	if errors.Is(err, project.ErrNoActiveProject) {
		return entity.Project{}, errors.New("NO_ACTIVE_PROJECT: 当前没有打开的工程")
	}
	return projectValue, err
}

func (h *ProjectService) GetConfig() (valueobject.AppConfig, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return h.service.Config(ctx)
}
