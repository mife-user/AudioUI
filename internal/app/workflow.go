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
	service *workflow.Service
}

func newWorkflowSetup(layout valueobject.Layout, projectService *project.Service, onChange ...func(string, int64, string)) *workflowSetup {
	artifactStores := persistence.NewArtifactStoreProvider(layout)
	contextBuilder := contextservice.NewBuilder(artifactStores)
	candidates := workflow.NewArtifactCandidateCounter(artifactStores)
	var changed func(string, int64, string)
	if len(onChange) > 0 {
		changed = onChange[0]
	}
	service := workflow.New(workflow.Options{
		Project:         projectService,
		Candidates:      candidates,
		CandidateReader: candidates,
		Workspace:       workspace.NewMover(layout),
		Pending:         persistence.NewPendingStoreProvider(layout),
		Artifacts:       artifactStores,
		ContextHasher:   workflowContextHasher{builder: contextBuilder},
		OnChange:        changed,
	})
	return &workflowSetup{handler: handler.NewWorkflowService(service), service: service}
}

type workflowContextHasher struct {
	builder *contextservice.Builder
}

func (h workflowContextHasher) InputHashes(ctx context.Context, project entity.Project, nodeID string) (valueobject.ContextHashes, error) {
	return h.builder.InputHashes(ctx, contextservice.Input{Project: project, NodeID: nodeID})
}
