import { useState } from 'react'
import type { AgentState } from '../api/agent'

type Props = {
  state: AgentState | null
  disabled?: boolean
  onSend: (text: string) => Promise<void>
}

export function PromptDock({ state, disabled = false, onSend }: Props) {
  const [text, setText] = useState('')
  const [sending, setSending] = useState(false)
  const busy = disabled || sending || !state || !state.cliAvailable || state.status === 'running' || state.status === 'resuming' || state.status === 'resume_failed'

  const submit = async () => {
    const value = text.trim()
    if (!value || busy) return
    setSending(true)
    try {
      await onSend(value)
      setText('')
    } catch {
      // App shows the error banner; keep the draft for retry.
    } finally {
      setSending(false)
    }
  }

  return (
    <div className="agent-prompt-dock">
      <div className="agent-prompt-inner">
        <div className="agent-prompt-label">
          <strong>M1 临时指令</strong>
          <span>M2 由提示词节点替代</span>
        </div>
        <div className="agent-prompt-box">
          <textarea
            aria-label="临时指令"
            rows={1}
            value={text}
            placeholder="发送一条指令…（Enter 发送，Shift+Enter 换行）"
            onChange={(event) => setText(event.target.value)}
            onKeyDown={(event) => {
              if (event.nativeEvent.isComposing) return
              if (event.key === 'Enter' && !event.shiftKey) {
                event.preventDefault()
                void submit()
              }
            }}
          />
          <button className="agent-send-btn" onClick={() => void submit()} disabled={busy || !text.trim()}>
            {sending ? '发送中' : '发送'}
          </button>
        </div>
      </div>
    </div>
  )
}
