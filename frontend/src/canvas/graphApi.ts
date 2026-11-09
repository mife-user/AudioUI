import { WorkflowService } from '../../bindings/audio-ui/internal/handler'
import type { AddNodeRequest } from '../../bindings/audio-ui/internal/handler'
import type { GraphMutationReceipt, NodeParams, ScriptVersionDetail, TopicCandidate } from '../../bindings/audio-ui/internal/domain/entity'

export type GraphAPI = {
  addNode: (request: AddNodeRequest) => Promise<GraphMutationReceipt>
  connect: (parentId: string, childId: string, candidate: number | null, replace: boolean, projectId: string) => Promise<GraphMutationReceipt>
  moveNode: (id: string, x: number, y: number, projectId: string) => Promise<GraphMutationReceipt>
  setNodeLayout: (id: string, x: number, y: number, width: number, height: number, projectId: string) => Promise<GraphMutationReceipt>
  selectNode: (id: string, projectId: string) => Promise<GraphMutationReceipt>
  updateNodeParams: (id: string, title: string, params: NodeParams, projectId: string) => Promise<GraphMutationReceipt>
  getTopicCandidates: (nodeId: string, projectId: string) => Promise<TopicCandidate[]>
  getScriptVersion: (nodeId: string, version: number, projectId: string) => Promise<ScriptVersionDetail>
}

export const graphAPI: GraphAPI = {
  addNode: (request) => WorkflowService.AddNode(request),
  connect: (parentId, childId, candidate, replace, projectId) => WorkflowService.Connect(parentId, childId, candidate, replace, projectId),
  moveNode: (id, x, y, projectId) => WorkflowService.MoveNode(id, x, y, projectId),
  setNodeLayout: (id, x, y, width, height, projectId) => WorkflowService.SetNodeLayout(id, x, y, width, height, projectId),
  selectNode: (id, projectId) => WorkflowService.SelectNode(id, projectId),
  updateNodeParams: (id, title, params, projectId) => WorkflowService.UpdateNodeParams(id, title, params, projectId),
  getTopicCandidates: async (nodeId, projectId) => (await WorkflowService.GetTopicCandidates(nodeId, projectId)) ?? [],
  getScriptVersion: (nodeId, version, projectId) => WorkflowService.GetScriptVersion(nodeId, version, projectId),
}

export type TopicCandidateFixture = Pick<TopicCandidate, 'title'> & Partial<Pick<TopicCandidate, 'index' | 'angle' | 'hook'>>
