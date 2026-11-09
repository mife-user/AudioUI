import { useCallback, useMemo, useRef, type ReactNode } from 'react'
import { useRunStore, type RunView } from '../stores/runStore'
import type { PermissionDecision } from '../api/runs'
import { RunRequestContext, getRunRequestGroups, type NodeLocator } from './runRequests'

type Props = {
  projectId: string | null
  runViews: Record<string, RunView>
  onPermission: (runId: string, requestId: string, decision: PermissionDecision) => Promise<void>
  onQuestion: (runId: string, requestId: string, answer: string, via: string) => Promise<void>
  children: ReactNode
}

export function RunRequestProvider({ projectId, runViews, onPermission, onQuestion, children }: Props) {
  const locatorRef = useRef<NodeLocator | null>(null)
  const groups = useMemo(() => getRunRequestGroups(runViews, projectId), [projectId, runViews])
  const respondPermission = useCallback(async (runId: string, requestId: string, decision: PermissionDecision) => {
    if (!projectId || useRunStore.getState().projectId !== projectId) return
    await onPermission(runId, requestId, decision)
  }, [onPermission, projectId])
  const respondQuestion = useCallback(async (runId: string, requestId: string, answer: string, via: string) => {
    if (!projectId || useRunStore.getState().projectId !== projectId) return
    await onQuestion(runId, requestId, answer, via)
  }, [onQuestion, projectId])
  const registerNodeLocator = useCallback((locator: NodeLocator) => {
    locatorRef.current = locator
    return () => {
      if (locatorRef.current === locator) locatorRef.current = null
    }
  }, [])
  const locateNode = useCallback((nodeId: string) => locatorRef.current?.(nodeId) ?? false, [])
  const value = useMemo(() => ({ groups, respondPermission, respondQuestion, locateNode, registerNodeLocator }), [groups, locateNode, registerNodeLocator, respondPermission, respondQuestion])

  return <RunRequestContext.Provider value={value}>{children}</RunRequestContext.Provider>
}
