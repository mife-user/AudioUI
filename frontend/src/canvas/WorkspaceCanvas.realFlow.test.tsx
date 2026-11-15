import { act, createEvent, fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactElement } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import reactFlowStyles from '@xyflow/react/dist/style.css?inline'
import workspaceStyles from '../styles/workspace.css?inline'
import { OutputKind, PendingStatus, RunStatus } from '../../bindings/audio-ui/internal/domain/valueobject'
import type { Node, RunStartResult, WorkspaceSnapshot } from '../../bindings/audio-ui/internal/domain/entity'
import type { GraphAPI } from './graphApi'
import { WorkspaceCanvas } from './WorkspaceCanvas'

const initialElementFromPoint = Object.getOwnPropertyDescriptor(document, 'elementFromPoint')
const initialSetPointerCapture = Object.getOwnPropertyDescriptor(SVGElement.prototype, 'setPointerCapture')

function rect(left: number, top: number, width: number, height: number): DOMRect {
  return {
    x: left,
    y: top,
    left,
    top,
    right: left + width,
    bottom: top + height,
    width,
    height,
    toJSON: () => ({}),
  } as DOMRect
}

class LayoutDOMMatrixReadOnly {
  readonly m22 = 1

  constructor(_transform = 'none') {}
}

class LayoutResizeObserver implements ResizeObserver {
  private readonly targets = new Set<Element>()

  constructor(private readonly callback: ResizeObserverCallback) {}

  observe(target: Element): void {
    this.targets.add(target)
    queueMicrotask(() => {
      if (!this.targets.has(target)) return
      const bounds = target.getBoundingClientRect()
      this.callback([{
        target,
        contentRect: bounds,
        borderBoxSize: [],
        contentBoxSize: [],
        devicePixelContentBoxSize: [],
      } as ResizeObserverEntry], this)
    })
  }

  unobserve(target: Element): void {
    this.targets.delete(target)
  }

  disconnect(): void {
    this.targets.clear()
  }
}

function makeNode(id: string, type: string, parent = '', overrides: Partial<Node> = {}): Node {
  return {
    id,
    type: type as Node['type'],
    title: id,
    parent,
    position: { x: 40, y: 60 },
    size: { width: 280, height: 230 },
    params: {},
    rounds: [],
    locked: false,
    stale: false,
    ...overrides,
  }
}

function makeSnapshot(): WorkspaceSnapshot {
  const topic = makeNode('n_topic', 'topic', '', {
    rounds: [{ n: 1, kind: OutputKind.OutputKindTopics, artifactId: 'sha256:topics', pendingId: 'pending_topics', runId: 'r_topics' }],
  })
  const candidate = makeNode('n_candidate', 'candidate', topic.id, {
    candidateSource: { topicNodeId: topic.id, round: 1, artifactId: 'sha256:topics', index: 0, runId: 'r_topics' },
  })
  const prompt = makeNode('n_prompt', 'prompt', candidate.id, { params: { instruction: '写一段说明。' } })
  const agent = makeNode('n_agent', 'agent', prompt.id)
  const script = makeNode('n_script', 'script', agent.id)
  const path = [topic.id, candidate.id, prompt.id, agent.id, script.id]
  return {
    project: { id: 'p_real_flow', name: '真实画布测试', revision: 4, activeNodeId: topic.id },
    nodes: [topic, candidate, prompt, agent, script],
    edges: [
      { id: 'e_candidate', sourceNodeId: topic.id, targetNodeId: candidate.id, kind: 'candidate_source', scriptVersion: 0 },
      { id: 'e_prompt', sourceNodeId: candidate.id, targetNodeId: prompt.id, kind: 'connection', scriptVersion: 0 },
      { id: 'e_agent', sourceNodeId: prompt.id, targetNodeId: agent.id, kind: 'connection', scriptVersion: 0 },
      { id: 'e_script', sourceNodeId: agent.id, targetNodeId: script.id, kind: 'connection', scriptVersion: 0 },
    ],
    nodePaths: { [topic.id]: [topic.id], [candidate.id]: path.slice(0, 2), [prompt.id]: path.slice(0, 3), [agent.id]: path.slice(0, 4), [script.id]: path },
    pending: [],
    runs: [],
    chains: [{ id: `c_${script.id}`, tailNodeId: script.id, path, nodeIds: path, ambiguous: false, label: '测试链', status: 'idle' }],
    lockedNodeIds: [],
  }
}

function makeAPI(): GraphAPI {
  return {
    addNode: vi.fn().mockResolvedValue({ projectId: 'p_real_flow', projectRevision: 5, nodeId: 'n_created' }),
    connect: vi.fn().mockResolvedValue({ projectId: 'p_real_flow', projectRevision: 5, nodeId: 'n_prompt' }),
    disconnect: vi.fn().mockResolvedValue({ projectId: 'p_real_flow', projectRevision: 5, nodeId: 'n_prompt' }),
    reconnectTarget: vi.fn().mockResolvedValue({ projectId: 'p_real_flow', projectRevision: 5, nodeId: 'n_prompt' }),
    moveNode: vi.fn().mockResolvedValue({ projectId: 'p_real_flow', projectRevision: 5, nodeId: 'n_prompt' }),
    setNodeLayout: vi.fn().mockResolvedValue({ projectId: 'p_real_flow', projectRevision: 5, nodeId: 'n_prompt' }),
    setNodeCollapsed: vi.fn().mockResolvedValue({ projectId: 'p_real_flow', projectRevision: 5, nodeId: 'n_prompt' }),
    setEdgeScriptVersion: vi.fn().mockResolvedValue({ projectId: 'p_real_flow', projectRevision: 5, nodeId: 'n_prompt' }),
    selectNode: vi.fn().mockResolvedValue({ projectId: 'p_real_flow', projectRevision: 5, nodeId: 'n_prompt' }),
    updateNodeParams: vi.fn().mockResolvedValue({ projectId: 'p_real_flow', projectRevision: 5, nodeId: 'n_topic' }),
    getTopicCandidates: vi.fn().mockResolvedValue([]),
    getScriptVersion: vi.fn().mockResolvedValue({ nodeId: 'n_script', version: { n: 1, artifactId: 'sha256:script', runId: 'r_script' }, payload: '{"title":"脚本","scenes":[{"narration":"正文","duration":1}]}' }),
  }
}

function setPointerTarget(target: Element | null): void {
  Object.defineProperty(document, 'elementFromPoint', { configurable: true, value: () => target })
}

describe('WorkspaceCanvas with real React Flow', () => {
  beforeEach(() => {
    vi.stubGlobal('ResizeObserver', LayoutResizeObserver)
    Object.defineProperty(SVGElement.prototype, 'setPointerCapture', { configurable: true, value: vi.fn() })
    Object.defineProperty(document, 'elementFromPoint', { configurable: true, value: () => null })
    vi.stubGlobal('DOMMatrixReadOnly', LayoutDOMMatrixReadOnly)
    vi.spyOn(HTMLElement.prototype, 'offsetWidth', 'get').mockImplementation(function (this: HTMLElement) {
      if (this.classList.contains('react-flow__node')) return 276
      if (this.classList.contains('react-flow')) return 820
      return 0
    })
    vi.spyOn(HTMLElement.prototype, 'offsetHeight', 'get').mockImplementation(function (this: HTMLElement) {
      if (this.classList.contains('react-flow__node')) return 180
      if (this.classList.contains('react-flow')) return 640
      return 0
    })
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
      if (this.classList.contains('react-flow__node')) return rect(40, 60, 276, 180)
      if (this.classList.contains('react-flow__handle')) {
        const nodeId = this.closest<HTMLElement>('.react-flow__node')?.dataset.id
        if (nodeId === 'n_new_agent' || nodeId === 'n_new_prompt') return rect(300, 90, 10, 10)
        return rect(312, 90, 8, 8)
      }
      if (this.classList.contains('react-flow') || this.classList.contains('canvas-flow-root')) return rect(0, 0, 820, 640)
      return rect(0, 0, 820, 640)
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    if (initialElementFromPoint) Object.defineProperty(document, 'elementFromPoint', initialElementFromPoint)
    else Reflect.deleteProperty(document, 'elementFromPoint')
    if (initialSetPointerCapture) Object.defineProperty(SVGElement.prototype, 'setPointerCapture', initialSetPointerCapture)
    else Reflect.deleteProperty(SVGElement.prototype, 'setPointerCapture')
  })

  it('renders measured handles and edges across Topic, Candidate, Prompt, Agent and Script', async () => {
    const onSelectNode = vi.fn().mockResolvedValue(undefined)
    const { container } = render(
      <WorkspaceCanvas
        snapshot={makeSnapshot()}
        onSelectNode={onSelectNode}
        api={makeAPI()}
        candidateFixtures={{ n_topic: [{ nodeId: 'n_candidate', round: 1, runId: 'r_topics', index: 0, title: '候选一', angle: '方向一', hook: '开场一' }] }}
      />,
    )

    await waitFor(() => {
      const nodes = ['n_topic', 'n_candidate', 'n_prompt', 'n_agent', 'n_script']
      nodes.forEach((id) => expect(container.querySelector(`.react-flow__node[data-id="${id}"]`)).toBeVisible())
    })
    expect(container.querySelector('[data-id="n_topic"] [data-handleid="topic-candidate-n_candidate"]')).toBeInTheDocument()
    expect(container.querySelector('[data-id="n_topic"] .canvas-node-head [data-handleid="topic-candidate-n_candidate"]')).toBeInTheDocument()
    expect(container.querySelector('[data-id="n_topic"] .topic-candidate-row [data-handleid="topic-candidate-n_candidate"]')).toBeNull()
    expect(container.querySelector('[data-id="n_candidate"] [data-handleid="candidate-output"]')).toBeInTheDocument()
    await waitFor(() => expect(container.querySelectorAll('.react-flow__edge-path')).toHaveLength(4))

    const promptWrapper = container.querySelector<HTMLElement>('.react-flow__node[data-id="n_prompt"]')
    const topicWrapper = container.querySelector<HTMLElement>('.react-flow__node[data-id="n_topic"]')
    if (!promptWrapper || !topicWrapper) throw new Error('canvas nodes missing from fixture')
    fireEvent.keyDown(promptWrapper, { key: 'Enter', code: 'Enter' })
    await waitFor(() => expect(onSelectNode).toHaveBeenCalledWith('n_prompt'))
    fireEvent.keyDown(topicWrapper, { key: ' ', code: 'Space' })
    await waitFor(() => expect(onSelectNode).toHaveBeenLastCalledWith('n_topic'))
  })

  it('renders one viewport-aware dot pattern whose spacing follows zoom', async () => {
    const { container } = render(
      <WorkspaceCanvas snapshot={makeSnapshot()} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={makeAPI()} />,
    )
    const background = await waitFor(() => {
      const element = container.querySelector<SVGElement>('.react-flow__background')
      if (!element) throw new Error('React Flow background missing from fixture')
      return element
    })
    const pattern = background.querySelector('pattern')
    const dot = pattern?.querySelector('.canvas-dot-pattern')
    const zoomIn = container.querySelector<HTMLButtonElement>('.react-flow__controls-zoomin')
    if (!pattern || !dot || !zoomIn) throw new Error('React Flow dot background missing from fixture')
    const initialWidth = pattern.getAttribute('width')

    expect(container.querySelectorAll('.react-flow__background')).toHaveLength(1)
    expect(background.style.zIndex).toBe('0')
    expect(pattern).toHaveAttribute('patternUnits', 'userSpaceOnUse')
    expect(Number(dot.getAttribute('r'))).toBeGreaterThan(0.5)
    expect(dot.tagName.toLowerCase()).toBe('circle')

    fireEvent.click(zoomIn)
    await waitFor(() => expect(pattern.getAttribute('width')).not.toBe(initialWidth))
  })

  it('keeps an unsaved Topic draft and its measured edge through collapse and expand', async () => {
    const api = makeAPI()
    const { container } = render(
      <WorkspaceCanvas snapshot={makeSnapshot()} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />,
    )
    const topicNode = await waitFor(() => container.querySelector<HTMLElement>('.react-flow__node[data-id="n_topic"] .topic-node')!)
    const topicDraft = container.querySelector<HTMLTextAreaElement>('#topic-idea-n_topic')
    if (!topicDraft) throw new Error('Topic draft missing from fixture')
    fireEvent.change(topicDraft, { target: { value: '尚未保存的想法' } })

    fireEvent.click(screen.getByRole('button', { name: '折叠 n_topic 正文' }))
    await waitFor(() => expect(api.setNodeCollapsed).toHaveBeenCalledWith('n_topic', true, 'p_real_flow'))
    await waitFor(() => expect(topicNode).toHaveClass('is-collapsed'))
    expect(topicDraft.isConnected).toBe(true)
    expect(topicDraft).toHaveValue('尚未保存的想法')
    expect(screen.getByRole('button', { name: '展开 n_topic 正文' })).toHaveAttribute('aria-expanded', 'false')
    await waitFor(() => expect(container.querySelectorAll('.react-flow__edge-path')).toHaveLength(4))

    fireEvent.click(screen.getByRole('button', { name: '展开 n_topic 正文' }))
    await waitFor(() => expect(api.setNodeCollapsed).toHaveBeenCalledWith('n_topic', false, 'p_real_flow'))
    await waitFor(() => expect(topicNode).not.toHaveClass('is-collapsed'))
    expect(topicDraft.isConnected).toBe(true)
    expect(topicDraft).toHaveValue('尚未保存的想法')
  })

  it('locates the real Candidate node from the scrollable Topic list', async () => {
    const onSelectNode = vi.fn().mockResolvedValue(undefined)
    render(<WorkspaceCanvas snapshot={makeSnapshot()} onSelectNode={onSelectNode} api={makeAPI()} />)

    fireEvent.click(await screen.findByRole('button', { name: '定位第 1 批候选 1' }))
    await waitFor(() => expect(onSelectNode).toHaveBeenCalledWith('n_candidate'))
  })

  it('opens creation on the blank pane but leaves the Prompt textarea context menu untouched', async () => {
    const { container } = render(
      <WorkspaceCanvas snapshot={makeSnapshot()} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={makeAPI()} />,
    )
    const promptInput = await waitFor(() => container.querySelector<HTMLTextAreaElement>('.react-flow__node[data-id="n_prompt"] textarea')!)
    const inputContextMenu = createEvent.contextMenu(promptInput, { clientX: 180, clientY: 140 })
    fireEvent(promptInput, inputContextMenu)
    expect(inputContextMenu.defaultPrevented).toBe(false)
    expect(screen.queryByRole('menu', { name: '新建节点' })).not.toBeInTheDocument()
    fireEvent.keyDown(promptInput, { key: 'F10', shiftKey: true })
    expect(screen.queryByRole('menu', { name: '新建节点' })).not.toBeInTheDocument()

    const pane = container.querySelector<HTMLElement>('.react-flow__pane')
    if (!pane) throw new Error('React Flow pane missing')
    fireEvent.contextMenu(pane, { clientX: 760, clientY: 580 })
    expect(await screen.findByRole('menu', { name: '新建节点' })).toBeInTheDocument()
    expect(screen.queryByRole('menu', { name: '选择节点类型' })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('menuitem', { name: '新增节点' }))
    expect(await screen.findByRole('menu', { name: '选择节点类型' })).toBeInTheDocument()
    fireEvent.keyDown(screen.getByRole('menuitem', { name: '选题' }), { key: 'Escape' })
    expect(screen.getByRole('menu', { name: '新建节点' })).toBeInTheDocument()
    expect(screen.queryByRole('menu', { name: '选择节点类型' })).not.toBeInTheDocument()
  })

  it.each(['n_topic', 'n_candidate', 'n_prompt', 'n_agent', 'n_script'])(
    'keeps eight invisible resize controls and connection ports on selected %s',
    async (nodeId) => {
      const styles = document.createElement('style')
      styles.textContent = `${reactFlowStyles}\n${workspaceStyles}`
      document.head.append(styles)
      try {
        const snapshot = makeSnapshot()
        snapshot.project = { ...snapshot.project!, activeNodeId: nodeId }
        const { container } = render(
          <WorkspaceCanvas snapshot={snapshot} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={makeAPI()} />,
        )
        const wrapper = await waitFor(() => {
          const node = container.querySelector<HTMLElement>(`.react-flow__node[data-id="${nodeId}"]`)
          if (!node) throw new Error(`canvas node ${nodeId} missing from fixture`)
          return node
        })
        expect(wrapper).toHaveClass('selected')
        expect(wrapper.querySelector('.node-input-handle')).toBeInTheDocument()
        expect(wrapper.querySelector('.node-output-handle')).toBeInTheDocument()
        expect(wrapper.querySelectorAll('.react-flow__resize-control.line.canvas-node-resize-line')).toHaveLength(4)
        expect(wrapper.querySelectorAll('.react-flow__resize-control.handle.canvas-node-resize-handle')).toHaveLength(4)
        const transparentReference = document.createElement('span')
        transparentReference.style.color = 'transparent'
        document.body.append(transparentReference)
        const transparentColor = getComputedStyle(transparentReference).color
        transparentReference.remove()
        expect(transparentColor).not.toBe('')

        for (const [position, cursor] of [
          ['top.left', 'nwse-resize'],
          ['top.right', 'nesw-resize'],
          ['bottom.left', 'nesw-resize'],
          ['bottom.right', 'nwse-resize'],
        ]) {
          const handle = wrapper.querySelector<HTMLElement>(`.react-flow__resize-control.handle.${position}`)
          if (!handle) throw new Error(`resize handle ${position} missing`)
          const computed = getComputedStyle(handle)
          expect(computed.width).toBe('10px')
          expect(computed.height).toBe('10px')
          expect(computed.borderTopWidth).toBe('1px')
          expect(computed.borderTopColor).toBe(transparentColor)
          expect(computed.backgroundColor).toBe(transparentColor)
          expect(computed.boxShadow).toBe('none')
          expect(computed.cursor).toBe(cursor)
          expect(computed.pointerEvents).not.toBe('none')
          expect(computed.visibility).not.toBe('hidden')
          expect(computed.display).not.toBe('none')
        }

        for (const [position, cursor, dimension] of [
          ['left', 'ew-resize', 'width'],
          ['right', 'ew-resize', 'width'],
          ['top', 'ns-resize', 'height'],
          ['bottom', 'ns-resize', 'height'],
        ] as const) {
          const line = wrapper.querySelector<HTMLElement>(`.react-flow__resize-control.line.${position}`)
          if (!line) throw new Error(`resize line ${position} missing`)
          const computed = getComputedStyle(line)
          expect(line.style.borderColor).toBe('transparent')
          expect(computed[dimension]).toBe('8px')
          expect(computed.cursor).toBe(cursor)
          expect(computed.pointerEvents).not.toBe('none')
        }
        expect(container.querySelectorAll('.react-flow__node:not(.selected) .react-flow__resize-control')).toHaveLength(0)
      } finally {
        styles.remove()
      }
    },
  )

  it.each([
    { control: 'line.left', dx: -20, dy: 0, left: true, top: false },
    { control: 'line.right', dx: 20, dy: 0, left: false, top: false },
    { control: 'line.top', dx: 0, dy: -20, left: false, top: true },
    { control: 'line.bottom', dx: 0, dy: 20, left: false, top: false },
    { control: 'handle.top.left', dx: -20, dy: -20, left: true, top: true },
    { control: 'handle.top.right', dx: 20, dy: -20, left: false, top: true },
    { control: 'handle.bottom.left', dx: -20, dy: 20, left: true, top: false },
    { control: 'handle.bottom.right', dx: 20, dy: 20, left: false, top: false },
  ])('persists the correct anchored layout when resizing from $control', async ({ control, dx, dy, left, top }) => {
    const api = makeAPI()
    const { container } = render(
      <WorkspaceCanvas snapshot={makeSnapshot()} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />,
    )
    await waitFor(() => expect(container.querySelectorAll('.react-flow__edge-path')).toHaveLength(4))
    const handle = container.querySelector<HTMLElement>(`.react-flow__node[data-id="n_topic"] .react-flow__resize-control.${control}`)
    if (!handle) throw new Error(`resize control ${control} missing`)

    const view = handle.ownerDocument.defaultView!
    // jsdom rejects the Window in MouseEventInit, while d3-drag needs event.view.
    const down = createEvent.mouseDown(handle, { button: 0, buttons: 1, clientX: 100, clientY: 100 })
    Object.defineProperty(down, 'view', { value: view })
    fireEvent(handle, down)
    fireEvent.mouseMove(view, { buttons: 1, clientX: 100 + dx, clientY: 100 + dy })
    const up = createEvent.mouseUp(view, { button: 0, clientX: 100 + dx, clientY: 100 + dy })
    Object.defineProperty(up, 'view', { value: view })
    fireEvent(view, up)

    await waitFor(() => expect(api.setNodeLayout).toHaveBeenCalledOnce())
    const [id, x, y, width, height, projectId] = vi.mocked(api.setNodeLayout).mock.calls[0]
    expect(id).toBe('n_topic')
    expect(projectId).toBe('p_real_flow')
    expect(api.moveNode).not.toHaveBeenCalled()
    expect(api.connect).not.toHaveBeenCalled()
    if (dx) {
      expect(width).toBeGreaterThan(276)
      if (left) expect(x + width).toBeCloseTo(40 + 276)
      else expect(x).toBe(40)
    } else {
      expect(x).toBe(40)
      expect(width).toBe(276)
    }
    if (dy) {
      expect(height).toBeGreaterThan(180)
      if (top) expect(y + height).toBeCloseTo(60 + 180)
      else expect(y).toBe(60)
    } else {
      expect(y).toBe(60)
      expect(height).toBe(180)
    }
  })

  it('applies keyboard layout edits to a locked path and persists geometry atomically', async () => {
    const snapshot = makeSnapshot()
    snapshot.lockedNodeIds = ['n_topic', 'n_candidate', 'n_prompt', 'n_agent', 'n_script']
    const api = makeAPI()
    const { container } = render(
      <WorkspaceCanvas snapshot={snapshot} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />,
    )
    const topicWrapper = await waitFor(() => container.querySelector<HTMLElement>('.react-flow__node[data-id="n_topic"]')!)
    expect(topicWrapper.querySelectorAll('.react-flow__resize-control.handle')).toHaveLength(4)
    expect(topicWrapper.querySelectorAll('.react-flow__resize-control.line')).toHaveLength(4)
    expect(screen.getByRole('button', { name: '调整 n_topic 卡片尺寸' })).toBeEnabled()

    fireEvent.click(screen.getByRole('button', { name: '调整 n_topic 卡片尺寸' }))
    fireEvent.change(screen.getByRole('spinbutton', { name: '宽度' }), { target: { value: '360' } })
    fireEvent.change(screen.getByRole('spinbutton', { name: '高度' }), { target: { value: '270' } })
    fireEvent.click(screen.getByRole('button', { name: '应用' }))

    await waitFor(() => expect(api.setNodeLayout).toHaveBeenCalledWith('n_topic', 40, 60, 360, 270, 'p_real_flow'))
    expect(api.moveNode).not.toHaveBeenCalled()
  })

  it('keeps the QuickBar and Script edge stable when selecting a chain and appending a version', async () => {
    const snapshot = makeSnapshot()
    const script = snapshot.nodes?.find((node) => node.id === 'n_script')
    if (!script) throw new Error('script missing from fixture')
    script.scriptVersions = [{ n: 1, artifactId: 'sha256:script-v1', runId: 'r_v1' }]
    const onSelectNode = vi.fn().mockResolvedValue(undefined)
    const api = makeAPI()
    const { container, rerender } = render(
      <WorkspaceCanvas snapshot={snapshot} onSelectNode={onSelectNode} api={api} candidateFixtures={{ n_topic: [{ index: 0, title: '候选一' }] }} />,
    )

    await waitFor(() => expect(container.querySelectorAll('.react-flow__edge-path')).toHaveLength(4))
    fireEvent.pointerEnter(screen.getByRole('navigation', { name: '链路选择' }))
    fireEvent.click(screen.getByRole('button', { name: /选择链路 测试链/ }))
    expect(onSelectNode).toHaveBeenCalledWith('n_script')

    const nextSnapshot: WorkspaceSnapshot = {
      ...snapshot,
      project: { ...snapshot.project!, revision: 5, activeNodeId: 'n_script' },
      nodes: snapshot.nodes?.map((node) => node.id === script.id
        ? { ...node, scriptVersions: [...(node.scriptVersions ?? []), { n: 2, artifactId: 'sha256:script-v2', runId: 'r_v2' }] }
        : node) ?? [],
    }
    rerender(
      <WorkspaceCanvas snapshot={nextSnapshot} onSelectNode={onSelectNode} api={api} candidateFixtures={{ n_topic: [{ index: 0, title: '候选一' }] }} />,
    )

    await waitFor(() => expect(container.querySelectorAll('.react-flow__edge-path')).toHaveLength(4))
    expect(screen.getByText('2 个不可变版本 · 最新 v2')).toBeInTheDocument()
    expect(container.querySelector('.canvas-message-error')?.textContent ?? '').not.toMatch(/渲染错误 (008|015)/)
  })

  it('keeps all edges visible when selecting a node rebuilds the controlled snapshot', async () => {
    const snapshot = makeSnapshot()
    const api = makeAPI()
    let rerender: (ui: ReactElement) => void = () => undefined
    const onSelectNode = vi.fn(async (id: string) => {
      const selected = { ...snapshot, project: { ...snapshot.project!, activeNodeId: id } }
      rerender(<WorkspaceCanvas snapshot={selected} onSelectNode={onSelectNode} api={api} candidateFixtures={{ n_topic: [{ index: 0, title: '候选一' }] }} />)
    })
    const view = render(
      <WorkspaceCanvas snapshot={snapshot} onSelectNode={onSelectNode} api={api} candidateFixtures={{ n_topic: [{ index: 0, title: '候选一' }] }} />,
    )
    rerender = view.rerender

    const promptWrapper = await waitFor(() => {
      const node = view.container.querySelector<HTMLElement>('.react-flow__node[data-id="n_prompt"]')
      if (!node) throw new Error('prompt wrapper not rendered')
      expect(node).toHaveStyle({ visibility: 'visible' })
      return node
    })
    await waitFor(() => expect(view.container.querySelectorAll('.react-flow__edge-path')).toHaveLength(4))

    fireEvent.keyDown(promptWrapper, { key: 'Enter', code: 'Enter' })
    await waitFor(() => expect(onSelectNode).toHaveBeenCalledWith('n_prompt'))
    await waitFor(() => expect(promptWrapper).toHaveClass('selected'))
    await waitFor(() => expect(view.container.querySelectorAll('.react-flow__edge-path')).toHaveLength(4))

    fireEvent.keyDown(promptWrapper, { key: 'ArrowRight', code: 'ArrowRight' })
    await waitFor(() => expect(api.moveNode).toHaveBeenCalledWith('n_prompt', expect.any(Number), 60, 'p_real_flow'), { timeout: 2000 })
    expect(vi.mocked(api.moveNode).mock.calls.at(-1)?.[1]).toBeGreaterThan(40)
    expect(view.container.querySelector('.canvas-message-error')?.textContent ?? '').not.toMatch(/渲染错误 (008|015)/)
  })

  it('loads the immutable Candidate content after the graph snapshot and preserves the edge layout', async () => {
    let resolveCandidates!: (candidates: Awaited<ReturnType<GraphAPI['getTopicCandidates']>>) => void
    const api = makeAPI()
    vi.mocked(api.getTopicCandidates).mockReturnValue(new Promise((resolve) => { resolveCandidates = resolve }))
    const { container } = render(<WorkspaceCanvas snapshot={makeSnapshot()} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />)

    await waitFor(() => expect(container.querySelector('[data-handleid="topic-candidate-n_candidate"]')).toBeInTheDocument())
    await waitFor(() => expect(container.querySelectorAll('.react-flow__edge-path')).toHaveLength(4))
    expect(container.querySelector('.canvas-message-error')?.textContent ?? '').not.toContain('渲染错误 008')

    await act(async () => {
      resolveCandidates([{ nodeId: 'n_candidate', round: 1, runId: 'r_topics', index: 0, title: '候选一', angle: '方向一', hook: '开场一' }])
      await Promise.resolve()
    })
    await waitFor(() => expect(container.querySelector('.candidate-node')).toHaveTextContent('方向一'))
    await waitFor(() => expect(container.querySelectorAll('.react-flow__edge-path')).toHaveLength(4))
    expect(container.querySelector('.canvas-message-error')?.textContent ?? '').not.toMatch(/渲染错误 (008|015)/)
  })

  it('uses exact Candidate node IDs and ignores same-index results from other nodes', async () => {
    const api = makeAPI()
    vi.mocked(api.getTopicCandidates).mockResolvedValue([
      { nodeId: 'n_candidate', round: 2, runId: 'r_topics_2', index: 2, title: '候选三', angle: '方向三', hook: '开场三' },
      { nodeId: 'n_other_candidate', round: 1, runId: 'r_topics_1', index: 2, title: '旧候选三', angle: '旧方向', hook: '旧开场' },
    ])
    const snapshot = makeSnapshot()
    const candidate = snapshot.nodes?.find((node) => node.id === 'n_candidate')
    if (!candidate) throw new Error('candidate missing from fixture')
    candidate.candidateSource = { topicNodeId: 'n_topic', round: 1, artifactId: 'sha256:topics', index: 2, runId: 'r_topics' }
    const { container } = render(<WorkspaceCanvas snapshot={snapshot} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />)

    await waitFor(() => expect(container.querySelector('.candidate-node')).toHaveTextContent('候选三'))
    expect(api.getTopicCandidates).toHaveBeenCalledWith('n_topic', 'p_real_flow')
    expect(container.querySelector('[data-id="n_topic"] [data-handleid="topic-candidate-n_candidate"]')).toBeInTheDocument()
  })

  it('shows candidate read errors without hiding the authoritative graph edge', async () => {
    const api = makeAPI()
    vi.mocked(api.getTopicCandidates).mockRejectedValue(new Error('CANDIDATE_INVALID'))
    const { container } = render(<WorkspaceCanvas snapshot={makeSnapshot()} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />)

    await waitFor(() => expect(container.querySelector('.topic-candidate-error')).toHaveTextContent('CANDIDATE_INVALID'))
    await waitFor(() => expect(container.querySelectorAll('.react-flow__edge-path')).toHaveLength(4))
  })

  it('ignores candidate results that arrive after the project changes', async () => {
    let resolveOld!: (candidates: Awaited<ReturnType<GraphAPI['getTopicCandidates']>>) => void
    const api = makeAPI()
    vi.mocked(api.getTopicCandidates).mockReturnValueOnce(new Promise((resolve) => { resolveOld = resolve }))
    const first = makeSnapshot()
    const { container, rerender } = render(<WorkspaceCanvas snapshot={first} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />)
    const second: WorkspaceSnapshot = {
      ...first,
      project: { ...first.project!, id: 'p_next', name: '另一个工程' },
    }
    rerender(<WorkspaceCanvas snapshot={second} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />)
    await waitFor(() => expect(api.getTopicCandidates).toHaveBeenCalledWith('n_topic', 'p_next'))
    resolveOld([{ index: 0, title: '旧工程候选', angle: '旧方向', hook: '旧钩子' }])

    await waitFor(() => expect(container.querySelector('.candidate-node')).not.toHaveTextContent('旧工程候选'))
  })

  it.each([
    ['选题', '#topic-idea-n_topic', 'n_topic'],
    ['提示词', '#prompt-instruction-n_prompt', 'n_prompt'],
  ])('keeps the %s input editable and focused during an automatic save', async (_label, selector, nodeId) => {
    let finishFirstSave!: (receipt: Awaited<ReturnType<GraphAPI['updateNodeParams']>>) => void
    const api = makeAPI()
    vi.mocked(api.updateNodeParams)
      .mockImplementationOnce(() => new Promise((resolve) => { finishFirstSave = resolve }))
      .mockResolvedValue({ projectId: 'p_real_flow', projectRevision: 6, nodeId })
    const { container } = render(
      <WorkspaceCanvas snapshot={makeSnapshot()} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />,
    )
    const input = await waitFor(() => {
      const element = container.querySelector<HTMLTextAreaElement>(selector)
      if (!element) throw new Error('editor missing from fixture')
      return element
    })
    input.focus()
    fireEvent.change(input, { target: { value: '第一段输入' } })
    await waitFor(() => expect(api.updateNodeParams).toHaveBeenCalledOnce())

    expect(input).toBeEnabled()
    expect(input).toHaveFocus()
    fireEvent.change(input, { target: { value: '第一段输入，继续编辑' } })
    await act(async () => {
      finishFirstSave({ projectId: 'p_real_flow', projectRevision: 5, nodeId })
      await Promise.resolve()
    })
    await waitFor(() => expect(api.updateNodeParams).toHaveBeenCalledTimes(2))
    expect(vi.mocked(api.updateNodeParams).mock.calls[1][2]).toEqual(expect.objectContaining(
      nodeId === 'n_topic' ? { idea: '第一段输入，继续编辑' } : { instruction: '第一段输入，继续编辑' },
    ))
    expect(input).toHaveValue('第一段输入，继续编辑')
    expect(input).toHaveFocus()
  })

  it.each([
    ['选题', '#topic-idea-n_topic', 'n_topic', 'idea'],
    ['提示词', '#prompt-instruction-n_prompt', 'n_prompt', 'instruction'],
  ])('keeps the latest %s draft when the previous save snapshot arrives', async (_label, selector, nodeId, field) => {
    const api = makeAPI()
    const original = makeSnapshot()
    const snapshots = new Map<number, WorkspaceSnapshot>()
    let releaseFirstSave!: (receipt: Awaited<ReturnType<GraphAPI['updateNodeParams']>>) => void
    let revision = 4
    let rerenderCanvas: (snapshot: WorkspaceSnapshot) => void = () => undefined
    vi.mocked(api.updateNodeParams).mockImplementation((id, title, params, ownerProjectId) => {
      revision += 1
      const currentRevision = revision
      const next: WorkspaceSnapshot = {
        ...original,
        project: { ...original.project!, revision: currentRevision },
        nodes: original.nodes?.map((node) => node.id === id ? { ...node, title, params } : node) ?? [],
      }
      snapshots.set(currentRevision, next)
      const receipt = { projectId: ownerProjectId, projectRevision: currentRevision, nodeId: id }
      return currentRevision === 5 ? new Promise((resolve) => { releaseFirstSave = resolve }) : Promise.resolve(receipt)
    })
    const onGraphMutation = vi.fn(async (receipt: Awaited<ReturnType<GraphAPI['updateNodeParams']>>) => {
      const next = snapshots.get(receipt.projectRevision)
      if (!next) throw new Error('snapshot missing from fixture')
      rerenderCanvas(next)
    })
    const props = { onSelectNode: vi.fn().mockResolvedValue(undefined), api, onGraphMutation }
    const view = render(<WorkspaceCanvas snapshot={original} {...props} />)
    rerenderCanvas = (snapshot) => view.rerender(<WorkspaceCanvas snapshot={snapshot} {...props} />)
    const input = await waitFor(() => {
      const element = view.container.querySelector<HTMLTextAreaElement>(selector)
      if (!element) throw new Error('editor missing from fixture')
      return element
    })
    fireEvent.change(input, { target: { value: '先保存的内容' } })
    await waitFor(() => expect(api.updateNodeParams).toHaveBeenCalledOnce())
    fireEvent.change(input, { target: { value: '最后输入的内容' } })
    await act(async () => {
      releaseFirstSave({ projectId: 'p_real_flow', projectRevision: 5, nodeId })
      await Promise.resolve()
    })
    expect(input).toHaveValue('最后输入的内容')
    await waitFor(() => expect(onGraphMutation).toHaveBeenCalledTimes(2))
    expect(input).toHaveValue('最后输入的内容')
    const latestSnapshot = snapshots.get(6)
    expect(latestSnapshot?.nodes?.find((node) => node.id === nodeId)?.params).toEqual(expect.objectContaining({ [field]: '最后输入的内容' }))
    view.unmount()
    const reopened = render(<WorkspaceCanvas snapshot={latestSnapshot!} {...props} />)
    await waitFor(() => expect(reopened.container.querySelector(selector)).toHaveValue('最后输入的内容'))
  })

  it('serializes saves across nodes and preserves the newer draft when snapshot synchronization fails', async () => {
    let failFirstSync!: (reason: Error) => void
    const firstSync = new Promise<void>((_resolve, reject) => { failFirstSync = reject })
    const onGraphMutation = vi.fn()
      .mockImplementationOnce(() => firstSync)
      .mockResolvedValue(undefined)
    const api = makeAPI()
    const { container } = render(
      <WorkspaceCanvas snapshot={makeSnapshot()} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} onGraphMutation={onGraphMutation} />,
    )
    const topic = await waitFor(() => container.querySelector<HTMLTextAreaElement>('#topic-idea-n_topic')!)
    const prompt = container.querySelector<HTMLTextAreaElement>('#prompt-instruction-n_prompt')
    if (!topic || !prompt) throw new Error('editors missing from fixture')
    fireEvent.change(topic, { target: { value: '选题新内容' } })
    await waitFor(() => expect(onGraphMutation).toHaveBeenCalledOnce())
    fireEvent.change(prompt, { target: { value: '提示词新内容' } })
    await waitFor(() => expect(container.querySelector('.prompt-parameters .topic-save-row')).toHaveTextContent('保存中'))
    expect(api.updateNodeParams).toHaveBeenCalledOnce()

    await act(async () => { failFirstSync(new Error('快照暂不可用')); await Promise.resolve() })
    await waitFor(() => expect(container.querySelector('.prompt-parameters .topic-save-error')).toHaveTextContent('WORKSPACE_SYNC_STALE'))
    expect(prompt).toHaveValue('提示词新内容')
    expect(api.updateNodeParams).toHaveBeenCalledOnce()
    fireEvent.click(screen.getByRole('button', { name: '仅重试快照同步' }))
    await waitFor(() => expect(prompt).toBeEnabled())
    const retry = container.querySelector<HTMLButtonElement>('.prompt-parameters .topic-retry-save')
    if (!retry) throw new Error('retry control missing from fixture')
    fireEvent.click(retry)
    await waitFor(() => expect(api.updateNodeParams).toHaveBeenCalledTimes(2))
    expect(vi.mocked(api.updateNodeParams).mock.calls[1][2]).toEqual(expect.objectContaining({ instruction: '提示词新内容' }))
    expect(prompt).toHaveValue('提示词新内容')
  })

  it('flushes current-project Topic and Prompt drafts before a project switch', async () => {
    let flushDrafts: ((projectId: string) => Promise<boolean>) | undefined
    const onRegisterDraftFlush = vi.fn((flush: (projectId: string) => Promise<boolean>) => {
      flushDrafts = flush
      return () => {
        if (flushDrafts === flush) flushDrafts = undefined
      }
    })
    const api = makeAPI()
    const { container } = render(
      <WorkspaceCanvas snapshot={makeSnapshot()} onSelectNode={vi.fn().mockResolvedValue(undefined)} onRegisterDraftFlush={onRegisterDraftFlush} api={api} />,
    )
    const topic = await waitFor(() => container.querySelector<HTMLTextAreaElement>('#topic-idea-n_topic')!)
    const prompt = container.querySelector<HTMLTextAreaElement>('#prompt-instruction-n_prompt')
    if (!prompt) throw new Error('Prompt editor missing from fixture')
    await waitFor(() => expect(onRegisterDraftFlush).toHaveBeenCalledOnce())

    fireEvent.change(topic, { target: { value: '切换前保存的选题草稿' } })
    fireEvent.change(prompt, { target: { value: '切换前保存的提示词草稿' } })
    await act(async () => { await expect(flushDrafts?.('p_real_flow')).resolves.toBe(true) })

    expect(vi.mocked(api.updateNodeParams).mock.calls.map(([nodeId]) => nodeId)).toEqual(['n_topic', 'n_prompt'])
  })

  it('retries only the pending snapshot receipt before switching projects', async () => {
    let flushDrafts: ((projectId: string) => Promise<boolean>) | undefined
    const onRegisterDraftFlush = vi.fn((flush: (projectId: string) => Promise<boolean>) => {
      flushDrafts = flush
      return () => {
        if (flushDrafts === flush) flushDrafts = undefined
      }
    })
    const api = makeAPI()
    const onGraphMutation = vi.fn()
      .mockRejectedValueOnce(new Error('快照暂不可用'))
      .mockResolvedValue(undefined)
    const { container } = render(
      <WorkspaceCanvas snapshot={makeSnapshot()} onSelectNode={vi.fn().mockResolvedValue(undefined)} onRegisterDraftFlush={onRegisterDraftFlush} api={api} onGraphMutation={onGraphMutation} />,
    )
    const topic = await waitFor(() => container.querySelector<HTMLTextAreaElement>('#topic-idea-n_topic')!)
    await waitFor(() => expect(onRegisterDraftFlush).toHaveBeenCalledOnce())
    fireEvent.change(topic, { target: { value: '已提交等待同步的选题' } })
    await waitFor(() => expect(onGraphMutation).toHaveBeenCalledOnce())

    await act(async () => { await expect(flushDrafts?.('p_real_flow')).resolves.toBe(true) })
    expect(onGraphMutation).toHaveBeenCalledTimes(2)
    expect(api.updateNodeParams).toHaveBeenCalledOnce()
    expect(screen.queryByRole('button', { name: '仅重试快照同步' })).not.toBeInTheDocument()
  })

  it('clears a pending receipt when a newer snapshot already includes its revision', async () => {
    const api = makeAPI()
    const original = makeSnapshot()
    const onGraphMutation = vi.fn().mockRejectedValue(new Error('快照读取暂不可用'))
    const onSelectNode = vi.fn().mockResolvedValue(undefined)
    const view = render(
      <WorkspaceCanvas snapshot={original} onSelectNode={onSelectNode} api={api} onGraphMutation={onGraphMutation} />,
    )
    const topic = await waitFor(() => view.container.querySelector<HTMLTextAreaElement>('#topic-idea-n_topic')!)
    fireEvent.change(topic, { target: { value: '后续快照已包含的修改' } })
    await waitFor(() => expect(onGraphMutation).toHaveBeenCalledOnce())
    const retry = await screen.findByRole('button', { name: '仅重试快照同步' })
    view.rerender(
      <WorkspaceCanvas
        snapshot={{ ...original, project: { ...original.project!, revision: 6 } }}
        onSelectNode={onSelectNode}
        api={api}
        onGraphMutation={onGraphMutation}
      />,
    )
    fireEvent.click(retry)

    await waitFor(() => expect(screen.queryByRole('button', { name: '仅重试快照同步' })).not.toBeInTheDocument())
    await waitFor(() => expect(topic).toBeEnabled())
    expect(onGraphMutation).toHaveBeenCalledTimes(2)
    expect(api.updateNodeParams).toHaveBeenCalledOnce()
  })

  it('blocks Agent start until path Prompt edits persist', async () => {
    const api = makeAPI()
    vi.mocked(api.updateNodeParams).mockRejectedValueOnce(new Error('disk full'))
    const onStartRun = vi.fn().mockResolvedValue({})
    const { container } = render(
      <WorkspaceCanvas snapshot={makeSnapshot()} onSelectNode={vi.fn().mockResolvedValue(undefined)} onStartRun={onStartRun} api={api} />,
    )
    const task = container.querySelector<HTMLTextAreaElement>('#prompt-instruction-n_prompt')
    const runButton = container.querySelector<HTMLButtonElement>('.agent-run-button')
    if (!task || !runButton) throw new Error('prompt or Agent controls missing from fixture')

    fireEvent.change(task, { target: { value: '保存失败时禁止发送。' } })
    fireEvent.click(runButton)
    fireEvent.click(screen.getByRole('button', { name: '确认并运行' }))
    await waitFor(() => expect(api.updateNodeParams).toHaveBeenCalledOnce())
    await waitFor(() => expect(container.querySelector('.topic-save-error')).toHaveTextContent('disk full'))

    expect(task).toHaveValue('保存失败时禁止发送。')
    expect(onStartRun).not.toHaveBeenCalled()
    expect(container.querySelector('.agent-run-error')).toHaveTextContent('RUN_START_FAILED: disk full')
  })

  it('blocks Agent start until the path Prompt snapshot is synchronized', async () => {
    const api = makeAPI()
    const onGraphMutation = vi.fn().mockRejectedValue(new Error('snapshot offline'))
    const onStartRun = vi.fn().mockResolvedValue({})
    const { container } = render(
      <WorkspaceCanvas
        snapshot={makeSnapshot()}
        onSelectNode={vi.fn().mockResolvedValue(undefined)}
        onGraphMutation={onGraphMutation}
        onStartRun={onStartRun}
        api={api}
      />,
    )
    const task = container.querySelector<HTMLTextAreaElement>('#prompt-instruction-n_prompt')
    const runButton = container.querySelector<HTMLButtonElement>('.agent-run-button')
    if (!task || !runButton) throw new Error('prompt or Agent controls missing from fixture')

    fireEvent.change(task, { target: { value: '快照同步失败时禁止发送。' } })
    fireEvent.click(runButton)
    fireEvent.click(screen.getByRole('button', { name: '确认并运行' }))

    await waitFor(() => expect(api.updateNodeParams).toHaveBeenCalledOnce())
    await waitFor(() => expect(onGraphMutation).toHaveBeenCalledOnce())
    await waitFor(() => expect(screen.getByRole('button', { name: '仅重试快照同步' })).toBeInTheDocument())
    expect(onStartRun).not.toHaveBeenCalled()
  })

  it('flushes only the target ancestors and keeps a downstream Prompt editable while starting', async () => {
    const snapshot = makeSnapshot()
    const downstreamPrompt = makeNode('n_prompt_b', 'prompt', 'n_script', { params: { instruction: '下游任务。' } })
    snapshot.nodes = [...(snapshot.nodes ?? []), downstreamPrompt]
    const leaf = snapshot.chains?.[0]
    if (!leaf) throw new Error('chain missing from fixture')
    snapshot.chains = [{
      ...leaf,
      id: 'c_n_prompt_b',
      tailNodeId: downstreamPrompt.id,
      path: [...(leaf.path ?? []), downstreamPrompt.id],
      label: '完整叶子链',
    }]

    let finishRun!: (result: RunStartResult) => void
    const onStartRun = vi.fn(() => new Promise<RunStartResult>((resolve) => { finishRun = resolve }))
    const api = makeAPI()
    const { container } = render(
      <WorkspaceCanvas snapshot={snapshot} onSelectNode={vi.fn().mockResolvedValue(undefined)} onStartRun={onStartRun} api={api} />,
    )
    const upstreamTask = await waitFor(() => container.querySelector<HTMLTextAreaElement>('#prompt-instruction-n_prompt')!)
    const downstreamTitle = container.querySelector<HTMLInputElement>('#prompt-title-n_prompt_b')
    const runButton = container.querySelector<HTMLButtonElement>('.agent-run-button')
    if (!downstreamTitle || !runButton) throw new Error('downstream Prompt or Agent controls missing from fixture')

    fireEvent.change(downstreamTitle, { target: { value: '' } })
    fireEvent.change(upstreamTask, { target: { value: '最新的上游任务。' } })
    fireEvent.click(runButton)
    fireEvent.click(screen.getByRole('button', { name: '确认并运行' }))

    await waitFor(() => expect(onStartRun).toHaveBeenCalledWith('n_agent', 'p_real_flow', {
      inputEdgeIds: undefined, outputNodeId: 'n_script', createOutput: false,
    }))
    expect(api.updateNodeParams).toHaveBeenCalledOnce()
    expect(vi.mocked(api.updateNodeParams).mock.calls[0][0]).toBe('n_prompt')
    expect(upstreamTask).toBeDisabled()
    expect(downstreamTitle).toBeEnabled()
    expect(container.querySelector('.react-flow__node[data-id="n_prompt_b"] .prompt-node')).not.toHaveClass('is-running-locked')
    expect(container.querySelector('.react-flow__node[data-id="n_script"] .script-node')).toHaveClass('is-running-locked')

    await act(async () => { finishRun({}); await Promise.resolve() })
  })

  it('keeps a newer failed receipt pending when an older position sync completes', async () => {
    let finishPositionSync!: () => void
    const positionSync = new Promise<void>((resolve) => { finishPositionSync = resolve })
    const api = makeAPI()
    vi.mocked(api.moveNode).mockResolvedValue({ projectId: 'p_real_flow', projectRevision: 5, nodeId: 'n_prompt' })
    vi.mocked(api.updateNodeParams).mockResolvedValue({ projectId: 'p_real_flow', projectRevision: 6, nodeId: 'n_topic' })
    let latestSyncAttempts = 0
    const onGraphMutation = vi.fn((receipt: Awaited<ReturnType<GraphAPI['moveNode']>>) => {
      if (receipt.projectRevision === 5) return positionSync
      if (latestSyncAttempts++ === 0) return Promise.reject(new Error('最新快照暂不可用'))
      return Promise.resolve()
    })
    const { container } = render(
      <WorkspaceCanvas snapshot={makeSnapshot()} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} onGraphMutation={onGraphMutation} />,
    )
    const promptNode = await waitFor(() => container.querySelector<HTMLElement>('.react-flow__node[data-id="n_prompt"]')!)
    fireEvent.keyDown(promptNode, { key: 'Enter', code: 'Enter' })
    await waitFor(() => expect(promptNode).toHaveClass('selected'))
    fireEvent.keyDown(promptNode, { key: 'ArrowRight', code: 'ArrowRight' })
    await waitFor(() => expect(api.moveNode).toHaveBeenCalledOnce(), { timeout: 2000 })
    const topic = container.querySelector<HTMLTextAreaElement>('#topic-idea-n_topic')
    if (!topic) throw new Error('Topic editor missing from fixture')
    fireEvent.change(topic, { target: { value: '并发保存选题' } })
    await waitFor(() => expect(onGraphMutation).toHaveBeenCalledTimes(2))
    await waitFor(() => expect(screen.getByRole('button', { name: '仅重试快照同步' })).toBeInTheDocument())

    await act(async () => { finishPositionSync(); await Promise.resolve() })
    expect(screen.getByRole('button', { name: '仅重试快照同步' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '仅重试快照同步' }))
    await waitFor(() => expect(onGraphMutation).toHaveBeenCalledTimes(3))
    expect(onGraphMutation).toHaveBeenNthCalledWith(3, expect.objectContaining({ projectRevision: 6 }))
    expect(api.moveNode).toHaveBeenCalledOnce()
    expect(api.updateNodeParams).toHaveBeenCalledOnce()
  })

  it('ignores a late failure for an older position receipt after a newer parameter receipt synced', async () => {
    let failPositionSync!: (reason: Error) => void
    const positionSync = new Promise<void>((_resolve, reject) => { failPositionSync = reject })
    const api = makeAPI()
    vi.mocked(api.moveNode).mockResolvedValue({ projectId: 'p_real_flow', projectRevision: 5, nodeId: 'n_prompt' })
    vi.mocked(api.updateNodeParams).mockResolvedValue({ projectId: 'p_real_flow', projectRevision: 6, nodeId: 'n_topic' })
    const onGraphMutation = vi.fn((receipt: Awaited<ReturnType<GraphAPI['moveNode']>>) =>
      receipt.projectRevision === 5 ? positionSync : Promise.resolve(),
    )
    const { container } = render(
      <WorkspaceCanvas snapshot={makeSnapshot()} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} onGraphMutation={onGraphMutation} />,
    )
    const promptNode = await waitFor(() => container.querySelector<HTMLElement>('.react-flow__node[data-id="n_prompt"]')!)
    fireEvent.keyDown(promptNode, { key: 'Enter', code: 'Enter' })
    await waitFor(() => expect(promptNode).toHaveClass('selected'))
    fireEvent.keyDown(promptNode, { key: 'ArrowRight', code: 'ArrowRight' })
    await waitFor(() => expect(api.moveNode).toHaveBeenCalledOnce(), { timeout: 2000 })
    const topic = container.querySelector<HTMLTextAreaElement>('#topic-idea-n_topic')
    if (!topic) throw new Error('Topic editor missing from fixture')
    fireEvent.change(topic, { target: { value: '更新后的选题' } })
    await waitFor(() => expect(onGraphMutation).toHaveBeenCalledTimes(2))
    expect(screen.queryByRole('button', { name: '仅重试快照同步' })).not.toBeInTheDocument()

    await act(async () => { failPositionSync(new Error('旧位置回执同步失败')); await Promise.resolve() })
    expect(screen.queryByRole('button', { name: '仅重试快照同步' })).not.toBeInTheDocument()
    expect(container.querySelector('.canvas-message-error')).toBeNull()
    expect(api.moveNode).toHaveBeenCalledOnce()
    expect(api.updateNodeParams).toHaveBeenCalledOnce()
  })

  it('shows the latest Agent failure while preserving a separate pending result count', async () => {
    const snapshot = makeSnapshot()
    snapshot.runs = [{
      id: 'r_failed', nodeId: 'n_agent', status: RunStatus.RunFailed,
      chainId: 'c_n_agent', chainPath: ['n_topic', 'n_candidate', 'n_prompt', 'n_agent'], estimatedTokens: 100,
      startedAt: '2026-11-05T00:00:00Z',
    }]
    snapshot.pending = [{
      id: 'pending_one', nodeId: 'n_agent', runId: 'r_previous', chainId: 'c_n_agent',
      kind: OutputKind.OutputKindScript, status: PendingStatus.PendingPending,
    }]
    const { container } = render(
      <WorkspaceCanvas snapshot={snapshot} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={makeAPI()} candidateFixtures={{ n_topic: [{ index: 0, title: '候选一' }] }} />,
    )

    await waitFor(() => expect(container.querySelector('.react-flow__node[data-id="n_agent"] .canvas-node-status')).toHaveTextContent('运行失败 · 待处理 1'))
  })

  it('offsets shared-target reconnect grips by edge identity and protects locked and Candidate edges', async () => {
    const snapshot = makeSnapshot()
    snapshot.edges = [...(snapshot.edges ?? []), {
      id: 'e_topic_agent', sourceNodeId: 'n_topic', targetNodeId: 'n_agent', kind: 'connection', scriptVersion: 0,
    }]
    const view = render(<WorkspaceCanvas snapshot={snapshot} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={makeAPI()} />)
    const firstGrip = await waitFor(() => {
      const current = view.container.querySelector<HTMLElement>('[data-testid="reconnect-grip-e_agent"]')
      if (!current) throw new Error('first reconnect grip missing')
      return current
    })
    const secondGrip = view.container.querySelector<HTMLElement>('[data-testid="reconnect-grip-e_topic_agent"]')

    expect(view.container.querySelector('.canvas-edge-reconnect-grip-mark')).not.toBeInTheDocument()
    expect(secondGrip).toBeInTheDocument()
    expect(firstGrip.getAttribute('cy')).not.toBe(secondGrip?.getAttribute('cy'))
    expect(view.container.querySelector('[data-testid="reconnect-grip-e_candidate"]')).not.toBeInTheDocument()

    snapshot.lockedNodeIds = ['n_prompt']
    view.rerender(<WorkspaceCanvas snapshot={snapshot} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={makeAPI()} />)
    await waitFor(() => expect(view.container.querySelector('[data-testid="reconnect-grip-e_agent"]')).not.toBeInTheDocument())
    expect(view.container.querySelector('[data-testid="reconnect-grip-e_topic_agent"]')).toBeInTheDocument()
  })

  it('submits the selected edge only after dropping on a real input handle', async () => {
    const api = makeAPI()
    const { container } = render(<WorkspaceCanvas snapshot={makeSnapshot()} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />)
    const grip = await waitFor(() => {
      const current = container.querySelector<HTMLElement>('[data-testid="reconnect-grip-e_agent"]')
      if (!current) throw new Error('reconnect grip missing')
      return current
    })
    const targetHandle = container.querySelector<HTMLElement>('[data-id="n_script"] .node-input-handle')
    if (!targetHandle) throw new Error('target input handle missing from fixture')
    setPointerTarget(targetHandle)

    fireEvent.pointerDown(grip, { button: 0, pointerId: 1, clientX: 120, clientY: 160 })
    fireEvent.pointerMove(grip, { pointerId: 1, clientX: 260, clientY: 220 })
    await waitFor(() => expect(container.querySelector('.canvas-edge-reconnect-preview')).toHaveClass('is-valid'))
    expect(targetHandle.closest('.react-flow__node')).toHaveClass('canvas-reconnect-drop-valid')
    fireEvent.pointerUp(grip, { button: 0, pointerId: 1, clientX: 260, clientY: 220 })

    await waitFor(() => expect(api.reconnectTarget).toHaveBeenCalledWith({
      projectId: 'p_real_flow',
      expectedEdge: { id: 'e_agent', sourceNodeId: 'n_prompt', targetNodeId: 'n_agent', kind: 'connection', scriptVersion: 0 },
      targetNodeId: 'n_script',
    }))
    expect(container.querySelectorAll('.react-flow__edge-path')).toHaveLength(4)
  })

  it.each([
    ['Agent', 'agent', 'n_new_agent'],
    ['提示词', 'prompt', 'n_new_prompt'],
  ] as const)('reconnects to a new %s through its pointer-transparent input handle', async (_label, type, targetNodeId) => {
    const snapshot = makeSnapshot()
    snapshot.nodes = [...(snapshot.nodes ?? []), makeNode(targetNodeId, type)]
    const api = makeAPI()
    const { container } = render(<WorkspaceCanvas snapshot={snapshot} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />)
    const grip = await waitFor(() => {
      const current = container.querySelector<HTMLElement>('[data-testid="reconnect-grip-e_agent"]')
      if (!current) throw new Error('reconnect grip missing')
      return current
    })
    const targetNode = container.querySelector<HTMLElement>(`.react-flow__node[data-id="${targetNodeId}"]`)
    const targetHandle = targetNode?.querySelector<HTMLElement>('.react-flow__handle.target')
    const pane = container.querySelector<HTMLElement>('.react-flow__pane')
    if (!targetNode || !targetHandle || !pane) throw new Error('new target input handle missing from fixture')
    setPointerTarget(pane)

    fireEvent.pointerDown(grip, { button: 0, pointerId: 1, clientX: 120, clientY: 160 })
    fireEvent.pointerMove(grip, { pointerId: 1, clientX: 305, clientY: 95 })
    await waitFor(() => expect(container.querySelector('.canvas-edge-reconnect-preview')).toHaveClass('is-valid'))
    fireEvent.pointerUp(grip, { button: 0, pointerId: 1, clientX: 305, clientY: 95 })

    await waitFor(() => expect(api.reconnectTarget).toHaveBeenCalledWith({
      projectId: 'p_real_flow',
      expectedEdge: { id: 'e_agent', sourceNodeId: 'n_prompt', targetNodeId: 'n_agent', kind: 'connection', scriptVersion: 0 },
      targetNodeId,
    }))
  })

  it('leaves the original edge unchanged on empty drops, cancellation, and backend rejection', async () => {
    const api = makeAPI()
    const { container } = render(<WorkspaceCanvas snapshot={makeSnapshot()} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />)
    const grip = await waitFor(() => {
      const current = container.querySelector<HTMLElement>('[data-testid="reconnect-grip-e_agent"]')
      if (!current) throw new Error('reconnect grip missing')
      return current
    })
    setPointerTarget(null)
    fireEvent.pointerDown(grip, { button: 0, pointerId: 1, clientX: 120, clientY: 160 })
    fireEvent.pointerMove(grip, { pointerId: 1, clientX: 200, clientY: 180 })
    expect(container.querySelector('.canvas-edge-reconnect-preview')).toHaveClass('is-invalid')
    fireEvent.pointerUp(grip, { button: 0, pointerId: 1, clientX: 200, clientY: 180 })
    expect(api.reconnectTarget).not.toHaveBeenCalled()
    expect(container.querySelectorAll('.react-flow__edge-path')).toHaveLength(4)

    const originalTargetHandle = container.querySelector<HTMLElement>('[data-id="n_agent"] .node-input-handle')
    if (!originalTargetHandle) throw new Error('original input handle missing from fixture')
    setPointerTarget(originalTargetHandle)
    fireEvent.pointerDown(grip, { button: 0, pointerId: 2, clientX: 120, clientY: 160 })
    fireEvent.pointerUp(grip, { button: 0, pointerId: 2, clientX: 220, clientY: 200 })
    expect(api.reconnectTarget).not.toHaveBeenCalled()

    const targetHandle = container.querySelector<HTMLElement>('[data-id="n_script"] .node-input-handle')
    if (!targetHandle) throw new Error('target input handle missing from fixture')
    setPointerTarget(targetHandle)
    fireEvent.pointerDown(grip, { button: 0, pointerId: 3, clientX: 120, clientY: 160 })
    fireEvent.pointerCancel(grip, { pointerId: 3 })
    fireEvent.pointerDown(grip, { button: 0, pointerId: 4, clientX: 120, clientY: 160 })
    fireEvent.keyDown(window, { key: 'Escape' })
    expect(api.reconnectTarget).not.toHaveBeenCalled()

    vi.mocked(api.reconnectTarget).mockRejectedValueOnce(new Error('CYCLE_DETECTED'))
    fireEvent.pointerDown(grip, { button: 0, pointerId: 5, clientX: 120, clientY: 160 })
    fireEvent.pointerUp(grip, { button: 0, pointerId: 5, clientX: 260, clientY: 220 })
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('CYCLE_DETECTED'))
    expect(container.querySelectorAll('.react-flow__edge-path')).toHaveLength(4)
    expect(api.reconnectTarget).toHaveBeenCalledOnce()
  })

  it('rejects an edge identity that changes while its target is being dragged', async () => {
    const api = makeAPI()
    const initial = makeSnapshot()
    const view = render(<WorkspaceCanvas snapshot={initial} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />)
    const grip = await waitFor(() => {
      const current = view.container.querySelector<SVGCircleElement>('[data-testid="reconnect-grip-e_agent"]')
      if (!current) throw new Error('reconnect grip missing')
      return current
    })
    const targetHandle = view.container.querySelector<HTMLElement>('[data-id="n_candidate"] .node-input-handle')
    if (!targetHandle) throw new Error('target input handle missing from fixture')
    setPointerTarget(targetHandle)
    fireEvent.pointerDown(grip, { button: 0, pointerId: 1, clientX: 120, clientY: 160 })

    const changed: WorkspaceSnapshot = {
      ...initial,
      project: { ...initial.project!, revision: 5 },
      edges: (initial.edges ?? []).map((edge) => edge.id === 'e_agent' ? { ...edge, targetNodeId: 'n_script' } : edge),
    }
    view.rerender(<WorkspaceCanvas snapshot={changed} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />)
    fireEvent.pointerUp(grip, { button: 0, pointerId: 1, clientX: 200, clientY: 200 })

    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('连接已发生变化'))
    expect(api.reconnectTarget).not.toHaveBeenCalled()
    expect(view.container.querySelectorAll('.react-flow__edge-path')).toHaveLength(4)
  })

  it('cancels a reconnect gesture when the project changes', async () => {
    const api = makeAPI()
    const initial = makeSnapshot()
    const view = render(<WorkspaceCanvas snapshot={initial} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />)
    const grip = await waitFor(() => {
      const current = view.container.querySelector<SVGCircleElement>('[data-testid="reconnect-grip-e_agent"]')
      if (!current) throw new Error('reconnect grip missing')
      return current
    })
    const targetHandle = view.container.querySelector<HTMLElement>('[data-id="n_script"] .node-input-handle')
    if (!targetHandle) throw new Error('target input handle missing from fixture')
    setPointerTarget(targetHandle)
    fireEvent.pointerDown(grip, { button: 0, pointerId: 1, clientX: 120, clientY: 160 })
    view.rerender(<WorkspaceCanvas snapshot={{
      ...initial,
      project: { ...initial.project!, id: 'p_next', name: '另一个工程' },
    }} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />)
    fireEvent.pointerUp(grip, { button: 0, pointerId: 1, clientX: 200, clientY: 200 })

    expect(api.reconnectTarget).not.toHaveBeenCalled()
    expect(view.container.querySelector('.workspace-canvas')).toHaveAttribute('data-project-id', 'p_next')
  })

  it('opens edge operations by click or keyboard and keeps Candidate sources protected', async () => {
    const snapshot = makeSnapshot()
    const script = snapshot.nodes?.find((node) => node.id === 'n_script')
    if (!script) throw new Error('Script node missing from fixture')
    script.scriptVersions = [
      { n: 1, artifactId: 'sha256:script-v1', runId: 'r_script_1' },
      { n: 2, artifactId: 'sha256:script-v2', runId: 'r_script_2' },
    ]
    const candidateSourceEdge = snapshot.edges?.find((edge) => edge.id === 'e_candidate')
    if (!candidateSourceEdge) throw new Error('Candidate source edge missing from fixture')
    snapshot.edges = [
      candidateSourceEdge,
      { id: 'e_script_prompt', sourceNodeId: 'n_script', targetNodeId: 'n_prompt', kind: 'connection', scriptVersion: 0 },
    ]
    const api = makeAPI()
    const { container } = render(<WorkspaceCanvas snapshot={snapshot} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />)
    const edge = await waitFor(() => {
      const current = container.querySelector<SVGGElement>('.react-flow__edge[data-id="e_script_prompt"]')
      if (!current) throw new Error('Script input edge missing from fixture')
      return current
    })
    fireEvent.click(edge, { clientX: 150, clientY: 180 })

    const version = await screen.findByRole('combobox', { name: '为 n_script → n_prompt 固定 Script 版本' })
    fireEvent.change(version, { target: { value: '2' } })
    await waitFor(() => expect(api.setEdgeScriptVersion).toHaveBeenCalledWith('e_script_prompt', 2, 'p_real_flow'))
    fireEvent.click(screen.getByRole('button', { name: '断开 n_script 到 n_prompt' }))
    fireEvent.click(screen.getByRole('button', { name: '确认断开' }))
    await waitFor(() => expect(api.disconnect).toHaveBeenCalledWith('e_script_prompt', 'p_real_flow'))

    const candidateEdge = container.querySelector<SVGGElement>('.react-flow__edge[data-id="e_candidate"]')
    if (!candidateEdge) throw new Error('Candidate source edge missing from fixture')
    fireEvent.keyDown(candidateEdge, { key: 'Enter' })
    expect(await screen.findByText('候选生成来源 · 不可断开或改接。')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /断开 n_topic 到 n_candidate/ })).not.toBeInTheDocument()
    expect(container.querySelector('[data-testid="reconnect-grip-e_candidate"]')).not.toBeInTheDocument()
  })
})
