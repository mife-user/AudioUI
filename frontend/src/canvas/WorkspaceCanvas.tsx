import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
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
import type { GraphMutationReceipt, Node, NodeParams, RunStartResult, WorkspaceSnapshot } from '../../bindings/audio-ui/internal/domain/entity'
import { startRun as startRunAPI } from '../api/runs'
import { graphAPI, type GraphAPI, type TopicCandidateFixture } from './graphApi'
import { allocateNodePosition } from './nodePosition'
import { buildFlowEdges, buildFlowModel, candidateAt, nodeMap, subtreeSize, topicRootId, type CanvasFlowNode } from './flowModel'
import { preserveCanvasNodeViewState } from './nodeState'
import { PromptNode } from './PromptNode'
import { TopicNode } from './TopicNode'

const nodeTypes = { topic: TopicNode, prompt: PromptNode }
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
  api?: GraphAPI
  candidateFixtures?: Record<string, TopicCandidateFixture[]>
  onStartRun?: (nodeId: string, projectId: string) => Promise<RunStartResult>
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

type ConnectIntent = {
  projectId: string
  parentId: string
  childId: string
  candidate: number | null
  oldParentId: string
  oldCandidate: number | null
  subtreeCount: number
  movesTree: boolean
}

function errorMessage(error: unknown): string {
  if (error instanceof Error) return error.message
  if (typeof error === 'string') return error
  if (error && typeof error === 'object' && 'message' in error) return String((error as { message: unknown }).message)
  return String(error)
}

function candidateFromHandle(handleId: string | null | undefined, parent: Node): number | null {
  if (parent.type !== 'topic') return null
  const match = /^candidate-(\d+)$/.exec(handleId ?? '')
  return match ? Number(match[1]) : null
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

function CanvasBody({ snapshot, onSelectNode, onGraphMutation, api, candidateFixtures, onStartRun }: Props & { api: GraphAPI }) {
  const projectId = snapshot.project?.id ?? ''
  const graphNodes = snapshot.nodes ?? []
  const [candidateState, setCandidateState] = useState<CandidateReadState>({ projectId: '', entries: {} })
  const [measuredHandleState, setMeasuredHandleState] = useState<MeasuredHandleState>({ projectId: '', byNode: {} })
  const candidateStateRef = useRef(candidateState)
  candidateStateRef.current = candidateState
  const currentProjectId = useRef(projectId)
  currentProjectId.current = projectId
  const onNodeHandlesMeasured = useCallback((nodeId: string, layoutKey: string, ownerProjectId: string) => {
    if (!ownerProjectId || currentProjectId.current !== ownerProjectId) return
    setMeasuredHandleState((current) => {
      const byNode = current.projectId === ownerProjectId ? current.byNode : {}
      if (byNode[nodeId] === layoutKey) return current
      return { projectId: ownerProjectId, byNode: { ...byNode, [nodeId]: layoutKey } }
    })
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
  const lastPersistedPositions = useRef(new Map<string, { x: number; y: number }>())
  const pendingPositions = useRef(new Map<string, PendingPosition>())
  const initialFit = useRef({ projectId: '', shouldFit: false, done: false })
  const createRequestId = useRef(0)
  const [nodes, setNodes] = useState<CanvasFlowNode[]>([])
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [busy, setBusy] = useState(false)
  const [connectSubmitting, setConnectSubmitting] = useState(false)
  const [paramsSaveCount, setParamsSaveCount] = useState(0)
  const [runSubmittingNodeId, setRunSubmittingNodeId] = useState('')
  const [pendingSync, setPendingSync] = useState<PendingSync | null>(null)
  const [pendingFocusNodeId, setPendingFocusNodeId] = useState('')
  const [confirmIntent, setConfirmIntent] = useState<ConnectIntent | null>(null)
  const [connectionTarget, setConnectionTarget] = useState('')
  const [candidateSelection, setCandidateSelection] = useState('0')
  const createInFlight = useRef(false)
  const connectInFlight = useRef(false)
  const runStartInFlight = useRef(false)
  const runRequestSequence = useRef(0)
  const hasActiveRun = (snapshot.runs ?? []).some((run) => run.status === 'starting' || run.status === 'running')
  const globalRunBusy = hasActiveRun || Boolean(runSubmittingNodeId)
  const globalRunBusyRef = useRef(globalRunBusy)
  globalRunBusyRef.current = globalRunBusy
  const mutationBlocked = busy || connectSubmitting || paramsSaveCount > 0 || pendingSync !== null
  const mutationBlockedRef = useRef(mutationBlocked)
  mutationBlockedRef.current = mutationBlocked
  const runStartBlockedRef = useRef(busy || connectSubmitting || pendingSync !== null)
  runStartBlockedRef.current = busy || connectSubmitting || pendingSync !== null
  const confirmCancelRef = useRef<HTMLButtonElement>(null)
  const confirmReturnFocus = useRef<HTMLElement | null>(null)

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
    setNotice(`${label}已提交，正在同步工程图。`)
    setError('')
    try {
      await onGraphMutation?.(receipt)
      if (currentProjectId.current !== receipt.projectId) return false
      setPendingSync(null)
      setNotice(`${label}已同步。`)
      if (focusNodeId) setPendingFocusNodeId(focusNodeId)
      return true
    } catch (failure) {
      if (currentProjectId.current !== receipt.projectId) return false
      setNotice('')
      setPendingSync({ receipt, label, focusNodeId })
      setError(`${label}已保存，但画布同步失败：${errorMessage(failure)}。请重试同步，不要重复创建。`)
      return false
    }
  }, [onGraphMutation])

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

  const addChild = useCallback((parentId: string, candidate: number | null) => {
    if (mutationBlockedRef.current) return
    const parent = graphNodes.find((node) => node.id === parentId)
    if (!parent) return
    const position = allocateNodePosition(graphNodes, {
      x: parent.position.x + 380,
      y: parent.position.y + (candidate ?? 0) * 155,
    })
    void addPrompt({ parentId, candidate, position })
  }, [api, graphNodes, projectId, syncMutation])

  const saveTopicParams = useCallback(async (nodeId: string, title: string, params: NodeParams, ownerProjectId: string) => {
    if (currentProjectId.current !== ownerProjectId) throw new Error('PROJECT_CHANGED: 工程已切换，未保存旧工程参数。')
    setParamsSaveCount((count) => count + 1)
    try {
      const receipt = await api.updateNodeParams(nodeId, title, params, ownerProjectId)
      if (receipt.projectId !== ownerProjectId) throw new Error('PROJECT_CHANGED: 参数保存回执所属工程不一致。')
      if (currentProjectId.current !== ownerProjectId) return false
      return await syncMutation(receipt, '选题参数')
    } finally {
      setParamsSaveCount((count) => Math.max(0, count - 1))
    }
  }, [api, syncMutation])

  const startTopicRun = useCallback(async (nodeId: string, ownerProjectId: string): Promise<RunStartResult> => {
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
      return await (onStartRun ?? startRunAPI)(nodeId, ownerProjectId)
    } catch (failure) {
      return { error: { code: 'RUN_START_FAILED', message: errorMessage(failure) } }
    } finally {
      if (runRequestSequence.current === requestId) {
        runStartInFlight.current = false
        setRunSubmittingNodeId('')
      }
    }
  }, [onStartRun, projectId])

  const model = useMemo(
    () => buildFlowModel(snapshot, addChild, candidateMap, mutationBlocked, {
      runSubmittingNodeId,
      candidateErrors,
      candidateLoading,
      onUpdateTopicParams: saveTopicParams,
      onStartTopicRun: startTopicRun,
      onNodeHandlesMeasured,
    }),
    [snapshot, addChild, candidateMap, candidateErrors, candidateLoading, mutationBlocked, runSubmittingNodeId, saveTopicParams, startTopicRun, onNodeHandlesMeasured],
  )

  useEffect(() => {
    setNodes((current) => preserveCanvasNodeViewState(model.nodes, current, projectId).map((node) => {
      const key = `${projectId}:${node.id}`
      const pending = pendingPositions.current.get(key)
      if (pending && pending.x === node.position.x && pending.y === node.position.y) {
        pendingPositions.current.delete(key)
        lastPersistedPositions.current.set(key, { ...node.position })
        return node
      }
      if (pending) return { ...node, position: pending }
      lastPersistedPositions.current.set(key, { ...node.position })
      return node
    }))
  }, [model, projectId])

  useEffect(() => {
    setConfirmIntent(null)
    confirmReturnFocus.current = null
    setError('')
    setNotice('')
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
    setCandidateSelection('0')
    for (const [key, timer] of moveTimers.current) {
      if (key.startsWith(`${projectId}:`)) continue
      window.clearTimeout(timer)
      moveTimers.current.delete(key)
      pendingPositions.current.delete(key)
      lastPersistedPositions.current.delete(key)
      moveQueues.current.delete(key)
    }
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
    for (const change of changes) {
      if (change.type === 'select' && change.selected) {
        setError('')
        setNotice('')
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
        const position = { x: change.position.x, y: change.position.y }
        pendingPositions.current.set(key, position)
        if (change.dragging !== true) schedulePositionSave(change.id, position)
      }
    }
  }, [onSelectNode, projectId, schedulePositionSave, snapshot.project?.activeNodeId])

  const selectedNode = nodes.find((node) => node.selected)?.data.node
  const byId = useMemo(() => nodeMap(graphNodes), [graphNodes])
  const connectionTargets = graphNodes.filter((node) => node.type === 'prompt' && node.id !== selectedNode?.id)
  const selectedCandidates = selectedNode?.type === 'topic'
    ? candidateMap[selectedNode.id] ?? []
    : []
  const edges = useMemo(() => buildFlowEdges(nodes, measuredHandleLayouts), [nodes, measuredHandleLayouts])

  useEffect(() => {
    setCandidateSelection('0')
  }, [selectedNode?.id])

  useEffect(() => {
    if (confirmIntent) confirmCancelRef.current?.focus()
  }, [confirmIntent])

  function dismissConfirmation(force = false) {
    if (connectSubmitting && !force) return
    setConfirmIntent(null)
    confirmReturnFocus.current?.focus()
    confirmReturnFocus.current = null
  }

  function canvasPosition() {
    const rect = flowRoot.current?.getBoundingClientRect()
    if (!rect) return { x: 0, y: 0 }
    return flow.screenToFlowPosition({
      x: rect.left + rect.width / 2,
      y: rect.top + rect.height / 2,
    })
  }

  async function addPrompt(input: { parentId?: string; candidate?: number | null; position: { x: number; y: number } }) {
    if (mutationBlockedRef.current || createInFlight.current) return
    createInFlight.current = true
    const requestId = ++createRequestId.current
    setBusy(true)
    setError('')
    setNotice('')
    try {
      const receipt = await api.addNode({
        projectId,
        type: 'prompt',
        title: '新提示词',
        parent: input.parentId ?? '',
        parentCandidate: input.candidate ?? null,
        position: allocateNodePosition(graphNodes, input.position),
        params: {},
      })
      if (currentProjectId.current !== projectId || receipt.projectId !== projectId) return
      await syncMutation(receipt, input.parentId ? '提示词' : '草稿提示词', receipt.nodeId)
    } catch (failure) {
      if (currentProjectId.current !== projectId) return
      setNotice('')
      setError(`创建提示词失败：${errorMessage(failure)}`)
    } finally {
      if (createRequestId.current === requestId) {
        createInFlight.current = false
        setBusy(false)
      }
    }
  }

  async function addTopic() {
    if (mutationBlockedRef.current || createInFlight.current) return
    createInFlight.current = true
    const requestId = ++createRequestId.current
    setBusy(true)
    setError('')
    setNotice('')
    try {
      const receipt = await api.addNode({
        projectId,
        type: 'topic',
        title: '新选题',
        parent: '',
        parentCandidate: null,
        position: allocateNodePosition(graphNodes, canvasPosition(), 'topic'),
        params: { candidateCount: 3 },
      })
      if (currentProjectId.current !== projectId || receipt.projectId !== projectId) return
      await syncMutation(receipt, '选题节点', receipt.nodeId)
    } catch (failure) {
      if (currentProjectId.current !== projectId) return
      setNotice('')
      setError(`创建选题节点失败：${errorMessage(failure)}`)
    } finally {
      if (createRequestId.current === requestId) {
        createInFlight.current = false
        setBusy(false)
      }
    }
  }

  async function retryPendingSync() {
    if (!pendingSync || busy) return
    setBusy(true)
    setError('')
    await syncMutation(pendingSync.receipt, pendingSync.label, pendingSync.focusNodeId)
    setBusy(false)
  }

  function validConnection(connection: Edge | Connection): boolean {
    if (!connection.source || !connection.target || connection.source === connection.target) return false
    const parent = byId.get(connection.source)
    const child = byId.get(connection.target)
    if (!parent || !child || child.type !== 'prompt' || parent.type !== 'topic' && !parent.parent) return false
    if (model.nodes.find((node) => node.id === parent.id)?.data.runningLocked) return false
    if (model.nodes.find((node) => node.id === child.id)?.data.runningLocked) return false
    if (parent.type === 'topic') {
      const candidate = candidateFromHandle(connection.sourceHandle, parent)
      return candidate !== null && Boolean(candidateAt(candidateMap[parent.id], candidate))
    }
    return connection.sourceHandle === 'prompt-output'
  }

  async function submitConnection(parentId: string, childId: string, candidate: number | null, replace: boolean) {
    if (connectInFlight.current || mutationBlockedRef.current) return
    connectInFlight.current = true
    setConnectSubmitting(true)
    const ownerProjectId = projectId
    setError('')
    setNotice('')
    try {
      const receipt = await api.connect(parentId, childId, candidate, replace, ownerProjectId)
      if (currentProjectId.current !== ownerProjectId || receipt.projectId !== ownerProjectId) return
      if (replace) dismissConfirmation(true)
      await syncMutation(receipt, '连接')
    } catch (failure) {
      if (currentProjectId.current !== ownerProjectId) return
      const message = errorMessage(failure)
      if (!replace && message.includes('REPARENT_CONFIRM_REQUIRED')) {
        confirmReturnFocus.current = document.activeElement instanceof HTMLElement ? document.activeElement : null
        const child = byId.get(childId)
        const oldParent = child?.parent ? byId.get(child.parent) : undefined
        const oldRoot = child?.parent ? topicRootId(graphNodes, child.id) : null
        const newRoot = topicRootId(graphNodes, parentId)
        setConfirmIntent({
          projectId: ownerProjectId,
          parentId,
          childId,
          candidate,
          oldParentId: oldParent?.id ?? '',
          oldCandidate: child?.parentCandidate ?? null,
          subtreeCount: subtreeSize(graphNodes, childId),
          movesTree: Boolean(oldRoot && newRoot && oldRoot !== newRoot),
        })
        return
      }
      if (replace) dismissConfirmation(true)
      setNotice('')
      setError(message)
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
    const parent = byId.get(connection.source)
    if (!parent) return
    const candidate = candidateFromHandle(connection.sourceHandle, parent)
    void submitConnection(connection.source, connection.target, candidate, false)
  }, [byId, candidateMap, model, mutationBlocked, projectId, snapshot])

  const onNodeClick: NodeMouseHandler<CanvasFlowNode> = () => {
    setError('')
    setNotice('')
  }

  async function connectSelectedTarget() {
    if (mutationBlocked || !selectedNode || !connectionTarget) return
    const selectedFlowNode = nodes.find((node) => node.id === selectedNode.id)
    if (!selectedFlowNode || selectedFlowNode.data.runningLocked) return
    const candidate = selectedNode.type === 'topic'
      ? Number(candidateSelection)
      : null
    const resolvedCandidate = Number.isInteger(candidate) ? candidate : null
    if (selectedNode.type === 'topic' && resolvedCandidate === null) {
      setError('请先选择一个可用的候选输出端口。')
      return
    }
    const sourceHandle = selectedNode.type === 'topic' ? `candidate-${resolvedCandidate}` : 'prompt-output'
    if (!validConnection({ source: selectedNode.id, sourceHandle, target: connectionTarget, targetHandle: 'prompt-input' })) {
      setError('连接目标不合法或位于运行锁路径中，工程图未修改。')
      return
    }
    await submitConnection(selectedNode.id, connectionTarget, resolvedCandidate, false)
  }

  const selectedCanParent = selectedNode
    && !model.nodes.find((node) => node.id === selectedNode.id)?.data.runningLocked
    && (selectedNode.type === 'prompt' && Boolean(selectedNode.parent) || selectedNode.type === 'topic' && selectedCandidates.length > 0)

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
        <button type="button" className="canvas-toolbar-btn" onClick={addTopic} disabled={mutationBlocked}>
          ＋ 新选题
        </button>
        <button type="button" className="canvas-toolbar-btn" onClick={() => void addPrompt({ position: canvasPosition() })} disabled={mutationBlocked}>
          ＋ 草稿提示词
        </button>
        {selectedCanParent && connectionTargets.length ? (
          <div className="canvas-keyboard-connect">
            <label htmlFor="connection-target">连接 / 改挂</label>
            <select id="connection-target" value={connectionTarget} onChange={(event) => setConnectionTarget(event.currentTarget.value)}>
              <option value="">选择提示词目标</option>
              {connectionTargets.map((target) => (
                <option key={target.id} value={target.id} disabled={(snapshot.lockedNodeIds ?? []).includes(target.id)}>
                  {target.title}{target.parent ? ' · 改挂' : ' · 草稿'}
                </option>
              ))}
            </select>
            {selectedNode?.type === 'topic' && selectedCandidates.length ? (
              <select value={candidateSelection} onChange={(event) => setCandidateSelection(event.currentTarget.value)} aria-label="选择候选输出端口">
                {selectedCandidates.map((candidate, index) => {
                  const candidateIndex = candidate.index ?? index
                  return <option key={`${selectedNode.id}-${candidateIndex}`} value={candidateIndex}>候选 {candidateIndex + 1} · {candidate.title}</option>
                })}
              </select>
            ) : null}
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

      {error ? (
        <div className="canvas-message canvas-message-error" role="alert">
          <span>{error}</span>
          {pendingSync ? <button type="button" className="canvas-retry-sync" onClick={() => void retryPendingSync()} disabled={busy}>仅重试快照同步</button> : null}
          <button type="button" onClick={() => setError('')} aria-label="关闭错误">×</button>
        </div>
      ) : null}
      {notice ? <div className="canvas-message" role="status">{notice}</div> : null}

      <div className="canvas-flow-root" ref={flowRoot}>
        {!graphNodes.length ? (
          <div className="canvas-empty-overlay" aria-label="空画布提示">
            <span aria-hidden="true">＋</span>
            <strong>从一个选题开始</strong>
            <p>新建选题，或先放置未连接的提示词草稿。</p>
          </div>
        ) : null}
        <ReactFlow<CanvasFlowNode>
          nodes={nodes}
          edges={edges}
          nodeTypes={nodeTypes}
          onNodesChange={onNodesChange}
          onConnect={onConnect}
          isValidConnection={validConnection}
          onNodeClick={onNodeClick}
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
          defaultEdgeOptions={{ type: 'smoothstep', focusable: false, selectable: false }}
          proOptions={{ hideAttribution: false }}
          aria-label="工作台节点画布；使用鼠标平移与缩放，双击节点聚焦。"
        >
          <Background variant={BackgroundVariant.Dots} gap={22} size={1} />
          <Controls showInteractive={false} position="bottom-right" />
          <Panel position="bottom-left" className="canvas-legend">
            <span><i className="legend-mark path-mark" />选中路径</span>
            <span><i className="legend-mark lock-mark" />运行锁</span>
            <span>滚轮缩放 · 拖动画布平移 · 双击节点聚焦</span>
          </Panel>
        </ReactFlow>
      </div>

      {confirmIntent ? (
        <div className="canvas-confirm-backdrop" role="presentation" onMouseDown={(event) => {
          if (event.target === event.currentTarget && !connectSubmitting) dismissConfirmation()
        }}>
          <section className="canvas-confirm-dialog" role="dialog" aria-modal="true" aria-labelledby="reparent-title" onKeyDown={(event) => {
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
            <h2 id="reparent-title">确认改挂节点</h2>
            <p>
              将 <strong>{byId.get(confirmIntent.childId)?.title ?? confirmIntent.childId}</strong> 改挂到 <strong>{byId.get(confirmIntent.parentId)?.title ?? confirmIntent.parentId}</strong>
              {confirmIntent.candidate !== null ? ` 的候选 ${confirmIntent.candidate + 1}` : ''}。
            </p>
            <ul>
              <li>受影响子树：{confirmIntent.subtreeCount} 个节点，改挂后将标记为过期。</li>
              {confirmIntent.oldParentId ? <li>原上游：{byId.get(confirmIntent.oldParentId)?.title ?? confirmIntent.oldParentId}{confirmIntent.oldCandidate !== null ? ` · 候选 ${confirmIntent.oldCandidate + 1}` : ''}。</li> : null}
              <li>现有内容仍基于旧上游上下文，改挂不会自动重跑。</li>
              {confirmIntent.movesTree ? <li>这是跨 topic 树改挂，服务端会迁移子树工作区 steps；失败时不会提交连线。</li> : <li>工作区 steps 不迁移；运行锁和图约束仍由服务端校验。</li>}
            </ul>
            <div className="canvas-confirm-actions">
              <button ref={confirmCancelRef} type="button" className="canvas-toolbar-btn" disabled={connectSubmitting} onClick={() => dismissConfirmation()}>取消</button>
              <button
                type="button"
                className="canvas-toolbar-btn canvas-confirm-primary"
                disabled={confirmIntent.projectId !== projectId || connectSubmitting || Boolean(pendingSync)}
                onClick={() => void submitConnection(confirmIntent.parentId, confirmIntent.childId, confirmIntent.candidate, true)}
              >
                {connectSubmitting ? '提交中…' : '确认改挂'}
              </button>
            </div>
          </section>
        </div>
      ) : null}
    </section>
  )
}

export function WorkspaceCanvas({ snapshot, onSelectNode, onGraphMutation, api = graphAPI, candidateFixtures, onStartRun = startRunAPI }: Props) {
  return (
    <ReactFlowProvider>
      <CanvasBody snapshot={snapshot} onSelectNode={onSelectNode} onGraphMutation={onGraphMutation} api={api} candidateFixtures={candidateFixtures} onStartRun={onStartRun} />
    </ReactFlowProvider>
  )
}
