import { create } from 'zustand'
import { RunStatus } from '../../bindings/audio-ui/internal/domain/valueobject'
import type { RunSnapshot, WorkspaceRunSummary } from '../../bindings/audio-ui/internal/domain/entity'
import type { PermissionRequest, QuestionRequest, RunEvent, TranscriptItem } from '../api/runs'

const MAX_BUFFERED_EVENTS = 1000

export type RunView = {
  summary: WorkspaceRunSummary | null
  snapshot: RunSnapshot | null
  lastSeq: number
  bufferedEvents: RunEvent[]
  loading: boolean
  needsSnapshot: boolean
  error: string
}

type RunStoreState = {
  projectId: string | null
  summaries: WorkspaceRunSummary[]
  runs: Record<string, RunView>
  selectedRunId: string | null
  setWorkspaceSnapshot: (projectId: string | null, summaries: WorkspaceRunSummary[]) => void
  selectRun: (runId: string) => void
  receiveEvent: (event: RunEvent) => void
  beginSnapshot: (runId: string) => void
  applySnapshot: (snapshot: RunSnapshot) => boolean
  failSnapshot: (runId: string, error: string) => void
  clear: () => void
}

type RunRecord = RunSnapshot['run']

function isActive(status: RunStatus | string | undefined): boolean {
  return status === RunStatus.RunStarting || status === RunStatus.RunRunning
}

function emptyView(summary: WorkspaceRunSummary | null = null): RunView {
  return {
    summary,
    snapshot: null,
    lastSeq: 0,
    bufferedEvents: [],
    loading: false,
    needsSnapshot: false,
    error: '',
  }
}

function upsertTranscript(items: TranscriptItem[], item: TranscriptItem): TranscriptItem[] {
  const index = items.findIndex((current) => current.id === item.id)
  if (index < 0) return [...items, item].slice(-500)

  const next = [...items]
  if (item.kind === 'message' && item.streaming && next[index].streaming) {
    next[index] = { ...next[index], text: next[index].text + item.text }
  } else {
    next[index] = { ...next[index], ...item }
  }
  return next
}

function uniqueByID<T extends { id: string }>(items: T[], item: T): T[] {
  return items.some((current) => current.id === item.id) ? items : [...items, item]
}

function applyEvent(snapshot: RunSnapshot, event: RunEvent): RunSnapshot {
  const run: RunRecord = { ...snapshot.run }
  let permissions: PermissionRequest[] = [...(snapshot.permissions ?? [])]
  let questions: QuestionRequest[] = [...(snapshot.questions ?? [])]

  if (event.type === 'status' && event.status) run.status = event.status
  if (event.type === 'message' && event.message) {
    run.transcript = upsertTranscript(run.transcript ?? [], event.message)
  }
  if (event.type === 'tool' && event.tool) {
    run.transcript = upsertTranscript(run.transcript ?? [], event.tool)
  }
  if (event.type === 'permission' && event.permission) {
    permissions = uniqueByID(permissions, event.permission)
  }
  if (event.type === 'question' && event.question) {
    questions = uniqueByID(questions, event.question)
  }
  if (event.type === 'request_resolved' && event.resolvedId) {
    if (event.resolvedKind === 'permission') {
      permissions = permissions.filter((request) => request.id !== event.resolvedId)
    } else if (event.resolvedKind === 'question') {
      questions = questions.filter((request) => request.id !== event.resolvedId)
    }
  }
  if (event.type === 'pending' && event.pendingId) run.pendingId = event.pendingId
  if (event.type === 'error' && event.error) {
    run.error = `${event.error.code}: ${event.error.message}`
  }
  if (event.actualInputTokens !== undefined) {
    run.actualInputTokens = event.actualInputTokens
    run.usageReported = true
  }

  return { ...snapshot, run, permissions, questions }
}

function bufferEvent(events: RunEvent[], event: RunEvent): RunEvent[] {
  if (events.some((current) => current.seq === event.seq)) return events
  return [...events, event].sort((a, b) => a.seq - b.seq).slice(-MAX_BUFFERED_EVENTS)
}

function replayEvents(snapshot: RunSnapshot, lastSeq: number, events: RunEvent[]) {
  let nextSnapshot = snapshot
  let nextSeq = lastSeq
  let gap = false
  const remaining: RunEvent[] = []

  for (const event of events) {
    if (event.seq <= nextSeq) continue
    if (gap || event.seq !== nextSeq + 1) {
      gap = true
      remaining.push(event)
      continue
    }
    nextSnapshot = applyEvent(nextSnapshot, event)
    nextSeq = event.seq
  }

  return { snapshot: nextSnapshot, lastSeq: nextSeq, bufferedEvents: remaining, gap }
}

export const useRunStore = create<RunStoreState>((set) => ({
  projectId: null,
  summaries: [],
  runs: {},
  selectedRunId: null,
  setWorkspaceSnapshot: (projectId, summaries) => set((current) => {
    if (!projectId) {
      return { projectId: null, summaries: [], runs: {}, selectedRunId: null }
    }

    const changedProject = current.projectId !== projectId
    const previousRuns = changedProject ? {} : current.runs
    const nextRuns: Record<string, RunView> = { ...previousRuns }
    for (const summary of summaries) {
      const existing = nextRuns[summary.id] ?? emptyView()
      nextRuns[summary.id] = {
        ...existing,
        summary,
        needsSnapshot: existing.needsSnapshot || (existing.snapshot === null && (isActive(summary.status) || summary.id === current.selectedRunId)),
      }
    }

    const activeRun = summaries.find((summary) => isActive(summary.status))
    const stillSelected = summaries.some((summary) => summary.id === current.selectedRunId)
    const selectedRunId = activeRun?.id ?? (stillSelected ? current.selectedRunId : summaries[0]?.id ?? null)
    if (selectedRunId) {
      const selected = nextRuns[selectedRunId] ?? emptyView(summaries.find((item) => item.id === selectedRunId) ?? null)
      nextRuns[selectedRunId] = {
        ...selected,
        needsSnapshot: selected.snapshot === null,
      }
    }

    return { projectId, summaries, runs: nextRuns, selectedRunId }
  }),
  selectRun: (runId) => set((current) => {
    const view = current.runs[runId] ?? emptyView(current.summaries.find((item) => item.id === runId) ?? null)
    return {
      selectedRunId: runId,
      runs: {
        ...current.runs,
        [runId]: { ...view, needsSnapshot: view.snapshot === null && !view.loading },
      },
    }
  }),
  receiveEvent: (event) => set((current) => {
    if (!current.projectId || event.projectId !== current.projectId || event.seq <= 0) return current
    const view = current.runs[event.runId] ?? emptyView()
    if (view.snapshot && event.seq <= view.lastSeq) return current
    const bufferedEvents = bufferEvent(view.bufferedEvents, event)
    if (!view.snapshot || view.loading || view.needsSnapshot) {
      return {
        runs: {
          ...current.runs,
          [event.runId]: {
            ...view,
            bufferedEvents,
            needsSnapshot: view.loading ? view.needsSnapshot : !view.snapshot || view.needsSnapshot,
          },
        },
      }
    }

    const replayed = replayEvents(view.snapshot, view.lastSeq, bufferedEvents)
    return {
      runs: {
        ...current.runs,
        [event.runId]: {
          ...view,
          snapshot: replayed.snapshot,
          lastSeq: replayed.lastSeq,
          bufferedEvents: replayed.bufferedEvents,
          needsSnapshot: replayed.gap,
          error: replayed.gap ? view.error : '',
        },
      },
    }
  }),
  beginSnapshot: (runId) => set((current) => {
    const view = current.runs[runId] ?? emptyView(current.summaries.find((item) => item.id === runId) ?? null)
    return {
      runs: {
        ...current.runs,
        [runId]: { ...view, loading: true, needsSnapshot: false, error: '' },
      },
    }
  }),
  applySnapshot: (snapshot) => {
    let accepted = false
    set((current) => {
      if (snapshot.projectId !== current.projectId || snapshot.runId !== snapshot.run.id) return current
      const existing = current.runs[snapshot.runId] ?? emptyView()
      const bufferedEvents = [...existing.bufferedEvents]
        .sort((a, b) => a.seq - b.seq)
        .filter((event) => event.projectId === snapshot.projectId && event.runId === snapshot.runId)
      const replayed = replayEvents(snapshot, snapshot.lastSeq, bufferedEvents)

      accepted = true
      return {
        runs: {
          ...current.runs,
          [snapshot.runId]: {
            ...existing,
            summary: existing.summary,
            snapshot: replayed.snapshot,
            lastSeq: replayed.lastSeq,
            bufferedEvents: replayed.bufferedEvents,
            loading: false,
            needsSnapshot: replayed.gap,
            error: '',
          },
        },
      }
    })
    return accepted
  },
  failSnapshot: (runId, error) => set((current) => {
    const view = current.runs[runId] ?? emptyView()
    return {
      runs: {
        ...current.runs,
        [runId]: { ...view, loading: false, needsSnapshot: false, error },
      },
    }
  }),
  clear: () => set({ projectId: null, summaries: [], runs: {}, selectedRunId: null }),
}))
