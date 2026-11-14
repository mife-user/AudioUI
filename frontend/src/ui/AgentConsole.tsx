import { useCallback, useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent as ReactKeyboardEvent, type PointerEvent as ReactPointerEvent } from 'react'
import { PendingStatus, RunStatus } from '../../bindings/audio-ui/internal/domain/valueobject'
import type { Node, PendingSummary, WorkspaceRunSummary } from '../../bindings/audio-ui/internal/domain/entity'
import type { PermissionDecision } from '../api/runs'
import type { RunView } from '../stores/runStore'
import type { PendingController } from '../hooks/usePendingController'
import { getRunRequestGroups, useRunRequestContext } from '../hooks/runRequests'
import { pendingOwnerNodeId } from '../canvas/flowModel'
import { PermissionCard } from './PermissionCard'
import { QuestionCard } from './QuestionCard'
import { RunList } from './RunList'
import { Transcript } from './Transcript'
import { PendingArtifactCard } from './PendingArtifactCard'

type PanelPosition = { left: number; top: number }
type PanelSize = { width: number; height: number }
type PanelGeometry = PanelPosition & PanelSize
type RecoveryPanel = 'requests' | 'results' | null

type ConsoleDrag = {
  onMove: (event: PointerEvent) => void
  onEnd: (event: PointerEvent) => void
}

const defaultPanelSize: PanelSize = { width: 390, height: 520 }

function clampPanelGeometry(geometry: PanelGeometry, viewportWidth: number, viewportHeight: number): PanelGeometry {
  const maxWidth = Math.max(0, viewportWidth - 24)
  const minWidth = Math.min(260, maxWidth)
  const width = Math.max(minWidth, Math.min(geometry.width, maxWidth))
  const maxHeight = Math.max(0, viewportHeight - 24)
  const minHeight = Math.min(180, maxHeight)
  const height = Math.max(minHeight, Math.min(geometry.height, maxHeight))
  const maxTop = Math.max(12, viewportHeight - height - 12)
  const top = Math.max(12, Math.min(geometry.top, maxTop))
  const maxLeft = Math.max(12, viewportWidth - width - 12)
  const left = Math.max(12, Math.min(geometry.left, maxLeft))
  return { left, top, width, height }
}

type Props = {
  runs: WorkspaceRunSummary[]
  projectId: string | null
  nodes?: Node[]
  runViews: Record<string, RunView>
  selectedRunId: string | null
  selectedRun: RunView | undefined
  actionError: string
  collapsed: boolean
  onToggle: () => void
  onSelectRun: (runId: string) => void
  onRetryRun: (runId: string) => void
  onPermission: (runId: string, requestId: string, decision: PermissionDecision) => Promise<void>
  onQuestion: (runId: string, requestId: string, answer: string, via: string) => Promise<void>
  onInterrupt: (runId: string) => Promise<void>
  pendingController: PendingController
}

function active(status: string | undefined): boolean {
  return status === RunStatus.RunStarting || status === RunStatus.RunRunning
}

function statusLabel(status: string | undefined): string {
  switch (status) {
    case RunStatus.RunStarting: return '启动中'
    case RunStatus.RunRunning: return '工作中'
    case RunStatus.RunSucceeded: return '完成'
    case RunStatus.RunFailed: return '失败'
    case RunStatus.RunInterrupted: return '已中断'
    case RunStatus.RunImported: return '历史导入'
    default: return '未执行'
  }
}

function tone(status: string | undefined): string {
  if (status === RunStatus.RunFailed) return 'bad'
  if (status === RunStatus.RunStarting || status === RunStatus.RunRunning) return 'ok'
  return 'warn'
}

function formatTime(value: string | undefined): string {
  if (!value) return '未知'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '未知' : date.toLocaleString()
}

function pendingStatusLabel(status: PendingSummary['status']): string {
  if (status === PendingStatus.PendingInvalid) return '无效'
  if (status === PendingStatus.PendingPending) return '待应用'
  return String(status)
}

function requestTypeSummary(permissions: number, questions: number): string {
  return [permissions ? `权限 ${permissions}` : '', questions ? `问题 ${questions}` : ''].filter(Boolean).join(' · ')
}

export function AgentConsole({
  runs,
  projectId,
  nodes = [],
  runViews,
  selectedRunId,
  selectedRun,
  actionError,
  collapsed,
  onToggle,
  onSelectRun,
  onRetryRun,
  onPermission,
  onQuestion,
  onInterrupt,
  pendingController,
}: Props) {
  const [recoveryPanel, setRecoveryPanel] = useState<RecoveryPanel>(null)
  const [recoveryRunIds, setRecoveryRunIds] = useState<Set<string>>(() => new Set())
  const [panelPosition, setPanelPosition] = useState<PanelPosition | null>(null)
  const [panelSize, setPanelSize] = useState<PanelSize>(defaultPanelSize)
  const panelRef = useRef<HTMLElement>(null)
  const railRef = useRef<HTMLElement>(null)
  const panelPositionRef = useRef<PanelPosition | null>(null)
  const panelSizeRef = useRef(panelSize)
  const consoleDragRef = useRef<ConsoleDrag | null>(null)
  const consoleResizeRef = useRef<ConsoleDrag | null>(null)
  const requestContext = useRunRequestContext()

  const commitPanelGeometry = useCallback((geometry: PanelGeometry) => {
    const position = { left: geometry.left, top: geometry.top }
    panelPositionRef.current = position
    setPanelPosition(position)
    const currentSize = panelSizeRef.current
    if (currentSize.width !== geometry.width || currentSize.height !== geometry.height) {
      const size = { width: geometry.width, height: geometry.height }
      panelSizeRef.current = size
      setPanelSize(size)
    }
    return position
  }, [])

  const placeConsole = useCallback((left: number, top: number) => {
    if (!collapsed) {
      const geometry = clampPanelGeometry({ ...panelSizeRef.current, left, top }, window.innerWidth, window.innerHeight)
      return commitPanelGeometry(geometry)
    }
    const element = railRef.current
    if (!element) return null
    const rect = element.getBoundingClientRect()
    const position = {
      left: Math.max(12, Math.min(left, window.innerWidth - rect.width - 12)),
      top: Math.max(12, Math.min(top, window.innerHeight - rect.height - 12)),
    }
    panelPositionRef.current = position
    setPanelPosition(position)
    return position
  }, [collapsed, commitPanelGeometry])

  const stopConsoleDrag = useCallback(() => {
    const drag = consoleDragRef.current
    if (!drag) return
    window.removeEventListener('pointermove', drag.onMove)
    window.removeEventListener('pointerup', drag.onEnd)
    window.removeEventListener('pointercancel', drag.onEnd)
    consoleDragRef.current = null
  }, [])

  const stopConsoleResize = useCallback(() => {
    const resize = consoleResizeRef.current
    if (!resize) return
    window.removeEventListener('pointermove', resize.onMove)
    window.removeEventListener('pointerup', resize.onEnd)
    window.removeEventListener('pointercancel', resize.onEnd)
    consoleResizeRef.current = null
  }, [])

  useLayoutEffect(() => {
    stopConsoleDrag()
    stopConsoleResize()
    if (!panelPositionRef.current && projectId) {
      const toolbar = document.querySelector<HTMLElement>('.canvas-toolbar')
      const topbar = document.querySelector<HTMLElement>('.topbar')
      const top = (toolbar?.getBoundingClientRect().bottom ?? topbar?.getBoundingClientRect().bottom ?? 0) + 12
      commitPanelGeometry(clampPanelGeometry({
        ...panelSizeRef.current,
        left: window.innerWidth - panelSizeRef.current.width - 16,
        top,
      }, window.innerWidth, window.innerHeight))
    }
    const keepConsoleVisible = () => {
      const position = panelPositionRef.current
      if (!position) return
      if (collapsed) {
        placeConsole(position.left, position.top)
      } else {
        commitPanelGeometry(clampPanelGeometry({ ...panelSizeRef.current, ...position }, window.innerWidth, window.innerHeight))
      }
    }
    keepConsoleVisible()
    window.addEventListener('resize', keepConsoleVisible)
    return () => window.removeEventListener('resize', keepConsoleVisible)
  }, [collapsed, projectId, commitPanelGeometry, placeConsole, stopConsoleDrag, stopConsoleResize])

  useEffect(() => stopConsoleDrag, [stopConsoleDrag])
  useEffect(() => stopConsoleResize, [stopConsoleResize])

  function consoleOrigin() {
    const consoleElement = collapsed ? railRef.current : panelRef.current
    if (!consoleElement) return null
    const rect = consoleElement.getBoundingClientRect()
    return panelPositionRef.current ?? { left: rect.left, top: rect.top }
  }

  function startConsoleDrag(event: ReactPointerEvent<HTMLElement>) {
    if (event.button !== 0 || (event.target as HTMLElement).closest('button')) return
    const consoleElement = collapsed ? railRef.current : panelRef.current
    if (!consoleElement) return
    event.preventDefault()
    const rect = consoleElement.getBoundingClientRect()
    const origin = placeConsole(rect.left, rect.top)
    if (!origin) return
    const pointerId = event.pointerId
    const startX = event.clientX
    const startY = event.clientY
    const onMove = (moveEvent: PointerEvent) => {
      if (moveEvent.pointerId !== pointerId) return
      placeConsole(origin.left + moveEvent.clientX - startX, origin.top + moveEvent.clientY - startY)
    }
    const onEnd = (endEvent: PointerEvent) => {
      if (endEvent.pointerId === pointerId) stopConsoleDrag()
    }
    consoleDragRef.current = { onMove, onEnd }
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onEnd)
    window.addEventListener('pointercancel', onEnd)
  }

  function moveConsoleWithKeyboard(event: ReactKeyboardEvent<HTMLElement>) {
    if (event.target !== event.currentTarget) return
    const step = event.shiftKey ? 32 : 8
    let deltaX = 0
    let deltaY = 0
    switch (event.key) {
      case 'ArrowLeft': deltaX = -step; break
      case 'ArrowRight': deltaX = step; break
      case 'ArrowUp': deltaY = -step; break
      case 'ArrowDown': deltaY = step; break
      default: return
    }
    event.preventDefault()
    const origin = consoleOrigin()
    if (origin) placeConsole(origin.left + deltaX, origin.top + deltaY)
  }

  function startConsoleResize(event: ReactPointerEvent<HTMLButtonElement>) {
    if (event.button !== 0) return
    event.preventDefault()
    event.stopPropagation()
    const position = panelPositionRef.current ?? {
      left: window.innerWidth - panelSizeRef.current.width - 16,
      top: (document.querySelector<HTMLElement>('.canvas-toolbar')?.getBoundingClientRect().bottom ?? 0) + 12,
    }
    const start = { ...panelSizeRef.current, ...position, x: event.clientX, y: event.clientY, pointerId: event.pointerId }
    const right = start.left + start.width
    const onMove = (moveEvent: PointerEvent) => {
      if (moveEvent.pointerId !== start.pointerId) return
      const width = start.width - (moveEvent.clientX - start.x)
      commitPanelGeometry(clampPanelGeometry({
        left: right - width,
        top: start.top,
        width,
        height: start.height + moveEvent.clientY - start.y,
      }, window.innerWidth, window.innerHeight))
    }
    const onEnd = (endEvent: PointerEvent) => {
      if (endEvent.pointerId === start.pointerId) stopConsoleResize()
    }
    consoleResizeRef.current = { onMove, onEnd }
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onEnd)
    window.addEventListener('pointercancel', onEnd)
  }

  function resizeConsoleWithKeyboard(event: ReactKeyboardEvent<HTMLButtonElement>) {
    const step = event.shiftKey ? 32 : 8
    let deltaWidth = 0
    let deltaHeight = 0
    switch (event.key) {
      case 'ArrowLeft': deltaWidth = step; break
      case 'ArrowRight': deltaWidth = -step; break
      case 'ArrowUp': deltaHeight = -step; break
      case 'ArrowDown': deltaHeight = step; break
      default: return
    }
    event.preventDefault()
    const position = panelPositionRef.current ?? { left: window.innerWidth - panelSize.width - 16, top: 12 }
    const width = panelSizeRef.current.width + deltaWidth
    commitPanelGeometry(clampPanelGeometry({
      left: position.left + panelSizeRef.current.width - width,
      top: position.top,
      width,
      height: panelSizeRef.current.height + deltaHeight,
    }, window.innerWidth, window.innerHeight))
  }

  function toggleRecoveryPanel(panel: Exclude<RecoveryPanel, null>) {
    setRecoveryPanel((current) => current === panel ? null : panel)
    if (collapsed) onToggle()
  }

  const requestGroups = requestContext?.groups ?? getRunRequestGroups(runViews, projectId)
  const respondPermission = requestContext?.respondPermission ?? onPermission
  const respondQuestion = requestContext?.respondQuestion ?? onQuestion
  const pendingItems = pendingController.pendingItems
  const recoveryPendingItems = pendingItems.filter((item) => !pendingOwnerNodeId(item, nodes))
  const selectedPending = pendingController.selectedPending
  const selectedPendingDetail = pendingController.selectedPendingDetail
  const selectedPendingOutputNode = pendingController.selectedOutputNode
  const requestsCount = requestGroups.reduce((total, item) => total + item.permissions.length + item.questions.length, 0)
  const activeRunCount = runs.filter((run) => active(run.status)).length
  const selectedStatus = selectedRun?.snapshot?.run.status ?? selectedRun?.summary?.status
  const consoleStatus = activeRunCount ? `${activeRunCount} 项运行中` : statusLabel(selectedStatus)

  useEffect(() => {
    if (collapsed) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onToggle()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [collapsed, onToggle])

  function revealRequestGroup(runId: string, nodeId: string) {
    const owner = nodes.find((node) => node.id === nodeId && (node.type === 'topic' || node.type === 'agent'))
    if (!owner || !requestContext?.locateNode(nodeId)) {
      setRecoveryRunIds((current) => new Set(current).add(runId))
      return
    }
    setRecoveryPanel(null)
  }

  return (
    <>
      {collapsed ? (
        <aside
          ref={railRef}
          className="agent-rail"
          role="group"
          tabIndex={0}
          aria-label="运行状态小窗，可拖动；使用方向键移动"
          aria-keyshortcuts="ArrowLeft ArrowRight ArrowUp ArrowDown"
          style={panelPosition ? { left: panelPosition.left, top: panelPosition.top, right: 'auto' } : undefined}
          onPointerDown={startConsoleDrag}
          onKeyDown={moveConsoleWithKeyboard}
        >
          <span className={`dot ${activeRunCount ? 'ok' : tone(selectedStatus)}`} aria-hidden="true" />
          <span className="agent-rail-state">{consoleStatus}</span>
          <button
            type="button"
            className={`agent-rail-count${recoveryPanel === 'requests' ? ' is-open' : ''}`}
            aria-label={`请求 ${requestsCount}`}
            aria-expanded={recoveryPanel === 'requests'}
            aria-controls="agent-console-recovery"
            onClick={() => toggleRecoveryPanel('requests')}
          >请求 {requestsCount}</button>
          <button
            type="button"
            className={`agent-rail-pending${recoveryPanel === 'results' ? ' is-open' : ''}`}
            aria-label={`结果 ${pendingItems.length}`}
            aria-expanded={recoveryPanel === 'results'}
            aria-controls="agent-console-recovery"
            onClick={() => toggleRecoveryPanel('results')}
          >结果 {pendingItems.length}</button>
          <button className="agent-icon-btn" onClick={onToggle} aria-label="展开运行面板" aria-expanded="false" aria-controls="agent-console-panel">⌄</button>
        </aside>
      ) : null}
      <aside
        ref={panelRef}
        id="agent-console-panel"
        className="agent-console"
        aria-label="运行面板"
        hidden={collapsed}
        style={{
          ...(panelPosition ? { left: panelPosition.left, top: panelPosition.top, right: 'auto' } : {}),
          width: panelSize.width,
          height: panelSize.height,
        }}
      >
        <div
          className="agent-console-head"
          role="group"
          tabIndex={0}
          aria-label="运行面板标题栏，可拖动；使用方向键移动"
          aria-keyshortcuts="ArrowLeft ArrowRight ArrowUp ArrowDown"
          onPointerDown={startConsoleDrag}
          onKeyDown={moveConsoleWithKeyboard}
        >
          <span className={`dot ${activeRunCount ? 'ok' : tone(selectedStatus)}`} />
          <span className="agent-console-title">AGENT</span>
          <span className="agent-console-state">{consoleStatus}</span>
          <button
            type="button"
            className={`agent-console-count${recoveryPanel === 'requests' ? ' is-open' : ''}`}
            aria-label={`请求 ${requestsCount}`}
            aria-expanded={recoveryPanel === 'requests'}
            aria-controls="agent-console-recovery"
            onClick={() => toggleRecoveryPanel('requests')}
          >请求 {requestsCount}</button>
          <button
            type="button"
            className={`agent-console-count${recoveryPanel === 'results' ? ' is-open' : ''}`}
            aria-label={`结果 ${pendingItems.length}`}
            aria-expanded={recoveryPanel === 'results'}
            aria-controls="agent-console-recovery"
            onClick={() => toggleRecoveryPanel('results')}
          >结果 {pendingItems.length}</button>
          <button className="agent-icon-btn" onClick={onToggle} aria-label="收起运行面板" aria-expanded="true" aria-controls="agent-console-panel">×</button>
        </div>
        <div className="agent-console-body">
          {actionError ? <div className="workspace-inline-error" role="alert">{actionError}</div> : null}
          {recoveryPanel ? (
            <section
              id="agent-console-recovery"
              className="agent-recovery-panel"
              aria-label={recoveryPanel === 'requests' ? '请求定位与恢复' : '结果异常恢复'}
            >
              <div className="agent-recovery-head">
                <strong>{recoveryPanel === 'requests' ? '请求定位与恢复' : '结果异常恢复'}</strong>
                <button type="button" className="agent-mini-btn" aria-label="关闭恢复入口" onClick={() => setRecoveryPanel(null)}>关闭</button>
              </div>
              <div className="agent-recovery-content">
                {recoveryPanel === 'requests' ? (
                  requestGroups.length ? requestGroups.map((group) => {
                    const owner = nodes.find((node) => node.id === group.nodeId && (node.type === 'topic' || node.type === 'agent'))
                    if (owner && !recoveryRunIds.has(group.runId)) {
                      return (
                        <div className="run-request-location" key={group.runId}>
                          <div>
                            <strong>{owner.title}</strong>
                            <span>{requestTypeSummary(group.permissions.length, group.questions.length)} · 运行 {group.runId}</span>
                          </div>
                          <button type="button" className="agent-mini-btn" onClick={() => revealRequestGroup(group.runId, group.nodeId)}>定位节点</button>
                        </div>
                      )
                    }
                    return (
                      <div className="run-request-group" key={group.runId}>
                        <div className="run-request-owner">无法定位发起节点 {group.nodeId} · 运行 {group.runId}；可在此恢复处理。</div>
                        {group.permissions.map((request) => (
                          <PermissionCard
                            key={`${group.runId}-permission-${request.id}`}
                            request={request}
                            onRespond={(requestId, decision) => respondPermission(group.runId, requestId, decision)}
                          />
                        ))}
                        {group.questions.map((request) => (
                          <QuestionCard
                            key={`${group.runId}-question-${request.id}`}
                            request={request}
                            onAnswer={(requestId, answer, via) => respondQuestion(group.runId, requestId, answer, via)}
                          />
                        ))}
                      </div>
                    )
                  }) : <div className="agent-console-empty">没有待确认请求</div>
                ) : recoveryPendingItems.length ? (
                  <>
                    <ul className="console-pending-list" aria-label="无法定位节点的待处理结果">
                      {recoveryPendingItems.map((item) => {
                        const resolved = pendingController.resolvedPendingIds.has(item.id)
                        return (
                          <li key={item.id}>
                            <button
                              type="button"
                              className={`console-pending-list-item${pendingController.selectedPendingId === item.id ? ' is-selected' : ''}`}
                              aria-pressed={pendingController.selectedPendingId === item.id}
                              disabled={!projectId || resolved}
                              onClick={() => void pendingController.selectFromConsole(item.id)}
                            >
                              <span className="console-pending-list-status">{resolved ? '已提交' : pendingStatusLabel(item.status)}</span>
                              <strong>{item.nodeId}</strong>
                              <span>{item.summary || item.kind}</span>
                            </button>
                          </li>
                        )
                      })}
                    </ul>
                    {selectedPending ? pendingController.surface === 'console' ? (
                      <PendingArtifactCard
                        summary={selectedPending}
                        detail={selectedPendingDetail}
                        outputNode={selectedPendingOutputNode}
                        loading={pendingController.loading}
                        loadError={pendingController.loadError}
                        actionError={pendingController.actionError}
                        forceConfirmation={pendingController.forceConfirmation}
                        actionBusy={pendingController.actionBusy}
                        resolved={pendingController.resolved}
                        syncError={pendingController.syncError}
                        onRetryDetail={pendingController.retryDetail}
                        onApply={() => void pendingController.apply()}
                        onDiscard={() => void pendingController.discard()}
                        onConfirmForce={() => void pendingController.apply(true)}
                        onCancelForce={pendingController.cancelForce}
                        onRetryWorkspace={() => void pendingController.retryWorkspace()}
                      />
                    ) : (
                      <p className="console-pending-message">请从归属节点卡片打开详情。</p>
                    ) : (
                      <p className="console-pending-message">选择一项以按需读取正文。</p>
                    )}
                  </>
                ) : pendingItems.length ? (
                  <div className="console-pending-empty">
                    <strong>待确认结果已归属到画布节点</strong>
                    <p>请在对应的选题、Agent 或 Script 卡片内查看和处理。</p>
                  </div>
                ) : (
                  <div className="console-pending-empty">
                    <strong>暂无待确认结果</strong>
                    <p>运行结果会在归属节点卡片中处理；无法定位的项目保留在此恢复。</p>
                  </div>
                )}
              </div>
            </section>
          ) : null}
          <section className="console-conversation" aria-label="运行记录与对话">
            <RunList runs={runs} selectedRunId={selectedRunId} onSelect={onSelectRun} />
            {selectedRun ? (
              <div className="run-detail">
                <div className="run-detail-head">
                  <div>
                    <span className="workspace-kicker">运行详情</span>
                    <strong>{selectedRun.snapshot?.runId ?? selectedRun.summary?.id ?? selectedRunId}</strong>
                  </div>
                  {active(selectedStatus) ? (
                    <button className="agent-mini-btn deny" type="button" onClick={() => void onInterrupt(selectedRunId || '')}>
                      停止运行
                    </button>
                  ) : null}
                </div>
                <dl className="run-metadata">
                  <div><dt>节点</dt><dd>{selectedRun.snapshot?.run.nodeId ?? selectedRun.summary?.nodeId ?? '—'}</dd></div>
                  <div><dt>开始时间</dt><dd>{formatTime(selectedRun.snapshot?.run.startedAt ?? selectedRun.summary?.startedAt)}</dd></div>
                  <div><dt>上下文哈希</dt><dd>{(selectedRun.snapshot?.run.contextHash ?? selectedRun.summary?.contextHash) ? `${selectedRun.snapshot?.run.contextHash ?? selectedRun.summary?.contextHash}`.slice(0, 12) : '未知'}</dd></div>
                  <div><dt>估算 tokens</dt><dd>{selectedRun.snapshot?.run.estimatedTokens ?? selectedRun.summary?.estimatedTokens ?? '未知'}</dd></div>
                  <div><dt>实际 input tokens</dt><dd>{typeof (selectedRun.snapshot?.run.actualInputTokens ?? selectedRun.summary?.actualInputTokens) === 'number' ? (selectedRun.snapshot?.run.actualInputTokens ?? selectedRun.summary?.actualInputTokens) : '未知'}</dd></div>
                </dl>
                {selectedRun.error ? (
                  <div className="workspace-inline-error" role="alert">
                    <span>运行快照读取失败：{selectedRun.error}</span>
                    <button className="agent-mini-btn" type="button" onClick={() => onRetryRun(selectedRunId || '')}>重试</button>
                  </div>
                ) : null}
                {!selectedRun.snapshot && selectedRun.loading ? <div className="agent-console-empty">正在恢复运行详情…</div> : null}
                {!selectedRun.snapshot && !selectedRun.loading && !selectedRun.error ? (
                  <div className="agent-console-empty">运行详情尚未载入。</div>
                ) : null}
                {selectedRun.snapshot?.run.error ? <div className="run-terminal-error" role="status">{selectedRun.snapshot.run.error}</div> : null}
                {selectedRun.snapshot ? <Transcript items={selectedRun.snapshot.run.transcript ?? []} /> : null}
              </div>
            ) : (
              <div className="console-no-run">暂无运行记录</div>
            )}
          </section>
        </div>
        <button
          type="button"
          className="agent-console-resize"
          aria-label="调整运行面板尺寸；方向键调整，按住 Shift 加快"
          aria-keyshortcuts="ArrowLeft ArrowRight ArrowUp ArrowDown"
          title="拖动左下角或使用方向键调整尺寸"
          onPointerDown={startConsoleResize}
          onKeyDown={resizeConsoleWithKeyboard}
        />
      </aside>
    </>
  )
}
