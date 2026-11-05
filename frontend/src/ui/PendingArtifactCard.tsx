import { OutputKind, PendingStatus } from '../../bindings/audio-ui/internal/domain/valueobject'
import type { PendingDetail, PendingSummary } from '../../bindings/audio-ui/internal/domain/entity'
import { decodePendingPayload } from '../api/pending'

type TopicPreview = {
  title: string
  angle: string
  hook: string
}

type Props = {
  summary: PendingSummary
  detail: PendingDetail | null
  loading: boolean
  loadError: string
  actionError: string
  forceConfirmation: boolean
  actionBusy: boolean
  resolved: boolean
  syncError: string
  onRetryDetail: () => void
  onApply: () => void
  onDiscard: () => void
  onConfirmForce: () => void
  onCancelForce: () => void
  onRetryWorkspace: () => void
}

function topicsFromPayload(payload: string): TopicPreview[] {
  try {
    const value: unknown = JSON.parse(payload)
    if (!value || typeof value !== 'object' || !('topics' in value) || !Array.isArray(value.topics)) return []
    return value.topics.flatMap((item): TopicPreview[] => {
      if (!item || typeof item !== 'object' || Array.isArray(item)) return []
      const candidate = item as Record<string, unknown>
      if (typeof candidate.title !== 'string' || typeof candidate.angle !== 'string' || typeof candidate.hook !== 'string') return []
      return [{ title: candidate.title, angle: candidate.angle, hook: candidate.hook }]
    })
  } catch {
    return []
  }
}

function statusLabel(status: string): string {
  if (status === PendingStatus.PendingInvalid) return '校验失败'
  if (status === PendingStatus.PendingPending) return '待应用'
  if (status === PendingStatus.PendingApplied) return '已应用'
  if (status === PendingStatus.PendingDiscarded) return '已丢弃'
  return status
}

export function PendingArtifactCard({
  summary,
  detail,
  loading,
  loadError,
  actionError,
  forceConfirmation,
  actionBusy,
  resolved,
  syncError,
  onRetryDetail,
  onApply,
  onDiscard,
  onConfirmForce,
  onCancelForce,
  onRetryWorkspace,
}: Props) {
  const pending = detail?.pending ?? summary
  const rawPayload = detail ? decodePendingPayload(detail.rawPayload) : ''
  const topics = pending.kind === OutputKind.OutputKindTopics ? topicsFromPayload(rawPayload) : []
  const canApply = pending.status === PendingStatus.PendingPending
    && pending.kind === OutputKind.OutputKindTopics
    && topics.length > 0

  return (
    <section className="console-pending-card" aria-label="待应用产物详情">
      <header className="console-pending-card-head">
        <div>
          <span className="workspace-kicker">{pending.kind === OutputKind.OutputKindTopics ? '选题候选' : pending.kind}</span>
          <strong>{pending.nodeId}</strong>
        </div>
        <span className={`console-pending-status${pending.status === PendingStatus.PendingInvalid ? ' is-invalid' : ''}`}>
          {statusLabel(pending.status)}
        </span>
      </header>
      {pending.summary ? <p className="console-pending-summary">{pending.summary}</p> : null}
      {pending.error ? <p className="console-pending-error" role="alert">{pending.error}</p> : null}

      {loading ? <p className="console-pending-message" role="status">正在读取待应用正文…</p> : null}
      {loadError ? (
        <div className="console-pending-error" role="alert">
          <span>正文读取失败：{loadError}</span>
          <button type="button" className="agent-mini-btn" onClick={onRetryDetail}>重试读取</button>
        </div>
      ) : null}
      {!loading && !loadError && !detail ? <p className="console-pending-message">选择待处理结果以读取正文。</p> : null}

      {detail && !loading && !loadError ? (
        <>
          {pending.kind === OutputKind.OutputKindTopics && pending.status === PendingStatus.PendingPending ? (
            topics.length ? (
              <ul className="console-pending-topics">
                {topics.map((topic, index) => (
                  <li key={`${index}-${topic.title}`}>
                    <strong>{index + 1}. {topic.title}</strong>
                    <span>{topic.angle}</span>
                    <span>{topic.hook}</span>
                  </li>
                ))}
              </ul>
            ) : <p className="console-pending-error" role="alert">候选正文无法解析，不能应用。</p>
          ) : (
            <details className="console-pending-raw">
              <summary>{pending.status === PendingStatus.PendingInvalid ? '查看原始输出' : '查看输出内容'}</summary>
              <pre>{rawPayload || '原始输出为空。'}</pre>
            </details>
          )}

          {forceConfirmation ? (
            <div className="console-pending-force" role="alertdialog" aria-labelledby="pending-force-title">
              <strong id="pending-force-title">应用依据已变化</strong>
              <p>参数或上游上下文已变化。确认后只忽略哈希差异，Schema、节点锁和运行锁仍由服务端校验。</p>
              <div className="console-pending-actions">
                <button type="button" className="agent-mini-btn" disabled={actionBusy} onClick={onCancelForce}>取消</button>
                <button type="button" className="agent-mini-btn allow" disabled={actionBusy} onClick={onConfirmForce}>
                  {actionBusy ? '提交中…' : '确认并强制应用'}
                </button>
              </div>
            </div>
          ) : null}

          {actionError ? <p className="console-pending-error" role="alert">{actionError}</p> : null}
          {resolved ? <p className="console-pending-message" role="status">操作已提交，正在同步工程状态；请勿重复操作。</p> : null}
          {syncError ? (
            <div className="console-pending-error" role="alert">
              <span>{syncError}</span>
              <button type="button" className="agent-mini-btn" disabled={actionBusy} onClick={onRetryWorkspace}>重试同步</button>
            </div>
          ) : null}

          {!forceConfirmation ? (
            <div className="console-pending-actions">
              {canApply ? (
                <button type="button" className="agent-mini-btn allow" disabled={actionBusy || resolved} onClick={onApply}>
                  {actionBusy ? '提交中…' : '应用'}
                </button>
              ) : null}
              {pending.status === PendingStatus.PendingPending || pending.status === PendingStatus.PendingInvalid ? (
                <button type="button" className="agent-mini-btn deny" disabled={actionBusy || resolved} onClick={onDiscard}>
                  {actionBusy ? '提交中…' : '丢弃'}
                </button>
              ) : null}
            </div>
          ) : null}
        </>
      ) : null}
    </section>
  )
}
