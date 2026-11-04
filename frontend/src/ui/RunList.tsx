import type { WorkspaceRunSummary } from '../../bindings/audio-ui/internal/domain/entity'
import { RunStatus } from '../../bindings/audio-ui/internal/domain/valueobject'

type Props = {
  runs: WorkspaceRunSummary[]
  selectedRunId: string | null
  onSelect: (runId: string) => void
}

function statusLabel(status: string): string {
  switch (status) {
    case RunStatus.RunStarting: return '启动中'
    case RunStatus.RunRunning: return '运行中'
    case RunStatus.RunSucceeded: return '完成'
    case RunStatus.RunFailed: return '失败'
    case RunStatus.RunInterrupted: return '已中断'
    case RunStatus.RunImported: return '导入历史'
    default: return status || '未知状态'
  }
}

export function RunList({ runs, selectedRunId, onSelect }: Props) {
  if (runs.length === 0) {
    return <div className="run-list-empty">暂无运行记录；节点执行入口将在后续任务接入。</div>
  }

  return (
    <section className="run-list" aria-label="运行历史">
      <div className="run-list-heading">运行历史 <span>{runs.length}</span></div>
      <ul>
        {runs.map((run) => (
          <li key={run.id}>
            <button
              className={`run-list-item${selectedRunId === run.id ? ' selected' : ''}`}
              type="button"
              aria-current={selectedRunId === run.id ? 'true' : undefined}
              onClick={() => onSelect(run.id)}
            >
              <span className={`run-status-mark ${run.status === RunStatus.RunRunning || run.status === RunStatus.RunStarting ? 'active' : ''}`} aria-hidden="true" />
              <span className="run-list-item-main">
                <span className="run-list-item-title">{run.nodeId} · {statusLabel(run.status)}</span>
                <span className="run-list-item-meta">{run.startedAt ? new Date(run.startedAt).toLocaleString() : '开始时间未知'} · {run.chainId}</span>
              </span>
            </button>
          </li>
        ))}
      </ul>
    </section>
  )
}
