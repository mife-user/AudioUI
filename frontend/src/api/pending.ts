import { WorkflowService } from '../../bindings/audio-ui/internal/handler'
import type { PendingDetail, Project } from '../../bindings/audio-ui/internal/domain/entity'

export type PendingAPI = {
  getPending: (id: string, projectId: string) => Promise<PendingDetail>
  applyPending: (id: string, force: boolean, projectId: string) => Promise<Project>
  discardPending: (id: string, projectId: string) => Promise<Project>
}

export const pendingAPI: PendingAPI = {
  getPending: (id, projectId) => WorkflowService.GetPending(id, projectId),
  applyPending: (id, force, projectId) => WorkflowService.ApplyPending(id, force, projectId),
  discardPending: (id, projectId) => WorkflowService.DiscardPending(id, projectId),
}

export function decodePendingPayload(rawPayload: string | null): string {
  if (!rawPayload) return ''
  const binary = atob(rawPayload)
  const bytes = Uint8Array.from(binary, (character) => character.charCodeAt(0))
  return new TextDecoder().decode(bytes)
}
