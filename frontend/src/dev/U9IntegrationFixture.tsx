import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type {
  GraphMutationReceipt,
  Node,
  PendingDetail,
  PendingSummary,
  RunRecord,
  RunSnapshot,
  RunStartResult,
  ScriptVersionDetail,
  WorkspaceRunSummary,
  WorkspaceSnapshot,
} from '../../bindings/audio-ui/internal/domain/entity'
import {
  NodeType,
  OutputKind,
  PendingStatus,
  PermissionTier,
  RunStatus,
  TranscriptKind,
} from '../../bindings/audio-ui/internal/domain/valueobject'
import type { PermissionRequest, QuestionRequest } from '../../bindings/audio-ui/internal/domain/valueobject'
import type { PermissionDecision, RunEvent } from '../api/runs'
import type { GraphAPI, TopicCandidateFixture } from '../canvas/graphApi'
import { usePendingController, PendingControllerProvider, type PendingController } from '../hooks/usePendingController'
import { RunRequestProvider } from '../hooks/RunRequestContext'
import { AgentConsole } from '../ui/AgentConsole'
import { AppShell } from '../ui/AppShell'
import { PendingArtifactCard } from '../ui/PendingArtifactCard'
import { WorkspacePage } from '../ui/WorkspacePage'
import { useRunStore } from '../stores/runStore'
import { useWorkspaceStore } from '../stores/workspaceStore'

const PROJECT_ID = 'fixture_project_u9'
const AGENT_NODE_ID = 'fixture_agent'
const SCRIPT_NODE_ID = 'fixture_script'
const RUN_ID = 'fixture_run_permissions'
const PENDING_RUN_ID = 'fixture_run_script_pending'
const PENDING_ID = 'fixture_pending_script'
const CHAIN_ID = 'fixture_chain'
const FIXTURE_TIME = '2026-11-16T12:00:00Z'
const SCRIPT_PAYLOAD = JSON.stringify({
  title: '街巷里的修表铺',
  scenes: [
    { narration: '清晨，卷帘门升起，秒针声先于街道苏醒。', duration: 8 },
    { narration: '老师傅把旧表拆开，零件在掌心排成一列。', duration: 12 },
  ],
})
const CANDIDATES: TopicCandidateFixture[] = [
  { nodeId: 'fixture_candidate', round: 1, runId: 'fixture_run_topics', index: 0, title: '街巷里的修表铺', angle: '手艺与时间', hook: '每天最早亮起的那盏灯' },
]

function encodedPayload(value: string): string {
  return btoa(Array.from(new TextEncoder().encode(value), (byte) => String.fromCharCode(byte)).join(''))
}

function makeWorkspaceSnapshot(): WorkspaceSnapshot {
  const topic: Node = {
    id: 'fixture_topic',
    type: NodeType.NodeTypeTopic,
    title: '城市里的老手艺',
    position: { x: 50, y: 80 },
    size: { width: 280, height: 250 },
    params: { idea: '记录仍在街巷中延续的手艺。', direction: '人物观察', candidateCount: 3 },
    rounds: [{ n: 1, kind: OutputKind.OutputKindTopics, artifactId: 'sha256:fixture-topics', pendingId: 'fixture_pending_topics_applied', runId: 'fixture_run_topics' }],
    locked: true,
    stale: false,
  }
  const candidate: Node = {
    id: 'fixture_candidate',
    type: NodeType.NodeTypeCandidate,
    title: '街巷里的修表铺',
    parent: topic.id,
    candidateSource: { topicNodeId: topic.id, round: 1, artifactId: 'sha256:fixture-topics', index: 0, runId: 'fixture_run_topics' },
    position: { x: 390, y: 80 },
    size: { width: 235, height: 160 },
    params: {},
    rounds: [],
    locked: false,
    stale: false,
  }
  const prompt: Node = {
    id: 'fixture_prompt',
    type: NodeType.NodeTypePrompt,
    title: '人物短片任务',
    parent: candidate.id,
    position: { x: 690, y: 80 },
    size: { width: 280, height: 230 },
    params: { instruction: '写一段克制、具体的短片旁白，突出修表铺的日常细节。', outputKind: OutputKind.OutputKindScript, constraints: { scenes: 2, duration: 20 } },
    rounds: [],
    locked: false,
    stale: false,
  }
  const agent: Node = {
    id: AGENT_NODE_ID,
    type: NodeType.NodeTypeAgent,
    title: '本地 Agent',
    parent: prompt.id,
    position: { x: 1030, y: 80 },
    size: { width: 235, height: 165 },
    params: {},
    rounds: [],
    locked: false,
    stale: false,
  }
  const script: Node = {
    id: SCRIPT_NODE_ID,
    type: NodeType.NodeTypeScript,
    title: '旁白脚本',
    parent: agent.id,
    position: { x: 1320, y: 80 },
    size: { width: 270, height: 205 },
    params: {},
    rounds: [{ n: 1, kind: OutputKind.OutputKindScript, artifactId: 'sha256:fixture-script-v1', pendingId: 'fixture_pending_old', runId: 'fixture_run_previous' }],
    scriptVersions: [{ n: 1, artifactId: 'sha256:fixture-script-v1', runId: 'fixture_run_previous' }],
    locked: false,
    stale: false,
  }
  const pending: PendingSummary = {
    id: PENDING_ID,
    nodeId: agent.id,
    runId: PENDING_RUN_ID,
    chainId: CHAIN_ID,
    kind: OutputKind.OutputKindScript,
    status: PendingStatus.PendingPending,
    summary: '新结果待确认追加到 v1',
    outputBinding: {
      bound: true,
      nodeId: script.id,
      parentNodeId: agent.id,
      baselineVersion: 1,
      baselineArtifactId: 'sha256:fixture-script-v1',
    },
    resultNodeId: script.id,
  }
  const runningSummary: WorkspaceRunSummary = {
    id: RUN_ID,
    nodeId: agent.id,
    status: RunStatus.RunRunning,
    chainId: CHAIN_ID,
    chainPath: [topic.id, candidate.id, prompt.id, agent.id],
    estimatedTokens: 840,
    contextHash: 'fixture-context-hash',
    startedAt: FIXTURE_TIME,
  }
  const pendingSummary: WorkspaceRunSummary = {
    id: PENDING_RUN_ID,
    nodeId: agent.id,
    status: RunStatus.RunSucceeded,
    chainId: CHAIN_ID,
    chainPath: [topic.id, candidate.id, prompt.id, agent.id],
    estimatedTokens: 920,
    contextHash: 'fixture-pending-context',
    startedAt: FIXTURE_TIME,
    finishedAt: FIXTURE_TIME,
  }
  return {
    project: { id: PROJECT_ID, name: 'U9 离线集成样例', revision: 4, activeNodeId: agent.id },
    nodes: [topic, candidate, prompt, agent, script],
    edges: [
      { id: 'fixture_edge_topic_candidate', sourceNodeId: topic.id, targetNodeId: candidate.id, kind: 'candidate_source', scriptVersion: 0 },
      { id: 'fixture_edge_candidate_prompt', sourceNodeId: candidate.id, targetNodeId: prompt.id, kind: 'connection', scriptVersion: 0 },
      { id: 'fixture_edge_prompt_agent', sourceNodeId: prompt.id, targetNodeId: agent.id, kind: 'connection', scriptVersion: 0 },
      { id: 'fixture_edge_agent_script', sourceNodeId: agent.id, targetNodeId: script.id, kind: 'connection', scriptVersion: 0 },
    ],
    nodePaths: {
      [topic.id]: [topic.id],
      [candidate.id]: [topic.id, candidate.id],
      [prompt.id]: [topic.id, candidate.id, prompt.id],
      [agent.id]: [topic.id, candidate.id, prompt.id, agent.id],
      [script.id]: [topic.id, candidate.id, prompt.id, agent.id, script.id],
    },
    pending: [pending],
    runs: [runningSummary, pendingSummary],
    chains: [{
      id: CHAIN_ID,
      tailNodeId: script.id,
      path: [topic.id, candidate.id, prompt.id, agent.id, script.id],
      nodeIds: [topic.id, candidate.id, prompt.id, agent.id, script.id],
      ambiguous: false,
      label: '修表铺短片',
      status: 'running',
    }],
    lockedNodeIds: [],
  }
}

function makePendingDetail(): PendingDetail {
  return {
    pending: {
      id: PENDING_ID,
      nodeId: AGENT_NODE_ID,
      runId: PENDING_RUN_ID,
      chainId: CHAIN_ID,
      chainPath: ['fixture_topic', 'fixture_candidate', 'fixture_prompt', AGENT_NODE_ID],
      kind: OutputKind.OutputKindScript,
      status: PendingStatus.PendingPending,
      payload: 'fixture/pending-script.json',
      summary: '新结果待确认追加到 v1',
      outputBinding: {
        bound: true,
        nodeId: SCRIPT_NODE_ID,
        parentNodeId: AGENT_NODE_ID,
        baselineVersion: 1,
        baselineArtifactId: 'sha256:fixture-script-v1',
      },
      outputSchemaVersion: 1,
      resultNodeId: SCRIPT_NODE_ID,
      createdAt: FIXTURE_TIME,
    },
    rawPayload: encodedPayload(SCRIPT_PAYLOAD),
  }
}

function makeRunSnapshot(
  runId: string,
  status: RunStatus,
  permissions: PermissionRequest[] = [],
  questions: QuestionRequest[] = [],
): RunSnapshot {
  const active = status === RunStatus.RunRunning
  return {
    runId,
    projectId: PROJECT_ID,
    lastSeq: 1,
    run: {
      id: runId,
      treeId: 'fixture_topic',
      nodeId: AGENT_NODE_ID,
      chainId: CHAIN_ID,
      chainPath: ['fixture_topic', 'fixture_candidate', 'fixture_prompt', AGENT_NODE_ID],
      status,
      contextVersion: 1,
      contextHash: 'fixture-context-hash',
      estimatedTokens: 840,
      tokenBudget: 1_000_000,
      startedAt: FIXTURE_TIME,
      finishedAt: active ? undefined : FIXTURE_TIME,
      pendingId: runId === PENDING_RUN_ID ? PENDING_ID : undefined,
      transcript: [{
        id: `${runId}_message_1`,
        kind: TranscriptKind.TranscriptMessage,
        role: 'assistant',
        text: '样例运行事件已加载；所有内容均为离线 fixture。',
        streaming: false,
        createdAt: FIXTURE_TIME,
      }],
    },
    permissions,
    questions,
  }
}

function makePermission(id: string): PermissionRequest {
  return {
    id,
    tool: 'Write',
    tier: PermissionTier.PermissionConfirm,
    risk: '写入当前运行目标工作区',
    summary: '写入 fixture-output.txt',
    paths: ['outputs/fixture-output.txt'],
    allowSession: false,
    expiresAt: '2099-01-01T00:00:00Z',
  }
}

function makeQuestion(id: string): QuestionRequest {
  return {
    id,
    question: '是否将这段内容保留为本次运行的旁白风格？',
    options: ['保留', '改为更简洁'],
    allowOther: true,
    expiresAt: '2099-01-01T00:00:00Z',
  }
}

function sizesFor(type: Node['type']): Node['size'] {
  if (type === NodeType.NodeTypeTopic) return { width: 280, height: 250 }
  if (type === NodeType.NodeTypeCandidate) return { width: 235, height: 160 }
  if (type === NodeType.NodeTypePrompt) return { width: 280, height: 230 }
  if (type === NodeType.NodeTypeAgent) return { width: 235, height: 165 }
  return { width: 270, height: 205 }
}

function FixturePendingPreview({ controller, onClose }: { controller: PendingController; onClose: () => void }) {
  const pending = controller.selectedPending
  if (!pending) return null
  return (
    <aside className="u9-fixture-pending-preview" aria-label="待确认 Script 夹具预览">
      <div className="u9-fixture-pending-preview-head">
        <strong>fixture 待确认 Script</strong>
        <button className="agent-icon-btn" type="button" aria-label="关闭待确认 Script 预览" onClick={onClose}>×</button>
      </div>
      <PendingArtifactCard
        summary={pending}
        detail={controller.selectedPendingDetail}
        outputNode={controller.selectedOutputNode}
        loading={controller.loading}
        loadError={controller.loadError}
        actionError={controller.actionError}
        forceConfirmation={controller.forceConfirmation}
        actionBusy={controller.actionBusy}
        resolved={controller.resolved}
        syncError={controller.syncError}
        onRetryDetail={controller.retryDetail}
        onApply={() => void controller.apply()}
        onDiscard={() => void controller.discard()}
        onConfirmForce={() => void controller.apply(true)}
        onCancelForce={controller.cancelForce}
        onRetryWorkspace={() => void controller.retryWorkspace()}
      />
    </aside>
  )
}

export function U9IntegrationFixture() {
  const [snapshot, setSnapshot] = useState(makeWorkspaceSnapshot)
  const snapshotRef = useRef(snapshot)
  const sequenceByRun = useRef<Record<string, number>>({ [RUN_ID]: 1, [PENDING_RUN_ID]: 1 })
  const nextNodeId = useRef(1)
  const [consoleCollapsed, setConsoleCollapsed] = useState(true)
  const [pendingPreviewOpen, setPendingPreviewOpen] = useState(false)
  const [workspaceFailure, setWorkspaceFailure] = useState(false)
  const [pendingReadFailure, setPendingReadFailure] = useState(false)
  const [pendingApplyFailure, setPendingApplyFailure] = useState(false)
  const [permissionFailure, setPermissionFailure] = useState(false)
  const [fixtureMessage, setFixtureMessage] = useState('初始样例包含工程、候选、Prompt、运行权限与待确认 Script。')

  const publishSnapshot = useCallback((next: WorkspaceSnapshot) => {
    snapshotRef.current = next
    setSnapshot(next)
    if (next.project) {
      useWorkspaceStore.getState().applySnapshot({ revision: next.project.revision, snapshot: next })
      useWorkspaceStore.getState().setActiveNode(next.project.activeNodeId)
      useRunStore.getState().setWorkspaceSnapshot(next.project.id, next.runs ?? [])
    }
  }, [])

  const commitGraph = useCallback((update: (current: WorkspaceSnapshot) => WorkspaceSnapshot, nodeId = ''): GraphMutationReceipt => {
    const current = snapshotRef.current
    if (!current.project || current.project.id !== PROJECT_ID) throw new Error('FIXTURE_PROJECT_CHANGED')
    const projectRevision = current.project.revision + 1
    const changed = update(current)
    publishSnapshot({ ...changed, project: { ...current.project, revision: projectRevision } })
    return { projectId: PROJECT_ID, projectRevision, nodeId }
  }, [publishSnapshot])

  const graphAPI = useMemo<GraphAPI>(() => ({
    addNode: async (request) => {
      const id = `fixture_node_${nextNodeId.current++}`
      const type = request.type as Node['type']
      const node: Node = {
        id,
        type,
        title: request.title,
        parent: request.parent || undefined,
        position: request.position,
        size: sizesFor(type),
        params: request.params,
        rounds: [],
        locked: false,
        stale: false,
      }
      return commitGraph((current) => ({
        ...current,
        nodes: [...(current.nodes ?? []), node],
        edges: request.parent ? [
          ...(current.edges ?? []),
          { id: `fixture_edge_${id}`, sourceNodeId: request.parent, targetNodeId: id, kind: 'connection', scriptVersion: 0 },
        ] : current.edges,
        nodePaths: { ...current.nodePaths, [id]: [...(current.nodePaths?.[request.parent] ?? []), id] },
      }), id)
    },
    connect: async (sourceId, targetId) => commitGraph((current) => ({
      ...current,
      edges: [...(current.edges ?? []), {
        id: `fixture_edge_${sourceId}_${targetId}_${current.project?.revision ?? 0}`,
        sourceNodeId: sourceId,
        targetNodeId: targetId,
        kind: 'connection',
        scriptVersion: 0,
      }],
    }), targetId),
    disconnect: async (edgeId) => commitGraph((current) => ({
      ...current,
      edges: (current.edges ?? []).filter((edge) => edge.id !== edgeId),
    })),
    reconnectTarget: async (request) => commitGraph((current) => ({
      ...current,
      edges: (current.edges ?? []).map((edge) => edge.id === request.expectedEdge.id
        ? { ...edge, targetNodeId: request.targetNodeId }
        : edge),
      nodes: (current.nodes ?? []).map((node) => node.id === request.expectedEdge.targetNodeId
        ? { ...node, parent: undefined }
        : node),
    }), request.targetNodeId),
    moveNode: async (id, x, y) => commitGraph((current) => ({
      ...current,
      nodes: (current.nodes ?? []).map((node) => node.id === id ? { ...node, position: { x, y } } : node),
    }), id),
    setNodeLayout: async (id, x, y, width, height) => commitGraph((current) => ({
      ...current,
      nodes: (current.nodes ?? []).map((node) => node.id === id
        ? { ...node, position: { x, y }, size: { width, height } }
        : node),
    }), id),
    setNodeCollapsed: async (id, collapsed) => commitGraph((current) => ({
      ...current,
      nodes: (current.nodes ?? []).map((node) => node.id === id ? { ...node, collapsed } : node),
    }), id),
    setEdgeScriptVersion: async (edgeId, version) => commitGraph((current) => ({
      ...current,
      edges: (current.edges ?? []).map((edge) => edge.id === edgeId ? { ...edge, scriptVersion: version } : edge),
    })),
    selectNode: async (id, projectId) => {
      if (projectId !== PROJECT_ID) throw new Error('FIXTURE_PROJECT_CHANGED')
      const current = snapshotRef.current
      const next = { ...current, project: current.project ? { ...current.project, activeNodeId: id } : null }
      publishSnapshot(next)
      return { projectId, projectRevision: current.project?.revision ?? 0, nodeId: id }
    },
    updateNodeParams: async (id, title, params) => commitGraph((current) => ({
      ...current,
      nodes: (current.nodes ?? []).map((node) => node.id === id ? { ...node, title, params } : node),
    }), id),
    getTopicCandidates: async () => CANDIDATES,
    getScriptVersion: async (nodeId, version) => {
      const node = snapshotRef.current.nodes?.find((item) => item.id === nodeId)
      const scriptVersion = node?.scriptVersions?.find((item) => item.n === version)
      if (!node || !scriptVersion) throw new Error('FIXTURE_SCRIPT_VERSION_NOT_FOUND')
      return { nodeId, version: scriptVersion, payload: SCRIPT_PAYLOAD } satisfies ScriptVersionDetail
    },
  }), [commitGraph, publishSnapshot])

  const onPendingRead = useCallback(async (id: string, projectId: string): Promise<PendingDetail> => {
    if (projectId !== PROJECT_ID || id !== PENDING_ID) throw new Error('FIXTURE_PENDING_NOT_FOUND')
    if (pendingReadFailure) throw new Error('FIXTURE_PENDING_READ_FAILED: 模拟正文读取失败。')
    return makePendingDetail()
  }, [pendingReadFailure])

  const refreshWorkspace = useCallback(async () => {
    if (workspaceFailure) throw new Error('FIXTURE_WORKSPACE_SYNC_FAILED: 模拟工程快照同步失败。')
    const current = snapshotRef.current
    if (current.project) {
      useWorkspaceStore.getState().applySnapshot({ revision: current.project.revision, snapshot: current })
    }
  }, [workspaceFailure])

  const pendingController = usePendingController({
    projectId: snapshot.project?.id ?? null,
    nodes: snapshot.nodes ?? [],
    pending: snapshot.pending ?? [],
    onGetPending: onPendingRead,
    onApplyPending: async (id, _force, projectId) => {
      if (projectId !== PROJECT_ID || id !== PENDING_ID) throw new Error('FIXTURE_PENDING_NOT_FOUND')
      if (pendingApplyFailure) throw new Error('FIXTURE_PENDING_APPLY_FAILED: 模拟追加冲突，原版本保持不变。')
      const current = snapshotRef.current
      const next: WorkspaceSnapshot = {
        ...current,
        project: current.project ? { ...current.project, revision: current.project.revision + 1 } : null,
        pending: (current.pending ?? []).filter((item) => item.id !== id),
        nodes: (current.nodes ?? []).map((node) => node.id === SCRIPT_NODE_ID
          ? {
            ...node,
            scriptVersions: [...(node.scriptVersions ?? []), { n: 2, artifactId: 'sha256:fixture-script-v2', runId: PENDING_RUN_ID }],
          }
          : node),
      }
      publishSnapshot(next)
    },
    onDiscardPending: async (id, projectId) => {
      if (projectId !== PROJECT_ID || id !== PENDING_ID) throw new Error('FIXTURE_PENDING_NOT_FOUND')
      const current = snapshotRef.current
      publishSnapshot({
        ...current,
        project: current.project ? { ...current.project, revision: current.project.revision + 1 } : null,
        pending: (current.pending ?? []).filter((item) => item.id !== id),
      })
    },
    onRefreshWorkspace: refreshWorkspace,
  })

  const emitRunEvent = useCallback((runId: string, type: RunEvent['type'], detail: Partial<RunEvent> = {}) => {
    const current = snapshotRef.current
    const summary = current.runs?.find((item) => item.id === runId)
    if (!summary) return
    const seq = (sequenceByRun.current[runId] ?? 1) + 1
    sequenceByRun.current[runId] = seq
    const event: RunEvent = {
      v: 1,
      seq,
      runId,
      projectId: PROJECT_ID,
      treeId: 'fixture_topic',
      chainId: summary.chainId,
      nodeId: summary.nodeId,
      chainPath: summary.chainPath ?? [],
      type,
      ...detail,
    }
    useRunStore.getState().receiveEvent(event)
    if (type === 'status' && event.status) {
      const next: WorkspaceSnapshot = {
        ...snapshotRef.current,
        runs: (snapshotRef.current.runs ?? []).map((item) => item.id === runId ? { ...item, status: event.status! } : item),
        chains: (snapshotRef.current.chains ?? []).map((item) => ({ ...item, status: event.status! })),
      }
      publishSnapshot(next)
    }
  }, [publishSnapshot])

  const onPermission = useCallback(async (runId: string, requestId: string, _decision: PermissionDecision) => {
    if (permissionFailure) throw new Error('FIXTURE_PERMISSION_RESPONSE_FAILED: 模拟权限响应失败。')
    emitRunEvent(runId, 'request_resolved', { resolvedKind: 'permission', resolvedId: requestId })
    setFixtureMessage(`已在内存中响应权限请求 ${requestId}；没有调用真实 Agent。`)
  }, [emitRunEvent, permissionFailure])

  const onQuestion = useCallback(async (runId: string, requestId: string) => {
    if (permissionFailure) throw new Error('FIXTURE_QUESTION_RESPONSE_FAILED: 模拟问题响应失败。')
    emitRunEvent(runId, 'request_resolved', { resolvedKind: 'question', resolvedId: requestId })
    setFixtureMessage(`已在内存中响应问题 ${requestId}；没有调用真实 Agent。`)
  }, [emitRunEvent, permissionFailure])

  const startRun = useCallback(async (nodeId: string, projectId: string): Promise<RunStartResult> => {
    if (projectId !== PROJECT_ID) return { error: { code: 'FIXTURE_PROJECT_CHANGED', message: 'fixture 工程不匹配。' } }
    const runId = `fixture_run_${sequenceByRun.current[RUN_ID] + 1}`
    sequenceByRun.current[runId] = 1
    const current = snapshotRef.current
    const nodePath = current.nodePaths?.[nodeId] ?? [nodeId]
    const summary: WorkspaceRunSummary = {
      id: runId,
      nodeId,
      status: RunStatus.RunRunning,
      chainId: CHAIN_ID,
      chainPath: nodePath,
      estimatedTokens: 760,
      startedAt: FIXTURE_TIME,
    }
    const run: RunRecord = {
      id: runId,
      treeId: 'fixture_topic',
      nodeId,
      chainId: CHAIN_ID,
      chainPath: nodePath,
      status: RunStatus.RunRunning,
      contextVersion: 1,
      estimatedTokens: 760,
      tokenBudget: 1_000_000,
      startedAt: FIXTURE_TIME,
      transcript: [],
    }
    publishSnapshot({ ...current, runs: [summary, ...(current.runs ?? [])] })
    useRunStore.getState().applySnapshot(makeRunSnapshot(runId, RunStatus.RunRunning))
    setFixtureMessage(`已建立内存运行 ${runId}；未启动 Agent。`)
    return { run }
  }, [publishSnapshot])

  const onInterrupt = useCallback(async (runId: string) => {
    emitRunEvent(runId, 'status', { status: RunStatus.RunInterrupted })
    setFixtureMessage(`运行 ${runId} 已在 fixture 中标记为中断。`)
  }, [emitRunEvent])

  const onRetryRun = useCallback((runId: string) => {
    const current = snapshotRef.current
    const summary = current.runs?.find((item) => item.id === runId)
    if (!summary) return
    sequenceByRun.current[runId] = 1
    publishSnapshot({
      ...current,
      runs: (current.runs ?? []).map((item) => item.id === runId
        ? { ...item, status: RunStatus.RunRunning, startedAt: FIXTURE_TIME, finishedAt: undefined }
        : item),
    })
    useRunStore.getState().applySnapshot(makeRunSnapshot(runId, RunStatus.RunRunning, [makePermission(`fixture_permission_retry_${runId}`)]))
    setFixtureMessage(`已将 ${runId} 重置为内存运行状态；未启动 Agent。`)
  }, [publishSnapshot])

  const resetFixture = useCallback(() => {
    const next = makeWorkspaceSnapshot()
    const permissionSnapshot = makeRunSnapshot(RUN_ID, RunStatus.RunRunning, [makePermission('fixture_permission_initial')])
    const pendingRunSnapshot = makeRunSnapshot(PENDING_RUN_ID, RunStatus.RunSucceeded)
    snapshotRef.current = next
    setSnapshot(next)
    useWorkspaceStore.getState().reset()
    useWorkspaceStore.getState().applySnapshot({ revision: next.project?.revision ?? 0, snapshot: next })
    useRunStore.getState().clear()
    useRunStore.getState().setWorkspaceSnapshot(PROJECT_ID, next.runs ?? [])
    useRunStore.getState().applySnapshot(permissionSnapshot)
    useRunStore.getState().applySnapshot(pendingRunSnapshot)
    sequenceByRun.current = { [RUN_ID]: 1, [PENDING_RUN_ID]: 1 }
    nextNodeId.current = 1
    setConsoleCollapsed(true)
    setPendingPreviewOpen(false)
    setWorkspaceFailure(false)
    setPendingReadFailure(false)
    setPendingApplyFailure(false)
    setPermissionFailure(false)
    setFixtureMessage('初始样例包含工程、候选、Prompt、运行权限与待确认 Script。')
  }, [])

  useEffect(() => {
    resetFixture()
  }, [resetFixture])

  const injectPermission = useCallback(() => {
    const id = `fixture_permission_${(sequenceByRun.current[RUN_ID] ?? 1) + 1}`
    emitRunEvent(RUN_ID, 'permission', { permission: makePermission(id) })
    setFixtureMessage(`已注入权限事件 ${id}。`)
  }, [emitRunEvent])

  const injectQuestion = useCallback(() => {
    const id = `fixture_question_${(sequenceByRun.current[RUN_ID] ?? 1) + 1}`
    emitRunEvent(RUN_ID, 'question', { question: makeQuestion(id) })
    setFixtureMessage(`已注入问题事件 ${id}。`)
  }, [emitRunEvent])

  const injectRunFailure = useCallback(() => {
    emitRunEvent(RUN_ID, 'status', { status: RunStatus.RunFailed })
    emitRunEvent(RUN_ID, 'error', { error: { code: 'FIXTURE_RUN_FAILED', message: '演示运行失败，不涉及真实 Agent。', recoverable: true } })
    setFixtureMessage('已注入失败状态与错误事件。')
  }, [emitRunEvent])

  const toggleWorkspaceFailure = useCallback(() => {
    const next = !workspaceFailure
    setWorkspaceFailure(next)
    if (next) useWorkspaceStore.getState().setError('FIXTURE_WORKSPACE_SYNC_FAILED: 模拟快照同步失败。')
    else {
      const current = snapshotRef.current
      if (current.project) useWorkspaceStore.getState().applySnapshot({ revision: current.project.revision, snapshot: current })
    }
    setFixtureMessage(next ? '已模拟工作区同步失败；画布快照仍保留。' : '已恢复工作区状态。')
  }, [workspaceFailure])

  const onRetryWorkspace = useCallback(async () => {
    if (workspaceFailure) throw new Error('FIXTURE_WORKSPACE_SYNC_FAILED: 模拟快照同步失败。')
    const current = snapshotRef.current
    if (current.project) useWorkspaceStore.getState().applySnapshot({ revision: current.project.revision, snapshot: current })
  }, [workspaceFailure])

  const runViews = useRunStore((store) => store.runs)
  const runSummaries = useRunStore((store) => store.summaries)
  const selectedRunId = useRunStore((store) => store.selectedRunId)
  const selectedRun = useRunStore((store) => selectedRunId ? store.runs[selectedRunId] : undefined)

  return (
    <AppShell className="u9-fixture-shell">
      <header className="topbar">
        <div className="brand">
          <span className="brand-mark" aria-hidden="true">AU</span>
          <span>AudioUI</span>
          <span className="brand-sub">U9 离线集成夹具</span>
        </div>
        <span className="topbar-spacer" />
        <span className="env-state" role="status">仅内存 · 不连接 Agent、工程服务或网络</span>
      </header>
      <section className="u9-fixture-tools" aria-label="U9 故障注入与样例控制">
        <div className="u9-fixture-tools-heading">
          <strong>受控场景</strong>
          <span role="status" aria-live="polite">{fixtureMessage}</span>
        </div>
        <div className="u9-fixture-tools-actions">
          <button className="agent-mini-btn" type="button" onClick={injectPermission}>注入权限事件</button>
          <button className="agent-mini-btn" type="button" onClick={injectQuestion}>注入问题事件</button>
          <button className="agent-mini-btn deny" type="button" onClick={injectRunFailure}>注入运行失败</button>
          <button className="agent-mini-btn" type="button" aria-pressed={workspaceFailure} onClick={toggleWorkspaceFailure}>切换工作区失败</button>
          <button className="agent-mini-btn" type="button" aria-pressed={pendingReadFailure} onClick={() => setPendingReadFailure((value) => !value)}>待确认读取失败：{pendingReadFailure ? '开' : '关'}</button>
          <button className="agent-mini-btn" type="button" aria-pressed={pendingApplyFailure} onClick={() => setPendingApplyFailure((value) => !value)}>确认追加失败：{pendingApplyFailure ? '开' : '关'}</button>
          <button className="agent-mini-btn" type="button" aria-pressed={permissionFailure} onClick={() => setPermissionFailure((value) => !value)}>请求响应失败：{permissionFailure ? '开' : '关'}</button>
          <button className="agent-mini-btn" type="button" onClick={() => {
            setPendingPreviewOpen(true)
            void pendingController.openFromNode(PENDING_ID, SCRIPT_NODE_ID)
          }}>打开待确认 Script</button>
          <button className="agent-mini-btn" type="button" onClick={resetFixture}>重置样例</button>
        </div>
      </section>
      <RunRequestProvider
        projectId={snapshot.project?.id ?? null}
        runViews={runViews}
        onPermission={onPermission}
        onQuestion={onQuestion}
      >
        <PendingControllerProvider controller={pendingController}>
          <div className="workspace-layout u9-fixture-layout">
            <WorkspacePage
              status={workspaceFailure ? 'error' : 'ready'}
              snapshot={snapshot}
              error={workspaceFailure ? 'FIXTURE_WORKSPACE_SYNC_FAILED: 模拟快照同步失败。' : ''}
              onRetry={onRetryWorkspace}
              onGraphMutation={() => undefined}
              onStartRun={startRun}
              api={graphAPI}
            />
            {pendingPreviewOpen ? (
              <FixturePendingPreview controller={pendingController} onClose={() => setPendingPreviewOpen(false)} />
            ) : null}
            <AgentConsole
              runs={runSummaries}
              projectId={snapshot.project?.id ?? null}
              nodes={snapshot.nodes ?? []}
              runViews={runViews}
              selectedRunId={selectedRunId}
              selectedRun={selectedRun}
              actionError=""
              collapsed={consoleCollapsed}
              onToggle={() => setConsoleCollapsed((value) => !value)}
              onSelectRun={(runId) => useRunStore.getState().selectRun(runId)}
              onRetryRun={onRetryRun}
              onPermission={onPermission}
              onQuestion={onQuestion}
              onInterrupt={onInterrupt}
              pendingController={pendingController}
            />
          </div>
        </PendingControllerProvider>
      </RunRequestProvider>
    </AppShell>
  )
}
