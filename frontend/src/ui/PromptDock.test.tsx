import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { PromptDock } from './PromptDock'
import type { AgentState } from '../api/agent'
import { SessionState } from '../../bindings/audio-ui/internal/domain/valueobject'

const state: AgentState = {
  version: 1, seq: 1, chainId: 'chain_default', sessionId: 's', displayId: 's',
  status: SessionState.SessionIdle, adapter: 'claude', cliAvailable: true, cliError: '',
  permissions: [], questions: [], transcript: [], notices: [], resumeError: '',
}

describe('PromptDock', () => {
  it('sends on Enter and clears the input', async () => {
    const onSend = vi.fn().mockResolvedValue(undefined)
    render(<PromptDock state={state} onSend={onSend} />)
    const input = screen.getByLabelText('临时指令') as HTMLTextAreaElement
    fireEvent.change(input, { target: { value: 'hello' } })
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(onSend).toHaveBeenCalledWith('hello')
    await waitFor(() => expect(input.value).toBe(''))
  })

  it('disables while running', () => {
    render(<PromptDock state={{ ...state, status: SessionState.SessionRunning }} onSend={vi.fn()} />)
    expect(screen.getByRole('button', { name: '发送' })).toBeDisabled()
  })
})
