import { beforeEach, describe, expect, it } from 'vitest'
import { useAgentStore } from './agentStore'
import type { AgentEvent, AgentState } from '../api/agent'
import { PermissionTier, SessionState, TranscriptKind } from '../../bindings/audio-ui/internal/domain/valueobject'

const baseState: AgentState = {
  version: 1,
  seq: 1,
  chainId: 'chain_default',
  sessionId: 'sess',
  displayId: 'sess',
  status: SessionState.SessionIdle,
  adapter: 'claude',
  cliAvailable: true,
  cliError: '',
  permissions: [],
  questions: [],
  transcript: [],
  notices: [],
  resumeError: '',
}

function event(seq: number, value: Partial<AgentEvent>): AgentEvent {
  return { v: 1, seq, ...value } as AgentEvent
}

describe('agentStore', () => {
  beforeEach(() => {
    useAgentStore.getState().clear()
    useAgentStore.getState().setSnapshot(baseState)
  })

  it('appends streamed text and replaces with final message', () => {
    useAgentStore.getState().applyEvent(event(2, { type: 'message', message: { id: 'm1', kind: TranscriptKind.TranscriptMessage, role: 'assistant', text: 'Hel', streaming: true, createdAt: '' } }))
    useAgentStore.getState().applyEvent(event(3, { type: 'message', message: { id: 'm1', kind: TranscriptKind.TranscriptMessage, role: 'assistant', text: 'lo', streaming: true, createdAt: '' } }))
    expect(useAgentStore.getState().state?.transcript?.[0].text).toBe('Hello')
    useAgentStore.getState().applyEvent(event(4, { type: 'message', message: { id: 'm1', kind: TranscriptKind.TranscriptMessage, role: 'assistant', text: 'Hello.', streaming: false, createdAt: '' } }))
    expect(useAgentStore.getState().state?.transcript?.[0].text).toBe('Hello.')
  })

  it('marks a gap for resync', () => {
    useAgentStore.getState().applyEvent(event(5, { type: 'message', message: { id: 'm1', kind: TranscriptKind.TranscriptMessage, role: 'assistant', text: 'x', streaming: false, createdAt: '' } }))
    expect(useAgentStore.getState().needsResync).toBe(true)
  })

  it('accepts events after a snapshot with a non-zero seq', () => {
    useAgentStore.getState().setSnapshot({ ...baseState, seq: 5 })
    useAgentStore.getState().applyEvent(event(6, { type: 'status', state: { ...baseState, seq: 6, status: SessionState.SessionRunning } }))
    expect(useAgentStore.getState().state?.status).toBe(SessionState.SessionRunning)
    expect(useAgentStore.getState().needsResync).toBe(false)
    expect(useAgentStore.getState().lastSeq).toBe(6)
  })

  it('adds permission and question cards', () => {
    useAgentStore.getState().applyEvent(event(2, { type: 'permission', permission: { id: 'p1', tool: 'Write', tier: PermissionTier.PermissionConfirm, risk: 'write', summary: 'Write', allowSession: true, expiresAt: '' } }))
    useAgentStore.getState().applyEvent(event(3, { type: 'question', question: { id: 'q1', question: 'Pick', options: ['A'], allowOther: true, expiresAt: '' } }))
    expect(useAgentStore.getState().state?.permissions).toHaveLength(1)
    expect(useAgentStore.getState().state?.questions).toHaveLength(1)
  })
})
