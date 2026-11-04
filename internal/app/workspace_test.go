package app

import (
	"testing"

	"audio-ui/internal/service/workflow"
)

func TestWorkspaceCoordinatorPublishesProjectScopedMonotonicChanges(t *testing.T) {
	bus := newWailsEventBus()
	coordinator := newWorkspaceCoordinator(bus)
	coordinator.Initialize("p_first", 4)
	coordinator.WorkflowChanged("p_stale", 90, workflow.ChangeReasonGraph)
	if revision, projectID := coordinator.Read(); revision != 1 || projectID != "p_first" {
		t.Fatalf("stale callback changed revision/project: %d %q", revision, projectID)
	}
	coordinator.WorkflowChanged("p_first", 5, workflow.ChangeReasonGraph)
	coordinator.WorkflowChanged("p_first", 6, workflow.ChangeReasonPending)
	coordinator.ProjectChanged("p_second", 1)
	coordinator.WorkflowChanged("p_first", 7, workflow.ChangeReasonRunLock)
	coordinator.WorkflowChanged("p_second", 1, workflow.ChangeReasonRunLock)
	revision, projectID := coordinator.Read()
	if revision != 5 || projectID != "p_second" {
		t.Fatalf("current revision/project = %d %q", revision, projectID)
	}
	if len(bus.pendingWorkspace) != 4 {
		t.Fatalf("workspace events = %+v", bus.pendingWorkspace)
	}
	for i, event := range bus.pendingWorkspace {
		if event.Revision != uint64(i+2) {
			t.Fatalf("event revisions = %+v", bus.pendingWorkspace)
		}
	}
	if event := bus.pendingWorkspace[0]; event.ProjectID != "p_first" || event.ProjectRevision != 5 || event.Reason != workflow.ChangeReasonGraph {
		t.Fatalf("graph event = %+v", event)
	}
	if event := bus.pendingWorkspace[1]; event.ProjectID != "p_first" || event.ProjectRevision != 6 || event.Reason != workflow.ChangeReasonPending {
		t.Fatalf("pending event = %+v", event)
	}
	if event := bus.pendingWorkspace[2]; event.ProjectID != "p_second" || event.ProjectRevision != 1 || event.Reason != workflow.ChangeReasonProject {
		t.Fatalf("project event = %+v", event)
	}
	if event := bus.pendingWorkspace[3]; event.ProjectID != "p_second" || event.ProjectRevision != 1 || event.Reason != workflow.ChangeReasonRunLock {
		t.Fatalf("run-lock event = %+v", event)
	}
}
