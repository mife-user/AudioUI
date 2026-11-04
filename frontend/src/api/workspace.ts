import { Events } from '@wailsio/runtime'
import { ProjectService, WorkspaceService } from '../../bindings/audio-ui/internal/handler'
import type { ProjectSummary, WorkspaceSnapshotResult } from '../../bindings/audio-ui/internal/domain/entity'

export type WorkspaceChangedEvent = {
  projectId: string
  revision: number
  reason: 'project' | 'graph' | 'pending' | 'run_lock'
  projectRevision: number
}

export type WorkspaceStatus = 'idle' | 'loading' | 'ready' | 'error'

function eventData<T>(payload: unknown): T {
  if (payload && typeof payload === 'object' && 'data' in payload) {
    return (payload as { data: T }).data
  }
  return payload as T
}

export async function getWorkspaceSnapshot(): Promise<WorkspaceSnapshotResult> {
  return await WorkspaceService.GetWorkspaceSnapshot()
}

export async function listProjects(): Promise<ProjectSummary[]> {
  return (await ProjectService.ListProjects()) ?? []
}

export async function newProject(name: string) {
  return await ProjectService.NewProject(name)
}

export async function openProject(id: string) {
  return await ProjectService.OpenProject(id)
}

export function subscribeWorkspaceEvents(handler: (event: WorkspaceChangedEvent) => void): () => void {
  return Events.On('workspace:changed', (payload: unknown) => handler(eventData<WorkspaceChangedEvent>(payload)))
}
