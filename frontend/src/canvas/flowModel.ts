import { MarkerType, type Edge, type Node as FlowNode } from '@xyflow/react'
import type { CandidateSource, Edge as ProjectEdge, Node, NodeParams, PendingSummary, RunStartResult, ScriptVersionDetail, WorkspaceSnapshot } from '../../bindings/audio-ui/internal/domain/entity'
import { OutputKind, PendingStatus } from '../../bindings/audio-ui/internal/domain/valueobject'
import type { TopicCandidateFixture } from './graphApi'
import type { CanvasEdgeData } from './edgeRouting'
import { canvasNodeRects } from './nodeRects'

export const COLLAPSED_NODE_HEIGHT = 56

export type ChildNodeType = 'prompt' | 'agent' | 'script'

export type CanvasNodeData = {
  node: Node
  runningLocked: boolean
  pathHighlighted: boolean
  hasInputs: boolean
  topicCandidateIndexes: number[]
  candidateNodeIds: Record<number, string>
  candidates: TopicCandidateFixture[]
  candidate?: TopicCandidateFixture
  candidateError?: string
  candidateLoading?: boolean
  outputScript?: Node
  outputAmbiguous?: boolean
  mutationsDisabled: boolean
  paramsEditingDisabled?: boolean
  projectId?: string
  runStatus?: string
  pendingCount?: number
  pendingResultIds?: string[]
  pendingOutputCount?: number
  pendingOutputIds?: string[]
  globalRunBusy?: boolean
  runSubmitting?: boolean
  onUpdateTopicParams?: (nodeId: string, title: string, params: NodeParams, projectId: string) => Promise<boolean | void>
  onUpdateNodeParams?: (nodeId: string, title: string, params: NodeParams, projectId: string) => Promise<boolean | void>
  onStartRun?: (nodeId: string, projectId: string) => Promise<RunStartResult>
  onGetScriptVersion?: (nodeId: string, version: number, projectId: string) => Promise<ScriptVersionDetail>
  onRegisterPromptFlush?: (nodeId: string, projectId: string, flush: () => Promise<boolean>) => () => void
  onRegisterTopicFlush?: (nodeId: string, projectId: string, flush: () => Promise<boolean>) => () => void
  onNodeHandlesMeasured?: (nodeId: string, layoutKey: string, projectId: string) => void
  onSetNodeLayout?: (nodeId: string, width: number, height: number) => void
  onSetNodeCollapsed?: (nodeId: string, collapsed: boolean) => void
  onLocateNode?: (nodeId: string) => void
  onAddChild: (nodeId: string, type: ChildNodeType) => void
}

export type CanvasFlowNode = FlowNode<CanvasNodeData, 'topic' | 'candidate' | 'prompt' | 'agent' | 'script'>

export function deriveHighlightedPath(snapshot: WorkspaceSnapshot): Set<string> {
  const activeNodeId = snapshot.project?.activeNodeId
  if (!activeNodeId) return new Set()
  const path = snapshot.nodePaths?.[activeNodeId]
  if (path?.length) return new Set(path)
  const leaf = (snapshot.chains ?? []).find((item) => item.tailNodeId === activeNodeId && item.ambiguous)
  return new Set(leaf?.nodeIds?.length ? leaf.nodeIds : [activeNodeId])
}

type CanvasCallbacks = {
  paramsEditingDisabled?: boolean
  runSubmittingNodeId?: string
  candidateErrors?: Record<string, string>
  candidateLoading?: Record<string, boolean>
  onUpdateTopicParams?: CanvasNodeData['onUpdateTopicParams']
  onUpdateNodeParams?: CanvasNodeData['onUpdateNodeParams']
  onStartRun?: CanvasNodeData['onStartRun']
  onGetScriptVersion?: CanvasNodeData['onGetScriptVersion']
  onRegisterPromptFlush?: CanvasNodeData['onRegisterPromptFlush']
  onRegisterTopicFlush?: CanvasNodeData['onRegisterTopicFlush']
  onNodeHandlesMeasured?: CanvasNodeData['onNodeHandlesMeasured']
  onSetNodeLayout?: CanvasNodeData['onSetNodeLayout']
  onSetNodeCollapsed?: CanvasNodeData['onSetNodeCollapsed']
  onLocateNode?: CanvasNodeData['onLocateNode']
}

export function candidateAt(candidates: TopicCandidateFixture[] | undefined, index: number | null | undefined): TopicCandidateFixture | undefined {
  if (index === null || index === undefined) return undefined
  return candidates?.find((candidate, position) => (candidate.index ?? position) === index)
}

export function inputHandleId(node: Node): string | null {
  switch (node.type) {
    case 'topic': return 'topic-input'
    case 'candidate': return 'candidate-input'
    case 'prompt': return 'prompt-input'
    case 'agent': return 'agent-input'
    case 'script': return 'script-input'
    default: return null
  }
}

export function outputHandleId(node: Node): string | null {
  switch (node.type) {
    case 'topic': return 'topic-output'
    case 'candidate': return 'candidate-output'
    case 'prompt': return 'prompt-output'
    case 'agent': return 'agent-output'
    case 'script': return 'script-output'
    default: return null
  }
}

function edgeSourceHandle(edge: ProjectEdge, parent: Node, child: Node): string | null {
  if (edge.kind === 'candidate_source') {
    if (parent.type !== 'topic' || child.type !== 'candidate' || !child.candidateSource) return null
    return `topic-candidate-${child.candidateSource.index}`
  }
  return outputHandleId(parent)
}

export function pendingOwnerNodeId(pending: PendingSummary, nodes: Node[]): string | null {
  const byId = new Map(nodes.map((node) => [node.id, node]))
  const source = byId.get(pending.nodeId)
  if (pending.kind === OutputKind.OutputKindTopics) return source?.type === 'topic' ? source.id : null
  if (pending.kind === OutputKind.OutputKindScript) {
    const targetId = pending.resultNodeId || (pending.outputBinding?.bound ? pending.outputBinding.nodeId : '')
    const target = targetId ? byId.get(targetId) : undefined
    if (target?.type === 'script') return target.id
    return source?.type === 'agent' ? source.id : null
  }
  return source?.type === 'topic' || source?.type === 'agent' ? source.id : null
}

export function nodeHandleLayoutKey(type: CanvasFlowNode['type'], data: CanvasNodeData): string {
  if (type === 'topic') {
    return JSON.stringify({
      type,
      collapsed: Boolean(data.node.collapsed),
      candidates: data.topicCandidateIndexes.map((index) => {
        const candidate = candidateAt(data.candidates, index)
        return { index, title: candidate?.title ?? '', angle: candidate?.angle ?? '', hook: candidate?.hook ?? '' }
      }),
    })
  }
  if (type === 'candidate') {
    return JSON.stringify({
      type,
      collapsed: Boolean(data.node.collapsed),
      sourceIndex: data.node.candidateSource?.index,
      title: data.candidate?.title ?? data.node.title,
      angle: data.candidate?.angle ?? '',
      hook: data.candidate?.hook ?? '',
    })
  }
  if (type === 'script') {
    return JSON.stringify({ type, collapsed: Boolean(data.node.collapsed), versions: data.node.scriptVersions ?? [] })
  }
  return JSON.stringify({ type, collapsed: Boolean(data.node.collapsed) })
}

export function buildFlowEdges(
  nodes: CanvasFlowNode[],
  projectEdges: ProjectEdge[],
  measuredHandleLayouts?: Record<string, string>,
): Edge<CanvasEdgeData, 'workspace'>[] {
  const byId = new Map(nodes.map((node) => [node.id, node]))
  const obstacles = canvasNodeRects(nodes)
  return projectEdges.flatMap((edge) => {
    const parentFlowNode = byId.get(edge.sourceNodeId)
    const childFlowNode = byId.get(edge.targetNodeId)
    if (!parentFlowNode || !childFlowNode) return []
    const child = childFlowNode.data.node
    const sourceHandle = edgeSourceHandle(edge, parentFlowNode.data.node, child)
    const targetHandle = inputHandleId(child)
    if (!sourceHandle || !targetHandle) return []
    if (measuredHandleLayouts && (
      measuredHandleLayouts[parentFlowNode.id] !== nodeHandleLayoutKey(parentFlowNode.type, parentFlowNode.data)
      || measuredHandleLayouts[childFlowNode.id] !== nodeHandleLayoutKey(childFlowNode.type, childFlowNode.data)
    )) return []
    const highlightedEdge = parentFlowNode.data.pathHighlighted && childFlowNode.data.pathHighlighted
    const runningLockedEdge = parentFlowNode.data.runningLocked && childFlowNode.data.runningLocked
    const className = [
      'workspace-edge',
      highlightedEdge && 'workspace-edge-highlighted',
      runningLockedEdge && 'workspace-edge-running-locked',
    ].filter(Boolean).join(' ')
    return [{
      id: edge.id,
      source: parentFlowNode.id,
      sourceHandle,
      target: childFlowNode.id,
      targetHandle,
      type: 'workspace',
      selectable: false,
      focusable: false,
      interactionWidth: 20,
      className,
      markerEnd: {
        type: MarkerType.ArrowClosed,
        color: runningLockedEdge ? 'var(--warn)' : highlightedEdge ? 'var(--accent)' : 'var(--edge)',
      },
      data: { sourceNodeId: parentFlowNode.id, targetNodeId: childFlowNode.id, obstacles },
      ariaLabel: `${parentFlowNode.data.node.title} 到 ${child.title}`,
    }]
  })
}

export function buildFlowModel(
  snapshot: WorkspaceSnapshot,
  onAddChild: CanvasNodeData['onAddChild'],
  candidateFixtures: Record<string, TopicCandidateFixture[]> = {},
  mutationsDisabled = false,
  callbacks: CanvasCallbacks = {},
): { nodes: CanvasFlowNode[]; edges: Edge[] } {
  const locked = new Set(snapshot.lockedNodeIds ?? [])
  const submittingNodeId = callbacks.runSubmittingNodeId
  if (submittingNodeId) {
    locked.add(submittingNodeId)
    for (const nodeId of snapshot.nodePaths?.[submittingNodeId] ?? []) locked.add(nodeId)
  }
  const highlighted = deriveHighlightedPath(snapshot)
  const projectNodes = snapshot.nodes ?? []
  const projectEdges = snapshot.edges ?? []
  const projectNodeById = new Map(projectNodes.map((node) => [node.id, node]))
  const projectId = snapshot.project?.id ?? ''
  const candidateIndexesByTopic = new Map<string, number[]>()
  const candidateNodeIdsByTopic = new Map<string, Record<number, string>>()
  const incomingCount = new Map<string, number>()
  const scriptsByAgent = new Map<string, Node[]>()
  for (const edge of projectEdges) {
    incomingCount.set(edge.targetNodeId, (incomingCount.get(edge.targetNodeId) ?? 0) + 1)
    const sourceNode = projectNodeById.get(edge.sourceNodeId)
    const targetNode = projectNodeById.get(edge.targetNodeId)
    if (edge.kind === 'candidate_source' && sourceNode?.type === 'topic'
      && targetNode?.type === 'candidate' && targetNode.candidateSource?.topicNodeId === sourceNode.id) {
      const indexes = candidateIndexesByTopic.get(sourceNode.id) ?? []
      indexes.push(targetNode.candidateSource.index)
      candidateIndexesByTopic.set(sourceNode.id, indexes)
      const candidateNodeIds = candidateNodeIdsByTopic.get(sourceNode.id) ?? {}
      candidateNodeIds[targetNode.candidateSource.index] = targetNode.id
      candidateNodeIdsByTopic.set(sourceNode.id, candidateNodeIds)
    }
    if (sourceNode?.type === 'agent' && targetNode?.type === 'script') {
      const scripts = scriptsByAgent.get(sourceNode.id) ?? []
      scripts.push(targetNode)
      scriptsByAgent.set(sourceNode.id, scripts)
    }
  }
  for (const indexes of candidateIndexesByTopic.values()) indexes.sort((left, right) => left - right)
  const outputForAgent = (id: string): Node | undefined => {
    const scripts = scriptsByAgent.get(id) ?? []
    return scripts.length === 1 && incomingCount.get(scripts[0].id) === 1 ? scripts[0] : undefined
  }
  if (submittingNodeId && projectNodeById.get(submittingNodeId)?.type === 'agent') {
    const outputScript = outputForAgent(submittingNodeId)
    if (outputScript) locked.add(outputScript.id)
  }

  const latestRunByNode = new Map<string, { status: string; startedAt: string }>()
  for (const run of snapshot.runs ?? []) {
    const previous = latestRunByNode.get(run.nodeId)
    const startedAt = run.startedAt ?? ''
    if (!previous || startedAt >= previous.startedAt) latestRunByNode.set(run.nodeId, { status: run.status, startedAt })
  }
  const pendingCountByNode = new Map<string, number>()
  const pendingResultIdsByNode = new Map<string, string[]>()
  const pendingOutputIdsByScript = new Map<string, string[]>()
  for (const pending of snapshot.pending ?? []) {
    if (pending.status !== PendingStatus.PendingPending && pending.status !== PendingStatus.PendingInvalid) continue
    const ownerId = pendingOwnerNodeId(pending, projectNodes)
    if (!ownerId) continue
    const ids = pendingResultIdsByNode.get(ownerId) ?? []
    ids.push(pending.id)
    pendingResultIdsByNode.set(ownerId, ids)
    pendingCountByNode.set(ownerId, ids.length)
    if (pending.kind === OutputKind.OutputKindScript && projectNodeById.get(ownerId)?.type === 'script') {
      pendingOutputIdsByScript.set(ownerId, ids)
    }
  }
  const globalRunBusy = Boolean(callbacks.runSubmittingNodeId)
    || (snapshot.runs ?? []).some((run) => run.status === 'starting' || run.status === 'running')
  const nodes: CanvasFlowNode[] = projectNodes.map((node) => {
    const source: CandidateSource | null | undefined = node.candidateSource
    const candidateTopicId = source?.topicNodeId ?? ''
    return {
      id: node.id,
      type: node.type as CanvasFlowNode['type'],
      dragHandle: '.canvas-node-head',
      position: { x: node.position.x, y: node.position.y },
      style: { width: node.size.width, height: node.collapsed ? COLLAPSED_NODE_HEIGHT : node.size.height },
      selected: node.id === snapshot.project?.activeNodeId,
      data: {
        node,
        runningLocked: locked.has(node.id),
        pathHighlighted: highlighted.has(node.id),
        hasInputs: (incomingCount.get(node.id) ?? 0) > 0,
        topicCandidateIndexes: candidateIndexesByTopic.get(node.id) ?? [],
        candidateNodeIds: candidateNodeIdsByTopic.get(node.id) ?? {},
        candidates: candidateFixtures[node.id] ?? [],
        candidate: source ? candidateAt(candidateFixtures[candidateTopicId], source.index) : undefined,
        candidateError: candidateTopicId ? callbacks.candidateErrors?.[candidateTopicId] : callbacks.candidateErrors?.[node.id],
        candidateLoading: candidateTopicId ? callbacks.candidateLoading?.[candidateTopicId] : callbacks.candidateLoading?.[node.id] ?? false,
        outputScript: node.type === 'agent' ? outputForAgent(node.id) : undefined,
        outputAmbiguous: node.type === 'agent' && (scriptsByAgent.get(node.id)?.length ?? 0) > 0 && !outputForAgent(node.id),
        mutationsDisabled,
        paramsEditingDisabled: callbacks.paramsEditingDisabled ?? mutationsDisabled,
        projectId,
        runStatus: latestRunByNode.get(node.id)?.status,
        pendingCount: pendingCountByNode.get(node.id) ?? 0,
        pendingResultIds: pendingResultIdsByNode.get(node.id) ?? [],
        pendingOutputCount: pendingOutputIdsByScript.get(node.id)?.length ?? 0,
        pendingOutputIds: pendingOutputIdsByScript.get(node.id) ?? [],
        globalRunBusy,
        runSubmitting: callbacks.runSubmittingNodeId === node.id,
        onUpdateTopicParams: callbacks.onUpdateTopicParams,
        onUpdateNodeParams: callbacks.onUpdateNodeParams,
        onStartRun: callbacks.onStartRun,
        onGetScriptVersion: callbacks.onGetScriptVersion,
        onRegisterPromptFlush: callbacks.onRegisterPromptFlush,
        onRegisterTopicFlush: callbacks.onRegisterTopicFlush,
        onNodeHandlesMeasured: callbacks.onNodeHandlesMeasured,
        onSetNodeLayout: callbacks.onSetNodeLayout,
        onSetNodeCollapsed: callbacks.onSetNodeCollapsed,
        onLocateNode: callbacks.onLocateNode,
        onAddChild,
      },
    }
  })

  const edges = buildFlowEdges(nodes, projectEdges)
  return { nodes, edges }
}

export function nodeMap(nodes: Node[]): Map<string, Node> {
  return new Map(nodes.map((node) => [node.id, node]))
}
