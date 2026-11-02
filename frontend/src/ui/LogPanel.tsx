import type { LogEntryDTO } from '../api/system'
import { copy } from './copy'

type Props = {
  entries: LogEntryDTO[]
  loading: boolean
  error: string
  onRefresh: () => void
}

function formatClock(value: string) {
  if (!value) return '--:--:--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleTimeString('zh-CN', { hour12: false })
}

export function LogPanel({ entries, loading, error, onRefresh }: Props) {
  return (
    <section className="card" aria-labelledby="log-title">
      <div className="card-head">
        <span className="card-title" id="log-title">{copy.logTitle}</span>
        <span className="card-sub">{copy.logSubtitle}</span>
        <div className="card-actions">
          <button className="version-badge" onClick={onRefresh} disabled={loading}>
            {copy.logRefresh}
          </button>
        </div>
      </div>
      {error ? (
        <div className="log-empty" role="alert">{error}</div>
      ) : entries.length === 0 ? (
        <div className="log-empty">{loading ? '加载中…' : copy.logEmpty}</div>
      ) : (
        <ul className="log-list">
          {entries.map((entry) => (
            <li key={`${entry.time}-${entry.level}-${entry.message}`}>
              <span className="log-time">{formatClock(entry.time)}</span>
              <span className={`log-level ${entry.level || 'info'}`}>{(entry.level || 'info').toUpperCase()}</span>
              <span className="log-msg">{entry.message}</span>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
