import { describe, expect, it, vi } from 'vitest'
import type { Node, WorkspaceSnapshot } from '../../bindings/audio-ui/internal/domain/entity'
import { OutputKind, PendingStatus, RunStatus } from '../../bindings/audio-ui/internal/domain/valueobject'
import { buildFlowEdges, buildFlowModel, deriveHighlightedPath, nodeHandleLayoutKey, subtreeSize, topicRootId } from './flowModel'

function makeNode(id: string, type: 'topic' | 'prompt', parent = '', parentCandidate?: number, overrides: Partial<Node> = {}): Node {
  return {
    id,
    type: type as Node['type'],
    title: id,
    parent,
    parentCandidate,
    position: { x: 0, y: 0 },
    params: {},
    rounds: [],
    locked: false,
    stale: false,
    ...overrides,
  }
}

function snapshot(nodes: Node[], activeNodeId = 'n_leaf'): WorkspaceSnapshot {
  return {
    project: { id: 'p_test', name: '测试', revision: 1, activeNodeId },
    nodes,
    pending: [],
    runs: [],
    chains: [{ id: 'c_n_leaf', tailNodeId: 'n_leaf', path: ['n_topic', 'n_prompt', 'n_leaf'], label: '链路', status: 'idle' }],
    lockedNodeIds: ['n_prompt'],
  }
}

describe('flowModel', () => {
  it('derives edges from the persisted parent and zero-based topic candidate', () => {
    const topic = makeNode('n_topic', 'topic')
    const prompt = makeNode('n_prompt', 'prompt', topic.id, 0)
    const draft = makeNode('n_draft', 'prompt')
    const model = buildFlowModel(snapshot([topic, prompt, draft]), vi.fn(), {
      [topic.id]: [{ title: '候选甲' }],
    })

    expect(model.edges).toHaveLength(1)
    expect(model.edges[0]).toMatchObject({
      source: topic.id,
      sourceHandle: 'candidate-0',
      target: prompt.id,
      targetHandle: 'prompt-input',
    })
    expect(model.nodes.find((node) => node.id === draft.id)?.data.node.parent).toBeFalsy()
  })

  it('uses the candidate DTO index rather than its array position', () => {
    const topic = makeNode('n_topic', 'topic')
    const child = makeNode('n_child', 'prompt', topic.id, 3)
    const model = buildFlowModel(snapshot([topic, child]), vi.fn(), {
      [topic.id]: [{ index: 3, title: '候选四' }],
    })

    expect(model.edges[0]?.sourceHandle).toBe('candidate-3')
  })

  it('waits for source and target handle layouts before exposing edges', () => {
    const topic = makeNode('n_topic', 'topic')
    const prompt = makeNode('n_prompt', 'prompt', topic.id, 0)
    const model = buildFlowModel(snapshot([topic, prompt]), vi.fn(), {
      [topic.id]: [{ index: 0, title: '候选甲' }],
    })
    const topicFlowNode = model.nodes.find((node) => node.id === topic.id)
    const promptFlowNode = model.nodes.find((node) => node.id === prompt.id)
    if (!topicFlowNode || !promptFlowNode) throw new Error('flow nodes missing from fixture')
    const ready = {
      [topic.id]: nodeHandleLayoutKey(topicFlowNode.type, topicFlowNode.data),
      [prompt.id]: nodeHandleLayoutKey(promptFlowNode.type, promptFlowNode.data),
    }

    expect(buildFlowEdges(model.nodes, {})).toEqual([])
    expect(buildFlowEdges(model.nodes, { [topic.id]: ready[topic.id] })).toEqual([])
    expect(buildFlowEdges(model.nodes, ready)).toHaveLength(1)
    expect(buildFlowEdges(model.nodes, ready)[0]?.sourceHandle).toBe('candidate-0')
  })

  it('keeps edges visible without inventing candidate ports before candidate data is available', () => {
    const topic = makeNode('n_topic', 'topic')
    const child = makeNode('n_child', 'prompt', topic.id, 2)
    const model = buildFlowModel(snapshot([topic, child]), vi.fn())

    expect(model.edges[0]?.sourceHandle).toBe('topic-connection-anchor')
    expect(model.nodes[0]?.data.candidates).toEqual([])
    expect(model.nodes[0]?.data.hasUnmappedChildren).toBe(true)
  })

  it('exposes the latest topic run state and active pending count without copying run rules', () => {
    const topic = makeNode('n_topic', 'topic')
    const view = snapshot([topic])
    view.runs = [
      { id: 'r_old', nodeId: topic.id, status: RunStatus.RunFailed, chainId: 'c_old', chainPath: [topic.id], estimatedTokens: 10, startedAt: '2026-11-03T00:00:00Z' },
      { id: 'r_new', nodeId: topic.id, status: RunStatus.RunSucceeded, chainId: 'c_new', chainPath: [topic.id], estimatedTokens: 10, startedAt: '2026-11-04T00:00:00Z' },
    ]
    view.pending = [{
      id: 'pending_one', nodeId: topic.id, runId: 'r_new', chainId: 'c_new', kind: OutputKind.OutputKindTopics,
      status: PendingStatus.PendingPending,
    }]

    const model = buildFlowModel(view, vi.fn())
    expect(model.nodes[0]?.data.runStatus).toBe('succeeded')
    expect(model.nodes[0]?.data.pendingCount).toBe(1)
    expect(model.nodes[0]?.data.globalRunBusy).toBe(false)
  })

  it('highlights only the backend-provided chain prefix through the active node', () => {
    const graph = snapshot([
      makeNode('n_topic', 'topic'),
      makeNode('n_prompt', 'prompt', 'n_topic', 0),
      makeNode('n_leaf', 'prompt', 'n_prompt'),
      makeNode('n_sibling', 'prompt', 'n_topic', 1),
    ], 'n_prompt')

    expect([...deriveHighlightedPath(graph)]).toEqual(['n_topic', 'n_prompt'])
    const model = buildFlowModel(graph, vi.fn())
    expect(model.nodes.find((node) => node.id === 'n_prompt')?.data).toMatchObject({
      runningLocked: true,
      pathHighlighted: true,
    })
    expect(model.nodes.find((node) => node.id === 'n_sibling')?.data.pathHighlighted).toBe(false)
    expect(model.edges.find((edge) => edge.target === 'n_prompt')?.className).toBe('workspace-edge-highlighted')
  })

  it('reports subtree and topic-tree impact for a reparent confirmation summary', () => {
    const nodes = [
      makeNode('n_topic_a', 'topic'),
      makeNode('n_topic_b', 'topic'),
      makeNode('n_parent', 'prompt', 'n_topic_b', 0),
      makeNode('n_child', 'prompt', 'n_parent'),
      makeNode('n_leaf', 'prompt', 'n_child'),
    ]

    expect(subtreeSize(nodes, 'n_child')).toBe(2)
    expect(topicRootId(nodes, 'n_child')).toBe('n_topic_b')
    expect(topicRootId(nodes, 'n_topic_a')).toBe('n_topic_a')
  })
})
