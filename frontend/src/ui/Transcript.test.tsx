import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { Transcript } from './Transcript'
import type { TranscriptItem } from '../api/runs'
import { TranscriptKind } from '../../bindings/audio-ui/internal/domain/valueobject'

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
    expect(screen.getByText('暂无运行内容')).toBeInTheDocument()
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

  it('keeps long tool summaries available without truncating their text', () => {
    const longPath = 'workspace/inputs/project/node_example/averyveryveryveryveryverylongunbrokenfilename_for_readability_demo.txt'
    render(<Transcript items={[item({ kind: TranscriptKind.TranscriptTool, toolSummary: longPath })]} />)
    expect(screen.getByText(longPath)).toBeInTheDocument()
  })

  it('follows new transcript content when already scrolled to the bottom', () => {
    const { container, rerender } = render(<Transcript items={[item({ text: '第一条' })]} />)
    const transcript = container.querySelector<HTMLElement>('.agent-console-transcript')
    if (!transcript) throw new Error('transcript container is missing')
    let scrollHeight = 600
    Object.defineProperty(transcript, 'scrollHeight', { configurable: true, get: () => scrollHeight })
    Object.defineProperty(transcript, 'clientHeight', { configurable: true, value: 200 })
    Object.defineProperty(transcript, 'scrollTop', { configurable: true, writable: true, value: 400 })
    fireEvent.scroll(transcript)

    scrollHeight = 900
    rerender(<Transcript items={[item({ text: '第一条更新' })]} />)
    expect(transcript.scrollTop).toBe(900)
  })

  it('preserves the current history position when new transcript content arrives', () => {
    const { container, rerender } = render(<Transcript items={[item({ text: '第一条' })]} />)
    const transcript = container.querySelector<HTMLElement>('.agent-console-transcript')
    if (!transcript) throw new Error('transcript container is missing')
    let scrollHeight = 600
    Object.defineProperty(transcript, 'scrollHeight', { configurable: true, get: () => scrollHeight })
    Object.defineProperty(transcript, 'clientHeight', { configurable: true, value: 200 })
    Object.defineProperty(transcript, 'scrollTop', { configurable: true, writable: true, value: 120 })
    fireEvent.scroll(transcript)

    scrollHeight = 900
    rerender(<Transcript items={[item({ text: '第一条更新' })]} />)
    expect(transcript.scrollTop).toBe(120)
  })
})
