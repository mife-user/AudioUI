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

function makeNode(id: string, type: string, parent = '', overrides: Partial<Node> = {}): Node {
  return {
    id,
    type: type as Node['type'],
    title: id,
    parent,
    position: { x: 40, y: 60 },
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
    candidateSource: { topicNodeId: topic.id, artifactId: 'sha256:topics', index: 0 },
  })
  const prompt = makeNode('n_prompt', 'prompt', candidate.id, { params: { instruction: '写一段说明。' } })
  const agent = makeNode('n_agent', 'agent', prompt.id)
  const script = makeNode('n_script', 'script', agent.id)
  return {
    project: { id: 'p_real_flow', name: '真实画布测试', revision: 4, activeNodeId: topic.id },
    nodes: [topic, candidate, prompt, agent, script],
    pending: [],
    runs: [],
    chains: [{ id: `c_${script.id}`, tailNodeId: script.id, path: [topic.id, candidate.id, prompt.id, agent.id, script.id], label: '测试链', status: 'idle' }],
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

  it('renders measured handles and edges across Topic, Candidate, Prompt, Agent and Script', async () => {
    const onSelectNode = vi.fn().mockResolvedValue(undefined)
    const { container } = render(
      <WorkspaceCanvas
        snapshot={makeSnapshot()}
        onSelectNode={onSelectNode}
        api={makeAPI()}
        candidateFixtures={{ n_topic: [{ index: 0, title: '候选一', angle: '方向一', hook: '开场一' }] }}
      />,
    )

    await waitFor(() => {
      const nodes = ['n_topic', 'n_candidate', 'n_prompt', 'n_agent', 'n_script']
      nodes.forEach((id) => expect(container.querySelector(`.react-flow__node[data-id="${id}"]`)).toBeVisible())
    })
    expect(container.querySelector('[data-id="n_topic"] [data-handleid="topic-candidate-0"]')).toBeInTheDocument()
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

    await waitFor(() => expect(container.querySelector('[data-handleid="topic-candidate-0"]')).toBeInTheDocument())
    await waitFor(() => expect(container.querySelectorAll('.react-flow__edge-path')).toHaveLength(4))
    expect(container.querySelector('.canvas-message-error')?.textContent ?? '').not.toContain('渲染错误 008')

    await act(async () => {
      resolveCandidates([{ index: 0, title: '候选一', angle: '方向一', hook: '开场一' }])
      await Promise.resolve()
    })
    await waitFor(() => expect(container.querySelector('.candidate-node')).toHaveTextContent('方向一'))
    await waitFor(() => expect(container.querySelectorAll('.react-flow__edge-path')).toHaveLength(4))
    expect(container.querySelector('.canvas-message-error')?.textContent ?? '').not.toMatch(/渲染错误 (008|015)/)
  })

  it('uses CandidateSource indexes rather than response positions', async () => {
    const api = makeAPI()
    vi.mocked(api.getTopicCandidates).mockResolvedValue([
      { index: 2, title: '候选三', angle: '方向三', hook: '开场三' },
      { index: 0, title: '候选一', angle: '方向一', hook: '开场一' },
    ])
    const snapshot = makeSnapshot()
    const candidate = snapshot.nodes?.find((node) => node.id === 'n_candidate')
    if (!candidate) throw new Error('candidate missing from fixture')
    candidate.candidateSource = { topicNodeId: 'n_topic', artifactId: 'sha256:topics', index: 2 }
    const { container } = render(<WorkspaceCanvas snapshot={snapshot} onSelectNode={vi.fn().mockResolvedValue(undefined)} api={api} />)

    await waitFor(() => expect(container.querySelector('.candidate-node')).toHaveTextContent('候选三'))
    expect(api.getTopicCandidates).toHaveBeenCalledWith('n_topic', 'p_real_flow')
    expect(container.querySelector('[data-id="n_topic"] [data-handleid="topic-candidate-2"]')).toBeInTheDocument()
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
    await waitFor(() => expect(api.updateNodeParams).toHaveBeenCalledOnce())
    await waitFor(() => expect(container.querySelector('.topic-save-error')).toHaveTextContent('disk full'))

    expect(task).toHaveValue('保存失败时禁止发送。')
    expect(onStartRun).not.toHaveBeenCalled()
    expect(container.querySelector('.agent-run-error')).toHaveTextContent('RUN_START_FAILED: disk full')
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
})
