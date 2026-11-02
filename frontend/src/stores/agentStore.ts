import { create } from 'zustand'
import type { AgentEvent, AgentState, PermissionRequest, QuestionRequest, TranscriptItem } from '../api/agent'

type AgentStore = {
  state: AgentState | null
  lastSeq: number
  needsResync: boolean
  setSnapshot: (state: AgentState) => void
  applyEvent: (event: AgentEvent) => void
  markResynced: () => void
  clear: () => void
}

function upsertTranscript(items: TranscriptItem[], item: TranscriptItem): TranscriptItem[] {
  const index = items.findIndex((current) => current.id === item.id)
  if (index < 0) {
    return [...items, item].slice(-200)
  }
  const next = [...items]
  if (item.kind === 'message' && item.streaming && next[index].streaming) {
    next[index] = { ...next[index], text: next[index].text + item.text }
  } else {
    next[index] = { ...next[index], ...item }
  }
  return next
}

function upsertPermission(items: PermissionRequest[], item: PermissionRequest): PermissionRequest[] {
  return items.some((current) => current.id === item.id) ? items : [...items, item]
}

function upsertQuestion(items: QuestionRequest[], item: QuestionRequest): QuestionRequest[] {
  return items.some((current) => current.id === item.id) ? items : [...items, item]
}

export const useAgentStore = create<AgentStore>((set) => ({
  state: null,
  lastSeq: 0,
  needsResync: false,
  setSnapshot: (state) => set({ state, lastSeq: state.seq, needsResync: false }),
  applyEvent: (event) =>
    set((current) => {
      if (!current.state) {
        return { ...current, needsResync: true }
      }
      if (event.seq > 0 && event.seq <= current.lastSeq) {
        return current
      }
      if (event.seq > 0 && event.seq !== current.lastSeq + 1) {
        return { ...current, needsResync: true }
      }
      const state = current.state
      let next = state
      if (event.type === 'status' && event.state) {
        next = { ...event.state }
      } else if (event.type === 'message' && event.message) {
        next = { ...state, transcript: upsertTranscript(state.transcript ?? [], event.message) }
      } else if (event.type === 'tool' && event.tool) {
        next = { ...state, transcript: upsertTranscript(state.transcript ?? [], event.tool) }
      } else if (event.type === 'permission' && event.permission) {
        next = { ...state, permissions: upsertPermission(state.permissions ?? [], event.permission) }
      } else if (event.type === 'question' && event.question) {
        next = { ...state, questions: upsertQuestion(state.questions ?? [], event.question) }
      } else if (event.type === 'error' && event.error) {
        next = { ...state, notices: [...(state.notices ?? []), event.error.message] }
      }
      return {
        ...current,
        state: next,
        lastSeq: event.seq > 0 ? event.seq : current.lastSeq,
        needsResync: false,
      }
    }),
  markResynced: () => set({ needsResync: false }),
  clear: () => set({ state: null, lastSeq: 0, needsResync: false }),
}))
