package app

import (
	"sync"

	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/service/workflow"
)

// workspaceCoordinator owns the process-local revision and emits invalidation
// events. It never calls back into project/workflow services.
type workspaceCoordinator struct {
	eventMu         sync.Mutex
	eventQueue      []valueobject.WorkspaceChanged
	publishing      bool
	mu              sync.Mutex
	revision        uint64
	projectID       string
	projectRevision int64
	bus             *wailsEventBus
}

func newWorkspaceCoordinator(bus *wailsEventBus) *workspaceCoordinator {
	return &workspaceCoordinator{revision: 1, bus: bus}
}

func (c *workspaceCoordinator) Initialize(projectID string, projectRevision int64) {
	c.eventMu.Lock()
	c.mu.Lock()
	c.projectID = projectID
	c.projectRevision = projectRevision
	c.mu.Unlock()
	c.eventMu.Unlock()
}

func (c *workspaceCoordinator) ProjectChanged(projectID string, projectRevision int64) {
	c.publish(projectID, projectRevision, workflow.ChangeReasonProject)
}

func (c *workspaceCoordinator) WorkflowChanged(projectID string, projectRevision int64, reason string) {
	c.publish(projectID, projectRevision, reason)
}

func (c *workspaceCoordinator) Read() (uint64, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.revision, c.projectID
}

func (c *workspaceCoordinator) publish(projectID string, projectRevision int64, reason string) {
	c.eventMu.Lock()
	c.mu.Lock()
	if reason != workflow.ChangeReasonProject && projectID != c.projectID {
		c.mu.Unlock()
		c.eventMu.Unlock()
		return
	}
	if reason == workflow.ChangeReasonProject && projectID == c.projectID && projectRevision == c.projectRevision {
		c.mu.Unlock()
		c.eventMu.Unlock()
		return
	}
	c.projectID = projectID
	c.projectRevision = projectRevision
	c.revision++
	event := valueobject.WorkspaceChanged{
		ProjectID: projectID, Revision: c.revision,
		Reason: reason, ProjectRevision: projectRevision,
	}
	c.mu.Unlock()
	if c.bus != nil {
		c.eventQueue = append(c.eventQueue, event)
		if c.publishing {
			c.eventMu.Unlock()
			return
		}
		c.publishing = true
		c.eventMu.Unlock()
		c.drainEvents()
		return
	}
	c.eventMu.Unlock()
}

func (c *workspaceCoordinator) drainEvents() {
	for {
		c.eventMu.Lock()
		if len(c.eventQueue) == 0 {
			c.publishing = false
			c.eventMu.Unlock()
			return
		}
		event := c.eventQueue[0]
		c.eventQueue[0] = valueobject.WorkspaceChanged{}
		c.eventQueue = c.eventQueue[1:]
		c.eventMu.Unlock()
		c.bus.PublishWorkspace(event)
	}
}
