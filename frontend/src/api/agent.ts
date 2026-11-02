import { Events } from '@wailsio/runtime'
import { AgentService } from '../../bindings/audio-ui/internal/handler'
import type {
  AgentState,
  PermissionRequest,
  QuestionRequest,
  TranscriptItem,
} from '../../bindings/audio-ui/internal/domain/valueobject'

export type { AgentState, PermissionRequest, QuestionRequest, TranscriptItem }

export type AgentEventType = 'status' | 'message' | 'tool' | 'permission' | 'question' | 'error'

export interface AgentEvent {
  v: number
  seq: number
  type: AgentEventType
  state?: AgentState
  message?: TranscriptItem
  tool?: TranscriptItem
  permission?: PermissionRequest
  question?: QuestionRequest
  error?: { code: string; message: string; recoverable: boolean }
}

export async function getAgentState(): Promise<AgentState> {
  return await AgentService.GetState()
}

export async function sendMessage(text: string): Promise<void> {
  await AgentService.SendMessage(text)
}

export async function respondPermission(id: string, decision: 'allow' | 'allow_session' | 'deny'): Promise<void> {
  await AgentService.RespondPermission(id, decision)
}

export async function respondQuestion(id: string, answer: string, via: string): Promise<void> {
  await AgentService.RespondQuestion(id, answer, via)
}

export async function newSession(): Promise<void> {
  await AgentService.NewSession()
}

export async function cancelResume(): Promise<void> {
  await AgentService.CancelResume()
}

export async function interrupt(): Promise<void> {
  await AgentService.Interrupt()
}

export function subscribeAgentEvents(handler: (event: AgentEvent) => void): () => void {
  const names: AgentEventType[] = ['status', 'message', 'tool', 'permission', 'question', 'error']
  const offs = names.map((name) =>
    Events.On(`agent:${name}`, (payload: unknown) => {
      const data = payload as { data?: AgentEvent } | AgentEvent
      handler('data' in data && data.data ? data.data : (data as AgentEvent))
    }),
  )
  return () => offs.forEach((off) => off())
}
