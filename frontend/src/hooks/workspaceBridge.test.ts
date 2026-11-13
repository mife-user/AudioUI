import { afterEach, describe, expect, it, vi } from 'vitest'
import { RunStatus, TranscriptKind } from '../../bindings/audio-ui/internal/domain/valueobject'
import type { Node, RunSnapshot, WorkspaceRunSummary, WorkspaceSnapshotResult } from '../../bindings/audio-ui/internal/domain/entity'
import type { RunEvent, TranscriptItem } from '../api/runs'
import type { WorkspaceChangedEvent } from '../api/workspace'
import { useRunStore } from '../stores/runStore'
import { useWorkspaceStore } from '../stores/workspaceStore'
import { WorkspaceBridge } from './workspaceBridge'
import type { WorkspaceBridgeAPI } from './workspaceBridge'

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

function runSummary(id: string, status = RunStatus.RunRunning): WorkspaceRunSummary {
  return {
    id, nodeId: `node_${id}`, status, chainId: `c_${id}`, chainPath: ['topic', `node_${id}`], estimatedTokens: 100,
  }
}

function workspaceResult(projectId: string | null, revision: number, runs: WorkspaceRunSummary[] = []): WorkspaceSnapshotResult {
  return {
    revision,
    snapshot: {
      project: projectId ? { id: projectId, name: projectId, revision: 5, activeNodeId: '' } : null,
      nodes: [], edges: [], nodePaths: {}, pending: [], runs, chains: [], lockedNodeIds: [],
    },
  }
}

function item(id: string, text: string): TranscriptItem {
  return { id, kind: TranscriptKind.TranscriptMessage, role: 'assistant', text, streaming: false, createdAt: '' }
}

function runSnapshot(projectId: string, runId: string, lastSeq: number, items: TranscriptItem[] = []): RunSnapshot {
  return {
    runId, projectId, lastSeq, permissions: [], questions: [],
    run: {
      id: runId, treeId: 'topic', nodeId: `node_${runId}`, chainId: `c_${runId}`,
      chainPath: ['topic', `node_${runId}`], status: RunStatus.RunRunning, contextVersion: 1, transcript: items,
    },
  }
}

function runEvent(projectId: string, runId: string, seq: number, text = `seq-${seq}`): RunEvent {
  return {
    v: 1, seq, runId, projectId, treeId: 'topic', nodeId: `node_${runId}`, chainId: `c_${runId}`,
    chainPath: ['topic', `node_${runId}`], type: 'message',
    message: item(`m-${seq}`, text),
  }
}

function makeAPI(initialResult: WorkspaceSnapshotResult = workspaceResult(null, 0)) {
  const workspaceListeners: ((event: WorkspaceChangedEvent) => void)[] = []
  const runListeners: ((event: RunEvent) => void)[] = []
  const getWorkspaceSnapshot = vi.fn().mockResolvedValue(initialResult)
  const getRunSnapshot = vi.fn().mockImplementation(async (runId: string) => runSnapshot(initialResult.snapshot.project?.id ?? '', runId, 0))
  const startRun = vi.fn().mockResolvedValue({})
  const newProject = vi.fn().mockResolvedValue(undefined)
  const openProject = vi.fn().mockResolvedValue(undefined)
  const interruptRun = vi.fn().mockResolvedValue(undefined)
  const respondRunPermission = vi.fn().mockResolvedValue(undefined)
  const respondRunQuestion = vi.fn().mockResolvedValue(undefined)
  const subscribeWorkspaceEvents = vi.fn((handler: (event: WorkspaceChangedEvent) => void) => {
    workspaceListeners.push(handler)
    return () => undefined
  })
  const subscribeRunEvents = vi.fn((handler: (event: RunEvent) => void) => {
    runListeners.push(handler)
    return () => undefined
  })
  const api = {
    getWorkspaceSnapshot,
    getRunSnapshot,
    startRun,
    newProject,
    openProject,
    interruptRun,
    respondRunPermission,
    respondRunQuestion,
    subscribeWorkspaceEvents,
    subscribeRunEvents,
  } as unknown as WorkspaceBridgeAPI
  return {
    api, workspaceListeners, runListeners,
    getWorkspaceSnapshot, getRunSnapshot, startRun, newProject, openProject, interruptRun, respondRunPermission, respondRunQuestion,
    subscribeWorkspaceEvents, subscribeRunEvents,
  }
}

const bridges: WorkspaceBridge[] = []
function bridge(api: WorkspaceBridgeAPI): WorkspaceBridge {
  const value = new WorkspaceBridge(api)
  bridges.push(value)
  return value
}

afterEach(() => {
  for (const value of bridges.splice(0)) value.stop()
  useWorkspaceStore.getState().reset()
  useRunStore.getState().clear()
})

describe('WorkspaceBridge', () => {
  it('subscribes to workspace and run events before requesting the first snapshot', async () => {
    const order: string[] = []
    const pendingSnapshot = deferred<WorkspaceSnapshotResult>()
    const setup = makeAPI()
    setup.subscribeWorkspaceEvents.mockImplementation((handler) => {
      order.push('workspace-subscribe')
      setup.workspaceListeners.push(handler)
      return () => undefined
    })
    setup.subscribeRunEvents.mockImplementation((handler) => {
      order.push('run-subscribe')
      setup.runListeners.push(handler)
      return () => undefined
    })
    setup.getWorkspaceSnapshot.mockImplementation(() => {
      order.push('snapshot')
      return pendingSnapshot.promise
    })

    const app = bridge(setup.api)
    app.start()
    expect(order).toEqual(['workspace-subscribe', 'run-subscribe', 'snapshot'])
    pendingSnapshot.resolve(workspaceResult(null, 0))
    await vi.waitFor(() => expect(useWorkspaceStore.getState().status).toBe('ready'))
  })

  it('buffers a run event that arrives before the workspace snapshot and restores one matching transcript', async () => {
    const pendingSnapshot = deferred<WorkspaceSnapshotResult>()
    const active = runSummary('r1')
    const setup = makeAPI()
    setup.getWorkspaceSnapshot
      .mockImplementationOnce(() => pendingSnapshot.promise)
      .mockResolvedValue(workspaceResult('p_one', 2, [active]))
    setup.getRunSnapshot.mockResolvedValue(runSnapshot('p_one', 'r1', 1, [item('m1', '已恢复')]))

    const app = bridge(setup.api)
    app.start()
    setup.runListeners[0](runEvent('p_one', 'r1', 1, '已恢复'))
    pendingSnapshot.resolve(workspaceResult('p_one', 1, [active]))

    await vi.waitFor(() => expect(useRunStore.getState().runs.r1?.snapshot?.run.transcript).toHaveLength(1))
    expect(useRunStore.getState().runs.r1?.snapshot?.run.transcript?.[0].text).toBe('已恢复')
  })

  it('waits for an in-flight graph refresh and its queued snapshot before resolving', async () => {
    const setup = makeAPI(workspaceResult('p_one', 1))
    const app = bridge(setup.api)
    app.start()
    await vi.waitFor(() => expect(useWorkspaceStore.getState().status).toBe('ready'))

    const pendingSnapshot = deferred<WorkspaceSnapshotResult>()
    const createdNode: Node = {
      id: 'n_created', type: 'topic' as Node['type'], title: '新选题', parent: '',
      position: { x: 0, y: 0 }, size: { width: 280, height: 250 }, params: {}, rounds: [], locked: false, stale: false,
    }
    const refreshed = workspaceResult('p_one', 3)
    refreshed.snapshot.nodes = [createdNode]
    setup.getWorkspaceSnapshot
      .mockImplementationOnce(() => pendingSnapshot.promise)
      .mockResolvedValueOnce(refreshed)
    setup.workspaceListeners[0]({ projectId: 'p_one', revision: 2, reason: 'graph', projectRevision: 2 })
    await vi.waitFor(() => expect(setup.getWorkspaceSnapshot).toHaveBeenCalledTimes(2))

    let resolvedBeforeNodeArrived = false
    const refresh = app.retryWorkspace()
    const completion = Promise.resolve(refresh).then(() => {
      resolvedBeforeNodeArrived = !useWorkspaceStore.getState().snapshot?.nodes?.some((node) => node.id === createdNode.id)
    })
    pendingSnapshot.resolve(workspaceResult('p_one', 2))
    await vi.waitFor(() => expect(useWorkspaceStore.getState().snapshot?.nodes?.map((node) => node.id)).toContain(createdNode.id))
    await completion

    expect(resolvedBeforeNodeArrived).toBe(false)
  })

  it('waits until a mutation receipt revision and node ID appear in the snapshot', async () => {
    const initial = workspaceResult('p_one', 1)
    if (initial.snapshot.project) initial.snapshot.project.revision = 1
    const setup = makeAPI(initial)
    const app = bridge(setup.api)
    app.start()
    await vi.waitFor(() => expect(useWorkspaceStore.getState().status).toBe('ready'))

    const stale = workspaceResult('p_one', 2)
    if (stale.snapshot.project) stale.snapshot.project.revision = 2
    const latest = workspaceResult('p_one', 3)
    if (latest.snapshot.project) latest.snapshot.project.revision = 3
    latest.snapshot.nodes = [{
      id: 'n_created', type: 'topic' as Node['type'], title: '新选题', parent: '',
      position: { x: 0, y: 0 }, size: { width: 280, height: 250 }, params: {}, rounds: [], locked: false, stale: false,
    }]
    setup.getWorkspaceSnapshot.mockResolvedValueOnce(stale).mockResolvedValueOnce(latest)

    await app.syncGraphMutation({ projectId: 'p_one', projectRevision: 3, nodeId: 'n_created' })

    expect(useWorkspaceStore.getState().snapshot?.project?.revision).toBe(3)
    expect(useWorkspaceStore.getState().snapshot?.nodes?.map((node) => node.id)).toContain('n_created')
  })

  it('rejects a mutation receipt after the active project changes', async () => {
    const setup = makeAPI(workspaceResult('p_new', 1))
    const app = bridge(setup.api)
    app.start()
    await vi.waitFor(() => expect(useWorkspaceStore.getState().status).toBe('ready'))

    await expect(app.syncGraphMutation({ projectId: 'p_old', projectRevision: 3, nodeId: 'n_created' }))
      .rejects.toThrow('PROJECT_CHANGED')
  })

  it('starts only in the loaded project and restores the returned run into the console', async () => {
    const setup = makeAPI(workspaceResult('p_one', 1))
    const record = runSnapshot('p_one', 'r_started', 0).run
    setup.startRun.mockResolvedValue({ run: record })
    const app = bridge(setup.api)
    app.start()
    await vi.waitFor(() => expect(useWorkspaceStore.getState().status).toBe('ready'))
    setup.getWorkspaceSnapshot.mockResolvedValue(workspaceResult('p_one', 2, [runSummary('r_started')]))

    const result = await app.startRun('n_topic', 'p_one')
    await vi.waitFor(() => expect(useRunStore.getState().runs.r_started?.snapshot?.run.id).toBe('r_started'))
    expect(result.run?.id).toBe('r_started')
    expect(setup.startRun).toHaveBeenCalledOnce()
    expect(setup.startRun).toHaveBeenCalledWith('n_topic', 'p_one', undefined)
    expect(useRunStore.getState().selectedRunId).toBe('r_started')

    const stale = await app.startRun('n_topic', 'p_old')
    expect(stale.error?.code).toBe('PROJECT_CHANGED')
    expect(setup.startRun).toHaveBeenCalledOnce()
  })

  it('routes frozen input and output options through the run API', async () => {
    const setup = makeAPI(workspaceResult('p_one', 1))
    const app = bridge(setup.api)
    app.start()
    await vi.waitFor(() => expect(useWorkspaceStore.getState().status).toBe('ready'))

    const options = { inputEdgeIds: ['e_script', 'e_prompt'], outputNodeId: 'n_script' }
    const result = await app.startRun('n_agent', 'p_one', options)

    expect(result.error).toBeUndefined()
    expect(setup.startRun).toHaveBeenCalledWith('n_agent', 'p_one', options)
  })

  it('preserves a run event and snapshot that arrive before the start response', async () => {
    const setup = makeAPI(workspaceResult('p_one', 1))
    const startResponse = deferred<Awaited<ReturnType<WorkspaceBridgeAPI['startRun']>>>()
    const runSnapshotResponse = deferred<RunSnapshot>()
    const record = runSnapshot('p_one', 'r_early', 1).run
    setup.startRun.mockReturnValue(startResponse.promise)
    setup.getRunSnapshot.mockReturnValue(runSnapshotResponse.promise)
    const app = bridge(setup.api)
    app.start()
    await vi.waitFor(() => expect(useWorkspaceStore.getState().status).toBe('ready'))

    const start = app.startRun('n_topic', 'p_one')
    setup.runListeners[0]({
      ...runEvent('p_one', 'r_early', 1),
      type: 'status',
      status: RunStatus.RunStarting,
    })
    await vi.waitFor(() => expect(setup.getRunSnapshot).toHaveBeenCalledWith('r_early'))
    startResponse.resolve({ run: record })
    await vi.waitFor(() => expect(useRunStore.getState().selectedRunId).toBe('r_early'))
    runSnapshotResponse.resolve(runSnapshot('p_one', 'r_early', 1))

    const result = await start
    expect(result.run?.id).toBe('r_early')
    expect(useRunStore.getState().runs.r_early?.snapshot?.run.status).toBe(RunStatus.RunRunning)
    expect(useRunStore.getState().selectedRunId).toBe('r_early')
  })

  it('re-pulls a run snapshot on a sequence gap and replays updates after that snapshot', async () => {
    const active = runSummary('r1')
    const setup = makeAPI(workspaceResult('p_one', 1, [active]))
    setup.getRunSnapshot
      .mockResolvedValueOnce(runSnapshot('p_one', 'r1', 1))
      .mockResolvedValueOnce(runSnapshot('p_one', 'r1', 2, [item('m2', '补齐序号 2')]))
    const app = bridge(setup.api)
    app.start()
    await vi.waitFor(() => expect(useRunStore.getState().runs.r1?.snapshot).not.toBeNull())

    setup.runListeners[0](runEvent('p_one', 'r1', 3, '序号 3'))
    await vi.waitFor(() => expect(useRunStore.getState().runs.r1?.lastSeq).toBe(3))
    expect(setup.getRunSnapshot).toHaveBeenCalledTimes(2)
    expect(useRunStore.getState().runs.r1?.snapshot?.run.transcript?.map((message) => message.text)).toEqual(['补齐序号 2', '序号 3'])
  })

  it('keeps a visible snapshot error and retries when requested', async () => {
    const setup = makeAPI(workspaceResult('p_one', 1))
    setup.getWorkspaceSnapshot
      .mockRejectedValueOnce(new Error('WORKSPACE_CHANGED'))
      .mockResolvedValueOnce(workspaceResult('p_one', 2))
    const app = bridge(setup.api)
    app.start()
    await vi.waitFor(() => expect(useWorkspaceStore.getState().status).toBe('error'))
    expect(useWorkspaceStore.getState().error).toContain('WORKSPACE_CHANGED')

    app.retryWorkspace()
    await vi.waitFor(() => expect(useWorkspaceStore.getState().status).toBe('ready'))
    expect(useWorkspaceStore.getState().revision).toBe(2)
  })

  it('rejects refresh callers when the workspace snapshot request fails', async () => {
    const setup = makeAPI(workspaceResult('p_one', 1))
    const app = bridge(setup.api)
    app.start()
    await vi.waitFor(() => expect(useWorkspaceStore.getState().status).toBe('ready'))
    setup.getWorkspaceSnapshot.mockRejectedValueOnce(new Error('snapshot unavailable'))

    await expect(app.retryWorkspace()).rejects.toThrow('snapshot unavailable')
    expect(useWorkspaceStore.getState().status).toBe('error')
  })

  it('drops late run snapshots and old-project events after switching projects', async () => {
    const active = runSummary('r_old')
    const pendingRun = deferred<RunSnapshot>()
    let current = workspaceResult('p_old', 1, [active])
    const setup = makeAPI(current)
    setup.getWorkspaceSnapshot.mockImplementation(async () => current)
    setup.getRunSnapshot.mockImplementation(() => pendingRun.promise)
    setup.openProject.mockImplementation(async () => {
      current = workspaceResult('p_new', 1)
    })
    const app = bridge(setup.api)
    app.start()
    await vi.waitFor(() => expect(setup.getRunSnapshot).toHaveBeenCalledTimes(1))
    const oldRunListener = setup.runListeners[0]

    await app.openProject('p_new')
    expect(useRunStore.getState().projectId).toBe('p_new')
    expect(useRunStore.getState().runs.r_old).toBeUndefined()

    pendingRun.resolve(runSnapshot('p_old', 'r_old', 0, [item('old', '旧工程迟到内容')]))
    oldRunListener(runEvent('p_old', 'r_old', 1, '旧工程迟到事件'))
    await Promise.resolve()

    expect(useRunStore.getState().projectId).toBe('p_new')
    expect(useRunStore.getState().runs.r_old).toBeUndefined()
  })

  it('starts a fresh project snapshot even when the previous project read is still pending', async () => {
    const oldSnapshot = deferred<WorkspaceSnapshotResult>()
    let current = workspaceResult('p_old', 1)
    const setup = makeAPI(current)
    setup.getWorkspaceSnapshot
      .mockImplementationOnce(() => oldSnapshot.promise)
      .mockImplementation(async () => current)
    setup.openProject.mockImplementation(async () => {
      current = workspaceResult('p_new', 1)
    })
    const app = bridge(setup.api)
    app.start()

    await app.openProject('p_new')
    expect(useWorkspaceStore.getState().status).toBe('ready')
    expect(useWorkspaceStore.getState().snapshot?.project?.id).toBe('p_new')

    oldSnapshot.resolve(workspaceResult('p_old', 99))
    await Promise.resolve()
    expect(useWorkspaceStore.getState().snapshot?.project?.id).toBe('p_new')
    expect(useWorkspaceStore.getState().revision).toBe(1)
  })

  it('propagates BUSY from the backend and reloads the still-active project', async () => {
    const setup = makeAPI(workspaceResult('p_one', 1))
    setup.openProject.mockRejectedValue(new Error('BUSY'))
    const app = bridge(setup.api)
    app.start()
    await vi.waitFor(() => expect(useWorkspaceStore.getState().status).toBe('ready'))

    await expect(app.openProject('p_other')).rejects.toThrow('BUSY')
    expect(useWorkspaceStore.getState().snapshot?.project?.id).toBe('p_one')
    expect(useWorkspaceStore.getState().status).toBe('ready')
  })

  it('routes run controls with runId and requestId', async () => {
    const setup = makeAPI(workspaceResult('p_one', 1))
    const app = bridge(setup.api)
    app.start()
    await vi.waitFor(() => expect(useWorkspaceStore.getState().status).toBe('ready'))

    await app.interrupt('r1')
    await app.respondPermission('r1', 'perm1', 'deny')
    await app.respondQuestion('r1', 'question1', 'other text', 'other')
    expect(setup.interruptRun).toHaveBeenCalledWith('r1')
    expect(setup.respondRunPermission).toHaveBeenCalledWith('r1', 'perm1', 'deny')
    expect(setup.respondRunQuestion).toHaveBeenCalledWith('r1', 'question1', 'other text', 'other')
  })
})
