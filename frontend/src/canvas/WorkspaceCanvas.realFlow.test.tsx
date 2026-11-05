import { act, fireEvent, render, waitFor } from '@testing-library/react'
import type { ReactElement } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { OutputKind, PendingStatus, RunStatus } from '../../bindings/audio-ui/internal/domain/valueobject'
import type { Node, WorkspaceSnapshot } from '../../bindings/audio-ui/internal/domain/entity'
import type { GraphAPI } from './graphApi'
import { WorkspaceCanvas } from './WorkspaceCanvas'

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

function makeNode(id: string, type: 'topic' | 'prompt', parent = '', parentCandidate?: number): Node {
  return {
    id,
    type: type as Node['type'],
    title: id,
    parent,
    parentCandidate,
    position: { x: 40, y: 60 },
    params: {},
    rounds: [],
    locked: false,
    stale: false,
  }
}

function makeSnapshot(): WorkspaceSnapshot {
  const topic = makeNode('n_topic', 'topic')
  const prompt = makeNode('n_prompt', 'prompt', topic.id, 0)
  return {
    project: { id: 'p_real_flow', name: '真实画布测试', revision: 4, activeNodeId: topic.id },
    nodes: [topic, prompt],
    pending: [],
    runs: [],
    chains: [{ id: `c_${prompt.id}`, tailNodeId: prompt.id, path: [topic.id, prompt.id], label: '测试链', status: 'idle' }],
    lockedNodeIds: [],
  }
}

function makeAPI(): GraphAPI {
  return {
    addNode: vi.fn().mockResolvedValue({ projectId: 'p_real_flow', projectRevision: 5, nodeId: 'n_created' }),
    connect: vi.fn().mockResolvedValue({ projectId: 'p_real_flow', projectRevision: 5, nodeId: 'n_prompt' }),
    moveNode: vi.fn().mockResolvedValue({ projectId: 'p_real_flow', projectRevision: 5, nodeId: 'n_prompt' }),
    selectNode: vi.fn().mockResolvedValue({ projectId: 'p_real_flow', projectRevision: 5, nodeId: 'n_prompt' }),
    updateNodeParams: vi.fn().mockResolvedValue({ projectId: 'p_real_flow', projectRevision: 5, nodeId: 'n_topic' }),
    getTopicCandidates: vi.fn().mockResolvedValue([]),
  }
}

describe('WorkspaceCanvas with real React Flow', () => {
  beforeEach(() => {
    vi.stubGlobal('ResizeObserver', LayoutResizeObserver)
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
      if (this.classList.contains('react-flow__handle')) return rect(312, 90, 8, 8)
      if (this.classList.contains('react-flow') || this.classList.contains('canvas-flow-root')) return rect(0, 0, 820, 640)
      return rect(0, 0, 820, 640)
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('renders measured node wrappers, candidate handles and SVG edges, and selects through the keyboard', async () => {
    const onSelectNode = vi.fn().mockResolvedValue(undefined)
    const { container } = render(
      <WorkspaceCanvas
        snapshot={makeSnapshot()}
        onSelectNode={onSelectNode}
        api={makeAPI()}
        candidateFixtures={{ n_topic: [{ title: '候选一' }, { title: '候选二' }] }}
      />,
    )

    const topicWrapper = await waitFor(() => {
      const node = container.querySelector<HTMLElement>('.react-flow__node[data-id="n_topic"]')
      if (!node) throw new Error('topic wrapper not rendered')
      expect(node).toHaveStyle({ visibility: 'visible' })
      return node
    })
    const promptWrapper = await waitFor(() => {
      const node = container.querySelector<HTMLElement>('.react-flow__node[data-id="n_prompt"]')
      if (!node) throw new Error('prompt wrapper not rendered')
      return node
    })
    expect(topicWrapper.querySelectorAll('.react-flow__handle')).toHaveLength(2)
    expect(topicWrapper.querySelector('[data-handleid="candidate-0"]')).toHaveAttribute('aria-label', '候选 1：候选一 输出端口')
    expect(topicWrapper.querySelector('[data-handleid="candidate-1"]')).toHaveAttribute('aria-label', '候选 2：候选二 输出端口')
    await waitFor(() => expect(container.querySelector('.react-flow__edge-path')).toBeInTheDocument())

    fireEvent.keyDown(promptWrapper!, { key: 'Enter', code: 'Enter' })
    await waitFor(() => expect(onSelectNode).toHaveBeenCalledWith('n_prompt'))
    fireEvent.keyDown(topicWrapper, { key: ' ', code: 'Space' })
    await waitFor(() => expect(onSelectNode).toHaveBeenLastCalledWith('n_topic'))
  })

  it('keeps rendered handles and edges when node selection rebuilds the controlled snapshot', async () => {
    const snapshot = makeSnapshot()
    const api = makeAPI()
    let rerender: (ui: ReactElement) => void = () => undefined
    const onSelectNode = vi.fn(async (id: string) => {
      const selected = {
        ...snapshot,
        project: { ...snapshot.project!, activeNodeId: id },
      }
      rerender(<WorkspaceCanvas snapshot={selected} onSelectNode={onSelectNode} api={api} candidateFixtures={{ n_topic: [{ title: '候选一' }] }} />)
    })
    const view = render(
      <WorkspaceCanvas snapshot={snapshot} onSelectNode={onSelectNode} api={api} candidateFixtures={{ n_topic: [{ title: '候选一' }] }} />,
    )
    rerender = view.rerender

    const promptWrapper = await waitFor(() => {
      const node = view.container.querySelector<HTMLElement>('.react-flow__node[data-id="n_prompt"]')
      if (!node) throw new Error('prompt wrapper not rendered')
      expect(node).toHaveStyle({ visibility: 'visible' })
      return node
    })
    await waitFor(() => expect(view.container.querySelector('.react-flow__edge-path')).toBeInTheDocument())

    fireEvent.keyDown(promptWrapper, { key: 'Enter', code: 'Enter' })
    await waitFor(() => expect(onSelectNode).toHaveBeenCalledWith('n_prompt'))
    await waitFor(() => expect(promptWrapper).toHaveClass('selected'))
    await waitFor(() => expect(view.container.querySelector('.react-flow__edge-path')).toBeInTheDocument())

    fireEvent.keyDown(promptWrapper, { key: 'ArrowRight', code: 'ArrowRight' })
    await waitFor(() => expect(api.moveNode).toHaveBeenCalledWith('n_prompt', expect.any(Number), 60, 'p_real_flow'), { timeout: 2000 })
    expect(vi.mocked(api.moveNode).mock.calls.at(-1)?.[1]).toBeGreaterThan(40)
    expect(view.container.querySelector('.canvas-message-error')?.textContent ?? '').not.toMatch(/渲染错误 (008|015)/)
  })

  it('waits for newly loaded candidate ports before drawing persisted edges', async () => {
    let resolveCandidates!: (candidates: Awaited<ReturnType<GraphAPI['getTopicCandidates']>>) => void
    const api = makeAPI()
    vi.mocked(api.getTopicCandidates).mockReturnValue(new Promise((resolve) => { resolveCandidates = resolve }))
    const snapshot = makeSnapshot()
    const topic = snapshot.nodes?.find((node) => node.id === 'n_topic')
    if (!topic) throw new Error('topic missing from fixture')
    topic.rounds = [{ n: 1, kind: OutputKind.OutputKindTopics, artifactId: 'sha256:applied', pendingId: 'pending_topics', runId: 'r_topics' }]
    const { container, rerender } = render(<WorkspaceCanvas snapshot={snapshot} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />)

    await waitFor(() => expect(container.querySelector('[data-handleid="topic-connection-anchor"]')).toBeInTheDocument())
    await waitFor(() => expect(container.querySelector('.react-flow__edge-path')).toBeInTheDocument())
    expect(container.querySelector('.canvas-message-error')?.textContent ?? '').not.toContain('渲染错误 008')

    await act(async () => {
      resolveCandidates([{ index: 0, title: '候选一', angle: '方向一', hook: '钩子一' }])
      await Promise.resolve()
    })
    await waitFor(() => expect(container.querySelector('[data-handleid="candidate-0"]')).toBeInTheDocument())
    await waitFor(() => expect(container.querySelector('.react-flow__edge-path')).toBeInTheDocument())
    expect(container.querySelector('.canvas-message-error')?.textContent ?? '').not.toContain('渲染错误 008')
    expect(container.querySelector('[data-id="edge-n_topic-n_prompt"] .react-flow__edge-path')).toBeInTheDocument()

    const refreshed = makeSnapshot()
    refreshed.project = { ...refreshed.project!, revision: 5 }
    const refreshedTopic = refreshed.nodes?.find((node) => node.id === 'n_topic')
    if (!refreshedTopic) throw new Error('refreshed topic missing from fixture')
    refreshedTopic.rounds = [{ n: 1, kind: OutputKind.OutputKindTopics, artifactId: 'sha256:applied', pendingId: 'pending_topics', runId: 'r_topics' }]
    rerender(<WorkspaceCanvas snapshot={refreshed} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />)
    await waitFor(() => expect(container.querySelector('[data-handleid="candidate-0"]')).toBeInTheDocument())
    expect(api.getTopicCandidates).toHaveBeenCalledTimes(1)
    await waitFor(() => expect(container.querySelector('[data-id="edge-n_topic-n_prompt"] .react-flow__edge-path')).toBeInTheDocument())
    expect(container.querySelector('.canvas-message-error')?.textContent ?? '').not.toContain('渲染错误 008')
  })

  it('reads applied candidates after the workspace refresh and uses stable indexes as handles', async () => {
    const api = makeAPI()
    vi.mocked(api.getTopicCandidates).mockResolvedValue([
      { index: 2, title: '候选三', angle: '方向三', hook: '钩子三' },
      { index: 0, title: '候选一', angle: '方向一', hook: '钩子一' },
    ])
    const initial = makeSnapshot()
    const { container, rerender } = render(<WorkspaceCanvas snapshot={initial} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />)
    expect(api.getTopicCandidates).not.toHaveBeenCalled()

    const snapshot = makeSnapshot()
    snapshot.project = { ...snapshot.project!, revision: 5 }
    const topic = snapshot.nodes?.find((node) => node.id === 'n_topic')
    if (!topic) throw new Error('topic missing from fixture')
    topic.rounds = [{ n: 1, kind: OutputKind.OutputKindTopics, artifactId: 'sha256:applied', pendingId: 'pending_topics', runId: 'r_topics' }]
    rerender(<WorkspaceCanvas snapshot={snapshot} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />)

    await waitFor(() => expect(container.querySelector('[data-handleid="candidate-2"]')).toBeInTheDocument())
    expect(container.querySelector('[data-handleid="candidate-0"]')).toHaveAttribute('aria-label', '候选 1：候选一 输出端口')
    expect(api.getTopicCandidates).toHaveBeenCalledWith('n_topic', 'p_real_flow')
  })

  it('shows candidate read errors and leaves the output port disabled', async () => {
    const snapshot = makeSnapshot()
    const topic = snapshot.nodes?.find((node) => node.id === 'n_topic')
    if (!topic) throw new Error('topic missing from fixture')
    topic.rounds = [{ n: 1, kind: OutputKind.OutputKindTopics, artifactId: 'sha256:broken', pendingId: 'pending_topics', runId: 'r_topics' }]
    const api = makeAPI()
    vi.mocked(api.getTopicCandidates).mockRejectedValue(new Error('CANDIDATE_INVALID'))
    const { container } = render(<WorkspaceCanvas snapshot={snapshot} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />)

    await waitFor(() => expect(container.querySelector('.topic-candidate-error')).toHaveTextContent('CANDIDATE_INVALID'))
    expect(container.querySelector('[data-handleid^="candidate-"]')).not.toBeInTheDocument()
  })

  it('ignores candidate results that arrive after the project changes', async () => {
    let resolveOld!: (candidates: Awaited<ReturnType<GraphAPI['getTopicCandidates']>>) => void
    const oldCandidates = new Promise<Awaited<ReturnType<GraphAPI['getTopicCandidates']>>>((resolve) => { resolveOld = resolve })
    const api = makeAPI()
    vi.mocked(api.getTopicCandidates).mockReturnValueOnce(oldCandidates).mockResolvedValueOnce([])
    const first = makeSnapshot()
    const topic = first.nodes?.find((node) => node.id === 'n_topic')
    if (!topic) throw new Error('topic missing from fixture')
    topic.rounds = [{ n: 1, kind: OutputKind.OutputKindTopics, artifactId: 'sha256:applied', pendingId: 'pending_topics', runId: 'r_topics' }]
    const { container, rerender } = render(<WorkspaceCanvas snapshot={first} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />)
    const second: WorkspaceSnapshot = {
      ...first,
      project: { ...first.project!, id: 'p_next', name: '另一个工程' },
    }
    rerender(<WorkspaceCanvas snapshot={second} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />)
    await waitFor(() => expect(api.getTopicCandidates).toHaveBeenCalledWith('n_topic', 'p_next'))
    resolveOld([{ index: 0, title: '旧工程候选', angle: '旧方向', hook: '旧钩子' }])

    await waitFor(() => expect(container.querySelector('[data-handleid^="candidate-"]')).not.toBeInTheDocument())
    expect(container.textContent).not.toContain('旧工程候选')
  })

  it('shows the latest run failure without hiding an older pending result', async () => {
    const snapshot = makeSnapshot()
    snapshot.runs = [{
      id: 'r_failed', nodeId: 'n_topic', status: RunStatus.RunFailed,
      chainId: 'c_n_topic', chainPath: ['n_topic'], estimatedTokens: 100,
      startedAt: '2026-11-05T00:00:00Z',
    }]
    snapshot.pending = [{
      id: 'pending_one', nodeId: 'n_topic', runId: 'r_previous', chainId: 'c_n_topic',
      kind: OutputKind.OutputKindTopics, status: PendingStatus.PendingPending,
    }]
    const { container } = render(
      <WorkspaceCanvas snapshot={snapshot} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={makeAPI()} />,
    )

    await waitFor(() => expect(container.querySelector('.react-flow__node[data-id="n_topic"] .canvas-node-status'))
      .toHaveTextContent('运行失败 · 待处理 1'))
  })
})
