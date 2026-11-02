package app

import (
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/handler"
	"audio-ui/internal/infra/persistence"
	"audio-ui/internal/infra/workspace"
	"audio-ui/internal/service/project"
	"audio-ui/internal/service/workflow"
)

type workflowSetup struct {
	handler *handler.WorkflowService
}

func newWorkflowSetup(layout valueobject.Layout, projectService *project.Service) *workflowSetup {
	service := workflow.New(workflow.Options{
		Project:    projectService,
		Candidates: workflow.NewArtifactCandidateCounter(persistence.NewArtifactStoreProvider(layout)),
		Workspace:  workspace.NewMover(layout),
	})
	return &workflowSetup{handler: handler.NewWorkflowService(service)}
}
