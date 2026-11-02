import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { Transcript } from './Transcript'
import type { TranscriptItem } from '../api/agent'
import { TranscriptKind } from '../../bindings/audio-ui/internal/domain/valueobject'

beforeEach(() => {
  Element.prototype.scrollIntoView = vi.fn()
})

function item(overrides: Partial<TranscriptItem> = {}): TranscriptItem {
  return {
    id: 'm1',
    kind: TranscriptKind.TranscriptMessage,
    role: 'assistant',
    text: '你好',
    streaming: false,
    createdAt: '',
    ...overrides,
  }
}

describe('Transcript', () => {
  it('shows the empty state', () => {
    render(<Transcript items={[]} />)
    expect(screen.getByText('还没有会话消息')).toBeInTheDocument()
  })

  it('renders user and assistant bubbles', () => {
    render(<Transcript items={[item({ id: 'm1', role: 'user', text: '写个脚本' }), item({ id: 'm2' })]} />)
    expect(screen.getByText('写个脚本')).toBeInTheDocument()
    expect(screen.getByText('你好')).toBeInTheDocument()
    expect(screen.getByText('YOU')).toBeInTheDocument()
    expect(screen.getByText('AI')).toBeInTheDocument()
  })

  it('marks streaming text with a cursor', () => {
    const { container } = render(<Transcript items={[item({ streaming: true })]} />)
    expect(container.querySelector('.agent-cursor')).not.toBeNull()
  })

  it('renders tool rows with localized status', () => {
    const items = [
      item({ id: 't1', kind: TranscriptKind.TranscriptTool, toolName: 'Write', toolStatus: 'running', toolSummary: 'Write notes.md' }),
      item({ id: 't2', kind: TranscriptKind.TranscriptTool, toolName: 'Bash', toolStatus: 'error', toolSummary: 'exit 1' }),
      item({ id: 't3', kind: TranscriptKind.TranscriptTool, toolName: 'Edit', toolStatus: 'denied', toolSummary: 'blocked' }),
    ]
    render(<Transcript items={items} />)

    expect(screen.getByText('Write notes.md')).toBeInTheDocument()
    expect(screen.getByText('进行中')).toBeInTheDocument()
    expect(screen.getByText('失败')).toBeInTheDocument()
    expect(screen.getByText('已拒绝')).toBeInTheDocument()
  })

  it('falls back to a generic tool label', () => {
    render(<Transcript items={[item({ kind: TranscriptKind.TranscriptTool, toolName: '', toolSummary: '', text: 'raw output' })]} />)
    expect(screen.getByText('工具')).toBeInTheDocument()
    expect(screen.getByText('raw output')).toBeInTheDocument()
  })
})
