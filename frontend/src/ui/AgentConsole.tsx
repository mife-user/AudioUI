import { useEffect, useMemo, useRef, useState } from 'react'
import { PendingStatus, OutputKind, RunStatus } from '../../bindings/audio-ui/internal/domain/valueobject'
import type { Node, PendingDetail, PendingSummary, WorkspaceRunSummary } from '../../bindings/audio-ui/internal/domain/entity'
import type { PermissionDecision, PermissionRequest, QuestionRequest } from '../api/runs'
import type { RunView } from '../stores/runStore'
import { PermissionCard } from './PermissionCard'
import { QuestionCard } from './QuestionCard'
import { RunList } from './RunList'
import { ConsoleTabs, type ConsoleTab } from './ConsoleTabs'
import { Transcript } from './Transcript'
import { PendingArtifactCard } from './PendingArtifactCard'

type Props = {
  runs: WorkspaceRunSummary[]
  projectId: string | null
  nodes: Node[]
  pending: PendingSummary[]
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
  onGetPending: (id: string, projectId: string) => Promise<PendingDetail>
  onApplyPending: (id: string, force: boolean, projectId: string) => Promise<void>
  onDiscardPending: (id: string, projectId: string) => Promise<void>
  onRefreshWorkspace: () => Promise<void>
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

function requestSummary(view: RunView): { runId: string; permissions: PermissionRequest[]; questions: QuestionRequest[] } | null {
  if (view.error || !active(view.snapshot?.run.status ?? view.summary?.status)) return null
  return {
    runId: view.snapshot?.runId ?? view.summary?.id ?? '',
    permissions: view.snapshot?.permissions ?? [],
    questions: view.snapshot?.questions ?? [],
  }
}

function formatTime(value: string | undefined): string {
  if (!value) return '未知'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '未知' : date.toLocaleString()
}

function messageOf(error: unknown): string {
  if (error instanceof Error) return error.message
  if (typeof error === 'string') return error
  if (error && typeof error === 'object' && 'message' in error) return String((error as { message: unknown }).message)
  return String(error)
}

function pendingStatusLabel(status: PendingSummary['status']): string {
  if (status === PendingStatus.PendingInvalid) return '无效'
  if (status === PendingStatus.PendingPending) return '待应用'
  return String(status)
}

function isPendingHashConflict(message: string): boolean {
  return message.includes('PARAMS_CHANGED') || message.includes('CONTEXT_CHANGED')
}

export function AgentConsole({
  runs,
  projectId,
  nodes,
  pending,
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
  onGetPending,
  onApplyPending,
  onDiscardPending,
  onRefreshWorkspace,
}: Props) {
  const [tab, setTab] = useState<ConsoleTab>('conversation')
  const pendingItems = useMemo(
    () => pending.filter((item) => item.status === PendingStatus.PendingPending || item.status === PendingStatus.PendingInvalid),
    [pending],
  )
  const [selectedPendingId, setSelectedPendingId] = useState('')
  const [pendingDetail, setPendingDetail] = useState<PendingDetail | null>(null)
  const [pendingLoading, setPendingLoading] = useState(false)
  const [pendingLoadError, setPendingLoadError] = useState('')
  const [pendingActionId, setPendingActionId] = useState('')
  const [pendingActionError, setPendingActionError] = useState('')
  const [forceConfirmationId, setForceConfirmationId] = useState('')
  const [pendingSyncError, setPendingSyncError] = useState('')
  const [resolvedPendingIds, setResolvedPendingIds] = useState<Set<string>>(() => new Set())
  const pendingGeneration = useRef(0)
  const pendingActionInFlight = useRef(false)
  const currentProjectId = useRef(projectId)
  currentProjectId.current = projectId
  const selectedPending = pendingItems.find((item) => item.id === selectedPendingId)
  const selectedPendingDetail = pendingDetail?.pending.id === selectedPendingId ? pendingDetail : null
  const isBoundScriptPending = selectedPendingDetail?.pending.kind === OutputKind.OutputKindScript
    && Boolean(selectedPendingDetail.pending.outputBinding?.bound)
  const selectedOutputNodeId = selectedPendingDetail?.pending.resultNodeId
    || selectedPendingDetail?.pending.outputBinding?.nodeId
  const selectedOutputNode = selectedOutputNodeId ? nodes.find((node) => node.id === selectedOutputNodeId) : undefined

  useEffect(() => {
    pendingGeneration.current += 1
    pendingActionInFlight.current = false
    setSelectedPendingId('')
    setPendingDetail(null)
    setPendingLoading(false)
    setPendingLoadError('')
    setPendingActionId('')
    setPendingActionError('')
    setForceConfirmationId('')
    setPendingSyncError('')
    setResolvedPendingIds(new Set())
  }, [projectId])

  useEffect(() => {
    const activeIDs = new Set(pendingItems.map((item) => item.id))
    setResolvedPendingIds((current) => new Set([...current].filter((id) => activeIDs.has(id))))
    if (selectedPendingId && !activeIDs.has(selectedPendingId)) {
      pendingGeneration.current += 1
      setSelectedPendingId('')
      setPendingDetail(null)
      setPendingLoading(false)
      setPendingLoadError('')
      setPendingActionError('')
      setForceConfirmationId('')
      setPendingSyncError('')
    }
  }, [pendingItems, selectedPendingId])

  async function loadPending(id: string) {
    if (!projectId) return
    const ownerProjectId = projectId
    const generation = ++pendingGeneration.current
    setSelectedPendingId(id)
    setPendingDetail(null)
    setPendingLoading(true)
    setPendingLoadError('')
    setPendingActionError('')
    setForceConfirmationId('')
    setPendingSyncError('')
    try {
      const detail = await onGetPending(id, ownerProjectId)
      if (generation !== pendingGeneration.current || currentProjectId.current !== ownerProjectId) return
      setPendingDetail(detail)
    } catch (failure) {
      if (generation !== pendingGeneration.current || currentProjectId.current !== ownerProjectId) return
      setPendingLoadError(messageOf(failure))
    } finally {
      if (generation === pendingGeneration.current && currentProjectId.current === ownerProjectId) setPendingLoading(false)
    }
  }

  async function mutatePending(action: 'apply' | 'discard', force = false) {
    const id = selectedPendingId
    const ownerProjectId = projectId
    if (!id || !ownerProjectId || pendingActionInFlight.current || resolvedPendingIds.has(id)) return
    pendingActionInFlight.current = true
    setPendingActionId(id)
    setPendingActionError('')
    setPendingSyncError('')
    try {
      if (action === 'apply') await onApplyPending(id, force, ownerProjectId)
      else await onDiscardPending(id, ownerProjectId)
      if (currentProjectId.current !== ownerProjectId) return
      setResolvedPendingIds((current) => new Set(current).add(id))
      setForceConfirmationId('')
      try {
        await onRefreshWorkspace()
      } catch (failure) {
        if (currentProjectId.current === ownerProjectId) {
          setPendingSyncError(`操作已成功，但工程状态同步失败：${messageOf(failure)}`)
        }
      }
    } catch (failure) {
      if (currentProjectId.current !== ownerProjectId) return
      const message = messageOf(failure)
      if (action === 'apply' && !force && !isBoundScriptPending && isPendingHashConflict(message)) {
        setForceConfirmationId(id)
        setPendingActionError('')
      } else {
        const actionLabel = action === 'apply' ? isBoundScriptPending ? '确认追加' : '应用' : '丢弃'
        setPendingActionError(`${actionLabel}失败：${message}`)
      }
    } finally {
      if (currentProjectId.current === ownerProjectId) {
        pendingActionInFlight.current = false
        setPendingActionId('')
      }
    }
  }

  async function retryWorkspace() {
    const ownerProjectId = projectId
    if (!ownerProjectId) return
    try {
      await onRefreshWorkspace()
      if (currentProjectId.current === ownerProjectId) setPendingSyncError('')
    } catch (failure) {
      if (currentProjectId.current === ownerProjectId) setPendingSyncError(`工程状态同步失败：${messageOf(failure)}`)
    }
  }

  const requestGroups = Object.values(runViews).flatMap((view) => {
    const summary = requestSummary(view)
    return summary ? [summary] : []
  })
  const requestsCount = requestGroups.reduce((total, item) => total + item.permissions.length + item.questions.length, 0)
  const selectedStatus = selectedRun?.snapshot?.run.status ?? selectedRun?.summary?.status
  const selectedChain = selectedRun?.snapshot?.run.chainId ?? selectedRun?.summary?.chainId

  if (collapsed) {
    return (
      <aside className="agent-rail" aria-label="运行控制台收起态">
        <span className={`dot ${tone(selectedStatus)}`} />
        <span className="agent-rail-count">{requestsCount || ''}</span>
        <span className="agent-rail-name">{selectedChain || '暂无运行'}</span>
        <span className="agent-rail-label">AGENT</span>
        <button className="agent-icon-btn" onClick={onToggle} aria-label="展开控制台">‹</button>
      </aside>
    )
  }

  return (
    <aside className="agent-console" aria-label="运行控制台">
      <div className="agent-console-head">
        <span className={`dot ${tone(selectedStatus)}`} />
        <span className="agent-console-title">AGENT</span>
        <span className="agent-console-state">{statusLabel(selectedStatus)}</span>
        <button className="agent-icon-btn" onClick={onToggle} aria-label="收起控制台">›</button>
      </div>
      <div className="agent-console-body">
        {actionError ? <div className="workspace-inline-error" role="alert">{actionError}</div> : null}
        <div className="agent-section-label">待确认请求 <span>{requestsCount}</span></div>
        {requestGroups.flatMap((group) => [
          ...group.permissions.map((request) => (
            <div className="run-request-group" key={`${group.runId}-permission-${request.id}`}>
              <div className="run-request-owner">运行 {group.runId}</div>
              <PermissionCard
                request={request}
                onRespond={(requestId, decision) => onPermission(group.runId, requestId, decision)}
              />
            </div>
          )),
          ...group.questions.map((request) => (
            <div className="run-request-group" key={`${group.runId}-question-${request.id}`}>
              <div className="run-request-owner">运行 {group.runId}</div>
              <QuestionCard
                request={request}
                onAnswer={(requestId, answer, via) => onQuestion(group.runId, requestId, answer, via)}
              />
            </div>
          )),
        ])}
        {requestsCount === 0 ? <div className="agent-console-empty">没有待确认请求</div> : null}

        <ConsoleTabs active={tab} pendingCount={pendingItems.length} onChange={setTab} />
        {tab === 'pending' ? (
          <section id="console-panel-pending" role="tabpanel" aria-labelledby="console-tab-pending" className="console-pending-panel">
            {pendingItems.length ? (
              <>
                <ul className="console-pending-list" aria-label="当前工程的待处理结果">
                  {pendingItems.map((item) => {
                    const resolved = resolvedPendingIds.has(item.id)
                    return (
                      <li key={item.id}>
                        <button
                          type="button"
                          className={`console-pending-list-item${selectedPendingId === item.id ? ' is-selected' : ''}`}
                          aria-pressed={selectedPendingId === item.id}
                          disabled={!projectId || resolved}
                          onClick={() => void loadPending(item.id)}
                        >
                          <span className="console-pending-list-status">{resolved ? '已提交' : pendingStatusLabel(item.status)}</span>
                          <strong>{item.nodeId}</strong>
                          <span>{item.summary || item.kind}</span>
                        </button>
                      </li>
                    )
                  })}
                </ul>
                {selectedPending ? (
                  <PendingArtifactCard
                    summary={selectedPending}
                    detail={pendingDetail?.pending.id === selectedPending.id ? pendingDetail : null}
                    outputNode={selectedOutputNode}
                    loading={pendingLoading}
                    loadError={pendingLoadError}
                    actionError={pendingActionError}
                    forceConfirmation={forceConfirmationId === selectedPending.id}
                    actionBusy={pendingActionId === selectedPending.id}
                    resolved={resolvedPendingIds.has(selectedPending.id)}
                    syncError={pendingSyncError}
                    onRetryDetail={() => void loadPending(selectedPending.id)}
                    onApply={() => void mutatePending('apply')}
                    onDiscard={() => void mutatePending('discard')}
                    onConfirmForce={() => void mutatePending('apply', true)}
                    onCancelForce={() => { setForceConfirmationId(''); setPendingActionError('') }}
                    onRetryWorkspace={() => void retryWorkspace()}
                  />
                ) : (
                  <p className="console-pending-message">选择一项以按需读取正文。</p>
                )}
              </>
            ) : (
              <div className="console-pending-empty">
                <strong>暂无待处理结果</strong>
                <p>结构化运行结果会出现在这里，应用后才会改变节点内容。</p>
              </div>
            )}
          </section>
        ) : (
          <section id="console-panel-conversation" role="tabpanel" aria-labelledby="console-tab-conversation" className="console-conversation">
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
        )}
      </div>
    </aside>
  )
}
