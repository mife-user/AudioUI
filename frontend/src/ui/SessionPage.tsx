import { useState } from 'react'
import type { AgentState } from '../api/agent'
import { AgentConsole } from './AgentConsole'
import { PromptDock } from './PromptDock'
import { Transcript } from './Transcript'
import { copy } from './copy'

type Props = {
  state: AgentState | null
  error: string
  onSend: (text: string) => Promise<void>
  onPermission: (id: string, decision: 'allow' | 'allow_session' | 'deny') => Promise<void>
  onQuestion: (id: string, answer: string, via: string) => Promise<void>
  onNewSession: () => Promise<void>
  onCancelResume: () => void
  onInterrupt: () => Promise<void>
}

export function SessionPage({ state, error, onSend, onPermission, onQuestion, onNewSession, onCancelResume, onInterrupt }: Props) {
  const [collapsed, setCollapsed] = useState(false)
  const items = state?.transcript ?? []

  return (
    <main className={`agent-layout${collapsed ? ' agent-railed' : ''}`}>
      <section className="agent-workspace" aria-label="会话">
        {!state?.cliAvailable ? (
          <div className="agent-banner" role="alert">
            <strong>Agent 不可用</strong>
            <span>{state?.cliError || '未找到原生 Claude CLI。'}</span>
            <code>irm https://claude.ai/install.ps1 | iex</code>
          </div>
        ) : null}
        {error ? <div className="agent-banner" role="alert">{error}</div> : null}
        <div className="agent-transcript">
          <div className="agent-session-head">
            <span className="agent-session-title">M1 会话</span>
            <span className="agent-session-meta">{state?.chainId || 'chain_default'} · {state?.displayId || '未连接'}</span>
            <span className="agent-session-state">{copy.m1Note}</span>
          </div>
          <Transcript items={items} />
        </div>
        <PromptDock state={state} onSend={onSend} />
      </section>
      <AgentConsole
        state={state}
        collapsed={collapsed}
        onToggle={() => setCollapsed((value) => !value)}
        onPermission={onPermission}
        onQuestion={onQuestion}
        onNewSession={onNewSession}
        onCancelResume={onCancelResume}
        onInterrupt={onInterrupt}
      />
    </main>
  )
}
