import { useEffect, useState } from 'react'
import type { PermissionRequest } from '../api/agent'

type Props = {
  request: PermissionRequest
  onRespond: (id: string, decision: 'allow' | 'allow_session' | 'deny') => Promise<void>
}

function remaining(expiresAt: string) {
  const end = new Date(expiresAt).getTime()
  if (Number.isNaN(end)) return 0
  return Math.max(0, Math.ceil((end - Date.now()) / 1000))
}

function format(seconds: number) {
  if (seconds <= 0) return '已超时'
  const m = String(Math.floor(seconds / 60)).padStart(2, '0')
  const s = String(seconds % 60).padStart(2, '0')
  return `${m}:${s}`
}

export function PermissionCard({ request, onRespond }: Props) {
  const [seconds, setSeconds] = useState(() => remaining(request.expiresAt))
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    setSeconds(remaining(request.expiresAt))
    const timer = window.setInterval(() => setSeconds(remaining(request.expiresAt)), 1000)
    return () => window.clearInterval(timer)
  }, [request.expiresAt])

  const respond = async (decision: 'allow' | 'allow_session' | 'deny') => {
    setBusy(true)
    try {
      await onRespond(request.id, decision)
    } finally {
      setBusy(false)
    }
  }

  return (
    <section className="agent-card agent-permission-card" aria-label="权限确认">
      <div className="agent-card-head">
        <span>需要确认</span>
        <span className="agent-countdown">{format(seconds)}</span>
      </div>
      <div className="agent-card-body">
        <div className="agent-mono">{request.summary || request.tool}</div>
        {request.paths && request.paths.length > 0 ? (
          <div className="agent-path-list">
            {request.paths.map((path) => <span key={path}>{path}</span>)}
          </div>
        ) : null}
        {request.command ? <div className="agent-mono agent-command">{request.command}</div> : null}
        <div className="agent-risk">风险：{request.risk || '工具调用'}</div>
        <div className="agent-card-actions">
          <button className="agent-mini-btn allow" disabled={busy || seconds <= 0} onClick={() => void respond('allow')}>允许一次</button>
          {request.allowSession ? (
            <button className="agent-mini-btn" disabled={busy || seconds <= 0} onClick={() => void respond('allow_session')}>本会话允许</button>
          ) : null}
          <button className="agent-mini-btn deny" disabled={busy} onClick={() => void respond('deny')}>拒绝</button>
        </div>
      </div>
    </section>
  )
}
