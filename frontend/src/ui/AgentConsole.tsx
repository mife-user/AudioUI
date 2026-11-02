import type { AgentState } from '../api/agent'
import { PermissionCard } from './PermissionCard'
import { QuestionCard } from './QuestionCard'

type Props = {
  state: AgentState | null
  collapsed: boolean
  onToggle: () => void
  onPermission: (id: string, decision: 'allow' | 'allow_session' | 'deny') => Promise<void>
  onQuestion: (id: string, answer: string, via: string) => Promise<void>
  onNewSession: () => Promise<void>
  onCancelResume: () => void
  onInterrupt: () => Promise<void>
}

function statusLabel(state: AgentState | null) {
  if (!state) return '连接中'
  if ((state.permissions?.length ?? 0) + (state.questions?.length ?? 0) > 0) return '等待处理'
  switch (state.status) {
    case 'running': return '工作中'
    case 'resuming': return '恢复中'
    case 'resume_failed': return '恢复失败'
    case 'unavailable': return '不可用'
    case 'error': return '需要处理'
    default: return '空闲'
  }
}

function tone(state: AgentState | null) {
  if (!state || !state.cliAvailable) return 'bad'
  if ((state.permissions?.length ?? 0) + (state.questions?.length ?? 0) > 0) return 'warn'
  if (state.status === 'error' || state.status === 'resume_failed' || state.status === 'unavailable') return 'bad'
  if (state.status === 'resuming') return 'warn'
  if (state.status === 'running') return 'ok'
  return 'ok'
}

export function AgentConsole({ state, collapsed, onToggle, onPermission, onQuestion, onNewSession, onCancelResume, onInterrupt }: Props) {
  const permissions = state?.permissions ?? []
  const questions = state?.questions ?? []
  const notices = state?.notices ?? []
  const pending = permissions.length + questions.length

  if (collapsed) {
    return (
      <aside className="agent-rail" aria-label="Agent 控制台收起态">
        <span className={`dot ${tone(state)}`} />
        <span className="agent-rail-count">{pending || ''}</span>
        <span className="agent-rail-name">{state?.chainId || 'chain_default'}</span>
        <span className="agent-rail-label">AGENT</span>
        <button className="agent-icon-btn" onClick={onToggle} aria-label="展开控制台">‹</button>
      </aside>
    )
  }

  return (
    <aside className="agent-console" aria-label="Agent 控制台">
      <div className="agent-console-head">
        <span className={`dot ${tone(state)}`} />
        <span className="agent-console-title">AGENT</span>
        <span className="agent-console-state">{statusLabel(state)}</span>
        <button className="agent-icon-btn" onClick={onToggle} aria-label="收起控制台">›</button>
      </div>
      <div className="agent-console-body">
        <div className="agent-chain-line">链路 <strong>{state?.chainId || 'chain_default'}</strong> · {state?.displayId || '未连接'}</div>
        {state?.status === 'running' ? (
          <button className="agent-mini-btn" onClick={() => void onInterrupt()}>停止当前回合</button>
        ) : null}
        {state?.status === 'resume_failed' ? (
          <section className="agent-card agent-resume-card">
            <div className="agent-card-head">恢复失败</div>
            <div className="agent-card-body">
              <div className="agent-question-text">{state.resumeError || '无法恢复上次会话。'}</div>
              <div className="agent-card-actions">
                <button className="agent-mini-btn allow" onClick={() => void onNewSession()}>新开会话</button>
                <button className="agent-mini-btn" onClick={onCancelResume}>取消</button>
              </div>
            </div>
          </section>
        ) : null}
        <div className="agent-section-label">待处理 <span>{pending}</span></div>
        {permissions.map((request) => (
          <PermissionCard key={request.id} request={request} onRespond={onPermission} />
        ))}
        {questions.map((request) => (
          <QuestionCard key={request.id} request={request} onAnswer={onQuestion} />
        ))}
        {pending === 0 ? <div className="agent-console-empty">没有待处理请求</div> : null}
        <div className="agent-section-label">通知</div>
        {notices.length === 0 ? (
          <div className="agent-console-empty">暂无通知</div>
        ) : (
          notices.slice(-6).map((notice, index) => (
            <div className="agent-notice" key={`${notice}-${index}`}><span className="dot warn" />{notice}</div>
          ))
        )}
        <div className="agent-console-foot">控制台不发送提示词；临时输入在底部。</div>
      </div>
    </aside>
  )
}
