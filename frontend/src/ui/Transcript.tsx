import { useEffect, useRef } from 'react'
import type { TranscriptItem } from '../api/agent'

type Props = {
  items: TranscriptItem[]
}

function toolStatusLabel(status = '') {
  switch (status) {
    case 'running':
      return '进行中'
    case 'ok':
      return '完成'
    case 'error':
      return '失败'
    case 'denied':
      return '已拒绝'
    default:
      return status
  }
}

export function Transcript({ items }: Props) {
  const endRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    endRef.current?.scrollIntoView({ block: 'end' })
  }, [items.length])

  if (items.length === 0) {
    return (
      <div className="agent-empty">
        <div className="agent-empty-title">还没有会话消息</div>
        <div className="agent-empty-sub">在底部临时指令栏输入一条消息，开始 M1 会话。</div>
      </div>
    )
  }

  return (
    <div className="agent-transcript-list">
      {items.map((item) =>
        item.kind === 'tool' ? (
          <div className="agent-tool-row" key={item.id}>
            <span className="agent-tool-name">{item.toolName || '工具'}</span>
            <span className="agent-tool-summary">{item.toolSummary || item.text}</span>
            <span className={`agent-tool-status ${item.toolStatus || ''}`}>{toolStatusLabel(item.toolStatus)}</span>
          </div>
        ) : (
          <article className={`agent-message ${item.role === 'user' ? 'user' : 'assistant'}`} key={item.id}>
            <div className="agent-avatar">{item.role === 'user' ? 'YOU' : 'AI'}</div>
            <div className="agent-bubble">
              {item.text}
              {item.streaming ? <span className="agent-cursor" aria-hidden="true" /> : null}
            </div>
          </article>
        ),
      )}
      <div ref={endRef} />
    </div>
  )
}
