import { WorkflowService } from '../../bindings/audio-ui/internal/handler'
import type { AddNodeRequest } from '../../bindings/audio-ui/internal/handler'
import type { GraphMutationReceipt, NodeParams, TopicCandidate } from '../../bindings/audio-ui/internal/domain/entity'

export type GraphAPI = {
  addNode: (request: AddNodeRequest) => Promise<GraphMutationReceipt>
  connect: (parentId: string, childId: string, candidate: number | null, replace: boolean, projectId: string) => Promise<GraphMutationReceipt>
  moveNode: (id: string, x: number, y: number, projectId: string) => Promise<GraphMutationReceipt>
  selectNode: (id: string, projectId: string) => Promise<GraphMutationReceipt>
  updateNodeParams: (id: string, title: string, params: NodeParams, projectId: string) => Promise<GraphMutationReceipt>
  getTopicCandidates: (nodeId: string, projectId: string) => Promise<TopicCandidate[]>
}

export const graphAPI: GraphAPI = {
  addNode: (request) => WorkflowService.AddNode(request),
  connect: (parentId, childId, candidate, replace, projectId) => WorkflowService.Connect(parentId, childId, candidate, replace, projectId),
  moveNode: (id, x, y, projectId) => WorkflowService.MoveNode(id, x, y, projectId),
  selectNode: (id, projectId) => WorkflowService.SelectNode(id, projectId),
  updateNodeParams: (id, title, params, projectId) => WorkflowService.UpdateNodeParams(id, title, params, projectId),
  getTopicCandidates: async (nodeId, projectId) => (await WorkflowService.GetTopicCandidates(nodeId, projectId)) ?? [],
}

export type TopicCandidateFixture = Pick<TopicCandidate, 'title'> & Partial<Pick<TopicCandidate, 'index' | 'angle' | 'hook'>>
