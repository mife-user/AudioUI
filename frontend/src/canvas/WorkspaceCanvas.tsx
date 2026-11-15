import { useCallback, useEffect, useMemo, useRef, useState, type KeyboardEvent, type MouseEvent as ReactMouseEvent } from 'react'
import {
  applyNodeChanges,
  Background,
  BackgroundVariant,
  Controls,
  Panel,
  ReactFlow,
  ReactFlowProvider,
  useReactFlow,
  type Connection,
  type Edge,
  type NodeChange,
  type NodeMouseHandler,
} from '@xyflow/react'
import type { Edge as ProjectEdge, GraphMutationReceipt, Node, NodeParams, RunStartOptions, RunStartResult, WorkspaceSnapshot } from '../../bindings/audio-ui/internal/domain/entity'
import { startRun as startRunAPI } from '../api/runs'
import { graphAPI, type GraphAPI, type TopicCandidateFixture } from './graphApi'
import { allocateNodePosition, NODE_LAYOUT_GAP } from './nodePosition'
import { COLLAPSED_NODE_HEIGHT, buildFlowEdges, buildFlowModel, inputHandleId, nodeMap, outputHandleId, type CanvasFlowNode, type ChildNodeType } from './flowModel'
import { preserveCanvasNodeViewState } from './nodeState'
import { type NodeLocator } from '../hooks/runRequests'
import { CanvasEdge } from './CanvasEdge'
import { CanvasConnectionLine } from './CanvasConnectionLine'
import { CandidateNode } from './CandidateNode'
import { CanvasContextMenu, type CanvasCreateType } from './CanvasContextMenu'
import { ChainQuickBar } from './ChainQuickBar'
import { AgentNode } from './AgentNode'
import { ScriptNode } from './ScriptNode'
import { PromptNode } from './PromptNode'
import { TopicNode } from './TopicNode'

const nodeTypes = {
  topic: TopicNode,
  candidate: CandidateNode,
  prompt: PromptNode,
  agent: AgentNode,
  script: ScriptNode,
}
const edgeTypes = { workspace: CanvasEdge }
const MOVE_DEBOUNCE_MS = 240
const EMPTY_HANDLE_LAYOUTS: Record<string, string> = {}

function scheduleFrame(callback: FrameRequestCallback): number {
  if (typeof window.requestAnimationFrame === 'function') return window.requestAnimationFrame(callback)
  return window.setTimeout(() => callback(Date.now()), 0)
}

function cancelScheduledFrame(handle: number): void {
  if (typeof window.cancelAnimationFrame === 'function') window.cancelAnimationFrame(handle)
  else window.clearTimeout(handle)
}

type Props = {
  snapshot: WorkspaceSnapshot
  onSelectNode: (id: string) => Promise<void>
  onGraphMutation?: (receipt: GraphMutationReceipt) => void | Promise<void>
  onRegisterDraftFlush?: (flush: (projectId: string) => Promise<boolean>) => () => void
  onRegisterNodeLocator?: (locator: NodeLocator) => () => void
  api?: GraphAPI
  candidateFixtures?: Record<string, TopicCandidateFixture[]>
  onStartRun?: (nodeId: string, projectId: string, options?: RunStartOptions) => Promise<RunStartResult>
  workspaceSyncError?: { message: string; retry: () => void }
}

type PendingSync = {
  receipt: GraphMutationReceipt
  label: string
  focusNodeId?: string
}

type PendingPosition = {
  x: number
  y: number
}

type PendingLayout = {
  x: number
  y: number
  width: number
  height: number
}

type CandidateReadEntry = {
  signature: string
  candidates?: TopicCandidateFixture[]
  loading: boolean
  error?: string
}

type CandidateReadState = {
  projectId: string
  entries: Record<string, CandidateReadEntry>
}

type CandidateReadQuery = {
  nodeId: string
  signature: string
}

type MeasuredHandleState = {
  projectId: string
  byNode: Record<string, string>
}

type RegisteredFlush = {
  projectId: string
  flush: () => Promise<boolean>
}

type ConnectionEditIntent = {
  projectId: string
  edgeId: string
  sourceId: string
  targetId: string
} & ({ mode: 'disconnect' } | { mode: 'replace'; nextSourceId: string })

type RelationPopoverState = {
  projectId: string
  edgeId: string
  x: number
  y: number
  returnFocus: Element
}

type CreateMenuState = {
  screenPosition: { x: number; y: number }
  flowPosition: { x: number; y: number }
}

function errorMessage(error: unknown): string {
  if (error instanceof Error) return error.message
  if (typeof error === 'string') return error
  if (error && typeof error === 'object' && 'message' in error) return String((error as { message: unknown }).message)
  return String(error)
}

function sameEdgeIdentity(left: ProjectEdge, right: ProjectEdge): boolean {
  return left.id === right.id
    && left.sourceNodeId === right.sourceNodeId
    && left.targetNodeId === right.targetNodeId
    && left.kind === right.kind
    && (left.scriptVersion ?? 0) === (right.scriptVersion ?? 0)
}

function candidateQueryKey(nodes: Node[] | undefined): string {
  const queries: CandidateReadQuery[] = []
  for (const node of nodes ?? []) {
    if (node.type !== 'topic' || !node.rounds?.length) continue
    const latest = node.rounds.reduce((current, round) => round.n > current.n ? round : current)
    if (latest.kind !== 'topics') continue
    queries.push({
      nodeId: node.id,
      signature: JSON.stringify([latest.n, latest.kind, latest.artifactId]),
    })
  }
  queries.sort((a, b) => a.nodeId.localeCompare(b.nodeId))
  return JSON.stringify(queries)
}

function CanvasBody({ snapshot, onSelectNode, onGraphMutation, onRegisterDraftFlush, onRegisterNodeLocator, api, candidateFixtures, onStartRun, workspaceSyncError }: Props & { api: GraphAPI }) {
  const projectId = snapshot.project?.id ?? ''
  const graphNodes = snapshot.nodes ?? []
  const [candidateState, setCandidateState] = useState<CandidateReadState>({ projectId: '', entries: {} })
  const [measuredHandleState, setMeasuredHandleState] = useState<MeasuredHandleState>({ projectId: '', byNode: {} })
  const candidateStateRef = useRef(candidateState)
  candidateStateRef.current = candidateState
  const currentProjectId = useRef(projectId)
  currentProjectId.current = projectId
  const snapshotRef = useRef(snapshot)
  snapshotRef.current = snapshot
  const onNodeHandlesMeasured = useCallback((nodeId: string, layoutKey: string, ownerProjectId: string) => {
    if (!ownerProjectId || currentProjectId.current !== ownerProjectId) return
    setMeasuredHandleState((current) => {
      const byNode = current.projectId === ownerProjectId ? current.byNode : {}
      if (byNode[nodeId] === layoutKey) return current
      return { projectId: ownerProjectId, byNode: { ...byNode, [nodeId]: layoutKey } }
    })
  }, [])
  const registerPromptFlush = useCallback((nodeId: string, ownerProjectId: string, flush: () => Promise<boolean>) => {
    const entry = { projectId: ownerProjectId, flush }
    promptFlushers.current.set(nodeId, entry)
    return () => {
      if (promptFlushers.current.get(nodeId) === entry) promptFlushers.current.delete(nodeId)
    }
  }, [])
  const registerTopicFlush = useCallback((nodeId: string, ownerProjectId: string, flush: () => Promise<boolean>) => {
    const entry = { projectId: ownerProjectId, flush }
    topicFlushers.current.set(nodeId, entry)
    return () => {
      if (topicFlushers.current.get(nodeId) === entry) topicFlushers.current.delete(nodeId)
    }
  }, [])
  const measuredHandleLayouts = measuredHandleState.projectId === projectId
    ? measuredHandleState.byNode
    : EMPTY_HANDLE_LAYOUTS
  const topicCandidateQueryKey = useMemo(() => candidateQueryKey(snapshot.nodes ?? undefined), [snapshot.nodes])
  const topicCandidateQueries = useMemo(() => JSON.parse(topicCandidateQueryKey) as CandidateReadQuery[], [topicCandidateQueryKey])
  const candidateStateForProject = candidateState.projectId === projectId ? candidateState : null
  const candidateData = useMemo(() => {
    if (candidateFixtures !== undefined) return { candidates: candidateFixtures, loading: {} as Record<string, boolean>, errors: {} as Record<string, string> }
    const candidates: Record<string, TopicCandidateFixture[]> = {}
    const loading: Record<string, boolean> = {}
    const errors: Record<string, string> = {}
    for (const query of topicCandidateQueries) {
      const entry = candidateStateForProject?.entries[query.nodeId]
      if (!entry || entry.signature !== query.signature) continue
      if (entry.candidates !== undefined) candidates[query.nodeId] = entry.candidates
      if (entry.loading) loading[query.nodeId] = true
      if (entry.error) errors[query.nodeId] = entry.error
    }
    return { candidates, loading, errors }
  }, [candidateFixtures, candidateStateForProject, topicCandidateQueries])
  const candidateMap = candidateData.candidates
  const candidateErrors = candidateData.errors
  const candidateLoading = candidateData.loading
  const snapshotNodeCount = useRef(snapshot.nodes?.length ?? 0)
  snapshotNodeCount.current = snapshot.nodes?.length ?? 0
  const flow = useReactFlow<CanvasFlowNode>()
  const flowRef = useRef(flow)
  flowRef.current = flow
  const flowRoot = useRef<HTMLDivElement>(null)
  const moveTimers = useRef(new Map<string, number>())
  const moveQueues = useRef(new Map<string, Promise<void>>())
  const layoutSaveQueues = useRef(new Map<string, Promise<void>>())
  const resizingNodeIds = useRef(new Set<string>())
  const lastPersistedPositions = useRef(new Map<string, { x: number; y: number }>())
  const lastPersistedSizes = useRef(new Map<string, { width: number; height: number }>())
  const pendingPositions = useRef(new Map<string, PendingPosition>())
  const pendingLayouts = useRef(new Map<string, PendingLayout>())
  const pendingCollapsed = useRef(new Map<string, boolean>())
  const collapsingNodes = useRef(new Set<string>())
  const initialFit = useRef({ projectId: '', shouldFit: false, done: false })
  const createRequestId = useRef(0)
  const [nodes, setNodes] = useState<CanvasFlowNode[]>([])
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [connectSubmitting, setConnectSubmitting] = useState(false)
  const [scriptVersionSavingEdgeIDs, setScriptVersionSavingEdgeIDs] = useState<Set<string>>(() => new Set())
  const [paramsSaveCount, setParamsSaveCount] = useState(0)
  const [runSubmittingNodeId, setRunSubmittingNodeId] = useState('')
  const [runSubmittingInputNodeIDs, setRunSubmittingInputNodeIDs] = useState<string[]>([])
  const [pendingSync, setPendingSync] = useState<PendingSync | null>(null)
  const pendingSyncRef = useRef<PendingSync | null>(null)
  const lastSyncedRevision = useRef({ projectId, revision: snapshot.project?.revision ?? 0 })
  const syncMutationRef = useRef<(receipt: GraphMutationReceipt, label: string, focusNodeId?: string) => Promise<boolean>>(async () => false)
  if (lastSyncedRevision.current.projectId !== projectId) {
    lastSyncedRevision.current = { projectId, revision: snapshot.project?.revision ?? 0 }
  } else if ((snapshot.project?.revision ?? 0) > lastSyncedRevision.current.revision) {
    lastSyncedRevision.current = { projectId, revision: snapshot.project?.revision ?? 0 }
  }
  const paramsSaveQueue = useRef(Promise.resolve())
  const [pendingFocusNodeId, setPendingFocusNodeId] = useState('')
  const [confirmIntent, setConfirmIntent] = useState<ConnectionEditIntent | null>(null)
  const [relationPopover, setRelationPopover] = useState<RelationPopoverState | null>(null)
  const [reconnectDropTarget, setReconnectDropTarget] = useState<{ nodeId: string; valid: boolean } | null>(null)
  const relationPopoverRef = useRef<HTMLDivElement>(null)
  const relationCloseRef = useRef<HTMLButtonElement>(null)
  const [replacementSources, setReplacementSources] = useState<Record<string, string>>({})
  const [connectionTarget, setConnectionTarget] = useState('')
  const [createMenu, setCreateMenu] = useState<CreateMenuState | null>(null)
  const createInFlight = useRef(false)
  const connectInFlight = useRef(false)
  const scriptVersionInFlight = useRef(new Set<string>())
  const runStartInFlight = useRef(false)
  const runRequestSequence = useRef(0)
  const promptFlushers = useRef(new Map<string, RegisteredFlush>())
  const topicFlushers = useRef(new Map<string, RegisteredFlush>())
  const hasActiveRun = (snapshot.runs ?? []).some((run) => run.status === 'starting' || run.status === 'running')
  const globalRunBusy = hasActiveRun || Boolean(runSubmittingNodeId)
  const globalRunBusyRef = useRef(globalRunBusy)
  globalRunBusyRef.current = globalRunBusy
  const paramsEditingDisabled = busy || connectSubmitting || pendingSync !== null
  const mutationBlocked = paramsEditingDisabled || paramsSaveCount > 0
  const mutationBlockedRef = useRef(mutationBlocked)
  mutationBlockedRef.current = mutationBlocked
  const runStartBlockedRef = useRef(busy || connectSubmitting || pendingSync !== null)
  runStartBlockedRef.current = busy || connectSubmitting || pendingSync !== null
  const confirmCancelRef = useRef<HTMLButtonElement>(null)
  const confirmReturnFocus = useRef<HTMLElement | null>(null)
  const locateNode = useCallback((nodeId: string) => {
    if (currentProjectId.current !== projectId || !flowRef.current.getNode(nodeId)) return false
    setError('')
    void onSelectNode(nodeId).catch((failure) => {
      if (currentProjectId.current === projectId) setError(`保存节点选择失败：${errorMessage(failure)}`)
    })
    void flowRef.current.fitView({ nodes: [{ id: nodeId }], padding: 0.35, duration: 180 })
    return true
  }, [onSelectNode, projectId])
  const openEdgeRelationAt = useCallback((edgeId: string, clientX: number, clientY: number, returnFocus: Element) => {
    const bounds = flowRoot.current?.getBoundingClientRect()
    if (!bounds) return
    const width = 296
    const height = 190
    setRelationPopover({
      projectId,
      edgeId,
      x: Math.max(8, Math.min(bounds.width - width - 8, clientX - bounds.left + 8)),
      y: Math.max(8, Math.min(bounds.height - height - 8, clientY - bounds.top + 8)),
      returnFocus,
    })
  }, [projectId])
  const openEdgeRelationFromKeyboard = useCallback((edgeId: string, anchor: Element) => {
    const bounds = anchor.getBoundingClientRect()
    openEdgeRelationAt(edgeId, bounds.left + bounds.width / 2, bounds.top + bounds.height / 2, anchor)
  }, [openEdgeRelationAt])
  const closeEdgeRelation = useCallback((returnFocus = false) => {
    const anchor = relationPopover?.returnFocus as (Element & { focus?: () => void }) | undefined
    setRelationPopover(null)
    if (returnFocus && anchor?.focus) scheduleFrame(() => anchor.focus?.())
  }, [relationPopover])

  useEffect(() => {
    if (!relationPopover) return
    const onPointerDown = (event: PointerEvent) => {
      if (!(event.target instanceof Element) || !relationPopoverRef.current?.contains(event.target)) closeEdgeRelation()
    }
    document.addEventListener('pointerdown', onPointerDown)
    return () => document.removeEventListener('pointerdown', onPointerDown)
  }, [closeEdgeRelation, relationPopover])

  useEffect(() => {
    if (!relationPopover) return
    if (relationPopover.projectId !== projectId || !(snapshot.edges ?? []).some((edge) => edge.id === relationPopover.edgeId)) {
      closeEdgeRelation()
    }
  }, [closeEdgeRelation, projectId, relationPopover, snapshot.edges])

  useEffect(() => {
    if (relationPopover) relationCloseRef.current?.focus()
  }, [relationPopover])

  useEffect(() => onRegisterNodeLocator?.(locateNode), [locateNode, onRegisterNodeLocator])

  const flushWorkspaceDrafts = useCallback(async (ownerProjectId: string) => {
    if (currentProjectId.current !== ownerProjectId) throw new Error('PROJECT_CHANGED: 工程已切换，未保存旧工程草稿。')
    if (busy || connectSubmitting) throw new Error('当前工程操作仍在提交，请等待完成后再切换。')

    const retryPendingSync = async () => {
      const pending = pendingSyncRef.current
      if (!pending || pending.receipt.projectId !== ownerProjectId) return true
      return await syncMutationRef.current(pending.receipt, pending.label, pending.focusNodeId)
    }

    if (!await retryPendingSync()) return false
    for (const node of graphNodes) {
      const registry = node.type === 'prompt' ? promptFlushers.current : node.type === 'topic' ? topicFlushers.current : null
      const registered = registry?.get(node.id)
      if (!registered || registered.projectId !== ownerProjectId) continue
      if (!await registered.flush()) return false
    }
    if (currentProjectId.current !== ownerProjectId) return false
    return await retryPendingSync()
  }, [busy, connectSubmitting, graphNodes])

  useEffect(() => onRegisterDraftFlush?.(flushWorkspaceDrafts), [flushWorkspaceDrafts, onRegisterDraftFlush])

  useEffect(() => {
    if (candidateFixtures !== undefined) {
      setCandidateState({ projectId, entries: {} })
      return undefined
    }
    let cancelled = false
    const currentEntries = candidateStateRef.current.projectId === projectId ? candidateStateRef.current.entries : {}
    const nextEntries: Record<string, CandidateReadEntry> = {}
    const queriesToRead: CandidateReadQuery[] = []
    for (const query of topicCandidateQueries) {
      const previous = currentEntries[query.nodeId]
      if (previous?.signature === query.signature && previous.candidates !== undefined) {
        nextEntries[query.nodeId] = previous
        continue
      }
      nextEntries[query.nodeId] = { signature: query.signature, loading: true }
      queriesToRead.push(query)
    }
    setCandidateState({ projectId, entries: nextEntries })
    for (const query of queriesToRead) {
      void api.getTopicCandidates(query.nodeId, projectId).then((candidates) => {
        if (cancelled || currentProjectId.current !== projectId) return
        setCandidateState((current) => {
          if (current.projectId !== projectId || current.entries[query.nodeId]?.signature !== query.signature) return current
          return {
            ...current,
            entries: { ...current.entries, [query.nodeId]: { signature: query.signature, candidates, loading: false } },
          }
        })
      }).catch((failure: unknown) => {
        if (cancelled || currentProjectId.current !== projectId) return
        setCandidateState((current) => {
          if (current.projectId !== projectId || current.entries[query.nodeId]?.signature !== query.signature) return current
          return {
            ...current,
            entries: { ...current.entries, [query.nodeId]: { signature: query.signature, loading: false, error: errorMessage(failure) } },
          }
        })
      })
    }
    return () => { cancelled = true }
  }, [api, candidateFixtures, projectId, snapshot.project?.revision, topicCandidateQueryKey, topicCandidateQueries])

  const syncMutation = useCallback(async (receipt: GraphMutationReceipt, label: string, focusNodeId?: string) => {
    try {
      await onGraphMutation?.(receipt)
      if (currentProjectId.current !== receipt.projectId) return false
      const current = lastSyncedRevision.current
      const revision = current.projectId === receipt.projectId ? Math.max(current.revision, receipt.projectRevision) : receipt.projectRevision
      lastSyncedRevision.current = { projectId: receipt.projectId, revision }
      const pending = pendingSyncRef.current
      if (pending?.receipt.projectId === receipt.projectId && pending.receipt.projectRevision > revision) return true
      pendingSyncRef.current = null
      setPendingSync(null)
      setError('')
      if (focusNodeId) setPendingFocusNodeId(focusNodeId)
      return true
    } catch (failure) {
      if (currentProjectId.current !== receipt.projectId) return false
      const current = lastSyncedRevision.current
      if (current.projectId === receipt.projectId && current.revision >= receipt.projectRevision) {
        const pending = pendingSyncRef.current
        if (pending?.receipt.projectId === receipt.projectId && pending.receipt.projectRevision <= current.revision) {
          pendingSyncRef.current = null
          setPendingSync(null)
          setError('')
        }
        return true
      }
      const existing = pendingSyncRef.current
      if (existing?.receipt.projectId === receipt.projectId && existing.receipt.projectRevision > receipt.projectRevision) return false
      const pending = { receipt, label, focusNodeId }
      pendingSyncRef.current = pending
      setPendingSync(pending)
      setError(`${label}已保存，但画布同步失败：${errorMessage(failure)}。请重试同步，不要重复创建。`)
      return false
    }
  }, [onGraphMutation])
  syncMutationRef.current = syncMutation

  const schedulePositionSave = useCallback((nodeId: string, position: PendingPosition) => {
    if (!projectId || currentProjectId.current !== projectId) return
    const ownerProjectId = projectId
    const key = `${ownerProjectId}:${nodeId}`
    const previous = moveTimers.current.get(key)
    if (previous) window.clearTimeout(previous)
    const timer = window.setTimeout(() => {
      moveTimers.current.delete(key)
      const earlier = moveQueues.current.get(key) ?? Promise.resolve()
      const next = earlier.catch(() => undefined).then(async () => {
        const latest = pendingPositions.current.get(key)
        if (currentProjectId.current !== ownerProjectId || !latest || latest.x !== position.x || latest.y !== position.y) return
        const lastSaved = lastPersistedPositions.current.get(key) ?? { ...position }
        try {
          const receipt = await api.moveNode(nodeId, position.x, position.y, ownerProjectId)
          if (currentProjectId.current !== ownerProjectId) return
          const synced = await syncMutation(receipt, '节点位置')
          if (synced) lastPersistedPositions.current.set(key, { ...position })
        } catch (failure) {
          if (currentProjectId.current !== ownerProjectId) return
          pendingPositions.current.delete(key)
          lastPersistedPositions.current.set(key, { ...lastSaved })
          setNodes((current) => current.map((node) => node.id === nodeId ? { ...node, position: lastSaved } : node))
          setError(`保存节点位置失败：${errorMessage(failure)}`)
        }
      })
      moveQueues.current.set(key, next)
      void next.then(() => {
        if (moveQueues.current.get(key) === next) moveQueues.current.delete(key)
      })
    }, MOVE_DEBOUNCE_MS)
    moveTimers.current.set(key, timer)
  }, [api, projectId, syncMutation])

  const saveNodeLayout = useCallback(async (nodeId: string, x: number, y: number, width: number, height: number) => {
    if (!projectId || currentProjectId.current !== projectId) return
    const ownerProjectId = projectId
    const key = `${ownerProjectId}:${nodeId}`
    const node = graphNodes.find((candidate) => candidate.id === nodeId)
    if (!node) return
    const previous = layoutSaveQueues.current.get(key) ?? Promise.resolve()
    const next = previous.catch(() => undefined).then(async () => {
      const latest = pendingLayouts.current.get(key)
      if (currentProjectId.current !== ownerProjectId || !latest
        || latest.x !== x || latest.y !== y || latest.width !== width || latest.height !== height) return
      try {
        const receipt = await api.setNodeLayout(nodeId, x, y, width, height, ownerProjectId)
        if (currentProjectId.current !== ownerProjectId) return
        if (receipt.projectId !== ownerProjectId) throw new Error('工程布局回执所属工程不一致。')
        const synced = await syncMutation(receipt, '节点布局')
        const pendingLayout = pendingLayouts.current.get(key)
        if (synced && pendingLayout?.x === x && pendingLayout.y === y && pendingLayout.width === width && pendingLayout.height === height) {
          pendingLayouts.current.delete(key)
          pendingPositions.current.delete(key)
          lastPersistedPositions.current.set(key, { x, y })
          lastPersistedSizes.current.set(key, { width, height })
        }
      } catch (failure) {
        if (currentProjectId.current !== ownerProjectId) return
        const pendingLayout = pendingLayouts.current.get(key)
        if (pendingLayout?.x !== x || pendingLayout.y !== y || pendingLayout.width !== width || pendingLayout.height !== height) return
        pendingLayouts.current.delete(key)
        pendingPositions.current.delete(key)
        const lastPosition = lastPersistedPositions.current.get(key) ?? node.position
        const lastSize = lastPersistedSizes.current.get(key) ?? node.size
        setNodes((current) => current.map((flowNode) => flowNode.id === nodeId
          ? {
              ...flowNode,
              position: { ...lastPosition },
              style: { ...flowNode.style, width: lastSize.width, height: lastSize.height },
            }
          : flowNode))
        setError(`保存节点布局失败：${errorMessage(failure)}`)
      }
    })
    layoutSaveQueues.current.set(key, next)
    void next.then(() => {
      if (layoutSaveQueues.current.get(key) === next) layoutSaveQueues.current.delete(key)
    })
    await next
  }, [api, graphNodes, projectId, syncMutation])

  const applyNodeLayout = useCallback((nodeId: string, width: number, height: number) => {
    if (!projectId || currentProjectId.current !== projectId || mutationBlockedRef.current) return
    const node = flowRef.current.getNode(nodeId)
    if (!node || (node.style?.width === width && node.style?.height === height)) return
    const position = { x: node.position.x, y: node.position.y }
    const layout = { ...position, width, height }
    const key = `${projectId}:${nodeId}`
    const timer = moveTimers.current.get(key)
    if (timer) {
      window.clearTimeout(timer)
      moveTimers.current.delete(key)
    }
    pendingPositions.current.delete(key)
    pendingLayouts.current.set(key, layout)
    setNodes((current) => current.map((flowNode) => flowNode.id === nodeId
      ? { ...flowNode, position, style: { ...flowNode.style, width, height } }
      : flowNode))
    void saveNodeLayout(nodeId, layout.x, layout.y, layout.width, layout.height)
  }, [projectId, saveNodeLayout])

  const applyNodeCollapsed = useCallback(async (nodeId: string, collapsed: boolean) => {
    if (!projectId || currentProjectId.current !== projectId || mutationBlockedRef.current) return
    const ownerProjectId = projectId
    const key = `${ownerProjectId}:${nodeId}`
    if (collapsingNodes.current.has(key)) return
    const node = flowRef.current.getNode(nodeId)
    if (!node || Boolean(node.data.node.collapsed) === collapsed) return
    collapsingNodes.current.add(key)
    pendingCollapsed.current.set(key, collapsed)
    setNodes((current) => current.map((flowNode) => flowNode.id === nodeId
      ? {
          ...flowNode,
          data: { ...flowNode.data, node: { ...flowNode.data.node, collapsed } },
          style: { ...flowNode.style, height: collapsed ? COLLAPSED_NODE_HEIGHT : flowNode.data.node.size.height },
        }
      : flowNode))
    try {
      const receipt = await api.setNodeCollapsed(nodeId, collapsed, ownerProjectId)
      if (currentProjectId.current !== ownerProjectId) return
      if (receipt.projectId !== ownerProjectId) throw new Error('节点折叠回执所属工程不一致。')
      await syncMutation(receipt, '节点折叠状态')
    } catch (failure) {
      if (currentProjectId.current !== ownerProjectId || pendingCollapsed.current.get(key) !== collapsed) return
      pendingCollapsed.current.delete(key)
      const storedNode = graphNodes.find((candidate) => candidate.id === nodeId)
      const restoredCollapsed = Boolean(storedNode?.collapsed)
      const pendingLayout = pendingLayouts.current.get(key)
      const expandedHeight = pendingLayout?.height ?? storedNode?.size.height ?? node.data.node.size.height
      setNodes((current) => current.map((flowNode) => flowNode.id === nodeId
        ? {
            ...flowNode,
            data: { ...flowNode.data, node: { ...flowNode.data.node, collapsed: restoredCollapsed } },
            style: { ...flowNode.style, height: restoredCollapsed ? COLLAPSED_NODE_HEIGHT : expandedHeight },
          }
        : flowNode))
      setError(`保存节点折叠状态失败：${errorMessage(failure)}`)
    } finally {
      collapsingNodes.current.delete(key)
    }
  }, [api, graphNodes, projectId, syncMutation])

  const selectCreatedNode = useCallback(async (nodeId: string) => {
    if (currentProjectId.current !== projectId) return
    try {
      await onSelectNode(nodeId)
    } catch (failure) {
      if (currentProjectId.current === projectId) setError(`节点已创建，但选中失败：${errorMessage(failure)}`)
    }
  }, [onSelectNode, projectId])

  const createNode = useCallback(async (type: ChildNodeType, parentId: string, position: Node['position'], selectAfterSync = false) => {
    if (mutationBlockedRef.current || createInFlight.current) return
    createInFlight.current = true
    const requestId = ++createRequestId.current
    const label = type === 'prompt' ? '提示词' : type === 'agent' ? 'Agent' : 'Script'
    setBusy(true)
    setError('')
    try {
      const receipt = await api.addNode({
        projectId,
        type,
        title: type === 'prompt' ? '新提示词' : type === 'agent' ? '新 Agent' : '新脚本',
        parent: parentId,
        parentCandidate: null,
        position,
        params: {},
      })
      if (currentProjectId.current !== projectId || receipt.projectId !== projectId) return
      const synced = await syncMutation(receipt, label, selectAfterSync ? undefined : receipt.nodeId)
      if (synced && selectAfterSync) await selectCreatedNode(receipt.nodeId)
    } catch (failure) {
      if (currentProjectId.current !== projectId) return
      setError(`创建${label}失败：${errorMessage(failure)}`)
    } finally {
      if (createRequestId.current === requestId) {
        createInFlight.current = false
        setBusy(false)
      }
    }
  }, [api, projectId, selectCreatedNode, syncMutation])

  const createTopic = useCallback(async (position: Node['position'], selectAfterSync = false) => {
    if (mutationBlockedRef.current || createInFlight.current) return
    createInFlight.current = true
    const requestId = ++createRequestId.current
    setBusy(true)
    setError('')
    try {
      const receipt = await api.addNode({
        projectId,
        type: 'topic',
        title: '新选题',
        parent: '',
        parentCandidate: null,
        position,
        params: { candidateCount: 3 },
      })
      if (currentProjectId.current !== projectId || receipt.projectId !== projectId) return
      const synced = await syncMutation(receipt, '选题节点', selectAfterSync ? undefined : receipt.nodeId)
      if (synced && selectAfterSync) await selectCreatedNode(receipt.nodeId)
    } catch (failure) {
      if (currentProjectId.current !== projectId) return
      setError(`创建选题节点失败：${errorMessage(failure)}`)
    } finally {
      if (createRequestId.current === requestId) {
        createInFlight.current = false
        setBusy(false)
      }
    }
  }, [api, projectId, selectCreatedNode, syncMutation])

  const addChild = useCallback((parentId: string, type: ChildNodeType) => {
    if (mutationBlockedRef.current) return
    const parent = graphNodes.find((node) => node.id === parentId)
    if (!parent) return
    const position = allocateNodePosition(graphNodes, {
      x: parent.position.x + parent.size.width + NODE_LAYOUT_GAP,
      y: parent.position.y,
    }, type)
    void createNode(type, parentId, position)
  }, [createNode, graphNodes])

  const saveNodeParams = useCallback(async (nodeId: string, title: string, params: NodeParams, ownerProjectId: string) => {
    if (currentProjectId.current !== ownerProjectId) throw new Error('PROJECT_CHANGED: 工程已切换，未保存旧工程参数。')
    setParamsSaveCount((count) => count + 1)
    const operation = paramsSaveQueue.current.catch(() => undefined).then(async () => {
      if (currentProjectId.current !== ownerProjectId) throw new Error('PROJECT_CHANGED: 工程已切换，未保存旧工程参数。')
      if (pendingSyncRef.current?.receipt.projectId === ownerProjectId) {
        throw new Error('WORKSPACE_SYNC_STALE: 请先重试工程快照同步，再保存参数。')
      }
      const receipt = await api.updateNodeParams(nodeId, title, params, ownerProjectId)
      if (receipt.projectId !== ownerProjectId) throw new Error('PROJECT_CHANGED: 参数保存回执所属工程不一致。')
      if (currentProjectId.current !== ownerProjectId) return false
      return await syncMutation(receipt, '节点参数')
    })
    paramsSaveQueue.current = operation.then(() => undefined, () => undefined)
    try {
      return await operation
    } finally {
      if (currentProjectId.current === ownerProjectId) setParamsSaveCount((count) => Math.max(0, count - 1))
    }
  }, [api, syncMutation])

  const updateEdgeScriptVersion = useCallback(async (edge: ProjectEdge, version: number) => {
    if (currentProjectId.current !== projectId || mutationBlockedRef.current || runStartInFlight.current || scriptVersionInFlight.current.has(edge.id)) return
    const current = snapshotRef.current
    const liveEdge = (current.edges ?? []).find((candidate) => candidate.id === edge.id)
    if (!liveEdge || liveEdge.sourceNodeId !== edge.sourceNodeId || liveEdge.targetNodeId !== edge.targetNodeId || liveEdge.kind !== 'connection') return
    if ((liveEdge.scriptVersion ?? 0) === version) return
    const source = (current.nodes ?? []).find((candidate) => candidate.id === liveEdge.sourceNodeId)
    if (source?.type !== 'script') return
    const locked = new Set(current.lockedNodeIds ?? [])
    if (locked.has(liveEdge.sourceNodeId) || locked.has(liveEdge.targetNodeId)) return

    scriptVersionInFlight.current.add(edge.id)
    setScriptVersionSavingEdgeIDs((currentIDs) => new Set(currentIDs).add(edge.id))
    try {
      const receipt = await api.setEdgeScriptVersion(edge.id, version, projectId)
      if (currentProjectId.current !== projectId || receipt.projectId !== projectId) return
      await syncMutation(receipt, 'Script 输入版本')
    } catch (failure) {
      if (currentProjectId.current === projectId) setError(`保存 Script 输入版本失败：${errorMessage(failure)}`)
    } finally {
      scriptVersionInFlight.current.delete(edge.id)
      setScriptVersionSavingEdgeIDs((currentIDs) => {
        const next = new Set(currentIDs)
        next.delete(edge.id)
        return next
      })
    }
  }, [api, projectId, syncMutation])

  const handleReconnectHoverTarget = useCallback((nodeId: string | null, valid: boolean) => {
    setReconnectDropTarget((current) => {
      if (!nodeId) return current === null ? current : null
      if (current?.nodeId === nodeId && current.valid === valid) return current
      return { nodeId, valid }
    })
  }, [])

  const reconnectTarget = useCallback(async (expectedProjectId: string, expectedEdge: ProjectEdge, targetNodeId: string) => {
    const ownerProjectId = expectedProjectId
    if (ownerProjectId !== projectId || currentProjectId.current !== ownerProjectId || mutationBlockedRef.current || runStartInFlight.current
      || connectInFlight.current || scriptVersionInFlight.current.has(expectedEdge.id)) return
    const current = snapshotRef.current
    const liveEdge = (current.edges ?? []).find((edge) => edge.id === expectedEdge.id)
    if (!liveEdge || !sameEdgeIdentity(liveEdge, expectedEdge)) {
      setError('连接已发生变化，原连接保持不变。请按当前画布重试。')
      return
    }
    if (liveEdge.kind !== 'connection' || targetNodeId === liveEdge.targetNodeId) return
    const target = (current.nodes ?? []).find((node) => node.id === targetNodeId)
    const locked = new Set(current.lockedNodeIds ?? [])
    if (!target || !inputHandleId(target) || locked.has(liveEdge.sourceNodeId) || locked.has(liveEdge.targetNodeId) || locked.has(targetNodeId)) {
      setError('连接目标不存在或处于运行锁中，原连接保持不变。')
      return
    }

    connectInFlight.current = true
    setConnectSubmitting(true)
    setError('')
    try {
      const receipt = await api.reconnectTarget({
        projectId: ownerProjectId,
        expectedEdge: { ...liveEdge },
        targetNodeId,
      })
      if (currentProjectId.current !== ownerProjectId || receipt.projectId !== ownerProjectId) return
      await syncMutation(receipt, '改接连接')
    } catch (failure) {
      if (currentProjectId.current === ownerProjectId) setError(errorMessage(failure))
    } finally {
      connectInFlight.current = false
      setConnectSubmitting(false)
    }
  }, [api, projectId, syncMutation])

  const startNodeRun = useCallback(async (nodeId: string, ownerProjectId: string, options?: RunStartOptions): Promise<RunStartResult> => {
    if (currentProjectId.current !== ownerProjectId) {
      return { error: { code: 'PROJECT_CHANGED', message: '工程已切换，未启动运行。' } }
    }
    if (runStartInFlight.current || globalRunBusyRef.current) {
      return { error: { code: 'BUSY', message: '已有运行正在进行，当前无法启动另一个运行。' } }
    }
    if (runStartBlockedRef.current) {
      return { error: { code: 'WORKSPACE_SYNC_STALE', message: '请先完成当前工程操作与快照同步。' } }
    }

    runStartInFlight.current = true
    const requestId = ++runRequestSequence.current
    setRunSubmittingNodeId(nodeId)
    try {
      const current = snapshotRef.current
      const incomingEdges = (current.edges ?? []).filter((edge) => edge.targetNodeId === nodeId && edge.kind === 'connection')
      const selectedEdges = new Set(options?.inputEdgeIds ?? incomingEdges.map((edge) => edge.id))
      const nodeById = new Map((current.nodes ?? []).map((node) => [node.id, node]))
      const selectedPromptIDs = new Set<string>()
      const selectedInputNodeIDs = new Set<string>()
      for (const edge of incomingEdges) {
        if (!selectedEdges.has(edge.id)) continue
        selectedInputNodeIDs.add(edge.sourceNodeId)
        if (nodeById.get(edge.sourceNodeId)?.type === 'prompt') selectedPromptIDs.add(edge.sourceNodeId)
      }
      setRunSubmittingInputNodeIDs([...selectedInputNodeIDs])
      for (const promptNodeID of selectedPromptIDs) {
        const registered = promptFlushers.current.get(promptNodeID)
        if (!registered || registered.projectId !== ownerProjectId) continue
        if (!await registered.flush()) {
          return { error: { code: 'WORKSPACE_SYNC_STALE', message: '提示词已提交，等待工程快照同步后再启动。' } }
        }
      }
      if (currentProjectId.current !== ownerProjectId) {
        return { error: { code: 'PROJECT_CHANGED', message: '工程已切换，未启动运行。' } }
      }
      if (pendingSyncRef.current?.receipt.projectId === ownerProjectId || runStartBlockedRef.current) {
        return { error: { code: 'WORKSPACE_SYNC_STALE', message: '请先完成当前工程操作与快照同步。' } }
      }
      if (onStartRun) return await onStartRun(nodeId, ownerProjectId, options)
      return await startRunAPI(nodeId, ownerProjectId, options)
    } catch (failure) {
      return { error: { code: 'RUN_START_FAILED', message: errorMessage(failure) } }
    } finally {
      if (runRequestSequence.current === requestId) {
        runStartInFlight.current = false
        setRunSubmittingNodeId('')
        setRunSubmittingInputNodeIDs([])
      }
    }
  }, [onStartRun, projectId])

  const model = useMemo(
    () => buildFlowModel(snapshot, addChild, candidateMap, mutationBlocked, {
      paramsEditingDisabled,
      runSubmittingNodeId,
      runSubmittingInputNodeIDs,
      candidateErrors,
      candidateLoading,
      onUpdateTopicParams: saveNodeParams,
      onUpdateNodeParams: saveNodeParams,
      onStartRun: startNodeRun,
      onGetScriptVersion: api.getScriptVersion,
      onRegisterPromptFlush: registerPromptFlush,
      onRegisterTopicFlush: registerTopicFlush,
      onNodeHandlesMeasured,
      onSetNodeLayout: applyNodeLayout,
      onSetNodeCollapsed: applyNodeCollapsed,
      onLocateNode: locateNode,
      reconnectDropTarget: reconnectDropTarget ?? undefined,
    }),
    [snapshot, addChild, candidateMap, candidateErrors, candidateLoading, mutationBlocked, paramsEditingDisabled, runSubmittingNodeId, runSubmittingInputNodeIDs, saveNodeParams, startNodeRun, api.getScriptVersion, registerPromptFlush, registerTopicFlush, onNodeHandlesMeasured, applyNodeLayout, applyNodeCollapsed, locateNode, reconnectDropTarget],
  )

  useEffect(() => {
    setNodes((current) => preserveCanvasNodeViewState(model.nodes, current, projectId).map((node) => {
      const key = `${projectId}:${node.id}`
      let nextNode = node
      const collapsedOverride = pendingCollapsed.current.get(key)
      if (collapsedOverride !== undefined) {
        if (Boolean(node.data.node.collapsed) === collapsedOverride) {
          pendingCollapsed.current.delete(key)
        } else {
          nextNode = {
            ...node,
            data: { ...node.data, node: { ...node.data.node, collapsed: collapsedOverride } },
            style: {
              ...node.style,
              height: collapsedOverride ? COLLAPSED_NODE_HEIGHT : node.data.node.size.height,
            },
          }
        }
      }
      const pending = pendingPositions.current.get(key)
      if (pending && pending.x === nextNode.position.x && pending.y === nextNode.position.y) {
        pendingPositions.current.delete(key)
        lastPersistedPositions.current.set(key, { ...nextNode.position })
        lastPersistedSizes.current.set(key, { ...nextNode.data.node.size })
        return nextNode
      }
      const pendingLayout = pendingLayouts.current.get(key)
      if (pendingLayout?.x === nextNode.position.x && pendingLayout.y === nextNode.position.y
        && pendingLayout.width === nextNode.data.node.size.width && pendingLayout.height === nextNode.data.node.size.height) {
        pendingLayouts.current.delete(key)
        lastPersistedPositions.current.set(key, { ...nextNode.position })
        lastPersistedSizes.current.set(key, { ...nextNode.data.node.size })
        return nextNode
      }
      if (pendingLayout) {
        return {
          ...nextNode,
          position: { x: pendingLayout.x, y: pendingLayout.y },
          style: {
            ...nextNode.style,
            width: pendingLayout.width,
            height: nextNode.data.node.collapsed ? COLLAPSED_NODE_HEIGHT : pendingLayout.height,
          },
        }
      }
      if (pending) return { ...nextNode, position: pending }
      lastPersistedPositions.current.set(key, { ...nextNode.position })
      lastPersistedSizes.current.set(key, { ...nextNode.data.node.size })
      return nextNode
    }))
  }, [model, projectId])

  useEffect(() => {
    setConfirmIntent(null)
    confirmReturnFocus.current = null
    setRelationPopover(null)
    setReconnectDropTarget(null)
    setError('')
    pendingSyncRef.current = null
    paramsSaveQueue.current = Promise.resolve()
    setPendingSync(null)
    setPendingFocusNodeId('')
    setBusy(false)
    setConnectSubmitting(false)
    setParamsSaveCount(0)
    runRequestSequence.current += 1
    runStartInFlight.current = false
    setRunSubmittingNodeId('')
    createRequestId.current += 1
    createInFlight.current = false
    connectInFlight.current = false
    initialFit.current = { projectId, shouldFit: Boolean(projectId && snapshotNodeCount.current), done: false }
    if (projectId) void flowRef.current.setViewport({ x: 0, y: 0, zoom: 1 }, { duration: 0 })
    setConnectionTarget('')
    setReplacementSources({})
    setCreateMenu(null)
    resizingNodeIds.current.clear()
    for (const [key, timer] of moveTimers.current) {
      if (key.startsWith(`${projectId}:`)) continue
      window.clearTimeout(timer)
      moveTimers.current.delete(key)
      pendingPositions.current.delete(key)
      lastPersistedPositions.current.delete(key)
      lastPersistedSizes.current.delete(key)
      moveQueues.current.delete(key)
    }
    for (const key of pendingLayouts.current.keys()) {
      if (!key.startsWith(`${projectId}:`)) pendingLayouts.current.delete(key)
    }
    for (const key of pendingCollapsed.current.keys()) {
      if (!key.startsWith(`${projectId}:`)) pendingCollapsed.current.delete(key)
    }
    collapsingNodes.current.clear()
  }, [projectId])

  useEffect(() => () => {
    for (const timer of moveTimers.current.values()) window.clearTimeout(timer)
    moveTimers.current.clear()
  }, [])

  useEffect(() => {
    const fit = initialFit.current
    if (!fit.shouldFit || fit.done || fit.projectId !== projectId || !model.nodes.length) return
    let frame = 0
    let secondFrame = 0
    frame = scheduleFrame(() => {
      secondFrame = scheduleFrame(() => {
        if (currentProjectId.current !== projectId) return
        void flow.fitView({ padding: 0.28, duration: 0 })
        fit.done = true
      })
    })
    return () => {
      cancelScheduledFrame(frame)
      cancelScheduledFrame(secondFrame)
    }
  }, [flow, model.nodes.length, projectId])

  useEffect(() => {
    if (!pendingFocusNodeId || !model.nodes.some((node) => node.id === pendingFocusNodeId)) return
    const nodeId = pendingFocusNodeId
    let frame = 0
    let secondFrame = 0
    frame = scheduleFrame(() => {
      secondFrame = scheduleFrame(() => {
        if (currentProjectId.current !== projectId) return
        void flow.fitView({ nodes: [{ id: nodeId }], padding: 0.35, duration: 180 })
        setPendingFocusNodeId('')
      })
    })
    return () => {
      cancelScheduledFrame(frame)
      cancelScheduledFrame(secondFrame)
    }
  }, [flow, model.nodes, pendingFocusNodeId, projectId])

  const onNodesChange = useCallback((changes: NodeChange<CanvasFlowNode>[]) => {
    setNodes((current) => applyNodeChanges(changes, current))
    const positionChanges = new Map(changes.flatMap((change) => change.type === 'position' && change.position
      ? [[change.id, { x: change.position.x, y: change.position.y }] as const]
      : []))
    const resizeStarted = new Set(changes.flatMap((change) => change.type === 'dimensions' && change.resizing === true
      ? [change.id]
      : []))
    const layoutChanges = new Set(changes.flatMap((change) => change.type === 'dimensions' && change.resizing === false && change.dimensions
      ? [change.id]
      : []))
    for (const nodeId of resizeStarted) {
      resizingNodeIds.current.add(nodeId)
      const key = `${projectId}:${nodeId}`
      const timer = moveTimers.current.get(key)
      if (timer) {
        window.clearTimeout(timer)
        moveTimers.current.delete(key)
      }
      pendingPositions.current.delete(key)
    }
    for (const nodeId of layoutChanges) resizingNodeIds.current.delete(nodeId)
    for (const change of changes) {
      if (change.type === 'select' && change.selected) {
        setError('')
        void onSelectNode(change.id).catch((failure) => {
          setNodes((current) => current.map((node) => ({
            ...node,
            selected: node.id === snapshot.project?.activeNodeId,
          })))
          setError(`保存节点选择失败：${errorMessage(failure)}`)
        })
      }
      if (change.type === 'position' && change.position) {
        const key = `${projectId}:${change.id}`
        if (resizingNodeIds.current.has(change.id) || layoutChanges.has(change.id) || pendingLayouts.current.has(key)) continue
        const position = { x: change.position.x, y: change.position.y }
        pendingPositions.current.set(key, position)
        if (change.dragging !== true) schedulePositionSave(change.id, position)
      }
      if (change.type === 'dimensions' && change.resizing === false && change.dimensions) {
        const flowNode = nodes.find((node) => node.id === change.id)
        if (!flowNode) continue
        const key = `${projectId}:${change.id}`
        const position = positionChanges.get(change.id) ?? pendingPositions.current.get(key) ?? flowNode.position
        const layout = { x: position.x, y: position.y, width: change.dimensions.width, height: change.dimensions.height }
        pendingLayouts.current.set(key, layout)
        const timer = moveTimers.current.get(key)
        if (timer) {
          window.clearTimeout(timer)
          moveTimers.current.delete(key)
        }
        pendingPositions.current.delete(key)
        setNodes((current) => current.map((node) => node.id === change.id
          ? {
              ...node,
              position,
              style: { ...node.style, width: change.dimensions?.width, height: change.dimensions?.height },
            }
          : node))
        void saveNodeLayout(change.id, position.x, position.y, change.dimensions.width, change.dimensions.height)
      }
    }
  }, [nodes, onSelectNode, projectId, saveNodeLayout, schedulePositionSave, snapshot.project?.activeNodeId])

  const selectedNode = nodes.find((node) => node.selected)?.data.node
  const selectChain = useCallback((tailNodeId: string) => {
    setError('')
    if (snapshot.project?.activeNodeId === tailNodeId) return
    void onSelectNode(tailNodeId).catch((failure) => {
      setError(`保存链路选择失败：${errorMessage(failure)}`)
    })
  }, [onSelectNode, snapshot.project?.activeNodeId])
  const byId = useMemo(() => nodeMap(graphNodes), [graphNodes])
  const connectionTargets = graphNodes.filter((node) => inputHandleId(node) && node.id !== selectedNode?.id)
  const incomingEdges = (snapshot.edges ?? []).filter((edge) => edge.targetNodeId === selectedNode?.id)
  const handleEdgeClick = useCallback((event: ReactMouseEvent, edge: Edge) => {
    setError('')
    openEdgeRelationAt(edge.id, event.clientX, event.clientY, event.currentTarget)
  }, [openEdgeRelationAt])
  const edges = useMemo(() => buildFlowEdges(nodes, snapshot.edges ?? [], measuredHandleLayouts, {
    projectId,
    mutationsDisabled: mutationBlocked,
    scriptVersionSavingEdgeIDs,
    onReconnectTarget: reconnectTarget,
    onReconnectHoverTarget: handleReconnectHoverTarget,
    onOpenRelation: openEdgeRelationFromKeyboard,
  }), [nodes, snapshot.edges, measuredHandleLayouts, projectId, mutationBlocked, scriptVersionSavingEdgeIDs, reconnectTarget, handleReconnectHoverTarget, openEdgeRelationFromKeyboard])
  const relationEdge = relationPopover?.projectId === projectId
    ? (snapshot.edges ?? []).find((edge) => edge.id === relationPopover.edgeId)
    : undefined
  const relationSource = relationEdge ? byId.get(relationEdge.sourceNodeId) : undefined
  const relationTarget = relationEdge ? byId.get(relationEdge.targetNodeId) : undefined
  const relationLocked = Boolean(relationEdge && (
    (snapshot.lockedNodeIds ?? []).includes(relationEdge.sourceNodeId)
    || (snapshot.lockedNodeIds ?? []).includes(relationEdge.targetNodeId)
  ))
  const relationDisabled = mutationBlocked || relationLocked
  const relationVersionSaving = relationEdge ? scriptVersionSavingEdgeIDs.has(relationEdge.id) : false

  useEffect(() => {
    if (confirmIntent) confirmCancelRef.current?.focus()
  }, [confirmIntent])

  function dismissConfirmation(force = false) {
    if (connectSubmitting && !force) return
    setConfirmIntent(null)
    confirmReturnFocus.current?.focus()
    confirmReturnFocus.current = null
  }

  const openCreateMenuAt = useCallback((x: number, y: number) => {
    const flowPosition = flowRef.current.screenToFlowPosition({ x, y })
    setCreateMenu({ screenPosition: { x, y }, flowPosition })
  }, [])

  const openCreateMenuAtCenter = useCallback(() => {
    const rect = flowRoot.current?.getBoundingClientRect()
    if (!rect) return
    openCreateMenuAt(rect.left + rect.width / 2, rect.top + rect.height / 2)
  }, [openCreateMenuAt])

  const closeCreateMenu = useCallback((returnFocus: boolean) => {
    setCreateMenu(null)
    if (returnFocus) scheduleFrame(() => flowRoot.current?.focus({ preventScroll: true }))
  }, [])

  const focusCanvas = useCallback(() => {
    flowRoot.current?.focus({ preventScroll: true })
  }, [])

  const handlePaneContextMenu = useCallback((event: ReactMouseEvent | MouseEvent) => {
    event.preventDefault()
    openCreateMenuAt(event.clientX, event.clientY)
  }, [openCreateMenuAt])

  const handleCanvasKeyDown = useCallback((event: KeyboardEvent<HTMLDivElement>) => {
    if (createMenu || !(event.key === 'ContextMenu' || (event.shiftKey && event.key === 'F10'))) return
    const target = event.target instanceof Element ? event.target : null
    if (target?.closest('input, textarea, select, button, [contenteditable="true"], .react-flow__node, .react-flow__controls, .react-flow__panel')) return
    event.preventDefault()
    openCreateMenuAtCenter()
  }, [createMenu, openCreateMenuAtCenter])

  const handleCreateMenuSelect = useCallback((type: CanvasCreateType) => {
    const request = createMenu
    closeCreateMenu(true)
    if (!request || mutationBlockedRef.current) return
    if (type === 'topic') void createTopic(request.flowPosition, true)
    else void createNode(type, '', request.flowPosition, true)
  }, [closeCreateMenu, createMenu, createNode, createTopic])

  async function retryPendingSync() {
    if (!pendingSync || busy) return
    setBusy(true)
    setError('')
    await syncMutation(pendingSync.receipt, pendingSync.label, pendingSync.focusNodeId)
    setBusy(false)
  }

  function validConnection(connection: Edge | Connection): boolean {
    if (!connection.source || !connection.target) return false
    const parent = byId.get(connection.source)
    const child = byId.get(connection.target)
    if (!parent || !child || !connection.sourceHandle || !connection.targetHandle) return false
    if (model.nodes.find((node) => node.id === parent.id)?.data.runningLocked) return false
    if (model.nodes.find((node) => node.id === child.id)?.data.runningLocked) return false
    return true
  }

  async function submitConnection(sourceId: string, targetId: string) {
    if (connectInFlight.current || mutationBlockedRef.current) return
    connectInFlight.current = true
    setConnectSubmitting(true)
    const ownerProjectId = projectId
    setError('')
    try {
      const receipt = await api.connect(sourceId, targetId, ownerProjectId)
      if (currentProjectId.current !== ownerProjectId || receipt.projectId !== ownerProjectId) return
      await syncMutation(receipt, '连接')
    } catch (failure) {
      if (currentProjectId.current === ownerProjectId) setError(errorMessage(failure))
    } finally {
      connectInFlight.current = false
      setConnectSubmitting(false)
    }
  }

  function requestConnectionEdit(intent: ConnectionEditIntent) {
    if (mutationBlockedRef.current) return
    confirmReturnFocus.current = document.activeElement instanceof HTMLElement ? document.activeElement : null
    setConfirmIntent(intent)
  }

  async function submitConnectionEdit() {
    if (!confirmIntent || connectInFlight.current || mutationBlockedRef.current) return
    const intent = confirmIntent
    connectInFlight.current = true
    setConnectSubmitting(true)
    setError('')
    try {
      const receipt = intent.mode === 'disconnect'
        ? await api.disconnect(intent.edgeId, intent.projectId)
        : await api.replaceConnection(intent.edgeId, intent.nextSourceId, intent.projectId)
      if (currentProjectId.current !== intent.projectId || receipt.projectId !== intent.projectId) return
      dismissConfirmation(true)
      await syncMutation(receipt, intent.mode === 'disconnect' ? '断开连接' : '改挂连接')
    } catch (failure) {
      if (currentProjectId.current !== intent.projectId) return
      dismissConfirmation(true)
      setError(errorMessage(failure))
    } finally {
      connectInFlight.current = false
      setConnectSubmitting(false)
    }
  }

  const onConnect = useCallback((connection: Connection) => {
    if (mutationBlocked || !validConnection(connection) || !connection.source || !connection.target) {
      if (!mutationBlocked) setError('连接目标不合法，工程图未修改。')
      return
    }
    void submitConnection(connection.source, connection.target)
  }, [model, mutationBlocked, projectId, snapshot])

  const onNodeClick: NodeMouseHandler<CanvasFlowNode> = () => {
    setError('')
  }

  async function connectSelectedTarget() {
    if (mutationBlocked || !selectedNode || !connectionTarget) return
    const selectedFlowNode = nodes.find((node) => node.id === selectedNode.id)
    if (!selectedFlowNode || selectedFlowNode.data.runningLocked) return
    const target = byId.get(connectionTarget)
    const sourceHandle = outputHandleId(selectedNode)
    const targetHandle = target ? inputHandleId(target) : null
    if (!sourceHandle || !targetHandle || !validConnection({
      source: selectedNode.id,
      sourceHandle,
      target: connectionTarget,
      targetHandle,
    })) {
      setError('连接目标不合法或位于运行锁路径中，工程图未修改。')
      return
    }
    await submitConnection(selectedNode.id, connectionTarget)
  }

  const selectedCanParent = selectedNode
    && Boolean(outputHandleId(selectedNode))
    && !model.nodes.find((node) => node.id === selectedNode.id)?.data.runningLocked

  return (
    <section
      className="workspace-canvas"
      aria-label="节点画布"
      data-project-id={snapshot.project?.id ?? ''}
      data-project-revision={snapshot.project?.revision ?? 0}
      data-node-count={graphNodes.length}
    >
      <div className="canvas-toolbar" role="toolbar" aria-label="画布操作">
        <div className="canvas-toolbar-title">
          <span className="workspace-kicker">工作台 / 节点图</span>
          <strong>{snapshot.project?.name}</strong>
        </div>
        {selectedCanParent && connectionTargets.length ? (
          <div className="canvas-keyboard-connect">
            <label htmlFor="connection-target">新增连接</label>
            <select id="connection-target" value={connectionTarget} onChange={(event) => setConnectionTarget(event.currentTarget.value)}>
              <option value="">选择节点目标</option>
              {connectionTargets.map((target) => (
                <option key={target.id} value={target.id} disabled={(snapshot.lockedNodeIds ?? []).includes(target.id)}>
                  {target.title}
                </option>
              ))}
            </select>
            <button type="button" className="canvas-toolbar-btn" onClick={() => void connectSelectedTarget()} disabled={mutationBlocked || !connectionTarget}>
              将目标连接到选中节点
            </button>
          </div>
        ) : null}
        <span className="canvas-toolbar-spacer" />
        <button type="button" className="canvas-toolbar-btn canvas-reset-btn" onClick={() => void flow.fitView({ padding: 0.28, duration: 180 })}>
          重置视图
        </button>
      </div>

      {selectedNode && incomingEdges.length ? (
        <details className="canvas-input-relations" key={`${projectId}:${selectedNode.id}`}>
          <summary>{selectedNode.title} 的输入关系 · {incomingEdges.length}</summary>
          <ul aria-label={`${selectedNode.title} 的输入关系`}>
            {incomingEdges.map((edge) => {
              const source = byId.get(edge.sourceNodeId)
              const sourceTitle = source?.title ?? edge.sourceNodeId
              const targetLocked = model.nodes.find((node) => node.id === selectedNode.id)?.data.runningLocked
              const sourceLocked = model.nodes.find((node) => node.id === edge.sourceNodeId)?.data.runningLocked
              const disabled = mutationBlocked || targetLocked || sourceLocked
              const versionSaving = scriptVersionSavingEdgeIDs.has(edge.id)
              const replacement = replacementSources[edge.id] ?? ''
              return (
                <li key={edge.id}>
                  <span className="canvas-input-source">{sourceTitle} → {selectedNode.title}</span>
                  {edge.kind === 'candidate_source' ? (
                    <span className="canvas-input-protected">候选生成来源 · 不可更改</span>
                  ) : (
                    <div className="canvas-input-actions">
                      {source?.type === 'script' ? (
                        <label>
                          固定版本
                          <select
                            aria-label={`为 ${sourceTitle} → ${selectedNode.title} 固定 Script 版本`}
                            value={edge.scriptVersion ?? 0}
                            disabled={disabled || versionSaving}
                            onChange={(event) => void updateEdgeScriptVersion(edge, Number(event.currentTarget.value))}
                          >
                            <option value={0}>未绑定</option>
                            {source.scriptVersions?.map((version) => <option key={version.n} value={version.n}>v{version.n}</option>)}
                          </select>
                        </label>
                      ) : null}
                      <label>
                        新上游
                        <select
                          aria-label={`为 ${sourceTitle} 到 ${selectedNode.title} 选择新上游`}
                          value={replacement}
                          disabled={disabled}
                          onChange={(event) => {
                            const value = event.currentTarget.value
                            setReplacementSources((current) => ({ ...current, [edge.id]: value }))
                          }}
                        >
                          <option value="">选择节点</option>
                          {graphNodes.filter((node) => node.id !== selectedNode.id && node.id !== edge.sourceNodeId).map((node) => (
                            <option key={node.id} value={node.id} disabled={(snapshot.lockedNodeIds ?? []).includes(node.id)}>{node.title}</option>
                          ))}
                        </select>
                      </label>
                      <button
                        type="button"
                        className="canvas-toolbar-btn"
                        disabled={disabled || !replacement}
                        aria-label={`改挂 ${sourceTitle} 到 ${selectedNode.title}`}
                        onClick={() => requestConnectionEdit({ projectId, edgeId: edge.id, sourceId: edge.sourceNodeId, targetId: edge.targetNodeId, mode: 'replace', nextSourceId: replacement })}
                      >改挂</button>
                      <button
                        type="button"
                        className="canvas-toolbar-btn"
                        disabled={disabled}
                        aria-label={`断开 ${sourceTitle} 到 ${selectedNode.title}`}
                        onClick={() => requestConnectionEdit({ projectId, edgeId: edge.id, sourceId: edge.sourceNodeId, targetId: edge.targetNodeId, mode: 'disconnect' })}
                      >断开</button>
                    </div>
                  )}
                </li>
              )
            })}
          </ul>
        </details>
      ) : null}

      <div
        className="canvas-flow-root"
        ref={flowRoot}
        tabIndex={0}
        aria-label="画布；按 Shift+F10 或菜单键打开新建菜单"
        onKeyDown={handleCanvasKeyDown}
      >
        {error || workspaceSyncError ? (
          <div className="canvas-message canvas-message-error" role="alert">
            <span>{error || `快照同步失败：${workspaceSyncError?.message || '未知错误'}`}</span>
            {pendingSync ? <button type="button" className="canvas-retry-sync" onClick={() => void retryPendingSync()} disabled={busy}>仅重试快照同步</button> : null}
            {workspaceSyncError ? <button type="button" className="canvas-retry-sync" onClick={workspaceSyncError.retry}>重试</button> : null}
            {error ? <button type="button" onClick={() => setError('')} aria-label="关闭错误">×</button> : null}
          </div>
        ) : null}
        {!graphNodes.length ? (
          <div className="canvas-empty-overlay" aria-label="空画布提示">
            <span aria-hidden="true">＋</span>
            <strong>从一个选题开始</strong>
            <p>在空白处右键，或按 Shift+F10 新建节点。</p>
          </div>
        ) : null}
        <ReactFlow<CanvasFlowNode>
          nodes={nodes}
          edges={edges}
          nodeTypes={nodeTypes}
          edgeTypes={edgeTypes}
          onNodesChange={onNodesChange}
          onConnect={onConnect}
          onPaneClick={focusCanvas}
          onPaneContextMenu={handlePaneContextMenu}
          isValidConnection={validConnection}
          onNodeClick={onNodeClick}
          onEdgeClick={handleEdgeClick}
          onNodeDoubleClick={(_event, node) => void flow.fitView({ nodes: [{ id: node.id }], padding: 0.35, duration: 180 })}
          onError={(code, message) => setError(`画布渲染错误 ${code}：${message}`)}
          minZoom={0.6}
          maxZoom={1.6}
          nodesDraggable={!mutationBlocked}
          nodesConnectable={!mutationBlocked}
          nodesFocusable
          edgesFocusable={false}
          elementsSelectable
          deleteKeyCode={null}
          panOnDrag
          zoomOnDoubleClick={false}
          defaultEdgeOptions={{ type: 'workspace', focusable: false, selectable: false }}
          connectionLineComponent={CanvasConnectionLine}
          proOptions={{ hideAttribution: false }}
          aria-label="工作台节点画布；使用鼠标平移与缩放，双击节点聚焦。"
        >
          <Background variant={BackgroundVariant.Dots} gap={22} size={2} patternClassName="canvas-dot-pattern" style={{ zIndex: 0 }} />
          <Controls showInteractive={false} position="bottom-right" />
          <Panel position="bottom-left" className="canvas-legend">
            <span><i className="legend-mark path-mark" />选中路径</span>
            <span><i className="legend-mark lock-mark" />运行锁</span>
            <span>滚轮缩放 · 拖动画布平移 · 双击节点聚焦</span>
          </Panel>
          <Panel position="bottom-center" className="canvas-quickbar-panel">
            <ChainQuickBar
              chains={snapshot.chains ?? []}
              nodes={graphNodes}
              activeNodeId={snapshot.project?.activeNodeId ?? ''}
              onSelectNode={selectChain}
            />
          </Panel>
        </ReactFlow>
        {relationPopover && relationEdge && relationSource && relationTarget ? (
          <section
            ref={relationPopoverRef}
            className="canvas-edge-relation-popover"
            role="dialog"
            aria-labelledby="canvas-edge-relation-title"
            style={{ left: relationPopover.x, top: relationPopover.y }}
            onPointerDown={(event) => event.stopPropagation()}
            onClick={(event) => event.stopPropagation()}
            onKeyDown={(event) => {
              if (event.key !== 'Escape') return
              event.preventDefault()
              closeEdgeRelation(true)
            }}
          >
            <div className="canvas-edge-relation-head">
              <strong id="canvas-edge-relation-title">{relationSource.title} → {relationTarget.title}</strong>
              <button ref={relationCloseRef} type="button" className="canvas-toolbar-btn" aria-label="关闭连接操作" onClick={() => closeEdgeRelation(true)}>×</button>
            </div>
            {relationEdge.kind === 'candidate_source' ? (
              <p>候选生成来源 · 不可断开或改接。</p>
            ) : (
              <>
                {relationSource.type === 'script' ? (
                  <label>
                    固定版本
                    <select
                      aria-label={`为 ${relationSource.title} → ${relationTarget.title} 固定 Script 版本`}
                      value={relationEdge.scriptVersion ?? 0}
                      disabled={relationDisabled || relationVersionSaving}
                      onChange={(event) => void updateEdgeScriptVersion(relationEdge, Number(event.currentTarget.value))}
                    >
                      <option value={0}>未绑定</option>
                      {relationSource.scriptVersions?.map((version) => <option key={version.n} value={version.n}>v{version.n}</option>)}
                    </select>
                  </label>
                ) : null}
                <div className="canvas-edge-relation-actions">
                  {relationLocked ? <span>运行锁定中</span> : null}
                  <button
                    type="button"
                    className="canvas-toolbar-btn"
                    disabled={relationDisabled}
                    aria-label={`断开 ${relationSource.title} 到 ${relationTarget.title}`}
                    onClick={() => requestConnectionEdit({
                      projectId,
                      edgeId: relationEdge.id,
                      sourceId: relationEdge.sourceNodeId,
                      targetId: relationEdge.targetNodeId,
                      mode: 'disconnect',
                    })}
                  >断开连接</button>
                </div>
              </>
            )}
          </section>
        ) : null}
      </div>
      {createMenu ? (
        <CanvasContextMenu
          screenPosition={createMenu.screenPosition}
          disabled={mutationBlocked}
          onCreate={handleCreateMenuSelect}
          onClose={closeCreateMenu}
        />
      ) : null}

      {confirmIntent ? (
        <div className="canvas-confirm-backdrop" role="presentation" onMouseDown={(event) => {
          if (event.target === event.currentTarget && !connectSubmitting) dismissConfirmation()
        }}>
          <section className="canvas-confirm-dialog" role="dialog" aria-modal="true" aria-labelledby="connection-edit-title" onKeyDown={(event) => {
            if (event.key === 'Escape') {
              event.preventDefault()
              if (!connectSubmitting) dismissConfirmation()
              return
            }
            if (event.key !== 'Tab') return
            const focusable = [...event.currentTarget.querySelectorAll<HTMLElement>('button:not(:disabled), [href], input:not(:disabled), select:not(:disabled), [tabindex]:not([tabindex="-1"])')]
            if (!focusable.length) {
              event.preventDefault()
              return
            }
            const first = focusable[0]
            const last = focusable[focusable.length - 1]
            if (event.shiftKey && document.activeElement === first) {
              event.preventDefault()
              last.focus()
            } else if (!event.shiftKey && document.activeElement === last) {
              event.preventDefault()
              first.focus()
            }
          }}>
            <h2 id="connection-edit-title">{confirmIntent.mode === 'disconnect' ? '确认断开连接' : '确认改挂连接'}</h2>
            <p>
              {confirmIntent.mode === 'disconnect' ? '断开' : '改挂'} <strong>{byId.get(confirmIntent.sourceId)?.title ?? confirmIntent.sourceId}</strong> → <strong>{byId.get(confirmIntent.targetId)?.title ?? confirmIntent.targetId}</strong>
              {confirmIntent.mode === 'replace' ? <>，将上游改为 <strong>{byId.get(confirmIntent.nextSourceId)?.title ?? confirmIntent.nextSourceId}</strong></> : null}。
            </p>
            <ul>
              <li>只修改所选普通连接，其他输入与候选生成来源不变，不自动重跑。</li>
              {confirmIntent.mode === 'disconnect' ? (
                <li>断开不移动或删除工作区文件；工作区与当前路径不匹配时不能运行。</li>
              ) : (
                <li>工作区迁移、运行锁与图约束由服务端校验；不满足安全条件时不会改挂。</li>
              )}
            </ul>
            <div className="canvas-confirm-actions">
              <button ref={confirmCancelRef} type="button" className="canvas-toolbar-btn" disabled={connectSubmitting} onClick={() => dismissConfirmation()}>取消</button>
              <button
                type="button"
                className="canvas-toolbar-btn canvas-confirm-primary"
                disabled={confirmIntent.projectId !== projectId || connectSubmitting || Boolean(pendingSync)}
                onClick={() => void submitConnectionEdit()}
              >
                {connectSubmitting ? '提交中…' : confirmIntent.mode === 'disconnect' ? '确认断开' : '确认改挂'}
              </button>
            </div>
          </section>
        </div>
      ) : null}
    </section>
  )
}

export function WorkspaceCanvas({ snapshot, onSelectNode, onGraphMutation, onRegisterDraftFlush, onRegisterNodeLocator, api = graphAPI, candidateFixtures, onStartRun = startRunAPI, workspaceSyncError }: Props) {
  return (
    <ReactFlowProvider>
      <CanvasBody snapshot={snapshot} onSelectNode={onSelectNode} onGraphMutation={onGraphMutation} onRegisterDraftFlush={onRegisterDraftFlush} onRegisterNodeLocator={onRegisterNodeLocator} api={api} candidateFixtures={candidateFixtures} onStartRun={onStartRun} workspaceSyncError={workspaceSyncError} />
    </ReactFlowProvider>
  )
}
