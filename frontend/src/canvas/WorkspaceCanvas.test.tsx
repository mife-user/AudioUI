import { vi } from 'vitest'
import type { PropsWithChildren } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import type { Connection, Edge, NodeChange, NodeMouseHandler } from '@xyflow/react'
import type { Node, WorkspaceSnapshot } from '../../bindings/audio-ui/internal/domain/entity'
import type { GraphAPI } from './graphApi'
import type { CanvasFlowNode } from './flowModel'
import { WorkspaceCanvas } from './WorkspaceCanvas'

type MockFlowProps = {
  nodes: CanvasFlowNode[]
  edges: Edge[]
  minZoom: number
  maxZoom: number
  onNodesChange: (changes: NodeChange<CanvasFlowNode>[]) => void
  onConnect: (connection: Connection) => void
  onNodeClick: (event: MouseEvent, node: CanvasFlowNode) => void
  onNodeDoubleClick: NodeMouseHandler<CanvasFlowNode>
}

const flowMocks = vi.hoisted(() => ({ fitView: vi.fn(), setViewport: vi.fn() }))

vi.mock('@xyflow/react', async () => {
  const React = await import('react')
  const flowModel = await import('./flowModel')
  const ReactFlow = (props: MockFlowProps) => {
    React.useEffect(() => {
      for (const node of props.nodes) {
        node.data.onNodeHandlesMeasured?.(node.id, flowModel.nodeHandleLayoutKey(node.type, node.data), node.data.projectId ?? '')
      }
    }, [props.nodes])
    return React.createElement('div', { 'data-testid': 'mock-flow' },
    React.createElement('output', { 'data-testid': 'edge-count' }, String(props.edges.length)),
    React.createElement('output', { 'data-testid': 'zoom-range' }, `${props.minZoom}-${props.maxZoom}`),
    React.createElement('output', { 'data-testid': 'child-position' }, (() => {
      const node = props.nodes.find((item) => item.id === 'n_child')
      return node ? `${node.position.x},${node.position.y}` : ''
    })()),
    ...props.nodes.map((node) => React.createElement('button', {
      key: node.id,
      type: 'button',
      onClick: (event: MouseEvent) => props.onNodeClick(event, node),
    }, node.data.node.title)),
    React.createElement('button', {
      type: 'button',
      onClick: () => props.onNodesChange([{ id: 'n_child', type: 'select', selected: true }]),
    }, 'simulate-select'),
    React.createElement('button', {
      type: 'button',
      onClick: () => {
        const node = props.nodes.find((item) => item.id === 'n_child')
        if (node) props.onNodeDoubleClick({} as Parameters<NodeMouseHandler<CanvasFlowNode>>[0], node)
      },
    }, 'simulate-double-click'),
    React.createElement('button', {
      type: 'button',
      onClick: () => props.onConnect({
        source: 'n_topic_a',
        sourceHandle: 'candidate-0',
        target: 'n_child',
        targetHandle: 'prompt-input',
      }),
    }, 'simulate-connect'),
    React.createElement('button', {
      type: 'button',
      onClick: () => props.onConnect({
        source: 'n_topic_a',
        sourceHandle: 'candidate-0',
        target: 'n_topic_a',
        targetHandle: 'prompt-input',
      }),
    }, 'simulate-invalid-connect'),
    React.createElement('button', {
      type: 'button',
      onClick: () => props.onConnect({
        source: 'n_topic_b',
        sourceHandle: 'candidate-1',
        target: 'n_child',
        targetHandle: 'prompt-input',
      }),
    }, 'simulate-candidate-change'),
    React.createElement('button', {
      type: 'button',
      onClick: () => {
        const node = props.nodes.find((item) => item.id === 'n_child')
        if (node) {
          const moved = { ...node, position: { x: 420, y: 160 } }
          props.onNodesChange([{ id: node.id, type: 'position', position: moved.position, dragging: false }])
        }
      },
    }, 'simulate-drag'),
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
    Controls: () => null,
    Panel: ({ children }: PropsWithChildren) => React.createElement('div', null, children),
    ReactFlow,
    ReactFlowProvider: ({ children }: PropsWithChildren) => React.createElement(React.Fragment, null, children),
    useReactFlow: () => ({
      fitView: flowMocks.fitView,
      setViewport: flowMocks.setViewport,
      screenToFlowPosition: (position: { x: number; y: number }) => position,
    }),
  }
})

function makeNode(id: string, type: 'topic' | 'prompt', parent = '', parentCandidate?: number): Node {
  return {
    id,
    type: type as Node['type'],
    title: id,
    parent,
    parentCandidate,
    position: { x: 20, y: 40 },
    params: {},
    rounds: [],
    locked: false,
    stale: false,
  }
}

function makeSnapshot(): WorkspaceSnapshot {
  const topicA = makeNode('n_topic_a', 'topic')
  const topicB = makeNode('n_topic_b', 'topic')
  const child = makeNode('n_child', 'prompt', topicB.id, 0)
  const descendant = makeNode('n_descendant', 'prompt', child.id)
  const draft = makeNode('n_draft', 'prompt')
  return {
    project: { id: 'p_test', name: '测试工程', revision: 2, activeNodeId: topicA.id },
    nodes: [topicA, topicB, child, descendant, draft],
    pending: [],
    runs: [],
    chains: [{ id: 'c_n_descendant', tailNodeId: descendant.id, path: [topicB.id, child.id, descendant.id], label: '测试链', status: 'idle' }],
    lockedNodeIds: [],
  }
}

function makeAPI(): GraphAPI {
  return {
    addNode: vi.fn().mockResolvedValue({ projectId: 'p_test', projectRevision: 3, nodeId: 'n_created' }),
    connect: vi.fn().mockResolvedValue({ projectId: 'p_test', projectRevision: 3, nodeId: 'n_child' }),
    moveNode: vi.fn().mockResolvedValue({ projectId: 'p_test', projectRevision: 3, nodeId: 'n_child' }),
    selectNode: vi.fn().mockResolvedValue({ projectId: 'p_test', projectRevision: 3, nodeId: 'n_child' }),
    updateNodeParams: vi.fn().mockResolvedValue({ projectId: 'p_test', projectRevision: 3, nodeId: 'n_topic_a' }),
    getTopicCandidates: vi.fn().mockResolvedValue([]),
  }
}

const reparentReceipt = { projectId: 'p_test', projectRevision: 3, nodeId: 'n_child' }

describe('WorkspaceCanvas', () => {
  afterEach(() => vi.useRealTimers())

  it('asks before replacing a parent and leaves the snapshot edge untouched until the backend succeeds', async () => {
    const api = makeAPI()
    vi.mocked(api.connect)
      .mockRejectedValueOnce(new Error('REPARENT_CONFIRM_REQUIRED: n_topic_b -> n_topic_a'))
      .mockResolvedValueOnce(reparentReceipt)
    render(
      <WorkspaceCanvas
        snapshot={makeSnapshot()}
        onSelectNode={vi.fn().mockResolvedValue(undefined)}
        api={api}
        candidateFixtures={{ n_topic_a: [{ title: '候选甲' }] }}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'simulate-connect' }))
    await waitFor(() => expect(screen.getByRole('dialog', { name: '确认改挂节点' })).toBeInTheDocument())
    expect(screen.getByText(/受影响子树：2 个节点/)).toBeInTheDocument()
    expect(screen.getByText(/跨 topic 树改挂/)).toBeInTheDocument()
    expect(screen.getByTestId('edge-count')).toHaveTextContent('2')
    expect(api.connect).toHaveBeenCalledTimes(1)
    expect(api.connect).toHaveBeenLastCalledWith('n_topic_a', 'n_child', 0, false, 'p_test')

    fireEvent.click(screen.getByRole('button', { name: '确认改挂' }))
    await waitFor(() => expect(api.connect).toHaveBeenCalledTimes(2))
    expect(api.connect).toHaveBeenLastCalledWith('n_topic_a', 'n_child', 0, true, 'p_test')
    expect(screen.getByTestId('edge-count')).toHaveTextContent('2')
  })

  it('cancels a reparent confirmation without issuing the replace command', async () => {
    const api = makeAPI()
    vi.mocked(api.connect).mockRejectedValueOnce(new Error('REPARENT_CONFIRM_REQUIRED'))
    render(
      <WorkspaceCanvas
        snapshot={makeSnapshot()}
        onSelectNode={vi.fn().mockResolvedValue(undefined)}
        api={api}
        candidateFixtures={{ n_topic_a: [{ title: '候选甲' }] }}
      />,
    )
    fireEvent.click(screen.getByRole('button', { name: 'simulate-connect' }))
    await waitFor(() => expect(screen.getByRole('dialog')).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: '取消' }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(api.connect).toHaveBeenCalledTimes(1)
    expect(screen.getByTestId('edge-count')).toHaveTextContent('2')
  })

  it('rejects an invalid target before calling the backend or changing edges', () => {
    const api = makeAPI()
    render(
      <WorkspaceCanvas
        snapshot={makeSnapshot()}
        onSelectNode={vi.fn().mockResolvedValue(undefined)}
        api={api}
        candidateFixtures={{ n_topic_a: [{ title: '候选甲' }] }}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'simulate-invalid-connect' }))
    expect(screen.getByRole('alert')).toHaveTextContent('连接目标不合法')
    expect(api.connect).not.toHaveBeenCalled()
    expect(screen.getByTestId('edge-count')).toHaveTextContent('2')
  })

  it('requires confirmation when the same topic parent changes candidate', async () => {
    const api = makeAPI()
    vi.mocked(api.connect)
      .mockRejectedValueOnce(new Error('REPARENT_CONFIRM_REQUIRED: n_topic_b -> n_topic_b'))
      .mockResolvedValueOnce(reparentReceipt)
    render(
      <WorkspaceCanvas
        snapshot={makeSnapshot()}
        onSelectNode={vi.fn().mockResolvedValue(undefined)}
        api={api}
        candidateFixtures={{ n_topic_b: [{ title: '旧候选' }, { title: '新候选' }] }}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'simulate-candidate-change' }))
    await waitFor(() => expect(screen.getByRole('dialog')).toBeInTheDocument())
    expect(screen.getByText(/候选 2/)).toBeInTheDocument()
    expect(screen.getByText(/原上游：n_topic_b · 候选 1/)).toBeInTheDocument()
    expect(screen.getByText(/工作区 steps 不迁移/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '确认改挂' }))
    await waitFor(() => expect(api.connect).toHaveBeenCalledTimes(2))
    expect(api.connect).toHaveBeenLastCalledWith('n_topic_b', 'n_child', 1, true, 'p_test')
  })

  it('shows a confirmed reparent failure and preserves the authoritative edge list', async () => {
    const api = makeAPI()
    vi.mocked(api.connect)
      .mockRejectedValueOnce(new Error('REPARENT_CONFIRM_REQUIRED'))
      .mockRejectedValueOnce(new Error('RUNNING_CHAIN_LOCKED'))
    render(
      <WorkspaceCanvas
        snapshot={makeSnapshot()}
        onSelectNode={vi.fn().mockResolvedValue(undefined)}
        api={api}
        candidateFixtures={{ n_topic_a: [{ title: '候选甲' }] }}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'simulate-connect' }))
    await waitFor(() => expect(screen.getByRole('dialog')).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: '确认改挂' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(screen.getByRole('alert')).toHaveTextContent('RUNNING_CHAIN_LOCKED')
    expect(screen.getByTestId('edge-count')).toHaveTextContent('2')
    expect(api.connect).toHaveBeenLastCalledWith('n_topic_a', 'n_child', 0, true, 'p_test')
  })

  it('creates topics and unconnected prompt drafts through workflow commands', async () => {
    const api = makeAPI()
    const onGraphMutation = vi.fn()
    render(<WorkspaceCanvas snapshot={makeSnapshot()} onSelectNode={vi.fn().mockResolvedValue(undefined)} onGraphMutation={onGraphMutation} api={api} />)

    fireEvent.click(screen.getByRole('button', { name: '＋ 新选题' }))
    await waitFor(() => expect(api.addNode).toHaveBeenCalledWith(expect.objectContaining({ projectId: 'p_test', type: 'topic', parent: '', params: { candidateCount: 3 } })))
    expect(onGraphMutation).toHaveBeenCalledTimes(1)
    fireEvent.click(screen.getByRole('button', { name: '＋ 草稿提示词' }))
    await waitFor(() => expect(api.addNode).toHaveBeenCalledWith(expect.objectContaining({ projectId: 'p_test', type: 'prompt', parent: '', params: {} })))
    expect(onGraphMutation).toHaveBeenCalledTimes(2)
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
    const { rerender } = render(
      <WorkspaceCanvas snapshot={initial} onSelectNode={vi.fn().mockResolvedValue(undefined)} onGraphMutation={onGraphMutation} api={api} />,
    )

    fireEvent.click(screen.getByRole('button', { name: '＋ 新选题' }))
    await waitFor(() => expect(api.addNode).toHaveBeenCalledOnce())
    await waitFor(() => expect(onGraphMutation).toHaveBeenCalledOnce())
    const disabledWhileSyncing = screen.getByRole('button', { name: '＋ 新选题' }).hasAttribute('disabled')

    rerender(<WorkspaceCanvas snapshot={refreshed} onSelectNode={vi.fn().mockResolvedValue(undefined)} onGraphMutation={onGraphMutation} api={api} />)
    expect(await screen.findByRole('button', { name: 'n_created' })).toBeInTheDocument()
    resolveSync()

    await waitFor(() => expect(screen.getByText('选题节点已同步。')).toBeInTheDocument())
    expect(disabledWhileSyncing).toBe(true)
    expect(screen.queryByText('选题节点已提交，正在同步工程图。')).not.toBeInTheDocument()
  })

  it('shows a visible error when the graph snapshot cannot be synchronized', async () => {
    const api = makeAPI()
    const onGraphMutation = vi.fn().mockRejectedValueOnce(new Error('同步失败')).mockResolvedValueOnce(undefined)
    render(<WorkspaceCanvas snapshot={makeSnapshot()} onSelectNode={vi.fn().mockResolvedValue(undefined)} onGraphMutation={onGraphMutation} api={api} />)

    fireEvent.click(screen.getByRole('button', { name: '＋ 新选题' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('选题节点已保存，但画布同步失败：同步失败')
    expect(api.addNode).toHaveBeenCalledOnce()
    expect(screen.getByRole('button', { name: '仅重试快照同步' })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '仅重试快照同步' }))
    await waitFor(() => expect(screen.getByText('选题节点已同步。')).toBeInTheDocument())
    expect(api.addNode).toHaveBeenCalledOnce()
  })

  it('enforces the zoom range and supports reset and double-click focus', () => {
    flowMocks.fitView.mockClear()
    const api = makeAPI()
    render(<WorkspaceCanvas snapshot={makeSnapshot()} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />)

    expect(screen.getByTestId('zoom-range')).toHaveTextContent('0.6-1.6')
    fireEvent.click(screen.getByRole('button', { name: '重置视图' }))
    expect(flowMocks.fitView).toHaveBeenCalledWith({ padding: 0.28, duration: 180 })
    fireEvent.click(screen.getByRole('button', { name: 'simulate-double-click' }))
    expect(flowMocks.fitView).toHaveBeenLastCalledWith({ nodes: [{ id: 'n_child' }], padding: 0.35, duration: 180 })
  })

  it('offers a keyboard path to connect or reparent an existing prompt to the selected output', async () => {
    const api = makeAPI()
    render(
      <WorkspaceCanvas
        snapshot={makeSnapshot()}
        onSelectNode={vi.fn().mockResolvedValue(undefined)}
        api={api}
        candidateFixtures={{ n_topic_a: [{ title: '候选甲' }] }}
      />,
    )

    fireEvent.change(screen.getByRole('combobox', { name: '连接 / 改挂' }), { target: { value: 'n_child' } })
    fireEvent.click(screen.getByRole('button', { name: '将目标连接到选中节点' }))
    await waitFor(() => expect(api.connect).toHaveBeenCalledWith('n_topic_a', 'n_child', 0, false, 'p_test'))
  })

  it('debounces position persistence and reports failure without changing the graph edge list', async () => {
    const api = makeAPI()
    vi.mocked(api.moveNode).mockRejectedValueOnce(new Error('save failed'))
    render(
      <WorkspaceCanvas
        snapshot={makeSnapshot()}
        onSelectNode={vi.fn().mockResolvedValue(undefined)}
        api={api}
      />,
    )

    vi.useFakeTimers()
    fireEvent.click(screen.getByRole('button', { name: 'simulate-drag' }))
    expect(api.moveNode).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(240)
    expect(api.moveNode).toHaveBeenCalledWith('n_child', 420, 160, 'p_test')
    await vi.waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('保存节点位置失败：save failed'))
    expect(screen.getByTestId('edge-count')).toHaveTextContent('2')
    expect(screen.getByTestId('child-position')).toHaveTextContent('20,40')
  })

  it('cancels queued position writes when the active project changes', async () => {
    const api = makeAPI()
    const original = makeSnapshot()
    const { rerender } = render(
      <WorkspaceCanvas snapshot={original} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />,
    )
    vi.useFakeTimers()
    fireEvent.click(screen.getByRole('button', { name: 'simulate-drag' }))
    const nextProject = {
      ...original,
      project: { ...original.project!, id: 'p_next', name: '另一个工程' },
    }
    rerender(<WorkspaceCanvas snapshot={nextProject} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />)
    await vi.advanceTimersByTimeAsync(240)

    expect(api.moveNode).not.toHaveBeenCalled()
  })

  it('persists selection and rolls it back when SelectNode fails', async () => {
    const api = makeAPI()
    const onSelectNode = vi.fn().mockRejectedValue(new Error('select failed'))
    render(<WorkspaceCanvas snapshot={makeSnapshot()} onSelectNode={onSelectNode} api={api} />)

    fireEvent.click(screen.getByRole('button', { name: 'simulate-select' }))
    await waitFor(() => expect(onSelectNode).toHaveBeenCalledWith('n_child'))
    expect(await screen.findByRole('alert')).toHaveTextContent('保存节点选择失败：select failed')
  })
})
