import type { Edge, Node as FlowNode } from '@xyflow/react'
import type { Node, WorkspaceSnapshot } from '../../bindings/audio-ui/internal/domain/entity'
import type { TopicCandidateFixture } from './graphApi'

export type CanvasNodeData = {
  node: Node
  runningLocked: boolean
  pathHighlighted: boolean
  hasUnmappedChildren: boolean
  candidates: TopicCandidateFixture[]
  mutationsDisabled: boolean
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

export function buildFlowModel(
  snapshot: WorkspaceSnapshot,
  onAddChild: CanvasNodeData['onAddChild'],
  candidateFixtures: Record<string, TopicCandidateFixture[]> = {},
  mutationsDisabled = false,
): { nodes: CanvasFlowNode[]; edges: Edge[] } {
  const locked = new Set(snapshot.lockedNodeIds ?? [])
  const highlighted = deriveHighlightedPath(snapshot)
  const projectNodes = snapshot.nodes ?? []
  const byId = new Map(projectNodes.map((node) => [node.id, node]))
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
        && !candidateFixtures[node.id]?.[child.parentCandidate ?? 0]),
      candidates: candidateFixtures[node.id] ?? [],
      mutationsDisabled,
      onAddChild,
    },
  }))

  const edges: Edge[] = projectNodes.flatMap((child) => {
    if (!child.parent || !byId.has(child.parent)) return []
    const parent = byId.get(child.parent)
    if (!parent) return []
    const sourceHandle = parent.type === 'topic'
      ? candidateFixtures[parent.id]?.[child.parentCandidate ?? 0]
        ? `candidate-${child.parentCandidate ?? 0}`
        : 'topic-connection-anchor'
      : 'prompt-output'
    const highlightedEdge = highlighted.has(parent.id) && highlighted.has(child.id)
    return [{
      id: `edge-${parent.id}-${child.id}`,
      source: parent.id,
      sourceHandle,
      target: child.id,
      targetHandle: 'prompt-input',
      type: 'smoothstep',
      selectable: false,
      focusable: false,
      className: highlightedEdge ? 'workspace-edge-highlighted' : 'workspace-edge',
      ariaLabel: `${parent.title} 到 ${child.title}`,
    }]
  })

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
