import type { Edge, Node as FlowNode } from '@xyflow/react'
import type { Node, NodeParams, RunStartResult, WorkspaceSnapshot } from '../../bindings/audio-ui/internal/domain/entity'
import type { TopicCandidateFixture } from './graphApi'

export type CanvasNodeData = {
  node: Node
  runningLocked: boolean
  pathHighlighted: boolean
  hasUnmappedChildren: boolean
  candidates: TopicCandidateFixture[]
  candidateError?: string
  candidateLoading?: boolean
  mutationsDisabled: boolean
  projectId?: string
  runStatus?: string
  pendingCount?: number
  globalRunBusy?: boolean
  runSubmitting?: boolean
  onUpdateTopicParams?: (nodeId: string, title: string, params: NodeParams, projectId: string) => Promise<boolean | void>
  onStartTopicRun?: (nodeId: string, projectId: string) => Promise<RunStartResult>
  onNodeHandlesMeasured?: (nodeId: string, layoutKey: string, projectId: string) => void
  onAddChild: (nodeId: string, candidate: number | null) => void
}

export type CanvasFlowNode = FlowNode<CanvasNodeData, 'topic' | 'prompt'>

export function deriveHighlightedPath(snapshot: WorkspaceSnapshot): Set<string> {
  const activeNodeId = snapshot.project?.activeNodeId
  if (!activeNodeId) return new Set()
  const chain = (snapshot.chains ?? []).find((item) => item.path?.includes(activeNodeId))
  if (chain?.path) return new Set(chain.path.slice(0, chain.path.indexOf(activeNodeId) + 1))
  return new Set([activeNodeId])
}

type TopicCallbacks = {
  runSubmittingNodeId?: string
  candidateErrors?: Record<string, string>
  candidateLoading?: Record<string, boolean>
  onUpdateTopicParams?: CanvasNodeData['onUpdateTopicParams']
  onStartTopicRun?: CanvasNodeData['onStartTopicRun']
  onNodeHandlesMeasured?: CanvasNodeData['onNodeHandlesMeasured']
}

export function candidateAt(candidates: TopicCandidateFixture[] | undefined, index: number | null | undefined): TopicCandidateFixture | undefined {
  if (index === null || index === undefined) return undefined
  return candidates?.find((candidate, position) => (candidate.index ?? position) === index)
}

export function nodeHandleLayoutKey(type: CanvasFlowNode['type'], data: CanvasNodeData): string {
  if (type === 'topic') {
    return JSON.stringify({
      type,
      candidates: data.candidates.map((candidate, position) => ({
        index: candidate.index ?? position,
        title: candidate.title,
        angle: candidate.angle ?? '',
        hook: candidate.hook ?? '',
      })),
      hasUnmappedChildren: data.hasUnmappedChildren,
      candidateError: data.candidateError ?? '',
      candidateLoading: Boolean(data.candidateLoading),
    })
  }
  return JSON.stringify({ type, hasOutput: Boolean(data.node.parent) })
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
    const sourceHandle = parentFlowNode.type === 'topic'
      ? candidateAt(parentFlowNode.data.candidates, child.parentCandidate)
        ? `candidate-${child.parentCandidate}`
        : 'topic-connection-anchor'
      : 'prompt-output'
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
      targetHandle: 'prompt-input',
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
  topicCallbacks: TopicCallbacks = {},
): { nodes: CanvasFlowNode[]; edges: Edge[] } {
  const locked = new Set(snapshot.lockedNodeIds ?? [])
  const highlighted = deriveHighlightedPath(snapshot)
  const projectNodes = snapshot.nodes ?? []
  const projectId = snapshot.project?.id ?? ''
  const latestRunByNode = new Map<string, { status: string; startedAt: string }>()
  for (const run of snapshot.runs ?? []) {
    const previous = latestRunByNode.get(run.nodeId)
    const startedAt = run.startedAt ?? ''
    if (!previous || startedAt >= previous.startedAt) latestRunByNode.set(run.nodeId, { status: run.status, startedAt })
  }
  const pendingCountByNode = new Map<string, number>()
  for (const pending of snapshot.pending ?? []) {
    if (pending.status !== 'pending' && pending.status !== 'invalid') continue
    pendingCountByNode.set(pending.nodeId, (pendingCountByNode.get(pending.nodeId) ?? 0) + 1)
  }
  const globalRunBusy = Boolean(topicCallbacks.runSubmittingNodeId)
    || (snapshot.runs ?? []).some((run) => run.status === 'starting' || run.status === 'running')
  const nodes: CanvasFlowNode[] = projectNodes.map((node) => ({
    id: node.id,
    type: node.type as CanvasFlowNode['type'],
    position: { x: node.position.x, y: node.position.y },
    selected: node.id === snapshot.project?.activeNodeId,
    data: {
      node,
      runningLocked: locked.has(node.id),
      pathHighlighted: highlighted.has(node.id),
      hasUnmappedChildren: projectNodes.some((child) => child.parent === node.id
        && !candidateAt(candidateFixtures[node.id], child.parentCandidate)),
      candidates: candidateFixtures[node.id] ?? [],
      candidateError: topicCallbacks.candidateErrors?.[node.id],
      candidateLoading: topicCallbacks.candidateLoading?.[node.id] ?? false,
      mutationsDisabled,
      projectId,
      runStatus: latestRunByNode.get(node.id)?.status,
      pendingCount: pendingCountByNode.get(node.id) ?? 0,
      globalRunBusy,
      runSubmitting: topicCallbacks.runSubmittingNodeId === node.id,
      onUpdateTopicParams: topicCallbacks.onUpdateTopicParams,
      onStartTopicRun: topicCallbacks.onStartTopicRun,
      onNodeHandlesMeasured: topicCallbacks.onNodeHandlesMeasured,
      onAddChild,
    },
  }))

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
