import { useEffect, useRef } from 'react'
import type { EnvironmentDTO, LogEntryDTO } from '../api/system'
import { EnvironmentPanel } from './EnvironmentPanel'
import { LogPanel } from './LogPanel'

type Props = {
  open: boolean
  env: EnvironmentDTO | null
  logs: LogEntryDTO[]
  refreshing: boolean
  logsLoading: boolean
  error: string
  logError: string
  onRefreshEnv: () => void
  onRefreshLogs: () => void
  onClose: () => void
}

export function SystemDrawer({ open, env, logs, refreshing, logsLoading, error, logError, onRefreshEnv, onRefreshLogs, onClose }: Props) {
  const ref = useRef<HTMLDivElement>(null)
  const closeRef = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    if (!open) return
    closeRef.current?.focus()
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [open, onClose])

  if (!open) return null
  return (
    <div className="agent-drawer-mask" onClick={(event) => { if (event.target === event.currentTarget) onClose() }}>
      <aside className="agent-drawer" ref={ref} aria-label="系统信息">
        <div className="agent-drawer-head">
          <span className="agent-drawer-title">系统</span>
          <button ref={closeRef} className="agent-icon-btn" aria-label="关闭系统抽屉" onClick={onClose}>×</button>
        </div>
        <div className="agent-drawer-body">
          {error ? <div className="agent-banner" role="alert">{error}</div> : null}
          {env ? (
            <EnvironmentPanel env={env} refreshing={refreshing} onRefresh={onRefreshEnv} />
          ) : (
            <div className="agent-console-empty">正在读取环境…</div>
          )}
          <LogPanel entries={logs} loading={logsLoading} error={logError} onRefresh={onRefreshLogs} />
        </div>
      </aside>
    </div>
  )
}
