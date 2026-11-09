import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { OutputKind, PendingStatus } from '../../bindings/audio-ui/internal/domain/valueobject'
import type { Node, PendingDetail, PendingSummary } from '../../bindings/audio-ui/internal/domain/entity'

type PendingSurface = 'console' | 'node' | null

export type PendingController = {
  pendingItems: PendingSummary[]
  selectedPendingId: string
  selectedPending: PendingSummary | undefined
  selectedPendingDetail: PendingDetail | null
  selectedOutputNode?: Node
  surface: PendingSurface
  surfaceNodeId: string
  loading: boolean
  loadError: string
  actionError: string
  forceConfirmation: boolean
  actionBusy: boolean
  resolved: boolean
  resolvedPendingIds: Set<string>
  syncError: string
  selectFromConsole: (id: string) => Promise<void>
  openFromNode: (id: string, nodeId: string) => Promise<void>
  retryDetail: () => void
  apply: (force?: boolean) => Promise<void>
  discard: () => Promise<void>
  cancelForce: () => void
  retryWorkspace: () => Promise<void>
  closeNodeSurface: () => void
}

type Options = {
  projectId: string | null
  nodes: Node[]
  pending: PendingSummary[]
  onGetPending: (id: string, projectId: string) => Promise<PendingDetail>
  onApplyPending: (id: string, force: boolean, projectId: string) => Promise<void>
  onDiscardPending: (id: string, projectId: string) => Promise<void>
  onRefreshWorkspace: () => Promise<void>
}

function messageOf(error: unknown): string {
  if (error instanceof Error) return error.message
  if (typeof error === 'string') return error
  if (error && typeof error === 'object' && 'message' in error) return String((error as { message: unknown }).message)
  return String(error)
}

function isPendingHashConflict(message: string): boolean {
  return message.includes('PARAMS_CHANGED') || message.includes('CONTEXT_CHANGED')
}

export function usePendingController({
  projectId,
  nodes,
  pending,
  onGetPending,
  onApplyPending,
  onDiscardPending,
  onRefreshWorkspace,
}: Options): PendingController {
  const pendingItems = useMemo(
    () => pending.filter((item) => item.status === PendingStatus.PendingPending || item.status === PendingStatus.PendingInvalid),
    [pending],
  )
  const [selectedPendingId, setSelectedPendingId] = useState('')
  const [pendingDetail, setPendingDetail] = useState<PendingDetail | null>(null)
  const [pendingLoading, setPendingLoading] = useState(false)
  const [pendingLoadError, setPendingLoadError] = useState('')
  const [pendingActionId, setPendingActionId] = useState('')
  const [pendingActionError, setPendingActionError] = useState('')
  const [forceConfirmationId, setForceConfirmationId] = useState('')
  const [pendingSyncError, setPendingSyncError] = useState('')
  const [resolvedPendingIds, setResolvedPendingIds] = useState<Set<string>>(() => new Set())
  const [surface, setSurface] = useState<PendingSurface>(null)
  const [surfaceNodeId, setSurfaceNodeId] = useState('')
  const pendingGeneration = useRef(0)
  const pendingActionInFlight = useRef(false)
  const currentProjectId = useRef(projectId)
  currentProjectId.current = projectId
  const selectedPending = pendingItems.find((item) => item.id === selectedPendingId)
  const selectedPendingDetail = pendingDetail?.pending.id === selectedPendingId ? pendingDetail : null
  const binding = selectedPendingDetail?.pending.outputBinding ?? selectedPending?.outputBinding
  const resultNodeId = selectedPendingDetail?.pending.resultNodeId
    || selectedPending?.resultNodeId
    || binding?.nodeId
  const selectedOutputNodeCandidate = resultNodeId ? nodes.find((node) => node.id === resultNodeId) : undefined
  const selectedOutputNode = selectedOutputNodeCandidate?.type === 'script' ? selectedOutputNodeCandidate : undefined
  const isBoundScriptPending = selectedPending?.kind === OutputKind.OutputKindScript && Boolean(binding?.bound)

  useEffect(() => {
    pendingGeneration.current += 1
    pendingActionInFlight.current = false
    setSelectedPendingId('')
    setPendingDetail(null)
    setPendingLoading(false)
    setPendingLoadError('')
    setPendingActionId('')
    setPendingActionError('')
    setForceConfirmationId('')
    setPendingSyncError('')
    setResolvedPendingIds(new Set())
    setSurface(null)
    setSurfaceNodeId('')
  }, [projectId])

  useEffect(() => {
    const activeIDs = new Set(pendingItems.map((item) => item.id))
    setResolvedPendingIds((current) => new Set([...current].filter((id) => activeIDs.has(id))))
    if (selectedPendingId && !activeIDs.has(selectedPendingId)) {
      pendingGeneration.current += 1
      setSelectedPendingId('')
      setPendingDetail(null)
      setPendingLoading(false)
      setPendingLoadError('')
      setPendingActionError('')
      setForceConfirmationId('')
      setPendingSyncError('')
      setSurface(null)
      setSurfaceNodeId('')
    }
  }, [pendingItems, selectedPendingId])

  const loadPending = useCallback(async (id: string, nextSurface: Exclude<PendingSurface, null>, nodeId = '') => {
    if (!projectId) return
    const ownerProjectId = projectId
    const generation = ++pendingGeneration.current
    setSelectedPendingId(id)
    setSurface(nextSurface)
    setSurfaceNodeId(nodeId)
    setPendingDetail(null)
    setPendingLoading(true)
    setPendingLoadError('')
    setPendingActionError('')
    setForceConfirmationId('')
    setPendingSyncError('')
    try {
      const detail = await onGetPending(id, ownerProjectId)
      if (generation !== pendingGeneration.current || currentProjectId.current !== ownerProjectId) return
      setPendingDetail(detail)
    } catch (failure) {
      if (generation !== pendingGeneration.current || currentProjectId.current !== ownerProjectId) return
      setPendingLoadError(messageOf(failure))
    } finally {
      if (generation === pendingGeneration.current && currentProjectId.current === ownerProjectId) setPendingLoading(false)
    }
  }, [onGetPending, projectId])

  const selectFromConsole = useCallback((id: string) => loadPending(id, 'console'), [loadPending])
  const openFromNode = useCallback((id: string, nodeId: string) => loadPending(id, 'node', nodeId), [loadPending])

  const mutatePending = useCallback(async (action: 'apply' | 'discard', force = false) => {
    const id = selectedPendingId
    const ownerProjectId = projectId
    if (!id || !ownerProjectId || pendingActionInFlight.current || resolvedPendingIds.has(id)) return
    pendingActionInFlight.current = true
    setPendingActionId(id)
    setPendingActionError('')
    setPendingSyncError('')
    try {
      if (action === 'apply') await onApplyPending(id, force, ownerProjectId)
      else await onDiscardPending(id, ownerProjectId)
      if (currentProjectId.current !== ownerProjectId) return
      setResolvedPendingIds((current) => new Set(current).add(id))
      setForceConfirmationId('')
      try {
        await onRefreshWorkspace()
      } catch (failure) {
        if (currentProjectId.current === ownerProjectId) {
          setPendingSyncError(`操作已成功，但工程状态同步失败：${messageOf(failure)}`)
        }
      }
    } catch (failure) {
      if (currentProjectId.current !== ownerProjectId) return
      const message = messageOf(failure)
      if (action === 'apply' && !force && !isBoundScriptPending && isPendingHashConflict(message)) {
        setForceConfirmationId(id)
        setPendingActionError('')
      } else {
        const actionLabel = action === 'apply' ? isBoundScriptPending ? '确认追加' : '应用' : '丢弃'
        setPendingActionError(`${actionLabel}失败：${message}`)
      }
    } finally {
      if (currentProjectId.current === ownerProjectId) {
        pendingActionInFlight.current = false
        setPendingActionId('')
      }
    }
  }, [isBoundScriptPending, onApplyPending, onDiscardPending, onRefreshWorkspace, projectId, resolvedPendingIds, selectedPendingId])

  const retryWorkspace = useCallback(async () => {
    const ownerProjectId = projectId
    if (!ownerProjectId) return
    try {
      await onRefreshWorkspace()
      if (currentProjectId.current === ownerProjectId) setPendingSyncError('')
    } catch (failure) {
      if (currentProjectId.current === ownerProjectId) setPendingSyncError(`工程状态同步失败：${messageOf(failure)}`)
    }
  }, [onRefreshWorkspace, projectId])

  const retryDetail = useCallback(() => {
    if (!selectedPendingId) return
    void loadPending(selectedPendingId, surface === 'node' ? 'node' : 'console', surfaceNodeId)
  }, [loadPending, selectedPendingId, surface, surfaceNodeId])

  const closeNodeSurface = useCallback(() => {
    if (surface === 'node') {
      setSurface(null)
      setSurfaceNodeId('')
    }
  }, [surface])

  return {
    pendingItems,
    selectedPendingId,
    selectedPending,
    selectedPendingDetail,
    selectedOutputNode,
    surface,
    surfaceNodeId,
    loading: pendingLoading,
    loadError: pendingLoadError,
    actionError: pendingActionError,
    forceConfirmation: forceConfirmationId === selectedPendingId,
    actionBusy: pendingActionId === selectedPendingId,
    resolved: resolvedPendingIds.has(selectedPendingId),
    resolvedPendingIds,
    syncError: pendingSyncError,
    selectFromConsole,
    openFromNode,
    retryDetail,
    apply: (force = false) => mutatePending('apply', force),
    discard: () => mutatePending('discard'),
    cancelForce: () => { setForceConfirmationId(''); setPendingActionError('') },
    retryWorkspace,
    closeNodeSurface,
  }
}

const PendingControllerContext = createContext<PendingController | null>(null)

export function PendingControllerProvider({ controller, children }: { controller: PendingController; children: ReactNode }) {
  return <PendingControllerContext.Provider value={controller}>{children}</PendingControllerContext.Provider>
}

export function usePendingControllerContext(): PendingController | null {
  return useContext(PendingControllerContext)
}
