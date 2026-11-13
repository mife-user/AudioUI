import { WorkflowService } from '../../bindings/audio-ui/internal/handler'
import type { AddNodeRequest } from '../../bindings/audio-ui/internal/handler'
import type { GraphMutationReceipt, NodeParams, ScriptVersionDetail } from '../../bindings/audio-ui/internal/domain/entity'

export type GraphAPI = {
  addNode: (request: AddNodeRequest) => Promise<GraphMutationReceipt>
  connect: (sourceId: string, targetId: string, projectId: string) => Promise<GraphMutationReceipt>
  disconnect: (edgeId: string, projectId: string) => Promise<GraphMutationReceipt>
  replaceConnection: (edgeId: string, sourceId: string, projectId: string) => Promise<GraphMutationReceipt>
  moveNode: (id: string, x: number, y: number, projectId: string) => Promise<GraphMutationReceipt>
  setNodeLayout: (id: string, x: number, y: number, width: number, height: number, projectId: string) => Promise<GraphMutationReceipt>
  setNodeCollapsed: (id: string, collapsed: boolean, projectId: string) => Promise<GraphMutationReceipt>
  setEdgeScriptVersion: (edgeId: string, version: number, projectId: string) => Promise<GraphMutationReceipt>
  selectNode: (id: string, projectId: string) => Promise<GraphMutationReceipt>
  updateNodeParams: (id: string, title: string, params: NodeParams, projectId: string) => Promise<GraphMutationReceipt>
  getTopicCandidates: (nodeId: string, projectId: string) => Promise<TopicCandidateFixture[]>
  getScriptVersion: (nodeId: string, version: number, projectId: string) => Promise<ScriptVersionDetail>
}

export const graphAPI: GraphAPI = {
  addNode: (request) => WorkflowService.AddNode(request),
  connect: (sourceId, targetId, projectId) => WorkflowService.Connect(sourceId, targetId, projectId),
  disconnect: (edgeId, projectId) => WorkflowService.Disconnect(edgeId, projectId),
  replaceConnection: (edgeId, sourceId, projectId) => WorkflowService.ReplaceConnection(edgeId, sourceId, projectId),
  moveNode: (id, x, y, projectId) => WorkflowService.MoveNode(id, x, y, projectId),
  setNodeLayout: (id, x, y, width, height, projectId) => WorkflowService.SetNodeLayout(id, x, y, width, height, projectId),
  setNodeCollapsed: (id, collapsed, projectId) => WorkflowService.SetNodeCollapsed(id, collapsed, projectId),
  setEdgeScriptVersion: (edgeId, version, projectId) => WorkflowService.SetEdgeScriptVersion(edgeId, version, projectId),
  selectNode: (id, projectId) => WorkflowService.SelectNode(id, projectId),
  updateNodeParams: (id, title, params, projectId) => WorkflowService.UpdateNodeParams(id, title, params, projectId),
  getTopicCandidates: async (nodeId, projectId) => (await WorkflowService.GetTopicCandidates(nodeId, projectId)) ?? [],
  getScriptVersion: (nodeId, version, projectId) => WorkflowService.GetScriptVersion(nodeId, version, projectId),
}

export type TopicCandidateFixture = {
  nodeId?: string
  round?: number
  runId?: string
  index?: number
  title: string
  angle?: string
  hook?: string
}
