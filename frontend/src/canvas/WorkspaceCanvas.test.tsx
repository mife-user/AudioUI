import { vi } from 'vitest'
import type { MouseEvent as ReactMouseEvent, PropsWithChildren } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import type { Connection, Edge, NodeChange, NodeMouseHandler } from '@xyflow/react'
import type { Node, WorkspaceSnapshot } from '../../bindings/audio-ui/internal/domain/entity'
import { OutputKind } from '../../bindings/audio-ui/internal/domain/valueobject'
import type { GraphAPI } from './graphApi'
import { nodeHandleLayoutKey, type CanvasFlowNode } from './flowModel'
import { WorkspaceCanvas } from './WorkspaceCanvas'

type MockFlowProps = {
  nodes: CanvasFlowNode[]
  edges: Edge[]
  minZoom: number
  maxZoom: number
  onNodesChange: (changes: NodeChange<CanvasFlowNode>[]) => void
  onConnect: (connection: Connection) => void
  onPaneClick: (event: ReactMouseEvent) => void
  onPaneContextMenu: (event: ReactMouseEvent | MouseEvent) => void
  onNodeClick: (event: MouseEvent, node: CanvasFlowNode) => void
  onNodeDoubleClick: NodeMouseHandler<CanvasFlowNode>
  children?: PropsWithChildren['children']
}

const flowMocks = vi.hoisted(() => ({
  fitView: vi.fn(),
  setViewport: vi.fn(),
  getNode: vi.fn((id: string) => ({ id })),
  screenToFlowPosition: vi.fn((position: { x: number; y: number }) => position),
}))

vi.mock('@xyflow/react', async () => {
  const React = await import('react')
  const ReactFlow = (props: MockFlowProps) => {
    React.useEffect(() => {
      for (const node of props.nodes) {
        node.data.onNodeHandlesMeasured?.(node.id, nodeHandleLayoutKey(node.type, node.data), node.data.projectId ?? '')
      }
    }, [props.nodes])
    const invokeAdd = (nodeId: string, type: 'prompt' | 'agent' | 'script') => {
      props.nodes.find((node) => node.id === nodeId)?.data.onAddChild(nodeId, type)
    }
    return React.createElement('div', { 'data-testid': 'mock-flow' },
      React.createElement('div', {
        'data-testid': 'mock-flow-pane',
        className: 'react-flow__pane',
        tabIndex: 0,
        onContextMenu: (event: ReactMouseEvent<HTMLDivElement>) => {
          if (event.target === event.currentTarget) props.onPaneContextMenu(event)
        },
        onClick: (event: ReactMouseEvent<HTMLDivElement>) => {
          if (event.target === event.currentTarget) props.onPaneClick(event)
        },
      }),
      React.createElement('output', { 'data-testid': 'edge-count' }, String(props.edges.length)),
      React.createElement('output', { 'data-testid': 'zoom-range' }, `${props.minZoom}-${props.maxZoom}`),
      React.createElement('output', { 'data-testid': 'child-position' }, (() => {
        const node = props.nodes.find((item) => item.id === 'n_prompt')
        return node ? `${node.position.x},${node.position.y}` : ''
      })()),
      ...props.nodes.map((node) => React.createElement('button', {
        key: node.id,
        type: 'button',
        onClick: (event: MouseEvent) => props.onNodeClick(event, node),
      }, node.data.node.title)),
      React.createElement('button', {
        type: 'button',
        onClick: () => props.onNodesChange([{ id: 'n_prompt', type: 'select', selected: true }]),
      }, 'simulate-select'),
      React.createElement('button', {
        type: 'button',
        onClick: () => {
          const node = props.nodes.find((item) => item.id === 'n_agent')
          if (node) props.onNodeDoubleClick({} as Parameters<NodeMouseHandler<CanvasFlowNode>>[0], node)
        },
      }, 'simulate-double-click'),
      React.createElement('button', {
        type: 'button',
        onClick: () => props.onConnect({
          source: 'n_candidate_a', sourceHandle: 'candidate-output',
          target: 'n_draft', targetHandle: 'prompt-input',
        }),
      }, 'simulate-connect'),
      React.createElement('button', {
        type: 'button',
        onClick: () => props.onConnect({
          source: 'n_prompt', sourceHandle: 'prompt-output',
          target: 'n_draft', targetHandle: 'prompt-input',
        }),
      }, 'simulate-invalid-type-connect'),
      React.createElement('button', {
        type: 'button',
        onClick: () => props.onConnect({
          source: 'n_candidate_a', sourceHandle: 'candidate-output',
          target: 'n_prompt', targetHandle: 'prompt-input',
        }),
      }, 'simulate-reparent'),
      React.createElement('button', {
        type: 'button',
        onClick: () => props.onConnect({
          source: 'n_candidate_a', sourceHandle: 'candidate-output',
          target: 'n_candidate_a', targetHandle: 'candidate-input',
        }),
      }, 'simulate-invalid-self-connect'),
      React.createElement('button', { type: 'button', onClick: () => invokeAdd('n_candidate_a', 'prompt') }, 'simulate-add-prompt'),
      React.createElement('button', { type: 'button', onClick: () => invokeAdd('n_prompt', 'agent') }, 'simulate-add-agent'),
      React.createElement('button', { type: 'button', onClick: () => invokeAdd('n_agent', 'script') }, 'simulate-add-script'),
      React.createElement('button', {
        type: 'button',
        onClick: () => {
          const node = props.nodes.find((item) => item.id === 'n_prompt')
          if (node) props.onNodesChange([{ id: node.id, type: 'position', position: { x: 420, y: 160 }, dragging: false }])
        },
      }, 'simulate-drag'),
      React.createElement('button', {
        type: 'button',
        onClick: () => props.onNodesChange([
          { id: 'n_prompt', type: 'position', position: { x: 22, y: 42 }, dragging: false },
          { id: 'n_prompt', type: 'dimensions', dimensions: { width: 340, height: 280 }, resizing: true },
        ]),
      }, 'simulate-resize-start'),
      React.createElement('button', {
        type: 'button',
        onClick: () => props.onNodesChange([
          { id: 'n_prompt', type: 'position', position: { x: 18, y: 38 }, dragging: false },
          { id: 'n_prompt', type: 'dimensions', dimensions: { width: 340, height: 280 }, resizing: false },
        ]),
      }, 'simulate-resize-end'),
      props.children,
    )
  }
  return {
    applyNodeChanges: (changes: NodeChange<CanvasFlowNode>[], nodes: CanvasFlowNode[]) => changes.reduce((current, change) => {
      if (change.type === 'select') return current.map((node) => node.id === change.id ? { ...node, selected: change.selected } : node)
      if (change.type !== 'position' || !change.position) return current
      return current.map((node) => node.id === change.id ? { ...node, position: change.position ?? node.position } : node)
    }, nodes),
    Background: () => null,
    BackgroundVariant: { Dots: 'dots' },
    MarkerType: { ArrowClosed: 'arrowclosed' },
    Controls: () => null,
    Panel: ({ children }: PropsWithChildren) => React.createElement('div', null, children),
    ReactFlow,
    ReactFlowProvider: ({ children }: PropsWithChildren) => React.createElement(React.Fragment, null, children),
    useReactFlow: () => ({
      fitView: flowMocks.fitView,
      setViewport: flowMocks.setViewport,
      getNode: flowMocks.getNode,
      screenToFlowPosition: flowMocks.screenToFlowPosition,
    }),
  }
})

function makeNode(id: string, type: string, parent = '', overrides: Partial<Node> = {}): Node {
  return {
    id,
    type: type as Node['type'],
    title: id,
    parent,
    position: { x: 20, y: 40 },
    size: { width: 280, height: 230 },
    params: {},
    rounds: [],
    locked: false,
    stale: false,
    ...overrides,
  }
}

function makeSnapshot(): WorkspaceSnapshot {
  const topicA = makeNode('n_topic_a', 'topic', '', {
    rounds: [{ n: 1, kind: OutputKind.OutputKindTopics, artifactId: 'sha256:topics-a', pendingId: 'pending-a', runId: 'r_topics_a' }],
  })
  const candidateA = makeNode('n_candidate_a', 'candidate', topicA.id, {
    candidateSource: { topicNodeId: topicA.id, round: 1, artifactId: 'sha256:topics-a', index: 0, runId: 'r_topics_a' },
  })
  const topicB = makeNode('n_topic_b', 'topic', '', {
    rounds: [{ n: 1, kind: OutputKind.OutputKindTopics, artifactId: 'sha256:topics-b', pendingId: 'pending-b', runId: 'r_topics_b' }],
  })
  const candidateB = makeNode('n_candidate_b', 'candidate', topicB.id, {
    candidateSource: { topicNodeId: topicB.id, round: 1, artifactId: 'sha256:topics-b', index: 0, runId: 'r_topics_b' },
  })
  const prompt = makeNode('n_prompt', 'prompt', candidateB.id, { params: { instruction: '写一段脚本。' } })
  const agent = makeNode('n_agent', 'agent', prompt.id)
  const draft = makeNode('n_draft', 'prompt')
  return {
    project: { id: 'p_test', name: '测试工程', revision: 2, activeNodeId: candidateA.id },
    nodes: [topicA, candidateA, topicB, candidateB, prompt, agent, draft],
    edges: [
      { id: 'e_topic_a', sourceNodeId: topicA.id, targetNodeId: candidateA.id, kind: 'candidate_source', scriptVersion: 0 },
      { id: 'e_topic_b', sourceNodeId: topicB.id, targetNodeId: candidateB.id, kind: 'candidate_source', scriptVersion: 0 },
      { id: 'e_prompt', sourceNodeId: candidateB.id, targetNodeId: prompt.id, kind: 'connection', scriptVersion: 0 },
      { id: 'e_agent', sourceNodeId: prompt.id, targetNodeId: agent.id, kind: 'connection', scriptVersion: 0 },
    ],
    nodePaths: {
      [topicA.id]: [topicA.id],
      [candidateA.id]: [topicA.id, candidateA.id],
      [topicB.id]: [topicB.id],
      [candidateB.id]: [topicB.id, candidateB.id],
      [prompt.id]: [topicB.id, candidateB.id, prompt.id],
      [agent.id]: [topicB.id, candidateB.id, prompt.id, agent.id],
      [draft.id]: [draft.id],
    },
    pending: [],
    runs: [],
    chains: [{ id: 'c_n_agent', tailNodeId: agent.id, path: [topicB.id, candidateB.id, prompt.id, agent.id], nodeIds: [topicB.id, candidateB.id, prompt.id, agent.id], ambiguous: false, label: '测试链', status: 'idle' }],
    lockedNodeIds: [],
  }
}

function makeAPI(): GraphAPI {
  return {
    addNode: vi.fn().mockResolvedValue({ projectId: 'p_test', projectRevision: 3, nodeId: 'n_created' }),
    connect: vi.fn().mockResolvedValue({ projectId: 'p_test', projectRevision: 3, nodeId: 'n_prompt' }),
    disconnect: vi.fn().mockResolvedValue({ projectId: 'p_test', projectRevision: 3, nodeId: 'n_prompt' }),
    reconnectTarget: vi.fn().mockResolvedValue({ projectId: 'p_test', projectRevision: 3, nodeId: 'n_prompt' }),
    moveNode: vi.fn().mockResolvedValue({ projectId: 'p_test', projectRevision: 3, nodeId: 'n_prompt' }),
    setNodeLayout: vi.fn().mockResolvedValue({ projectId: 'p_test', projectRevision: 3, nodeId: 'n_prompt' }),
    setNodeCollapsed: vi.fn().mockResolvedValue({ projectId: 'p_test', projectRevision: 3, nodeId: 'n_prompt' }),
    setEdgeScriptVersion: vi.fn().mockResolvedValue({ projectId: 'p_test', projectRevision: 3, nodeId: 'n_prompt' }),
    selectNode: vi.fn().mockResolvedValue({ projectId: 'p_test', projectRevision: 3, nodeId: 'n_prompt' }),
    updateNodeParams: vi.fn().mockResolvedValue({ projectId: 'p_test', projectRevision: 3, nodeId: 'n_prompt' }),
    getTopicCandidates: vi.fn().mockResolvedValue([]),
    getScriptVersion: vi.fn().mockResolvedValue({ nodeId: 'n_script', version: { n: 1, artifactId: 'sha256:script', runId: 'r_script' }, payload: '{"title":"脚本","scenes":[{"narration":"正文","duration":1}]}' }),
  }
}

async function createFromCanvasMenu(type: string, x = 320, y = 240) {
  fireEvent.contextMenu(screen.getByTestId('mock-flow-pane'), { clientX: x, clientY: y })
  fireEvent.click(await screen.findByRole('menuitem', { name: '新增节点' }))
  fireEvent.click(await screen.findByRole('menuitem', { name: type }))
}

describe('WorkspaceCanvas', () => {
  afterEach(() => {
    vi.useRealTimers()
    flowMocks.screenToFlowPosition.mockImplementation((position) => position)
  })

  it('registers a locator that selects the node and focuses its viewport', async () => {
    let locateNode: ((nodeId: string) => boolean) | undefined
    const onSelectNode = vi.fn().mockResolvedValue(undefined)
    const onRegisterNodeLocator = vi.fn((locator: (nodeId: string) => boolean) => {
      locateNode = locator
      return () => { locateNode = undefined }
    })
    render(
      <WorkspaceCanvas
        snapshot={makeSnapshot()}
        onSelectNode={onSelectNode}
        onRegisterNodeLocator={onRegisterNodeLocator}
        api={makeAPI()}
      />,
    )
    await waitFor(() => expect(locateNode).toBeDefined())
    flowMocks.fitView.mockClear()
    const locate = locateNode
    if (!locate) throw new Error('node locator was not registered')

    expect(locate('n_agent')).toBe(true)
    expect(onSelectNode).toHaveBeenCalledWith('n_agent')
    expect(flowMocks.fitView).toHaveBeenCalledWith({ nodes: [{ id: 'n_agent' }], padding: 0.35, duration: 180 })
  })

  it('persists the final size and position together only after resizing ends', async () => {
    const api = makeAPI()
    render(<WorkspaceCanvas snapshot={makeSnapshot()} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />)

    fireEvent.click(screen.getByRole('button', { name: 'simulate-resize-start' }))
    expect(api.setNodeLayout).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'simulate-resize-end' }))
    await waitFor(() => expect(api.setNodeLayout).toHaveBeenCalledOnce())
    expect(api.setNodeLayout).toHaveBeenCalledWith('n_prompt', 18, 38, 340, 280, 'p_test')
    expect(api.moveNode).not.toHaveBeenCalled()
  })

  it('adds an upstream without replacing existing edges or showing a reparent confirmation', async () => {
    const api = makeAPI()
    render(<WorkspaceCanvas snapshot={makeSnapshot()} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />)

    fireEvent.click(screen.getByRole('button', { name: 'simulate-reparent' }))
    await waitFor(() => expect(api.connect).toHaveBeenCalledWith('n_candidate_a', 'n_prompt', 'p_test'))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(screen.getByTestId('edge-count')).toHaveTextContent('4')
  })

  it('keeps the canvas viewport structure stable as an Agent is selected and its inputs change', () => {
    const snapshot = makeSnapshot()
    snapshot.project = { ...snapshot.project!, activeNodeId: 'n_agent' }
    const { container, rerender } = render(<WorkspaceCanvas snapshot={snapshot} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={makeAPI()} />)
    const toolbar = container.querySelector('.canvas-toolbar')
    const flowRoot = container.querySelector('.canvas-flow-root')

    expect(toolbar?.nextElementSibling).toBe(flowRoot)
    expect(container.querySelector('.canvas-keyboard-connect')).toBeNull()
    expect(container.querySelector('.canvas-input-relations')).toBeNull()
    expect(screen.queryByText('新增连接')).not.toBeInTheDocument()
    expect(screen.queryByText('将目标连接到选中节点')).not.toBeInTheDocument()

    const nextSnapshot = {
      ...snapshot,
      edges: [...(snapshot.edges ?? []), {
        id: 'e_candidate_agent', sourceNodeId: 'n_candidate_a', targetNodeId: 'n_agent', kind: 'connection', scriptVersion: 0,
      }],
    }
    rerender(<WorkspaceCanvas snapshot={nextSnapshot} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={makeAPI()} />)

    expect(toolbar?.nextElementSibling).toBe(flowRoot)
    expect(container.querySelector('.canvas-input-relations')).toBeNull()
  })

  it('delegates same-type and self connection legality to the backend', async () => {
    const api = makeAPI()
    vi.mocked(api.connect).mockRejectedValueOnce(new Error('CYCLE_DETECTED: cycle'))
      .mockRejectedValueOnce(new Error('SELF_CONNECT: self'))
    render(<WorkspaceCanvas snapshot={makeSnapshot()} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />)

    fireEvent.click(screen.getByRole('button', { name: 'simulate-invalid-type-connect' }))
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('CYCLE_DETECTED'))
    expect(api.connect).toHaveBeenCalledWith('n_prompt', 'n_draft', 'p_test')
    fireEvent.click(screen.getByRole('button', { name: 'simulate-invalid-self-connect' }))
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('SELF_CONNECT'))
    expect(api.connect).toHaveBeenCalledWith('n_candidate_a', 'n_candidate_a', 'p_test')
    expect(screen.getByTestId('edge-count')).toHaveTextContent('4')
  })

  it('shows node types in a second menu and keeps the original transformed click position', async () => {
    const api = makeAPI()
    const onSelectNode = vi.fn().mockResolvedValue(undefined)
    render(<WorkspaceCanvas snapshot={makeSnapshot()} onSelectNode={onSelectNode} onGraphMutation={vi.fn()} api={api} />)
    await waitFor(() => expect(flowMocks.fitView).toHaveBeenCalled())
    flowMocks.fitView.mockClear()
    flowMocks.screenToFlowPosition.mockImplementation(({ x, y }) => ({ x: (x - 100) / 2, y: (y - 60) / 2 }))

    expect(screen.queryByRole('button', { name: '新选题' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '草稿提示词' })).not.toBeInTheDocument()
    fireEvent.contextMenu(screen.getByTestId('mock-flow-pane'), { clientX: 380, clientY: 260 })
    expect(await screen.findByRole('menu', { name: '新建节点' })).toBeInTheDocument()
    expect(screen.queryByRole('menu', { name: '选择节点类型' })).not.toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: '提示词' })).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('menuitem', { name: '新增节点' }))
    expect(await screen.findByRole('menu', { name: '选择节点类型' })).toBeInTheDocument()
    expect(screen.getByText('候选')).toBeInTheDocument()
    expect(screen.getByText('由选题生成')).toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: '候选' })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('menuitem', { name: '提示词' }))

    await waitFor(() => expect(api.addNode).toHaveBeenCalledOnce())
    expect(api.addNode).toHaveBeenCalledWith(expect.objectContaining({
      type: 'prompt',
      parent: '',
      parentCandidate: null,
      position: { x: 140, y: 100 },
    }))
    expect(onSelectNode).toHaveBeenCalledWith('n_created')
    expect(flowMocks.fitView).not.toHaveBeenCalled()
  })

  it('opens the submenu with the keyboard and returns focus one level at a time', async () => {
    const api = makeAPI()
    const { container } = render(<WorkspaceCanvas snapshot={makeSnapshot()} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />)
    const canvas = container.querySelector<HTMLElement>('.canvas-flow-root')
    if (!canvas) throw new Error('canvas root missing')
    vi.spyOn(canvas, 'getBoundingClientRect').mockReturnValue({
      x: 100, y: 50, left: 100, top: 50, right: 700, bottom: 450, width: 600, height: 400,
      toJSON: () => ({}),
    })

    fireEvent.keyDown(canvas, { key: 'F10', shiftKey: true })
    const addNodeItem = await screen.findByRole('menuitem', { name: '新增节点' })
    expect(document.activeElement).toBe(addNodeItem)
    fireEvent.keyDown(addNodeItem, { key: 'ArrowRight' })
    const topicItem = await screen.findByRole('menuitem', { name: '选题' })
    expect(document.activeElement).toBe(topicItem)
    fireEvent.keyDown(topicItem, { key: 'ArrowDown' })
    expect(document.activeElement).toBe(screen.getByRole('menuitem', { name: '提示词' }))
    fireEvent.keyDown(screen.getByRole('menuitem', { name: '提示词' }), { key: 'ArrowDown' })
    expect(document.activeElement).toBe(screen.getByRole('menuitem', { name: 'Agent' }))
    fireEvent.keyDown(screen.getByRole('menuitem', { name: 'Agent' }), { key: 'Escape' })
    expect(screen.getByRole('menu', { name: '新建节点' })).toBeInTheDocument()
    expect(screen.queryByRole('menu', { name: '选择节点类型' })).not.toBeInTheDocument()
    expect(document.activeElement).toBe(addNodeItem)
    fireEvent.keyDown(addNodeItem, { key: 'Escape' })
    expect(screen.queryByRole('menu', { name: '新建节点' })).not.toBeInTheDocument()
    await waitFor(() => expect(document.activeElement).toBe(canvas))

    fireEvent.keyDown(canvas, { key: 'ContextMenu' })
    const secondAddNodeItem = await screen.findByRole('menuitem', { name: '新增节点' })
    fireEvent.keyDown(secondAddNodeItem, { key: 'Enter' })
    const secondTopicItem = await screen.findByRole('menuitem', { name: '选题' })
    fireEvent.keyDown(secondTopicItem, { key: 'ArrowDown' })
    fireEvent.keyDown(screen.getByRole('menuitem', { name: '提示词' }), { key: 'ArrowDown' })
    fireEvent.keyDown(screen.getByRole('menuitem', { name: 'Agent' }), { key: 'ArrowDown' })
    const scriptItem = screen.getByRole('menuitem', { name: '脚本' })
    expect(document.activeElement).toBe(scriptItem)
    fireEvent.keyDown(scriptItem, { key: 'Enter' })
    await waitFor(() => expect(api.addNode).toHaveBeenCalledOnce())
    expect(api.addNode).toHaveBeenCalledWith(expect.objectContaining({ type: 'script', position: { x: 400, y: 250 }, parent: '' }))
  })

  it('reports a failed creation and allows a deliberate retry', async () => {
    const api = makeAPI()
    vi.mocked(api.addNode).mockRejectedValueOnce(new Error('disk full'))
    render(<WorkspaceCanvas snapshot={makeSnapshot()} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />)

    await createFromCanvasMenu('提示词')
    expect(await screen.findByRole('alert')).toHaveTextContent('创建提示词失败：disk full')
    expect(api.addNode).toHaveBeenCalledOnce()

    await createFromCanvasMenu('提示词')
    await waitFor(() => expect(api.addNode).toHaveBeenCalledTimes(2))
  })

  it('blocks another menu selection while a node creation is in flight', async () => {
    const api = makeAPI()
    let resolveAddNode!: (receipt: Awaited<ReturnType<GraphAPI['addNode']>>) => void
    vi.mocked(api.addNode).mockImplementation(() => new Promise((resolve) => { resolveAddNode = resolve }))
    const onGraphMutation = vi.fn().mockResolvedValue(undefined)
    render(
      <WorkspaceCanvas
        snapshot={makeSnapshot()}
        onSelectNode={vi.fn().mockResolvedValue(undefined)}
        onGraphMutation={onGraphMutation}
        api={api}
      />,
    )

    await createFromCanvasMenu('选题')
    await waitFor(() => expect(api.addNode).toHaveBeenCalledOnce())
    fireEvent.contextMenu(screen.getByTestId('mock-flow-pane'), { clientX: 400, clientY: 300 })
    fireEvent.click(await screen.findByRole('menuitem', { name: '新增节点' }))
    const agentItem = await screen.findByRole('menuitem', { name: 'Agent' })
    expect(agentItem).toBeDisabled()
    fireEvent.click(agentItem)
    expect(api.addNode).toHaveBeenCalledOnce()

    resolveAddNode({ projectId: 'p_test', projectRevision: 3, nodeId: 'n_created' })
    await waitFor(() => expect(api.addNode).toHaveBeenCalledOnce())
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it.each([
    { label: 'upper-left', x: 2, y: 2, side: 'right' },
    { label: 'upper-right', x: 798, y: 2, side: 'left' },
    { label: 'lower-left', x: 2, y: 598, side: 'right' },
    { label: 'lower-right', x: 798, y: 598, side: 'left' },
  ])('keeps both menu levels in the viewport at the $label corner', async ({ x, y, side }) => {
    const originalWidth = window.innerWidth
    const originalHeight = window.innerHeight
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 800 })
    Object.defineProperty(window, 'innerHeight', { configurable: true, value: 600 })
    try {
      render(<WorkspaceCanvas snapshot={makeSnapshot()} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={makeAPI()} />)
      fireEvent.contextMenu(screen.getByTestId('mock-flow-pane'), { clientX: x, clientY: y })
      const menu = await screen.findByRole('menu', { name: '新建节点' })
      vi.spyOn(menu, 'getBoundingClientRect').mockImplementation(() => {
        const left = Number.parseFloat(menu.style.left) || 0
        const top = Number.parseFloat(menu.style.top) || 0
        return { x: left, y: top, left, top, right: left + 158, bottom: top + 40, width: 158, height: 40, toJSON: () => ({}) }
      })
      fireEvent.resize(window)
      await waitFor(() => {
        expect(Number.parseFloat(menu.style.left)).toBeGreaterThanOrEqual(8)
        expect(Number.parseFloat(menu.style.top)).toBeGreaterThanOrEqual(8)
      })
      fireEvent.click(screen.getByRole('menuitem', { name: '新增节点' }))
      const submenu = await screen.findByRole('menu', { name: '选择节点类型' })
      vi.spyOn(submenu, 'getBoundingClientRect').mockImplementation(() => ({
        x: 0, y: 0, left: 0, top: 0, right: 218, bottom: 240, width: 218, height: 240, toJSON: () => ({}),
      }))
      fireEvent.resize(window)

      await waitFor(() => expect(submenu).toHaveAttribute('data-side', side))
      const menuLeft = Number.parseFloat(menu.style.left)
      const menuTop = Number.parseFloat(menu.style.top)
      const submenuLeft = side === 'right' ? menuLeft + 158 + 4 : menuLeft - 218 - 4
      const submenuTop = menuTop + Number.parseFloat(submenu.style.top)
      expect(menuLeft).toBeGreaterThanOrEqual(8)
      expect(menuLeft + 158).toBeLessThanOrEqual(window.innerWidth - 8)
      expect(menuTop).toBeGreaterThanOrEqual(8)
      expect(menuTop + 40).toBeLessThanOrEqual(window.innerHeight - 8)
      expect(submenuLeft).toBeGreaterThanOrEqual(8)
      expect(submenuLeft + 218).toBeLessThanOrEqual(window.innerWidth - 8)
      expect(submenuTop).toBeGreaterThanOrEqual(8)
      expect(submenuTop + 240).toBeLessThanOrEqual(window.innerHeight - 8)
    } finally {
      Object.defineProperty(window, 'innerWidth', { configurable: true, value: originalWidth })
      Object.defineProperty(window, 'innerHeight', { configurable: true, value: originalHeight })
    }
  })

  it('creates v2 node types only through workflow commands', async () => {
    const api = makeAPI()
    const onGraphMutation = vi.fn()
    render(<WorkspaceCanvas snapshot={makeSnapshot()} onSelectNode={vi.fn().mockResolvedValue(undefined)} onGraphMutation={onGraphMutation} api={api} />)

    await createFromCanvasMenu('选题')
    await waitFor(() => expect(api.addNode).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(onGraphMutation).toHaveBeenCalledTimes(1))
    await createFromCanvasMenu('提示词')
    await waitFor(() => expect(api.addNode).toHaveBeenCalledTimes(2))
    await waitFor(() => expect(onGraphMutation).toHaveBeenCalledTimes(2))
    fireEvent.click(screen.getByRole('button', { name: 'simulate-add-prompt' }))
    await waitFor(() => expect(api.addNode).toHaveBeenCalledTimes(3))
    await waitFor(() => expect(onGraphMutation).toHaveBeenCalledTimes(3))
    fireEvent.click(screen.getByRole('button', { name: 'simulate-add-agent' }))
    await waitFor(() => expect(api.addNode).toHaveBeenCalledTimes(4))
    await waitFor(() => expect(onGraphMutation).toHaveBeenCalledTimes(4))
    fireEvent.click(screen.getByRole('button', { name: 'simulate-add-script' }))
    await waitFor(() => expect(api.addNode).toHaveBeenCalledTimes(5))
    await waitFor(() => expect(onGraphMutation).toHaveBeenCalledTimes(5))
    expect(api.addNode).toHaveBeenCalledWith(expect.objectContaining({ type: 'prompt', parent: 'n_candidate_a', parentCandidate: null }))
    expect(api.addNode).toHaveBeenCalledWith(expect.objectContaining({ type: 'agent', parent: 'n_prompt', parentCandidate: null }))
    expect(api.addNode).toHaveBeenCalledWith(expect.objectContaining({ type: 'script', parent: 'n_agent', parentCandidate: null }))
    expect(onGraphMutation).toHaveBeenCalledTimes(5)
  })

  it('waits for the refreshed snapshot before finishing topic creation', async () => {
    const api = makeAPI()
    let resolveSync!: () => void
    const sync = new Promise<void>((resolve) => { resolveSync = resolve })
    const onGraphMutation = vi.fn(() => sync)
    const initial = makeSnapshot()
    const createdTopic = makeNode('n_created', 'topic')
    const refreshed: WorkspaceSnapshot = {
      ...initial,
      project: { ...initial.project!, revision: 3 },
      nodes: [...initial.nodes!, createdTopic],
    }
    const { rerender } = render(<WorkspaceCanvas snapshot={initial} onSelectNode={vi.fn().mockResolvedValue(undefined)} onGraphMutation={onGraphMutation} api={api} />)

    await createFromCanvasMenu('选题')
    await waitFor(() => expect(api.addNode).toHaveBeenCalledOnce())
    await waitFor(() => expect(onGraphMutation).toHaveBeenCalledOnce())
    fireEvent.contextMenu(screen.getByTestId('mock-flow-pane'), { clientX: 400, clientY: 300 })
    fireEvent.click(await screen.findByRole('menuitem', { name: '新增节点' }))
    const itemWhileSyncing = await screen.findByRole('menuitem', { name: '选题' })
    const disabledWhileSyncing = itemWhileSyncing.hasAttribute('disabled')

    rerender(<WorkspaceCanvas snapshot={refreshed} onSelectNode={vi.fn().mockResolvedValue(undefined)} onGraphMutation={onGraphMutation} api={api} />)
    expect(await screen.findByRole('button', { name: 'n_created' })).toBeInTheDocument()
    resolveSync()

    await waitFor(() => expect(screen.getByRole('menuitem', { name: '选题' })).toBeEnabled())
    expect(disabledWhileSyncing).toBe(true)
    expect(screen.queryByText('选题节点已提交，正在同步工程图。')).not.toBeInTheDocument()
    expect(screen.queryByText('选题节点已同步。')).not.toBeInTheDocument()
  })

  it('shows a visible error and only retries snapshot synchronization after a saved command', async () => {
    const api = makeAPI()
    const onGraphMutation = vi.fn().mockRejectedValueOnce(new Error('同步失败')).mockResolvedValueOnce(undefined)
    const { container } = render(<WorkspaceCanvas snapshot={makeSnapshot()} onSelectNode={vi.fn().mockResolvedValue(undefined)} onGraphMutation={onGraphMutation} api={api} />)

    await createFromCanvasMenu('选题')
    const syncError = await screen.findByRole('alert')
    expect(syncError).toHaveTextContent('选题节点已保存，但画布同步失败：同步失败')
    expect(container.querySelector('.canvas-flow-root')?.contains(syncError)).toBe(true)
    expect(api.addNode).toHaveBeenCalledOnce()
    fireEvent.click(screen.getByRole('button', { name: '仅重试快照同步' }))
    await waitFor(() => expect(onGraphMutation).toHaveBeenCalledTimes(2))
    fireEvent.contextMenu(screen.getByTestId('mock-flow-pane'), { clientX: 400, clientY: 300 })
    fireEvent.click(await screen.findByRole('menuitem', { name: '新增节点' }))
    expect(screen.getByRole('menuitem', { name: '选题' })).toBeEnabled()
    expect(api.addNode).toHaveBeenCalledOnce()
    expect(screen.queryByText('选题节点已同步。')).not.toBeInTheDocument()
  })

  it('renders workspace snapshot sync errors inside the canvas without hiding retry', () => {
    const retry = vi.fn()
    const { container } = render(
      <WorkspaceCanvas
        snapshot={makeSnapshot()}
        onSelectNode={vi.fn().mockResolvedValue(undefined)}
        api={makeAPI()}
        workspaceSyncError={{ message: '读取失败', retry }}
      />,
    )

    const syncError = screen.getByRole('alert')
    expect(syncError).toHaveTextContent('快照同步失败：读取失败')
    expect(container.querySelector('.canvas-flow-root')?.contains(syncError)).toBe(true)
    fireEvent.click(screen.getByRole('button', { name: '重试' }))
    expect(retry).toHaveBeenCalledOnce()
  })

  it('enforces the zoom range and supports reset and double-click focus', () => {
    flowMocks.fitView.mockClear()
    render(<WorkspaceCanvas snapshot={makeSnapshot()} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={makeAPI()} />)

    expect(screen.getByTestId('zoom-range')).toHaveTextContent('0.6-1.6')
    fireEvent.click(screen.getByRole('button', { name: '重置视图' }))
    expect(flowMocks.fitView).toHaveBeenCalledWith({ padding: 0.28, duration: 180 })
    fireEvent.click(screen.getByRole('button', { name: 'simulate-double-click' }))
    expect(flowMocks.fitView).toHaveBeenLastCalledWith({ nodes: [{ id: 'n_agent' }], padding: 0.35, duration: 180 })
  })

  it('debounces position persistence and reports failure without changing the graph edges', async () => {
    const api = makeAPI()
    vi.mocked(api.moveNode).mockRejectedValueOnce(new Error('save failed'))
    render(<WorkspaceCanvas snapshot={makeSnapshot()} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />)

    vi.useFakeTimers()
    fireEvent.click(screen.getByRole('button', { name: 'simulate-drag' }))
    expect(api.moveNode).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(240)
    expect(api.moveNode).toHaveBeenCalledWith('n_prompt', 420, 160, 'p_test')
    await vi.waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('保存节点位置失败：save failed'))
    expect(screen.getByTestId('edge-count')).toHaveTextContent('4')
    expect(screen.getByTestId('child-position')).toHaveTextContent('20,40')
  })

  it('cancels queued position writes when the active project changes', async () => {
    const api = makeAPI()
    const original = makeSnapshot()
    const { rerender } = render(<WorkspaceCanvas snapshot={original} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />)
    vi.useFakeTimers()
    fireEvent.click(screen.getByRole('button', { name: 'simulate-drag' }))
    const nextProject = { ...original, project: { ...original.project!, id: 'p_next', name: '另一个工程' } }
    rerender(<WorkspaceCanvas snapshot={nextProject} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />)
    await vi.advanceTimersByTimeAsync(240)
    expect(api.moveNode).not.toHaveBeenCalled()
  })

  it('selects a backend leaf chain through SelectNode without graph mutation or run start', async () => {
    const api = makeAPI()
    const onSelectNode = vi.fn().mockResolvedValue(undefined)
    const onGraphMutation = vi.fn()
    const onStartRun = vi.fn()
    render(
      <WorkspaceCanvas
        snapshot={makeSnapshot()}
        onSelectNode={onSelectNode}
        onGraphMutation={onGraphMutation}
        onStartRun={onStartRun}
        api={api}
      />,
    )

    const quickbar = screen.getByRole('navigation', { name: '链路选择' })
    fireEvent.pointerEnter(quickbar)
    fireEvent.click(screen.getByRole('button', { name: /选择链路 测试链/ }))
    await waitFor(() => expect(onSelectNode).toHaveBeenCalledWith('n_agent'))
    expect(onGraphMutation).not.toHaveBeenCalled()
    expect(onStartRun).not.toHaveBeenCalled()
    expect(api.addNode).not.toHaveBeenCalled()
    expect(api.connect).not.toHaveBeenCalled()
  })

  it('persists selection and rolls it back when SelectNode fails', async () => {
    const api = makeAPI()
    const onSelectNode = vi.fn().mockRejectedValue(new Error('select failed'))
    render(<WorkspaceCanvas snapshot={makeSnapshot()} onSelectNode={onSelectNode} api={api} />)

    fireEvent.click(screen.getByRole('button', { name: 'simulate-select' }))
    await waitFor(() => expect(onSelectNode).toHaveBeenCalledWith('n_prompt'))
    expect(await screen.findByRole('alert')).toHaveTextContent('保存节点选择失败：select failed')
  })
})
