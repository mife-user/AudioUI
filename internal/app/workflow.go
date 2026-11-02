package app

import (
	"context"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/handler"
	"audio-ui/internal/infra/persistence"
	"audio-ui/internal/infra/workspace"
	contextservice "audio-ui/internal/service/context"
	"audio-ui/internal/service/project"
	"audio-ui/internal/service/workflow"
)

type workflowSetup struct {
	handler *handler.WorkflowService
}

func newWorkflowSetup(layout valueobject.Layout, projectService *project.Service) *workflowSetup {
	artifactStores := persistence.NewArtifactStoreProvider(layout)
	contextBuilder := contextservice.NewBuilder(artifactStores)
	service := workflow.New(workflow.Options{
		Project:       projectService,
		Candidates:    workflow.NewArtifactCandidateCounter(artifactStores),
		Workspace:     workspace.NewMover(layout),
		Pending:       persistence.NewPendingStoreProvider(layout),
		Artifacts:     artifactStores,
		ContextHasher: workflowContextHasher{builder: contextBuilder},
	})
	return &workflowSetup{handler: handler.NewWorkflowService(service)}
}

type workflowContextHasher struct {
	builder *contextservice.Builder
}

func (h workflowContextHasher) InputHashes(ctx context.Context, project entity.Project, nodeID string) (valueobject.ContextHashes, error) {
	return h.builder.InputHashes(ctx, contextservice.Input{Project: project, NodeID: nodeID})
}
