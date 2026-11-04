import { fireEvent, render, waitFor } from '@testing-library/react'
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
    expect(container.querySelector('.react-flow__edge-path')).toBeInTheDocument()

    fireEvent.keyDown(promptWrapper!, { key: 'Enter', code: 'Enter' })
    await waitFor(() => expect(onSelectNode).toHaveBeenCalledWith('n_prompt'))
    fireEvent.keyDown(topicWrapper, { key: ' ', code: 'Space' })
    await waitFor(() => expect(onSelectNode).toHaveBeenLastCalledWith('n_topic'))
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
