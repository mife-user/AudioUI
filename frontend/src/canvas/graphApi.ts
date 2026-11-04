import { WorkflowService } from '../../bindings/audio-ui/internal/handler'
import type { AddNodeRequest } from '../../bindings/audio-ui/internal/handler'
import type { GraphMutationReceipt } from '../../bindings/audio-ui/internal/domain/entity'

export type GraphAPI = {
  addNode: (request: AddNodeRequest) => Promise<GraphMutationReceipt>
  connect: (parentId: string, childId: string, candidate: number | null, replace: boolean, projectId: string) => Promise<GraphMutationReceipt>
  moveNode: (id: string, x: number, y: number, projectId: string) => Promise<GraphMutationReceipt>
  selectNode: (id: string, projectId: string) => Promise<GraphMutationReceipt>
}

export const graphAPI: GraphAPI = {
  addNode: (request) => WorkflowService.AddNode(request),
  connect: (parentId, childId, candidate, replace, projectId) => WorkflowService.Connect(parentId, childId, candidate, replace, projectId),
  moveNode: (id, x, y, projectId) => WorkflowService.MoveNode(id, x, y, projectId),
  selectNode: (id, projectId) => WorkflowService.SelectNode(id, projectId),
}

export type TopicCandidateFixture = {
  title: string
  angle?: string
  hook?: string
}
