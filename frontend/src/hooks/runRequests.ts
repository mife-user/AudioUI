import { createContext, useContext } from 'react'
import { RunStatus } from '../../bindings/audio-ui/internal/domain/valueobject'
import type { PermissionDecision, PermissionRequest, QuestionRequest } from '../api/runs'
import type { RunView } from '../stores/runStore'

export type RunRequestGroup = {
  projectId: string
  runId: string
  nodeId: string
  chainId: string
  permissions: PermissionRequest[]
  questions: QuestionRequest[]
}

export type NodeLocator = (nodeId: string) => boolean

export type RunRequestContextValue = {
  groups: RunRequestGroup[]
  respondPermission: (runId: string, requestId: string, decision: PermissionDecision) => Promise<void>
  respondQuestion: (runId: string, requestId: string, answer: string, via: string) => Promise<void>
  locateNode: (nodeId: string) => boolean
  registerNodeLocator: (locator: NodeLocator) => () => void
}

export const RunRequestContext = createContext<RunRequestContextValue | null>(null)

function isActive(status: string | undefined): boolean {
  return status === RunStatus.RunStarting || status === RunStatus.RunRunning
}

export function getRunRequestGroups(runViews: Record<string, RunView>, projectId: string | null): RunRequestGroup[] {
  if (!projectId) return []
  return Object.values(runViews).flatMap((view) => {
    const snapshot = view.snapshot
    if (view.error || !snapshot || snapshot.projectId !== projectId || !isActive(snapshot.run.status ?? view.summary?.status)) return []
    const permissions = snapshot.permissions ?? []
    const questions = snapshot.questions ?? []
    if (!permissions.length && !questions.length) return []
    return [{
      projectId,
      runId: snapshot.runId,
      nodeId: snapshot.run.nodeId,
      chainId: snapshot.run.chainId,
      permissions,
      questions,
    }]
  })
}

export function useRunRequestContext(): RunRequestContextValue | null {
  return useContext(RunRequestContext)
}
