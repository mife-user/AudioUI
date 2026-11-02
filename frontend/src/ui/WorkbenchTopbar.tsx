import type { AgentState } from '../api/agent'
import { copy } from './copy'

type Props = {
  state: AgentState | null
  version: string
  onAbout: () => void
  onSystem: () => void
}

function statusOf(state: AgentState | null) {
  if (!state) return { tone: 'warn', label: '连接中' }
  if (!state.cliAvailable) return { tone: 'bad', label: 'Agent 不可用' }
  if ((state.permissions?.length ?? 0) + (state.questions?.length ?? 0) > 0) {
    return { tone: 'warn', label: '待处理' }
  }
  switch (state.status) {
    case 'running':
      return { tone: 'ok', label: '工作中' }
    case 'resuming':
      return { tone: 'warn', label: '恢复中' }
    case 'resume_failed':
      return { tone: 'bad', label: '恢复失败' }
    case 'error':
      return { tone: 'bad', label: '需要处理' }
    case 'unavailable':
      return { tone: 'bad', label: 'Agent 不可用' }
    default:
      return { tone: 'ok', label: '空闲' }
  }
}

export function WorkbenchTopbar({ state, version, onAbout, onSystem }: Props) {
  const status = statusOf(state)
  const toneClass = status.tone === 'ok' ? '' : ` ${status.tone === 'warn' ? 'warn' : 'bad'}`
  return (
    <header className="topbar">
      <div className="brand">
        <span className="brand-mark" aria-hidden="true">AU</span>
        <span>{copy.brand}</span>
        <span className="brand-sub">{copy.brandSub}</span>
      </div>
      <div className="topbar-spacer" />
      <div className="env-state">
        <span className={`dot${toneClass}`} />
        <span>{status.label}</span>
      </div>
      <button className="version-badge" onClick={onSystem}>系统</button>
      <button className="version-badge" onClick={onAbout} aria-haspopup="dialog">
        v{version || 'dev'}
      </button>
    </header>
  )
}
