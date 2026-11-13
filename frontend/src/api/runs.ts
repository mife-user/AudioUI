import { Events } from '@wailsio/runtime'
import { RunService } from '../../bindings/audio-ui/internal/handler'
import type { RunStartOptions, RunStartResult } from '../../bindings/audio-ui/internal/domain/entity'
import type { RunSnapshot, RunSummary, WorkspaceRunSummary } from '../../bindings/audio-ui/internal/domain/entity'
import type { PermissionRequest, QuestionRequest, RunStatus, TranscriptItem } from '../../bindings/audio-ui/internal/domain/valueobject'

export type RunEventError = { code: string; message: string; recoverable: boolean }

export type RunEventType =
  | 'status'
  | 'message'
  | 'tool'
  | 'permission'
  | 'question'
  | 'error'
  | 'pending'
  | 'request_resolved'

export type RunEvent = {
  v: number
  seq: number
  runId: string
  projectId: string
  treeId: string
  chainId: string
  nodeId: string
  chainPath: string[]
  type: RunEventType
  status?: RunStatus
  message?: TranscriptItem
  tool?: TranscriptItem
  permission?: PermissionRequest
  question?: QuestionRequest
  resolvedKind?: 'permission' | 'question'
  resolvedId?: string
  pendingId?: string
  error?: RunEventError
  actualInputTokens?: number
}

export type { PermissionRequest, QuestionRequest, RunSnapshot, RunStartOptions, RunStartResult, RunStatus, RunSummary, TranscriptItem, WorkspaceRunSummary }
export type PermissionDecision = 'allow' | 'allow_session' | 'deny'

function eventData<T>(payload: unknown): T {
  if (payload && typeof payload === 'object' && 'data' in payload) {
    return (payload as { data: T }).data
  }
  return payload as T
}

const runEventNames: RunEventType[] = [
  'status', 'message', 'tool', 'permission', 'question', 'error', 'pending', 'request_resolved',
]

export async function startRun(nodeId: string, projectId: string, options: RunStartOptions = {}): Promise<RunStartResult> {
  return await RunService.Start(nodeId, projectId, options)
}

export async function getRunSnapshot(runId: string): Promise<RunSnapshot> {
  return await RunService.GetRunSnapshot(runId)
}

export async function interruptRun(runId: string): Promise<void> {
  await RunService.Interrupt(runId)
}

export async function respondRunPermission(runId: string, requestId: string, decision: PermissionDecision): Promise<void> {
  await RunService.RespondPermission(runId, requestId, decision)
}

export async function respondRunQuestion(runId: string, requestId: string, answer: string, via: string): Promise<void> {
  await RunService.RespondQuestion(runId, requestId, answer, via)
}

export function subscribeRunEvents(handler: (event: RunEvent) => void): () => void {
  const unsubscribers = runEventNames.map((name) =>
    Events.On(`run:${name}`, (payload: unknown) => handler(eventData<RunEvent>(payload))),
  )
  return () => unsubscribers.forEach((unsubscribe) => unsubscribe())
}
