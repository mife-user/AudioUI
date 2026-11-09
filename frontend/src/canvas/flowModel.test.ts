import { describe, expect, it, vi } from 'vitest'
import type { Node, WorkspaceSnapshot } from '../../bindings/audio-ui/internal/domain/entity'
import { OutputKind, PendingStatus, RunStatus } from '../../bindings/audio-ui/internal/domain/valueobject'
import { buildFlowEdges, buildFlowModel, deriveHighlightedPath, nodeHandleLayoutKey, subtreeSize, topicRootId } from './flowModel'

function makeNode(id: string, type: string, parent = '', overrides: Partial<Node> = {}): Node {
  return {
    id,
    type: type as Node['type'],
    title: id,
    parent,
    position: { x: 0, y: 0 },
    size: { width: 280, height: 230 },
    params: {},
    rounds: [],
    locked: false,
    stale: false,
    ...overrides,
  }
}

function snapshot(nodes: Node[], activeNodeId = 'n_prompt'): WorkspaceSnapshot {
  return {
    project: { id: 'p_test', name: '测试', revision: 1, activeNodeId },
    nodes,
    pending: [],
    runs: [],
    chains: [{ id: 'c_n_prompt', tailNodeId: 'n_prompt', path: ['n_topic', 'n_candidate', 'n_prompt'], label: '链路', status: 'idle' }],
    lockedNodeIds: ['n_prompt'],
  }
}

describe('flowModel', () => {
  it('uses each persisted node size for its React Flow layout', () => {
    const topic = makeNode('n_topic', 'topic', '', { size: { width: 280, height: 250 } })
    const candidate = makeNode('n_candidate', 'candidate', topic.id, { size: { width: 235, height: 160 } })
    const prompt = makeNode('n_prompt', 'prompt', candidate.id, { size: { width: 320, height: 240 } })
    const agent = makeNode('n_agent', 'agent', prompt.id, { size: { width: 235, height: 165 } })
    const script = makeNode('n_script', 'script', agent.id, { size: { width: 270, height: 205 } })
    const model = buildFlowModel(snapshot([topic, candidate, prompt, agent, script]), vi.fn())

    expect(model.nodes.map((node) => node.style)).toEqual([
      { width: 280, height: 250 },
      { width: 235, height: 160 },
      { width: 320, height: 240 },
      { width: 235, height: 165 },
      { width: 270, height: 205 },
    ])
  })

  it('derives all v2 edges and handles from authoritative node relationships', () => {
    const topic = makeNode('n_topic', 'topic')
    const candidate = makeNode('n_candidate', 'candidate', topic.id, {
      candidateSource: { topicNodeId: topic.id, artifactId: 'sha256:topics', index: 2 },
    })
    const prompt = makeNode('n_prompt', 'prompt', candidate.id)
    const agent = makeNode('n_agent', 'agent', prompt.id)
    const script = makeNode('n_script', 'script', agent.id, {
      scriptVersions: [{ n: 1, artifactId: 'sha256:script', runId: 'r_script' }],
    })
    const nextPrompt = makeNode('n_next_prompt', 'prompt', script.id)
    const graph = snapshot([topic, candidate, prompt, agent, script, nextPrompt])
    const model = buildFlowModel(graph, vi.fn(), {
      [topic.id]: [{ index: 2, title: '候选三', angle: '角度', hook: '开场' }],
    })

    expect(model.nodes.map((node) => node.type)).toEqual(['topic', 'candidate', 'prompt', 'agent', 'script', 'prompt'])
    expect(model.edges.map((edge) => [edge.sourceHandle, edge.targetHandle])).toEqual([
      ['topic-candidate-2', 'candidate-input'],
      ['candidate-output', 'prompt-input'],
      ['prompt-output', 'agent-input'],
      ['agent-output', 'script-input'],
      ['script-output', 'prompt-input'],
    ])
    expect(model.nodes.find((node) => node.id === candidate.id)?.data.candidate).toEqual({
      index: 2, title: '候选三', angle: '角度', hook: '开场',
    })
    expect(model.nodes.find((node) => node.id === agent.id)?.data.outputScript?.id).toBe(script.id)
  })

  it('waits for dynamic topic candidate handles and both endpoint layouts before drawing', () => {
    const topic = makeNode('n_topic', 'topic')
    const candidate = makeNode('n_candidate', 'candidate', topic.id, {
      candidateSource: { topicNodeId: topic.id, artifactId: 'sha256:topics', index: 0 },
    })
    const graph = snapshot([topic, candidate])
    const model = buildFlowModel(graph, vi.fn())
    const topicFlowNode = model.nodes.find((node) => node.id === topic.id)
    const candidateFlowNode = model.nodes.find((node) => node.id === candidate.id)
    if (!topicFlowNode || !candidateFlowNode) throw new Error('flow nodes missing from fixture')
    const ready = {
      [topic.id]: nodeHandleLayoutKey(topicFlowNode.type, topicFlowNode.data),
      [candidate.id]: nodeHandleLayoutKey(candidateFlowNode.type, candidateFlowNode.data),
    }

    expect(buildFlowEdges(model.nodes, {})).toEqual([])
    expect(buildFlowEdges(model.nodes, { [topic.id]: ready[topic.id] })).toEqual([])
    expect(buildFlowEdges(model.nodes, ready)).toHaveLength(1)
    expect(buildFlowEdges(model.nodes, ready)[0]?.sourceHandle).toBe('topic-candidate-0')
  })

  it('exposes Agent run state and its direct Script binding without deriving extra graph rules', () => {
    const topic = makeNode('n_topic', 'topic')
    const candidate = makeNode('n_candidate', 'candidate', topic.id, {
      candidateSource: { topicNodeId: topic.id, artifactId: 'sha256:topics', index: 0 },
    })
    const prompt = makeNode('n_prompt', 'prompt', candidate.id)
    const agent = makeNode('n_agent', 'agent', prompt.id)
    const script = makeNode('n_script', 'script', agent.id)
    const view = snapshot([topic, candidate, prompt, agent, script])
    view.runs = [
      { id: 'r_old', nodeId: agent.id, status: RunStatus.RunFailed, chainId: 'c_old', chainPath: [topic.id, candidate.id, prompt.id, agent.id], estimatedTokens: 10, startedAt: '2026-11-03T00:00:00Z' },
      { id: 'r_new', nodeId: agent.id, status: RunStatus.RunSucceeded, chainId: 'c_new', chainPath: [topic.id, candidate.id, prompt.id, agent.id], estimatedTokens: 10, startedAt: '2026-11-04T00:00:00Z' },
    ]
    view.pending = [{
      id: 'pending_one', nodeId: agent.id, runId: 'r_new', chainId: 'c_new', kind: OutputKind.OutputKindScript,
      status: PendingStatus.PendingPending,
      outputBinding: { bound: true, nodeId: script.id, parentNodeId: agent.id, baselineVersion: 0 },
      resultNodeId: script.id,
    }]

    const model = buildFlowModel(view, vi.fn())
    expect(model.nodes.find((node) => node.id === agent.id)?.data).toMatchObject({
      runStatus: 'succeeded',
      pendingCount: 1,
      globalRunBusy: false,
      outputScript: script,
    })
    expect(model.nodes.find((node) => node.id === script.id)?.data.pendingOutputCount).toBe(1)
  })

  it('keeps pending Script results on the frozen output after the Agent is rebound', () => {
    const topic = makeNode('n_topic', 'topic')
    const candidate = makeNode('n_candidate', 'candidate', topic.id, {
      candidateSource: { topicNodeId: topic.id, artifactId: 'sha256:topics', index: 0 },
    })
    const prompt = makeNode('n_prompt', 'prompt', candidate.id)
    const agent = makeNode('n_agent', 'agent', prompt.id)
    const movedAgent = makeNode('n_moved_agent', 'agent', candidate.id)
    const currentScript = makeNode('n_script_current', 'script', agent.id)
    const frozenScript = makeNode('n_script_frozen', 'script', movedAgent.id)
    const view = snapshot([topic, candidate, prompt, agent, movedAgent, currentScript, frozenScript])
    view.pending = [{
      id: 'pending_frozen', nodeId: agent.id, runId: 'r_old', chainId: 'c_old', kind: OutputKind.OutputKindScript,
      status: PendingStatus.PendingPending,
      outputBinding: { bound: true, nodeId: frozenScript.id, parentNodeId: agent.id, baselineVersion: 1 },
      resultNodeId: frozenScript.id,
    }]

    const model = buildFlowModel(view, vi.fn())
    expect(model.nodes.find((node) => node.id === frozenScript.id)?.data.pendingOutputCount).toBe(1)
    expect(model.nodes.find((node) => node.id === frozenScript.id)?.data.pendingOutputIds).toEqual(['pending_frozen'])
    expect(model.nodes.find((node) => node.id === currentScript.id)?.data.pendingOutputCount).toBe(0)
  })

  it('locks only the in-flight target path and its direct Script output', () => {
    const topic = makeNode('n_topic', 'topic')
    const candidate = makeNode('n_candidate', 'candidate', topic.id, {
      candidateSource: { topicNodeId: topic.id, artifactId: 'sha256:topics', index: 0 },
    })
    const prompt = makeNode('n_prompt', 'prompt', candidate.id)
    const agent = makeNode('n_agent', 'agent', prompt.id)
    const script = makeNode('n_script', 'script', agent.id)
    const nextPrompt = makeNode('n_next_prompt', 'prompt', script.id)
    const graph = snapshot([topic, candidate, prompt, agent, script, nextPrompt], agent.id)
    graph.lockedNodeIds = []
    graph.chains = [{
      id: 'c_n_next_prompt',
      tailNodeId: nextPrompt.id,
      path: [topic.id, candidate.id, prompt.id, agent.id, script.id, nextPrompt.id],
      label: '链路',
      status: 'idle',
    }]

    expect([...deriveHighlightedPath(graph)]).toEqual([topic.id, candidate.id, prompt.id, agent.id])
    const model = buildFlowModel(graph, vi.fn(), {}, false, { runSubmittingNodeId: agent.id })
    for (const id of [topic.id, candidate.id, prompt.id, agent.id, script.id]) {
      expect(model.nodes.find((node) => node.id === id)?.data.runningLocked).toBe(true)
    }
    expect(model.nodes.find((node) => node.id === nextPrompt.id)?.data.runningLocked).toBe(false)
    expect(model.nodes.find((node) => node.id === agent.id)?.data.globalRunBusy).toBe(true)
    expect(model.edges.find((edge) => edge.target === prompt.id)?.className).toBe('workspace-edge-highlighted')
  })

  it('changes Script handle measurement keys when immutable version history changes', () => {
    const agent = makeNode('n_agent', 'agent', 'n_prompt')
    const script = makeNode('n_script', 'script', agent.id, {
      scriptVersions: [{ n: 1, artifactId: 'sha256:script-v1', runId: 'r_one' }],
    })
    const before = buildFlowModel(snapshot([agent, script]), vi.fn()).nodes.find((node) => node.id === script.id)
    const after = buildFlowModel(snapshot([agent, {
      ...script,
      scriptVersions: [
        ...(script.scriptVersions ?? []),
        { n: 2, artifactId: 'sha256:script-v2', runId: 'r_two' },
      ],
    }]), vi.fn()).nodes.find((node) => node.id === script.id)
    if (!before || !after) throw new Error('script flow node missing')

    expect(nodeHandleLayoutKey(before.type, before.data)).not.toBe(nodeHandleLayoutKey(after.type, after.data))
  })

  it('reports subtree size and root identity for v2 candidate branches', () => {
    const nodes = [
      makeNode('n_topic', 'topic'),
      makeNode('n_candidate', 'candidate', 'n_topic', {
        candidateSource: { topicNodeId: 'n_topic', artifactId: 'sha256:topics', index: 0 },
      }),
      makeNode('n_prompt', 'prompt', 'n_candidate'),
      makeNode('n_agent', 'agent', 'n_prompt'),
    ]

    expect(subtreeSize(nodes, 'n_prompt')).toBe(2)
    expect(topicRootId(nodes, 'n_agent')).toBe('n_topic')
    expect(topicRootId(nodes, 'n_topic')).toBe('n_topic')
  })
})
