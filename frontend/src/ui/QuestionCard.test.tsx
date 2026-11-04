import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { QuestionCard } from './QuestionCard'
import type { QuestionRequest } from '../api/runs'

const request: QuestionRequest = {
  id: 'q1',
  question: '选择哪种剪辑节奏？',
  options: ['174', '87'],
  allowOther: true,
  expiresAt: new Date(Date.now() + 60000).toISOString(),
}

describe('QuestionCard', () => {
  it('returns the selected option', () => {
    const onAnswer = vi.fn().mockResolvedValue(undefined)
    render(<QuestionCard request={request} onAnswer={onAnswer} />)

    expect(screen.getByText('选择哪种剪辑节奏？')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '174' }))
    expect(onAnswer).toHaveBeenCalledWith('q1', '174', 'option')
  })

  it('submits a typed answer as "other"', () => {
    const onAnswer = vi.fn().mockResolvedValue(undefined)
    render(<QuestionCard request={request} onAnswer={onAnswer} />)

    const input = screen.getByLabelText('其他回答')
    fireEvent.change(input, { target: { value: ' 慢一点 ' } })
    fireEvent.click(screen.getByRole('button', { name: '提交' }))
    expect(onAnswer).toHaveBeenCalledWith('q1', '慢一点', 'other')
  })

  it('submits with Enter but not while composing IME input', () => {
    const onAnswer = vi.fn().mockResolvedValue(undefined)
    render(<QuestionCard request={request} onAnswer={onAnswer} />)

    const input = screen.getByLabelText('其他回答')
    fireEvent.change(input, { target: { value: '自定义' } })
    fireEvent.keyDown(input, { key: 'Enter', isComposing: true })
    expect(onAnswer).not.toHaveBeenCalled()

    fireEvent.keyDown(input, { key: 'Enter' })
    expect(onAnswer).toHaveBeenCalledWith('q1', '自定义', 'other')
  })

  it('disables answers while a response is in flight', async () => {
    let release: () => void = () => {}
    const onAnswer = vi.fn().mockImplementation(() => new Promise<void>((resolve) => { release = resolve }))
    render(<QuestionCard request={request} onAnswer={onAnswer} />)

    fireEvent.click(screen.getByRole('button', { name: '174' }))
    expect(screen.getByRole('button', { name: '87' })).toBeDisabled()
    expect(screen.getByRole('button', { name: '提交' })).toBeDisabled()

    release()
    await Promise.resolve()
  })

  it('hides the free-text field when other answers are disabled', () => {
    render(<QuestionCard request={{ ...request, allowOther: false }} onAnswer={vi.fn()} />)
    expect(screen.queryByLabelText('其他回答')).not.toBeInTheDocument()
  })

  it('shows a countdown until the question expires', () => {
    render(<QuestionCard request={request} onAnswer={vi.fn()} />)
    expect(screen.getByText(/^\d{2}:\d{2}$/)).toBeInTheDocument()
  })

  it('locks the card once the question has expired', () => {
    const onAnswer = vi.fn()
    render(<QuestionCard request={{ ...request, expiresAt: new Date(Date.now() - 1000).toISOString() }} onAnswer={onAnswer} />)

    expect(screen.getByText('已超时')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '174' })).toBeDisabled()
    expect(screen.getByLabelText('其他回答')).toBeDisabled()

    fireEvent.click(screen.getByRole('button', { name: '174' }))
    expect(onAnswer).not.toHaveBeenCalled()
  })
})
