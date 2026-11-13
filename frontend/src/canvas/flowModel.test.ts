import { describe, expect, it, vi } from 'vitest'
import type { Edge, Node, WorkspaceSnapshot } from '../../bindings/audio-ui/internal/domain/entity'
import { OutputKind, PendingStatus, RunStatus } from '../../bindings/audio-ui/internal/domain/valueobject'
import { buildFlowEdges, buildFlowModel, deriveHighlightedPath, inputHandleId, nodeHandleLayoutKey, outputHandleId } from './flowModel'

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
  const byId = new Map(nodes.map((node) => [node.id, node]))
  const edges: Edge[] = nodes.filter((node) => node.parent && byId.has(node.parent)).map((node) => ({
    id: `edge-${node.parent}-${node.id}`,
    sourceNodeId: node.parent ?? '',
    targetNodeId: node.id,
    kind: node.candidateSource?.topicNodeId === node.parent ? 'candidate_source' : 'connection',
    scriptVersion: 0,
  }))
  const nodePaths: Record<string, string[]> = {}
  for (const node of nodes) {
    const path: string[] = []
    let current: Node | undefined = node
    while (current && !path.includes(current.id)) {
      path.unshift(current.id)
      current = current.parent ? byId.get(current.parent) : undefined
    }
    nodePaths[node.id] = path
  }
  return {
    project: { id: 'p_test', name: '测试', revision: 1, activeNodeId },
    nodes,
    edges,
    nodePaths,
    pending: [],
    runs: [],
    chains: [{ id: 'c_n_prompt', tailNodeId: 'n_prompt', path: ['n_topic', 'n_candidate', 'n_prompt'], nodeIds: ['n_topic', 'n_candidate', 'n_prompt'], ambiguous: false, label: '链路', status: 'idle' }],
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

  it('uses a compact display height without overwriting persisted node dimensions', () => {
    const expanded = makeNode('n_topic', 'topic', '', { size: { width: 360, height: 280 } })
    const collapsed = { ...expanded, collapsed: true }
    const expandedFlowNode = buildFlowModel(snapshot([expanded]), vi.fn()).nodes[0]
    const collapsedFlowNode = buildFlowModel(snapshot([collapsed]), vi.fn()).nodes[0]

    expect(expandedFlowNode?.style).toEqual({ width: 360, height: 280 })
    expect(collapsedFlowNode?.style).toEqual({ width: 360, height: 56 })
    expect(collapsedFlowNode?.data.node.size).toEqual({ width: 360, height: 280 })
    expect(nodeHandleLayoutKey(expandedFlowNode!.type, expandedFlowNode!.data))
      .not.toBe(nodeHandleLayoutKey(collapsedFlowNode!.type, collapsedFlowNode!.data))
  })

  it('derives all v2 edges and handles from authoritative node relationships', () => {
    const topic = makeNode('n_topic', 'topic')
    const candidate = makeNode('n_candidate', 'candidate', topic.id, {
      candidateSource: { topicNodeId: topic.id, round: 1, artifactId: 'sha256:topics', index: 2, runId: 'r_topics' },
    })
    const prompt = makeNode('n_prompt', 'prompt', candidate.id)
    const agent = makeNode('n_agent', 'agent', prompt.id)
    const script = makeNode('n_script', 'script', agent.id, {
      scriptVersions: [{ n: 1, artifactId: 'sha256:script', runId: 'r_script' }],
    })
    const nextPrompt = makeNode('n_next_prompt', 'prompt', script.id)
    const graph = snapshot([topic, candidate, prompt, agent, script, nextPrompt])
    const model = buildFlowModel(graph, vi.fn(), {
      [topic.id]: [{ nodeId: candidate.id, round: 1, runId: 'r_topics', index: 2, title: '候选三', angle: '角度', hook: '开场' }],
    })

    expect(model.nodes.map((node) => node.type)).toEqual(['topic', 'candidate', 'prompt', 'agent', 'script', 'prompt'])
    expect(model.edges.map((edge) => [edge.sourceHandle, edge.targetHandle])).toEqual([
      ['topic-candidate-n_candidate', 'candidate-input'],
      ['candidate-output', 'prompt-input'],
      ['prompt-output', 'agent-input'],
      ['agent-output', 'script-input'],
      ['script-output', 'prompt-input'],
    ])
    expect(model.edges[0]).toMatchObject({
      type: 'workspace',
      className: 'workspace-edge workspace-edge-highlighted',
      interactionWidth: 20,
      markerEnd: { type: 'arrowclosed', color: 'var(--accent)' },
    })
    expect(model.edges[0]?.data?.obstacles).toContainEqual({
      nodeId: candidate.id, x: 0, y: 0, width: candidate.size.width, height: candidate.size.height,
    })
    expect(model.nodes.find((node) => node.id === candidate.id)?.data.candidate).toEqual({
      nodeId: candidate.id, round: 1, runId: 'r_topics', index: 2, title: '候选三', angle: '角度', hook: '开场',
    })
    expect(model.nodes.find((node) => node.id === agent.id)?.data.outputScript?.id).toBe(script.id)
  })

  it('keeps duplicate candidate indexes distinct by batch and node identity', () => {
    const topic = makeNode('n_topic', 'topic')
    const first = makeNode('n_candidate_first', 'candidate', topic.id, {
      candidateSource: { topicNodeId: topic.id, round: 1, artifactId: 'sha256:topics-1', index: 0, runId: 'r_topics_1' },
    })
    const second = makeNode('n_candidate_second', 'candidate', topic.id, {
      candidateSource: { topicNodeId: topic.id, round: 2, artifactId: 'sha256:topics-2', index: 0, runId: 'r_topics_2' },
    })
    const view = snapshot([topic, first, second])
    const model = buildFlowModel(view, vi.fn(), {
      [topic.id]: [
        { nodeId: second.id, round: 2, index: 0, runId: 'r_topics_2', title: '新候选' },
        { nodeId: first.id, round: 1, index: 0, runId: 'r_topics_1', title: '旧候选' },
      ],
    })
    const topicData = model.nodes.find((node) => node.id === topic.id)?.data

    expect(topicData?.topicCandidates.map((candidate) => candidate.nodeId)).toEqual([first.id, second.id])
    expect(model.nodes.find((node) => node.id === first.id)?.data.candidate?.title).toBe('旧候选')
    expect(model.nodes.find((node) => node.id === second.id)?.data.candidate?.title).toBe('新候选')
    expect(model.edges.map((edge) => edge.sourceHandle)).toEqual([
      `topic-candidate-${first.id}`, `topic-candidate-${second.id}`,
    ])
  })

  it('waits for dynamic topic candidate handles and both endpoint layouts before drawing', () => {
    const topic = makeNode('n_topic', 'topic')
    const candidate = makeNode('n_candidate', 'candidate', topic.id, {
      candidateSource: { topicNodeId: topic.id, round: 1, artifactId: 'sha256:topics', index: 0, runId: 'r_topics' },
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

    expect(buildFlowEdges(model.nodes, graph.edges ?? [], {})).toEqual([])
    expect(buildFlowEdges(model.nodes, graph.edges ?? [], { [topic.id]: ready[topic.id] })).toEqual([])
    expect(buildFlowEdges(model.nodes, graph.edges ?? [], ready)).toHaveLength(1)
    expect(buildFlowEdges(model.nodes, graph.edges ?? [], ready)[0]?.sourceHandle).toBe('topic-candidate-n_candidate')
  })

  it('updates edge obstacles from live node positions and resize dimensions', () => {
    const topic = makeNode('n_topic', 'topic')
    const candidate = makeNode('n_candidate', 'candidate', topic.id, {
      candidateSource: { topicNodeId: topic.id, round: 1, artifactId: 'sha256:topics', index: 0, runId: 'r_topics' },
    })
    const view = snapshot([topic, candidate])
    const model = buildFlowModel(view, vi.fn())
    const resizedNodes = model.nodes.map((node) => node.id === candidate.id
      ? { ...node, position: { x: 500, y: 220 }, style: { ...node.style, width: 420, height: 260 } }
      : node)
    const edge = buildFlowEdges(resizedNodes, view.edges ?? [])[0]

    expect(edge?.data?.obstacles).toContainEqual({
      nodeId: candidate.id, x: 500, y: 220, width: 420, height: 260,
    })
  })

  it('uses live measurements before saved dimensions while a node is resizing', () => {
    const topic = makeNode('n_topic', 'topic')
    const candidate = makeNode('n_candidate', 'candidate', topic.id, {
      candidateSource: { topicNodeId: topic.id, round: 1, artifactId: 'sha256:topics', index: 0, runId: 'r_topics' },
    })
    const view = snapshot([topic, candidate])
    const model = buildFlowModel(view, vi.fn())
    const resizingNodes = model.nodes.map((node) => node.id === candidate.id
      ? { ...node, measured: { width: 420, height: 260 } }
      : node)
    const edge = buildFlowEdges(resizingNodes, view.edges ?? [])[0]

    expect(edge?.data?.obstacles).toContainEqual({
      nodeId: candidate.id, x: 0, y: 0, width: 420, height: 260,
    })
  })

  it('exposes Agent run state and its direct Script binding without deriving extra graph rules', () => {
    const topic = makeNode('n_topic', 'topic')
    const candidate = makeNode('n_candidate', 'candidate', topic.id, {
      candidateSource: { topicNodeId: topic.id, round: 1, artifactId: 'sha256:topics', index: 0, runId: 'r_topics' },
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
      pendingCount: 0,
      pendingResultIds: [],
      globalRunBusy: false,
      outputScript: script,
    })
    expect(model.nodes.find((node) => node.id === script.id)?.data.pendingOutputCount).toBe(1)
  })

  it('keeps pending Script results on the frozen output after the Agent is rebound', () => {
    const topic = makeNode('n_topic', 'topic')
    const candidate = makeNode('n_candidate', 'candidate', topic.id, {
      candidateSource: { topicNodeId: topic.id, round: 1, artifactId: 'sha256:topics', index: 0, runId: 'r_topics' },
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

  it('routes Topic results and unlocated Script results to their source nodes', () => {
    const topic = makeNode('n_topic', 'topic')
    const agent = makeNode('n_agent', 'agent', 'n_prompt')
    const view = snapshot([topic, agent])
    view.pending = [
      {
        id: 'pending_topics', nodeId: topic.id, runId: 'r_topics', chainId: 'c_topic',
        kind: OutputKind.OutputKindTopics, status: PendingStatus.PendingInvalid,
      },
      {
        id: 'pending_script', nodeId: agent.id, runId: 'r_script', chainId: 'c_agent',
        kind: OutputKind.OutputKindScript, status: PendingStatus.PendingInvalid,
        outputBinding: { bound: true, nodeId: 'n_missing_script', parentNodeId: agent.id, baselineVersion: 1 },
        resultNodeId: 'n_missing_script',
      },
    ]

    const model = buildFlowModel(view, vi.fn())
    expect(model.nodes.find((node) => node.id === topic.id)?.data.pendingResultIds).toEqual(['pending_topics'])
    expect(model.nodes.find((node) => node.id === agent.id)?.data.pendingResultIds).toEqual(['pending_script'])
  })

  it('locks only the in-flight target path and its direct Script output', () => {
    const topic = makeNode('n_topic', 'topic')
    const candidate = makeNode('n_candidate', 'candidate', topic.id, {
      candidateSource: { topicNodeId: topic.id, round: 1, artifactId: 'sha256:topics', index: 0, runId: 'r_topics' },
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
      nodeIds: [topic.id, candidate.id, prompt.id, agent.id, script.id, nextPrompt.id],
      ambiguous: false,
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
    expect(model.edges.find((edge) => edge.target === prompt.id)?.className).toContain('workspace-edge-highlighted')
    expect(model.edges.find((edge) => edge.target === prompt.id)?.className).toContain('workspace-edge-running-locked')
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

  it('renders all authoritative edges for multiple upstreams without reading parent', () => {
    const prompt = makeNode('n_prompt', 'prompt', 'n_unrelated')
    const script = makeNode('n_script', 'script')
    const agent = makeNode('n_agent', 'agent', 'n_unrelated')
    const view = snapshot([prompt, script, agent], agent.id)
    view.edges = [
      { id: 'edge_a', sourceNodeId: prompt.id, targetNodeId: agent.id, kind: 'connection', scriptVersion: 0 },
      { id: 'edge_b', sourceNodeId: script.id, targetNodeId: agent.id, kind: 'connection', scriptVersion: 0 },
    ]
    view.nodePaths = {}
    view.chains = [{ id: 'c_n_agent', tailNodeId: agent.id, path: [], nodeIds: [prompt.id, script.id, agent.id], ambiguous: true, label: '汇合', status: 'idle' }]

    const model = buildFlowModel(view, vi.fn())
    expect(model.edges.map((edge) => [edge.id, edge.source, edge.target])).toEqual([
      ['edge_a', prompt.id, agent.id], ['edge_b', script.id, agent.id],
    ])
    expect([...deriveHighlightedPath(view)]).toEqual([prompt.id, script.id, agent.id])
    expect(model.nodes.find((node) => node.id === agent.id)?.data.hasInputs).toBe(true)
  })

  it('offers both handle types on every unconnected node', () => {
    for (const type of ['topic', 'candidate', 'prompt', 'agent', 'script']) {
      const value = makeNode(`n_${type}`, type)
      expect(inputHandleId(value)).toBe(`${type}-input`)
      expect(outputHandleId(value)).toBe(`${type}-output`)
    }
  })

  it('does not guess an output binding when an Agent has multiple Script targets', () => {
    const agent = makeNode('n_agent', 'agent')
    const first = makeNode('n_script_a', 'script')
    const second = makeNode('n_script_b', 'script')
    const view = snapshot([agent, first, second])
    view.edges = [
      { id: 'edge_a', sourceNodeId: agent.id, targetNodeId: first.id, kind: 'connection', scriptVersion: 0 },
      { id: 'edge_b', sourceNodeId: agent.id, targetNodeId: second.id, kind: 'connection', scriptVersion: 0 },
    ]
    const data = buildFlowModel(view, vi.fn()).nodes.find((node) => node.id === agent.id)?.data
    expect(data?.outputScript).toBeUndefined()
    expect(data?.outputAmbiguous).toBe(true)
  })
})
