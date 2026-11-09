import { useState } from 'react'
import { PendingStatus, RunStatus } from '../../bindings/audio-ui/internal/domain/valueobject'
import type { PendingSummary, WorkspaceRunSummary } from '../../bindings/audio-ui/internal/domain/entity'
import type { PermissionDecision, PermissionRequest, QuestionRequest } from '../api/runs'
import type { RunView } from '../stores/runStore'
import type { PendingController } from '../hooks/usePendingController'
import { PermissionCard } from './PermissionCard'
import { QuestionCard } from './QuestionCard'
import { RunList } from './RunList'
import { ConsoleTabs, type ConsoleTab } from './ConsoleTabs'
import { Transcript } from './Transcript'
import { PendingArtifactCard } from './PendingArtifactCard'

type Props = {
  runs: WorkspaceRunSummary[]
  projectId: string | null
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

function pendingStatusLabel(status: PendingSummary['status']): string {
  if (status === PendingStatus.PendingInvalid) return '无效'
  if (status === PendingStatus.PendingPending) return '待应用'
  return String(status)
}

export function AgentConsole({
  runs,
  projectId,
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
  const [tab, setTab] = useState<ConsoleTab>('conversation')
  const pendingItems = pendingController.pendingItems
  const selectedPending = pendingController.selectedPending
  const selectedPendingDetail = pendingController.selectedPendingDetail
  const selectedPendingOutputNode = pendingController.selectedOutputNode
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
                  <p className="console-pending-message">请从结果所属 Script 卡片打开详情。</p>
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
