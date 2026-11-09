import type { Edge, Node as FlowNode } from '@xyflow/react'
import type { CandidateSource, Node, NodeParams, PendingSummary, RunStartResult, ScriptVersionDetail, WorkspaceSnapshot } from '../../bindings/audio-ui/internal/domain/entity'
import { OutputKind, PendingStatus } from '../../bindings/audio-ui/internal/domain/valueobject'
import type { TopicCandidateFixture } from './graphApi'

export type ChildNodeType = 'prompt' | 'agent' | 'script'

export type CanvasNodeData = {
  node: Node
  runningLocked: boolean
  pathHighlighted: boolean
  topicCandidateIndexes: number[]
  candidates: TopicCandidateFixture[]
  candidate?: TopicCandidateFixture
  candidateError?: string
  candidateLoading?: boolean
  outputScript?: Node
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
  onAddChild: (nodeId: string, type: ChildNodeType) => void
}

export type CanvasFlowNode = FlowNode<CanvasNodeData, 'topic' | 'candidate' | 'prompt' | 'agent' | 'script'>

export function deriveHighlightedPath(snapshot: WorkspaceSnapshot): Set<string> {
  const activeNodeId = snapshot.project?.activeNodeId
  if (!activeNodeId) return new Set()
  const chain = (snapshot.chains ?? []).find((item) => item.path?.includes(activeNodeId))
  if (chain?.path) return new Set(chain.path.slice(0, chain.path.indexOf(activeNodeId) + 1))
  return new Set([activeNodeId])
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
}

export function candidateAt(candidates: TopicCandidateFixture[] | undefined, index: number | null | undefined): TopicCandidateFixture | undefined {
  if (index === null || index === undefined) return undefined
  return candidates?.find((candidate, position) => (candidate.index ?? position) === index)
}

export function inputHandleId(node: Node): string | null {
  switch (node.type) {
    case 'candidate': return 'candidate-input'
    case 'prompt': return 'prompt-input'
    case 'agent': return 'agent-input'
    case 'script': return 'script-input'
    default: return null
  }
}

export function outputHandleId(node: Node): string | null {
  switch (node.type) {
    case 'candidate': return 'candidate-output'
    case 'prompt': return node.parent ? 'prompt-output' : null
    case 'agent': return node.parent ? 'agent-output' : null
    case 'script': return node.parent ? 'script-output' : null
    default: return null
  }
}

function edgeSourceHandle(parent: Node, child: Node): string | null {
  if (parent.type === 'topic') {
    if (child.type !== 'candidate' || !child.candidateSource) return null
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
      candidates: data.topicCandidateIndexes.map((index) => {
        const candidate = candidateAt(data.candidates, index)
        return { index, title: candidate?.title ?? '', angle: candidate?.angle ?? '', hook: candidate?.hook ?? '' }
      }),
    })
  }
  if (type === 'candidate') {
    return JSON.stringify({
      type,
      sourceIndex: data.node.candidateSource?.index,
      title: data.candidate?.title ?? data.node.title,
      angle: data.candidate?.angle ?? '',
      hook: data.candidate?.hook ?? '',
    })
  }
  if (type === 'script') {
    return JSON.stringify({ type, hasParent: Boolean(data.node.parent), versions: data.node.scriptVersions ?? [] })
  }
  return JSON.stringify({ type, hasParent: Boolean(data.node.parent) })
}

export function buildFlowEdges(
  nodes: CanvasFlowNode[],
  measuredHandleLayouts?: Record<string, string>,
): Edge[] {
  const byId = new Map(nodes.map((node) => [node.id, node]))
  return nodes.flatMap((childFlowNode) => {
    const child = childFlowNode.data.node
    if (!child.parent) return []
    const parentFlowNode = byId.get(child.parent)
    if (!parentFlowNode) return []
    const sourceHandle = edgeSourceHandle(parentFlowNode.data.node, child)
    const targetHandle = inputHandleId(child)
    if (!sourceHandle || !targetHandle) return []
    if (measuredHandleLayouts && (
      measuredHandleLayouts[parentFlowNode.id] !== nodeHandleLayoutKey(parentFlowNode.type, parentFlowNode.data)
      || measuredHandleLayouts[childFlowNode.id] !== nodeHandleLayoutKey(childFlowNode.type, childFlowNode.data)
    )) return []
    const highlightedEdge = parentFlowNode.data.pathHighlighted && childFlowNode.data.pathHighlighted
    return [{
      id: `edge-${parentFlowNode.id}-${childFlowNode.id}`,
      source: parentFlowNode.id,
      sourceHandle,
      target: childFlowNode.id,
      targetHandle,
      type: 'smoothstep',
      selectable: false,
      focusable: false,
      className: highlightedEdge ? 'workspace-edge-highlighted' : 'workspace-edge',
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
  const submittingPath = submittingNodeId
    ? (snapshot.chains ?? []).find((chain) => chain.path?.includes(submittingNodeId))?.path ?? []
    : []
  const submittingIndex = submittingNodeId ? submittingPath.indexOf(submittingNodeId) : -1
  for (const nodeId of submittingIndex < 0 ? [] : submittingPath.slice(0, submittingIndex + 1)) locked.add(nodeId)
  const highlighted = deriveHighlightedPath(snapshot)
  const projectNodes = snapshot.nodes ?? []
  const projectId = snapshot.project?.id ?? ''
  const candidateIndexesByTopic = new Map<string, number[]>()
  const scriptByAgent = new Map<string, Node>()
  for (const node of projectNodes) {
    if (node.type === 'candidate' && node.parent && node.candidateSource?.topicNodeId === node.parent) {
      const indexes = candidateIndexesByTopic.get(node.parent) ?? []
      indexes.push(node.candidateSource.index)
      candidateIndexesByTopic.set(node.parent, indexes)
    }
    if (node.type === 'script' && node.parent) scriptByAgent.set(node.parent, node)
  }
  for (const indexes of candidateIndexesByTopic.values()) indexes.sort((left, right) => left - right)
  if (projectNodes.find((node) => node.id === submittingNodeId)?.type === 'agent') {
    const outputScript = scriptByAgent.get(submittingNodeId ?? '')
    if (outputScript) locked.add(outputScript.id)
  }

  const latestRunByNode = new Map<string, { status: string; startedAt: string }>()
  for (const run of snapshot.runs ?? []) {
    const previous = latestRunByNode.get(run.nodeId)
    const startedAt = run.startedAt ?? ''
    if (!previous || startedAt >= previous.startedAt) latestRunByNode.set(run.nodeId, { status: run.status, startedAt })
  }
  const projectNodeById = new Map(projectNodes.map((node) => [node.id, node]))
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
      style: { width: node.size.width, height: node.size.height },
      selected: node.id === snapshot.project?.activeNodeId,
      data: {
        node,
        runningLocked: locked.has(node.id),
        pathHighlighted: highlighted.has(node.id),
        topicCandidateIndexes: candidateIndexesByTopic.get(node.id) ?? [],
        candidates: candidateFixtures[node.id] ?? [],
        candidate: source ? candidateAt(candidateFixtures[candidateTopicId], source.index) : undefined,
        candidateError: candidateTopicId ? callbacks.candidateErrors?.[candidateTopicId] : callbacks.candidateErrors?.[node.id],
        candidateLoading: candidateTopicId ? callbacks.candidateLoading?.[candidateTopicId] : callbacks.candidateLoading?.[node.id] ?? false,
        outputScript: node.type === 'agent' ? scriptByAgent.get(node.id) : undefined,
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
        onAddChild,
      },
    }
  })

  const edges = buildFlowEdges(nodes)
  return { nodes, edges }
}

export function nodeMap(nodes: Node[]): Map<string, Node> {
  return new Map(nodes.map((node) => [node.id, node]))
}

export function subtreeSize(nodes: Node[], rootId: string): number {
  const children = new Map<string, string[]>()
  for (const node of nodes) {
    if (!node.parent) continue
    const group = children.get(node.parent) ?? []
    group.push(node.id)
    children.set(node.parent, group)
  }
  const visited = new Set<string>()
  const visit = (id: string) => {
    if (visited.has(id)) return
    visited.add(id)
    for (const childId of children.get(id) ?? []) visit(childId)
  }
  visit(rootId)
  return visited.size
}

export function topicRootId(nodes: Node[], nodeId: string): string | null {
  const byId = nodeMap(nodes)
  const visited = new Set<string>()
  let current = byId.get(nodeId)
  while (current && !visited.has(current.id)) {
    if (current.type === 'topic') return current.id
    visited.add(current.id)
    current = current.parent ? byId.get(current.parent) : undefined
  }
  return null
}
