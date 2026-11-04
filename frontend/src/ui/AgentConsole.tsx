import { useState } from 'react'
import { RunStatus } from '../../bindings/audio-ui/internal/domain/valueobject'
import type { WorkspaceRunSummary } from '../../bindings/audio-ui/internal/domain/entity'
import type { PermissionDecision, PermissionRequest, QuestionRequest } from '../api/runs'
import type { RunView } from '../stores/runStore'
import { PermissionCard } from './PermissionCard'
import { QuestionCard } from './QuestionCard'
import { RunList } from './RunList'
import { ConsoleTabs, type ConsoleTab } from './ConsoleTabs'
import { Transcript } from './Transcript'

type Props = {
  runs: WorkspaceRunSummary[]
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

export function AgentConsole({
  runs,
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
}: Props) {
  const [tab, setTab] = useState<ConsoleTab>('conversation')
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

        <ConsoleTabs active={tab} pendingCount={0} onChange={setTab} />
        {tab === 'pending' ? (
          <section id="console-panel-pending" role="tabpanel" aria-labelledby="console-tab-pending" className="console-pending-empty">
            <strong>暂无待处理内容</strong>
            <p>待应用产物预览与操作将在 M2-10B 接入。</p>
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
