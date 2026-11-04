import { useEffect, useState } from 'react'
import type { QuestionRequest } from '../api/runs'

type Props = {
  request: QuestionRequest
  onAnswer: (id: string, answer: string, via: string) => Promise<void>
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

export function QuestionCard({ request, onAnswer }: Props) {
  const [other, setOther] = useState('')
  const [busy, setBusy] = useState(false)
  const [seconds, setSeconds] = useState(() => remaining(request.expiresAt))

  useEffect(() => {
    setSeconds(remaining(request.expiresAt))
    const timer = window.setInterval(() => setSeconds(remaining(request.expiresAt)), 1000)
    return () => window.clearInterval(timer)
  }, [request.expiresAt])

  const expired = seconds <= 0

  const answer = async (value: string, via: string) => {
    if (!value.trim() || busy || expired) return
    setBusy(true)
    try {
      await onAnswer(request.id, value.trim(), via)
    } finally {
      setBusy(false)
    }
  }

  return (
    <section className="agent-card agent-question-card" aria-label="需要选择">
      <div className="agent-card-head">
        <span>需要选择</span>
        <span className="agent-countdown">{format(seconds)}</span>
      </div>
      <div className="agent-card-body">
        <div className="agent-question-text">{request.question}</div>
        <div className="agent-option-list">
          {request.options?.map((option) => (
            <button className="agent-option" key={option} disabled={busy || expired} onClick={() => void answer(option, 'option')}>
              {option}
            </button>
          ))}
        </div>
        {request.allowOther ? (
          <div className="agent-other-row">
            <input
              aria-label="其他回答"
              value={other}
              placeholder="其他…"
              disabled={expired}
              onChange={(event) => setOther(event.target.value)}
              onKeyDown={(event) => {
                if (event.nativeEvent.isComposing) return
                if (event.key === 'Enter') void answer(other, 'other')
              }}
            />
            <button className="agent-mini-btn allow" disabled={busy || expired || !other.trim()} onClick={() => void answer(other, 'other')}>
              提交
            </button>
          </div>
        ) : null}
      </div>
    </section>
  )
}
