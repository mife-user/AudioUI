import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AgentConsole } from './AgentConsole'
import type { AgentState } from '../api/agent'
import { SessionState } from '../../bindings/audio-ui/internal/domain/valueobject'

const state: AgentState = {
  version: 1, seq: 1, chainId: 'chain_default', sessionId: 's', displayId: 's',
  status: SessionState.SessionIdle, adapter: 'claude', cliAvailable: true, cliError: '',
  permissions: [], questions: [], transcript: [], notices: [], resumeError: '',
}

describe('AgentConsole', () => {
  it('does not provide a prompt input', () => {
    render(
      <AgentConsole
        state={state}
        collapsed={false}
        onToggle={() => {}}
        onPermission={vi.fn()}
        onQuestion={vi.fn()}
        onNewSession={vi.fn()}
        onCancelResume={() => {}}
        onInterrupt={vi.fn()}
      />,
    )
    expect(screen.queryByRole('textbox')).toBeNull()
    expect(screen.getByText('AGENT')).toBeInTheDocument()
  })

  it('shows the collapsed rail', () => {
    render(
      <AgentConsole
        state={state}
        collapsed
        onToggle={() => {}}
        onPermission={vi.fn()}
        onQuestion={vi.fn()}
        onNewSession={vi.fn()}
        onCancelResume={() => {}}
        onInterrupt={vi.fn()}
      />,
    )
    expect(screen.getByText('chain_default')).toBeInTheDocument()
  })
})
