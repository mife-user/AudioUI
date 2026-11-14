import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { OutputKind, PendingStatus, PermissionTier, RunStatus, TranscriptKind } from '../../bindings/audio-ui/internal/domain/valueobject'
import type { Node, PendingDetail, PendingSummary, RunSnapshot, WorkspaceRunSummary } from '../../bindings/audio-ui/internal/domain/entity'
import type { RunView } from '../stores/runStore'
import { usePendingController } from '../hooks/usePendingController'
import { AgentConsole } from './AgentConsole'

const summary: WorkspaceRunSummary = {
  id: 'r_history',
  nodeId: 'n_topic',
  status: RunStatus.RunSucceeded,
  chainId: 'c_n_topic',
  chainPath: ['n_topic'],
  estimatedTokens: 120,
  contextHash: '1234567890abcdef',
  startedAt: '2026-11-03T12:00:00Z',
}

const runSnapshot: RunSnapshot = {
  runId: summary.id,
  projectId: 'p_demo',
  lastSeq: 3,
  run: {
    id: summary.id,
    treeId: 'n_topic',
    nodeId: summary.nodeId,
    chainId: summary.chainId,
    chainPath: ['n_topic'],
    status: RunStatus.RunSucceeded,
    contextVersion: 1,
    contextHash: summary.contextHash,
    estimatedTokens: summary.estimatedTokens,
    startedAt: summary.startedAt,
    transcript: [{
      id: 'm1', kind: TranscriptKind.TranscriptMessage, role: 'assistant', text: '已完成',
      streaming: false, createdAt: summary.startedAt ?? '',
    }],
  },
  permissions: [],
  questions: [],
}

const historyView: RunView = {
  summary,
  snapshot: runSnapshot,
  lastSeq: 3,
  bufferedEvents: [],
  loading: false,
  needsSnapshot: false,
  error: '',
}

const pendingSummary: PendingSummary = {
  id: 'pending_topics',
  nodeId: 'n_topic',
  runId: 'r_topics',
  chainId: 'c_n_topic',
  kind: OutputKind.OutputKindTopics,
  status: PendingStatus.PendingPending,
  summary: '3 个候选',
}

const scriptPendingSummary: PendingSummary = {
  id: 'pending_script',
  nodeId: 'n_agent',
  runId: 'r_script_new',
  chainId: 'c_n_agent',
  kind: OutputKind.OutputKindScript,
  status: PendingStatus.PendingPending,
  summary: '追加到 v1',
  outputBinding: {
    bound: true,
    nodeId: 'n_script',
    parentNodeId: 'n_agent',
    baselineVersion: 1,
    baselineArtifactId: 'sha256:script-v1',
  },
  resultNodeId: 'n_script',
}

const scriptNode: Node = {
  id: 'n_script',
  type: 'script' as Node['type'],
  title: '旁白脚本',
  parent: 'n_agent',
  position: { x: 0, y: 0 },
  size: { width: 270, height: 205 },
  params: {},
  rounds: [],
  scriptVersions: [{ n: 1, artifactId: 'sha256:script-v1', runId: 'r_script_old' }],
  locked: false,
  stale: false,
}

const topicsPayload = JSON.stringify({ topics: [
  { title: '候选甲', angle: '人物观察', hook: '凌晨出发' },
  { title: '候选乙', angle: '城市声音', hook: '末班车' },
  { title: '候选丙', angle: '手艺记录', hook: '灯仍亮着' },
] })

function encodePayload(payload: string): string {
  return btoa(Array.from(new TextEncoder().encode(payload), (byte) => String.fromCharCode(byte)).join(''))
}

function pendingDetail(status: PendingSummary['status'] = PendingStatus.PendingPending, payload = topicsPayload): PendingDetail {
  return {
    pending: {
      ...pendingSummary,
      status,
      error: status === PendingStatus.PendingInvalid ? 'SCHEMA_INVALID: topics must contain 3 to 10 candidates' : '',
      payload: 'pending/pending_topics.json',
      chainPath: ['n_topic'],
      createdAt: '2026-11-05T12:00:00Z',
    },
    rawPayload: encodePayload(payload),
  }
}

function scriptPendingDetail(): PendingDetail {
  const previous = pendingDetail().pending
  return {
    pending: {
      ...previous,
      id: scriptPendingSummary.id,
      nodeId: scriptPendingSummary.nodeId,
      runId: scriptPendingSummary.runId,
      chainId: scriptPendingSummary.chainId,
      kind: OutputKind.OutputKindScript,
      status: PendingStatus.PendingPending,
      summary: scriptPendingSummary.summary,
      outputBinding: {
        bound: true,
        nodeId: scriptNode.id,
        parentNodeId: 'n_agent',
        baselineVersion: 1,
        baselineArtifactId: 'sha256:script-v1',
      },
      outputSchemaVersion: 1,
      resultNodeId: scriptNode.id,
    },
    rawPayload: encodePayload(JSON.stringify({
      title: '旁白脚本修订版',
      scenes: [{ narration: '从鼓点切分看节奏变化。', duration: 12 }],
    })),
  }
}

type ConsoleHarnessProps = Omit<React.ComponentProps<typeof AgentConsole>, 'pendingController'> & {
  nodes: Node[]
  pending: PendingSummary[]
  onGetPending: (id: string, projectId: string) => Promise<PendingDetail>
  onApplyPending: (id: string, force: boolean, projectId: string) => Promise<void>
  onDiscardPending: (id: string, projectId: string) => Promise<void>
  onRefreshWorkspace: () => Promise<void>
}

function ConsoleHarness({ props }: { props: ConsoleHarnessProps }) {
  const { nodes, pending, onGetPending, onApplyPending, onDiscardPending, onRefreshWorkspace, ...consoleProps } = props
  const pendingController = usePendingController({
    projectId: props.projectId,
    nodes,
    pending,
    onGetPending,
    onApplyPending,
    onDiscardPending,
    onRefreshWorkspace,
  })
  return <AgentConsole {...consoleProps} nodes={nodes} pendingController={pendingController} />
}

function renderConsole(overrides: Partial<ConsoleHarnessProps> = {}) {
  const props: ConsoleHarnessProps = {
    runs: [summary],
    projectId: 'p_demo',
    nodes: [],
    pending: [],
    runViews: { [summary.id]: historyView },
    selectedRunId: summary.id,
    selectedRun: historyView,
    actionError: '',
    collapsed: false,
    onToggle: vi.fn(),
    onSelectRun: vi.fn(),
    onRetryRun: vi.fn(),
    onPermission: vi.fn().mockResolvedValue(undefined),
    onQuestion: vi.fn().mockResolvedValue(undefined),
    onInterrupt: vi.fn().mockResolvedValue(undefined),
    onGetPending: vi.fn().mockResolvedValue(pendingDetail()),
    onApplyPending: vi.fn().mockResolvedValue(undefined),
    onDiscardPending: vi.fn().mockResolvedValue(undefined),
    onRefreshWorkspace: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  }
  return { ...render(<ConsoleHarness props={props} />), props }
}

function selectRecoveryPending(nodeId: string) {
  const recovery = screen.getByRole('region', { name: '结果异常恢复' })
  fireEvent.click(within(recovery).getByRole('button', { name: new RegExp(nodeId) }))
}

describe('AgentConsole', () => {
  it('shows run history and transcript without a free-chat input', () => {
    renderConsole()
    expect(screen.getByText('r_history')).toBeInTheDocument()
    expect(screen.getByText('已完成')).toBeInTheDocument()
    expect(screen.queryByRole('textbox', { name: '自由聊天' })).not.toBeInTheDocument()
    expect(document.querySelector('textarea')).toBeNull()
  })

  it('keeps terminal history read-only and does not invent unknown token usage', () => {
    renderConsole()
    expect(screen.queryByRole('button', { name: '停止运行' })).not.toBeInTheDocument()
    expect(screen.getByText('未知')).toBeInTheDocument()
  })

  it('opens the result recovery entry on demand without a persistent tab or free-chat input', () => {
    renderConsole()
    expect(screen.queryByRole('tablist', { name: '运行控制台' })).not.toBeInTheDocument()
    expect(screen.queryByText('结果异常恢复')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '结果 0' }))
    expect(screen.getByText('暂无待确认结果')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '应用' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '丢弃' })).not.toBeInTheDocument()
  })

  it('loads pending payload on selection, previews candidates, then applies and refreshes', async () => {
    const onGetPending = vi.fn().mockResolvedValue(pendingDetail())
    const onApplyPending = vi.fn().mockResolvedValue(undefined)
    const onRefreshWorkspace = vi.fn().mockResolvedValue(undefined)
    renderConsole({ pending: [pendingSummary], onGetPending, onApplyPending, onRefreshWorkspace })
    fireEvent.click(screen.getByRole('button', { name: /^结果 / }))
    expect(onGetPending).not.toHaveBeenCalled()

    selectRecoveryPending('n_topic')
    expect(await screen.findByText(/候选甲/)).toBeInTheDocument()
    expect(screen.getByText('人物观察')).toBeInTheDocument()
    expect(screen.getByText('凌晨出发')).toBeInTheDocument()
    expect(onGetPending).toHaveBeenCalledWith('pending_topics', 'p_demo')

    fireEvent.click(screen.getByRole('button', { name: '应用' }))
    await waitFor(() => expect(onApplyPending).toHaveBeenCalledWith('pending_topics', false, 'p_demo'))
    await waitFor(() => expect(onRefreshWorkspace).toHaveBeenCalledOnce())
    expect(screen.getByText(/操作已提交/)).toBeInTheDocument()
  })

  it('prevents duplicate pending mutations while an action is in flight', async () => {
    let resolveApply!: () => void
    const onApplyPending = vi.fn(() => new Promise<void>((resolve) => { resolveApply = resolve }))
    const onRefreshWorkspace = vi.fn().mockResolvedValue(undefined)
    renderConsole({ pending: [pendingSummary], onApplyPending, onRefreshWorkspace })
    fireEvent.click(screen.getByRole('button', { name: /^结果 / }))
    selectRecoveryPending('n_topic')
    await screen.findByText(/候选甲/)
    const apply = screen.getByRole('button', { name: '应用' })

    fireEvent.click(apply)
    fireEvent.click(apply)
    expect(onApplyPending).toHaveBeenCalledOnce()
    resolveApply()
    await waitFor(() => expect(onRefreshWorkspace).toHaveBeenCalledOnce())
  })

  it('shows invalid raw output and only allows discarding it', async () => {
    const invalid = { ...pendingSummary, status: PendingStatus.PendingInvalid, error: 'SCHEMA_INVALID: topics must contain 3 to 10 candidates' }
    const onApplyPending = vi.fn().mockResolvedValue(undefined)
    const onDiscardPending = vi.fn().mockResolvedValue(undefined)
    renderConsole({
      pending: [invalid],
      onGetPending: vi.fn().mockResolvedValue(pendingDetail(PendingStatus.PendingInvalid, '{"topics":[]}')),
      onApplyPending,
      onDiscardPending,
    })
    fireEvent.click(screen.getByRole('button', { name: /^结果 / }))
    selectRecoveryPending('n_topic')

    expect(await screen.findByText('{"topics":[]}')).toBeInTheDocument()
    expect(screen.getByText(/SCHEMA_INVALID/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '应用' })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '丢弃' }))
    await waitFor(() => expect(onDiscardPending).toHaveBeenCalledWith('pending_topics', 'p_demo'))
    expect(onApplyPending).not.toHaveBeenCalled()
  })

  it('requires explicit confirmation before force-applying changed inputs', async () => {
    const onApplyPending = vi.fn()
      .mockRejectedValueOnce(new Error('PARAMS_CHANGED'))
      .mockRejectedValueOnce(new Error('CONTEXT_CHANGED'))
      .mockResolvedValueOnce(undefined)
    renderConsole({ pending: [pendingSummary], onApplyPending })
    fireEvent.click(screen.getByRole('button', { name: /^结果 / }))
    selectRecoveryPending('n_topic')
    await screen.findByText(/候选甲/)

    fireEvent.click(screen.getByRole('button', { name: '应用' }))
    const confirmation = await screen.findByRole('alertdialog')
    expect(confirmation).toHaveTextContent('参数或上游上下文已变化')
    fireEvent.click(screen.getByRole('button', { name: '取消' }))
    expect(screen.getByRole('button', { name: '应用' })).toBeInTheDocument()
    expect(onApplyPending).toHaveBeenCalledTimes(1)

    fireEvent.click(screen.getByRole('button', { name: '应用' }))
    await screen.findByRole('alertdialog')
    fireEvent.click(screen.getByRole('button', { name: '确认并强制应用' }))
    await waitFor(() => expect(onApplyPending).toHaveBeenNthCalledWith(3, 'pending_topics', true, 'p_demo'))
  })

  it('ignores a late pending detail after the active project changes', async () => {
    let resolveDetail!: (detail: PendingDetail) => void
    const pendingLoad = new Promise<PendingDetail>((resolve) => { resolveDetail = resolve })
    const { rerender, props } = renderConsole({ pending: [pendingSummary], onGetPending: vi.fn(() => pendingLoad) })
    fireEvent.click(screen.getByRole('button', { name: /^结果 / }))
    selectRecoveryPending('n_topic')
    rerender(<ConsoleHarness props={{ ...props, projectId: 'p_other', pending: [] }} />)
    resolveDetail(pendingDetail())
    await waitFor(() => expect(screen.queryByText('候选甲')).not.toBeInTheDocument())
    expect(screen.getByText('暂无待确认结果')).toBeInTheDocument()
  })

  it('routes permission responses with both run and request IDs', () => {
    const request = {
      id: 'permission-1', tool: 'Write', tier: PermissionTier.PermissionConfirm, risk: '写文件', summary: 'Write file',
      allowSession: false, expiresAt: new Date(Date.now() + 60_000).toISOString(),
    }
    const activeSnapshot: RunSnapshot = {
      ...runSnapshot,
      runId: 'r_active',
      run: { ...runSnapshot.run, id: 'r_active', status: RunStatus.RunRunning, transcript: [] },
      permissions: [request],
    }
    const activeSummary = { ...summary, id: 'r_active', status: RunStatus.RunRunning }
    const activeView: RunView = { ...historyView, summary: activeSummary, snapshot: activeSnapshot }
    const onPermission = vi.fn().mockResolvedValue(undefined)
    renderConsole({ runs: [activeSummary], runViews: { r_active: activeView }, selectedRunId: 'r_active', selectedRun: activeView, onPermission })

    fireEvent.click(screen.getByRole('button', { name: '请求 1' }))
    fireEvent.click(screen.getByRole('button', { name: '允许一次' }))
    expect(onPermission).toHaveBeenCalledWith('r_active', 'permission-1', 'allow')
  })

  it('shows a collapsed floating status with unresolved counts', () => {
    const { container } = renderConsole({ collapsed: true })
    expect(container.querySelector('.agent-rail-state')).toHaveTextContent('完成')
    expect(container.querySelector('.agent-rail-count')).toHaveTextContent('请求 0')
    expect(container.querySelector('.agent-rail-pending')).toHaveTextContent('结果 0')
    expect(screen.getByRole('button', { name: '展开运行面板' })).toBeInTheDocument()
    expect(document.getElementById('agent-console-panel')).toHaveAttribute('hidden')
  })

  it('keeps the run panel mounted with its scroll position while collapsed', () => {
    const { container, rerender, props } = renderConsole()
    const body = container.querySelector<HTMLElement>('.agent-console-body')
    if (!body) throw new Error('run panel body is missing')
    body.scrollTop = 120

    rerender(<ConsoleHarness props={{ ...props, collapsed: true }} />)

    expect(container.querySelector('.agent-console-body')).toBe(body)
    expect(body.scrollTop).toBe(120)
    expect(document.getElementById('agent-console-panel')).toHaveAttribute('hidden')
  })

  it('moves the panel by its titlebar, clamps it to the viewport, and preserves position when collapsed', () => {
    const { container, rerender, props } = renderConsole()
    const panel = container.querySelector<HTMLElement>('#agent-console-panel')
    const header = container.querySelector<HTMLElement>('.agent-console-head')
    if (!panel || !header) throw new Error('run panel is missing')
    const rect = new DOMRect(100, 64, 390, 520)
    vi.spyOn(panel, 'getBoundingClientRect').mockReturnValue(rect)

    fireEvent.pointerDown(header, { pointerId: 1, button: 0, clientX: 120, clientY: 80 })
    fireEvent.pointerMove(window, { pointerId: 1, clientX: window.innerWidth + 100, clientY: window.innerHeight + 100 })
    fireEvent.pointerUp(window, { pointerId: 1 })

    const left = `${window.innerWidth - rect.width - 12}px`
    const top = `${window.innerHeight - rect.height - 12}px`
    expect(panel.style.left).toBe(left)
    expect(panel.style.top).toBe(top)

    rerender(<ConsoleHarness props={{ ...props, collapsed: true }} />)
    rerender(<ConsoleHarness props={{ ...props, collapsed: false }} />)
    expect(panel.style.left).toBe(left)
    expect(panel.style.top).toBe(top)

    const closeButton = screen.getByRole('button', { name: '收起运行面板' })
    fireEvent.pointerDown(closeButton, { pointerId: 2, button: 0, clientX: 20, clientY: 20 })
    fireEvent.pointerMove(window, { pointerId: 2, clientX: 100, clientY: 100 })
    fireEvent.pointerUp(window, { pointerId: 2 })
    expect(panel.style.left).toBe(left)
    fireEvent.click(closeButton)
    expect(props.onToggle).toHaveBeenCalledOnce()
  })

  it('moves the panel with arrow keys from its focusable titlebar', () => {
    const { container } = renderConsole()
    const panel = container.querySelector<HTMLElement>('#agent-console-panel')
    const header = container.querySelector<HTMLElement>('.agent-console-head')
    if (!panel || !header) throw new Error('run panel is missing')
    vi.spyOn(panel, 'getBoundingClientRect').mockReturnValue(new DOMRect(100, 64, 390, 520))

    fireEvent.keyDown(header, { key: 'ArrowRight' })
    fireEvent.keyDown(header, { key: 'ArrowDown', shiftKey: true })

    expect(panel.style.left).toBe('622px')
    expect(panel.style.top).toBe('44px')
  })

  it('places the expanded panel below the current canvas toolbar', () => {
    const toolbar = document.createElement('div')
    toolbar.className = 'canvas-toolbar'
    vi.spyOn(toolbar, 'getBoundingClientRect').mockReturnValue(new DOMRect(0, 100, 1200, 54))
    document.body.append(toolbar)
    const { container } = renderConsole()
    const panel = container.querySelector<HTMLElement>('#agent-console-panel')
    if (!panel) throw new Error('run panel is missing')

    expect(panel.style.top).toBe('166px')
    expect(panel.style.width).toBe('390px')
    expect(panel.style.height).toBe('520px')
    toolbar.remove()
  })

  it('resizes the panel with pointer and keyboard and preserves size across collapse', () => {
    const { container, rerender, props } = renderConsole()
    const panel = container.querySelector<HTMLElement>('#agent-console-panel')
    const resize = screen.getByRole('button', { name: /调整运行面板尺寸/ })
    if (!panel) throw new Error('run panel is missing')
    vi.spyOn(panel, 'getBoundingClientRect').mockReturnValue(new DOMRect(600, 100, 390, 520))

    fireEvent.pointerDown(resize, { pointerId: 10, button: 0, clientX: 600, clientY: 620 })
    fireEvent.pointerMove(window, { pointerId: 10, clientX: 560, clientY: 660 })
    fireEvent.pointerUp(window, { pointerId: 10 })
    expect(panel.style.width).toBe('430px')
    expect(panel.style.height).toBe('560px')

    fireEvent.keyDown(resize, { key: 'ArrowRight' })
    fireEvent.keyDown(resize, { key: 'ArrowDown', shiftKey: true })
    expect(panel.style.width).toBe('422px')
    expect(panel.style.height).toBe('592px')

    rerender(<ConsoleHarness props={{ ...props, collapsed: true }} />)
    rerender(<ConsoleHarness props={{ ...props, collapsed: false }} />)
    expect(panel.style.width).toBe('422px')
    expect(panel.style.height).toBe('592px')
  })

  it('clamps expanded size and position when the viewport shrinks', () => {
    const originalWidth = window.innerWidth
    const originalHeight = window.innerHeight
    const { container } = renderConsole()
    const panel = container.querySelector<HTMLElement>('#agent-console-panel')
    const resize = screen.getByRole('button', { name: /调整运行面板尺寸/ })
    if (!panel) throw new Error('run panel is missing')

    fireEvent.pointerDown(resize, { pointerId: 11, button: 0, clientX: 0, clientY: 0 })
    fireEvent.pointerMove(window, { pointerId: 11, clientX: -300, clientY: 100 })
    fireEvent.pointerUp(window, { pointerId: 11 })
    expect(panel.style.width).toBe('690px')
    expect(panel.style.height).toBe('620px')

    try {
      Object.defineProperty(window, 'innerWidth', { configurable: true, value: 500 })
      Object.defineProperty(window, 'innerHeight', { configurable: true, value: 400 })
      fireEvent.resize(window)
      expect(panel.style.width).toBe('476px')
      expect(panel.style.height).toBe('376px')
      expect(panel.style.left).toBe('12px')
      expect(panel.style.top).toBe('12px')
    } finally {
      Object.defineProperty(window, 'innerWidth', { configurable: true, value: originalWidth })
      Object.defineProperty(window, 'innerHeight', { configurable: true, value: originalHeight })
      fireEvent.resize(window)
    }
  })

  it('moves the collapsed rail, clamps it, and shares its position with the expanded panel', () => {
    const { container, rerender, props } = renderConsole({ collapsed: true, pending: [pendingSummary] })
    const rail = container.querySelector<HTMLElement>('.agent-rail')
    const panel = container.querySelector<HTMLElement>('#agent-console-panel')
    if (!rail || !panel) throw new Error('run console is missing')
    vi.spyOn(rail, 'getBoundingClientRect').mockReturnValue(new DOMRect(500, 64, 280, 42))
    vi.spyOn(panel, 'getBoundingClientRect').mockReturnValue(new DOMRect(100, 64, 390, 520))

    fireEvent.pointerDown(rail.querySelector('.agent-rail-state')!, { pointerId: 3, button: 0, clientX: 520, clientY: 80 })
    fireEvent.pointerMove(window, { pointerId: 3, clientX: window.innerWidth + 100, clientY: window.innerHeight + 100 })
    fireEvent.pointerUp(window, { pointerId: 3 })

    expect(rail.style.left).toBe(`${window.innerWidth - 280 - 12}px`)
    expect(rail.style.top).toBe(`${window.innerHeight - 42 - 12}px`)
    expect(container.querySelector('.agent-rail-pending')).toHaveTextContent('结果 1')

    rerender(<ConsoleHarness props={{ ...props, collapsed: false }} />)
    expect(panel.style.left).toBe(`${window.innerWidth - 390 - 12}px`)
    expect(panel.style.top).toBe(`${window.innerHeight - 520 - 12}px`)
    rerender(<ConsoleHarness props={{ ...props, collapsed: true }} />)
    const collapsedAgain = container.querySelector<HTMLElement>('.agent-rail')
    expect(collapsedAgain?.style.left).toBe(`${window.innerWidth - 390 - 12}px`)
    expect(collapsedAgain?.style.top).toBe(`${window.innerHeight - 520 - 12}px`)
    expect(container.querySelector('#agent-console-panel')).toBe(panel)
  })

  it('moves the collapsed rail with arrow keys and does not drag from its button', () => {
    const { container, props } = renderConsole({ collapsed: true })
    const rail = container.querySelector<HTMLElement>('.agent-rail')
    if (!rail) throw new Error('collapsed run console is missing')
    vi.spyOn(rail, 'getBoundingClientRect').mockReturnValue(new DOMRect(100, 64, 260, 42))

    fireEvent.keyDown(rail, { key: 'ArrowRight' })
    fireEvent.keyDown(rail, { key: 'ArrowDown', shiftKey: true })
    expect(rail.style.left).toBe('626px')
    expect(rail.style.top).toBe('44px')

    const expand = screen.getByRole('button', { name: '展开运行面板' })
    fireEvent.pointerDown(expand, { pointerId: 4, button: 0, clientX: 120, clientY: 80 })
    fireEvent.pointerMove(window, { pointerId: 4, clientX: 300, clientY: 200 })
    fireEvent.pointerUp(window, { pointerId: 4 })
    expect(rail.style.left).toBe('626px')
    fireEvent.click(expand)
    expect(props.onToggle).toHaveBeenCalledOnce()
  })

  it('reclamps the collapsed rail after viewport resize and fits the panel when expanded', () => {
    const originalWidth = window.innerWidth
    const { container, rerender, props } = renderConsole({ collapsed: true })
    const rail = container.querySelector<HTMLElement>('.agent-rail')
    const panel = container.querySelector<HTMLElement>('#agent-console-panel')
    if (!rail || !panel) throw new Error('run console is missing')
    vi.spyOn(rail, 'getBoundingClientRect').mockReturnValue(new DOMRect(500, 64, 260, 42))
    vi.spyOn(panel, 'getBoundingClientRect').mockReturnValue(new DOMRect(100, 64, 390, 520))

    try {
      Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1000 })
      fireEvent.pointerDown(rail, { pointerId: 6, button: 0, clientX: 520, clientY: 80 })
      fireEvent.pointerMove(window, { pointerId: 6, clientX: 2000, clientY: 80 })
      fireEvent.pointerUp(window, { pointerId: 6 })
      expect(rail.style.left).toBe('728px')

      Object.defineProperty(window, 'innerWidth', { configurable: true, value: 500 })
      fireEvent.resize(window)
      expect(rail.style.left).toBe('228px')

      rerender(<ConsoleHarness props={{ ...props, collapsed: false }} />)
      expect(panel.style.left).toBe('98px')
    } finally {
      Object.defineProperty(window, 'innerWidth', { configurable: true, value: originalWidth })
      fireEvent.resize(window)
    }
  })

  it('stops dragging the collapsed rail when the pointer is cancelled', () => {
    const { container } = renderConsole({ collapsed: true })
    const rail = container.querySelector<HTMLElement>('.agent-rail')
    if (!rail) throw new Error('collapsed run console is missing')
    vi.spyOn(rail, 'getBoundingClientRect').mockReturnValue(new DOMRect(100, 64, 260, 42))

    fireEvent.pointerDown(rail, { pointerId: 5, button: 0, clientX: 120, clientY: 80 })
    fireEvent.pointerCancel(window, { pointerId: 5 })
    const leftAfterCancel = rail.style.left
    const topAfterCancel = rail.style.top
    fireEvent.pointerMove(window, { pointerId: 5, clientX: 400, clientY: 300 })

    expect(rail.style.left).toBe(leftAfterCancel)
    expect(rail.style.top).toBe(topAfterCancel)
  })

  it('shows a notice when a pending Script result is already assigned to its frozen target node', () => {
    renderConsole({
      pending: [scriptPendingSummary],
      nodes: [scriptNode],
      onGetPending: vi.fn().mockResolvedValue(scriptPendingDetail()),
    })
    fireEvent.click(screen.getByRole('button', { name: /^结果 / }))

    expect(screen.getByText('待确认结果已归属到画布节点')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /n_agent/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '确认追加' })).not.toBeInTheDocument()
  })

  it('confirms Script results from the recovery path and never offers force on a version conflict', async () => {
    const onApplyPending = vi.fn().mockRejectedValue(new Error('OUTPUT_VERSION_CONFLICT'))
    renderConsole({
      pending: [scriptPendingSummary],
      onGetPending: vi.fn().mockResolvedValue(scriptPendingDetail()),
      onApplyPending,
    })
    fireEvent.click(screen.getByRole('button', { name: /^结果 / }))
    selectRecoveryPending('n_agent')
    await screen.findByRole('button', { name: '确认追加' })

    fireEvent.click(screen.getByRole('button', { name: '确认追加' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('确认追加失败：OUTPUT_VERSION_CONFLICT')
    expect(onApplyPending).toHaveBeenCalledWith('pending_script', false, 'p_demo')
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '确认并强制应用' })).not.toBeInTheDocument()
  })

  it('shows an invalid Script reply as raw output without a confirmation action', async () => {
    const invalid = scriptPendingDetail()
    invalid.pending.status = PendingStatus.PendingInvalid
    invalid.pending.error = 'SCHEMA_INVALID: script scenes are invalid'
    renderConsole({
      pending: [{ ...scriptPendingSummary, status: PendingStatus.PendingInvalid, error: invalid.pending.error }],
      onGetPending: vi.fn().mockResolvedValue(invalid),
    })
    fireEvent.click(screen.getByRole('button', { name: /^结果 / }))
    selectRecoveryPending('n_agent')

    expect(await screen.findByText(/SCHEMA_INVALID: script scenes are invalid/)).toBeInTheDocument()
    expect(screen.getByText(/查看原始输出/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '确认追加' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '丢弃结果' })).toBeInTheDocument()
  })

  it('discards a bound Script result without changing the displayed original version', async () => {
    const onApplyPending = vi.fn().mockResolvedValue(undefined)
    const onDiscardPending = vi.fn().mockResolvedValue(undefined)
    renderConsole({
      pending: [scriptPendingSummary],
      onGetPending: vi.fn().mockResolvedValue(scriptPendingDetail()),
      onApplyPending,
      onDiscardPending,
    })
    fireEvent.click(screen.getByRole('button', { name: /^结果 / }))
    selectRecoveryPending('n_agent')
    await screen.findByText('目标 Script')
    fireEvent.click(screen.getByRole('button', { name: '丢弃结果' }))

    await waitFor(() => expect(onDiscardPending).toHaveBeenCalledWith('pending_script', 'p_demo'))
    expect(onApplyPending).not.toHaveBeenCalled()
    expect(screen.getByText('sha256:script-v1')).toBeInTheDocument()
  })

  it('prevents duplicate Script confirmation and retries only snapshot sync after a committed result', async () => {
    let resolveApply!: () => void
    const onApplyPending = vi.fn(() => new Promise<void>((resolve) => { resolveApply = resolve }))
    const onRefreshWorkspace = vi.fn().mockRejectedValueOnce(new Error('快照读取失败')).mockResolvedValueOnce(undefined)
    renderConsole({
      pending: [scriptPendingSummary],
      onGetPending: vi.fn().mockResolvedValue(scriptPendingDetail()),
      onApplyPending,
      onRefreshWorkspace,
    })
    fireEvent.click(screen.getByRole('button', { name: /^结果 / }))
    selectRecoveryPending('n_agent')
    await screen.findByRole('button', { name: '确认追加' })
    const confirm = screen.getByRole('button', { name: '确认追加' })

    fireEvent.click(confirm)
    fireEvent.click(confirm)
    expect(onApplyPending).toHaveBeenCalledOnce()
    resolveApply()
    expect(await screen.findByText(/操作已成功，但工程状态同步失败/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '重试同步' }))
    await waitFor(() => expect(onRefreshWorkspace).toHaveBeenCalledTimes(2))
    expect(onApplyPending).toHaveBeenCalledOnce()
  })
})
