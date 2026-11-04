import { getRunSnapshot, interruptRun, respondRunPermission, respondRunQuestion, startRun, subscribeRunEvents } from '../api/runs'
import type { PermissionDecision, RunEvent, RunStartResult } from '../api/runs'
import { getWorkspaceSnapshot, newProject, openProject, subscribeWorkspaceEvents } from '../api/workspace'
import type { WorkspaceChangedEvent } from '../api/workspace'
import type { GraphMutationReceipt } from '../../bindings/audio-ui/internal/domain/entity'
import { useRunStore } from '../stores/runStore'
import { useWorkspaceStore } from '../stores/workspaceStore'

const MAX_BUFFERED_EVENTS = 1000

export type WorkspaceBridgeAPI = {
  getWorkspaceSnapshot: typeof getWorkspaceSnapshot
  getRunSnapshot: typeof getRunSnapshot
  startRun: typeof startRun
  newProject: typeof newProject
  openProject: typeof openProject
  interruptRun: typeof interruptRun
  respondRunPermission: typeof respondRunPermission
  respondRunQuestion: typeof respondRunQuestion
  subscribeWorkspaceEvents: typeof subscribeWorkspaceEvents
  subscribeRunEvents: typeof subscribeRunEvents
}

const defaultAPI: WorkspaceBridgeAPI = {
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
}

function messageOf(error: unknown): string {
  if (error instanceof Error) return error.message
  if (typeof error === 'string') return error
  if (error && typeof error === 'object' && 'message' in error) {
    return String((error as { message: unknown }).message)
  }
  return String(error)
}

export class WorkspaceBridge {
  private generation = 0
  private active = false
  private workspaceLoaded = false
  private workspaceLoading = false
  private workspaceLoadingGeneration: number | null = null
  private workspaceRefreshPromise: Promise<void> | null = null
  private workspaceReloadRequested = false
  private workspaceEvents: WorkspaceChangedEvent[] = []
  private runEvents: RunEvent[] = []
  private unsubscribe: (() => void)[] = []
  private runLoads = new Map<string, Promise<void>>()

  constructor(private readonly api: WorkspaceBridgeAPI = defaultAPI) {}

  start(): void {
    if (this.active) return
    this.active = true
    const generation = ++this.generation
    this.workspaceLoaded = false
    this.workspaceLoading = false
    this.workspaceLoadingGeneration = null
    this.workspaceRefreshPromise = null
    this.workspaceReloadRequested = false
    this.workspaceEvents = []
    this.runEvents = []
    useWorkspaceStore.getState().reset()
    useRunStore.getState().clear()
    useWorkspaceStore.getState().setLoading()
    this.bindEvents(generation)
    void this.refreshWorkspace(generation)
  }

  stop(): void {
    if (!this.active) return
    this.active = false
    this.generation += 1
    this.unsubscribeAll()
    this.workspaceLoaded = false
    this.workspaceLoading = false
    this.workspaceLoadingGeneration = null
    this.workspaceRefreshPromise = null
    this.workspaceReloadRequested = false
    this.workspaceEvents = []
    this.runEvents = []
    this.runLoads.clear()
    useWorkspaceStore.getState().reset()
    useRunStore.getState().clear()
  }

  async retryWorkspace(): Promise<void> {
    if (!this.active) return
    const generation = this.generation
    await this.refreshWorkspace(generation)
    if (!this.isCurrent(generation)) return
    const workspace = useWorkspaceStore.getState()
    if (workspace.status === 'error') throw new Error(workspace.error || '工作台快照同步失败')
  }

  async syncGraphMutation(receipt: GraphMutationReceipt): Promise<void> {
    const generation = this.generation
    if (!this.isCurrent(generation)) throw new Error('工作台尚未初始化')

    for (let attempt = 0; attempt < 3; attempt += 1) {
      await this.retryWorkspace()
      if (!this.isCurrent(generation)) throw new Error('工程已切换，放弃旧工程快照同步。')

      const snapshot = useWorkspaceStore.getState().snapshot
      const project = snapshot?.project
      if (!project || project.id !== receipt.projectId) {
        throw new Error('PROJECT_CHANGED: 当前工程与图命令所属工程不一致。')
      }
      const nodePresent = !receipt.nodeId || (snapshot.nodes ?? []).some((node) => node.id === receipt.nodeId)
      if (project.revision >= receipt.projectRevision && nodePresent) return
    }

    throw new Error(`WORKSPACE_SYNC_STALE: 工程 ${receipt.projectId} 的 revision ${receipt.projectRevision} 尚未出现在工作台快照中。`)
  }

  async startRun(nodeId: string, projectId: string): Promise<RunStartResult> {
    const generation = this.generation
    const currentProjectId = useWorkspaceStore.getState().snapshot?.project?.id ?? null
    if (!this.isCurrent(generation) || currentProjectId !== projectId) {
      return { error: { code: 'PROJECT_CHANGED', message: '工程已切换，未启动运行。' } }
    }

    const result = await this.api.startRun(nodeId, projectId)
    if (!this.isCurrent(generation)) {
      return { error: { code: 'PROJECT_CHANGED', message: '工程已切换，运行结果未并入当前工作台。' } }
    }
    if ((useWorkspaceStore.getState().snapshot?.project?.id ?? null) !== projectId) {
      return { error: { code: 'PROJECT_CHANGED', message: '当前工程与运行所属工程不一致。' } }
    }
    if (result.run?.id) {
      useRunStore.getState().selectRun(result.run.id)
      await this.loadRunSnapshot(result.run.id, generation, true)
      void this.refreshWorkspace(generation)
    }
    return result
  }

  selectRun(runId: string): void {
    useRunStore.getState().selectRun(runId)
    this.loadRunSnapshot(runId, this.generation)
  }

  retryRun(runId: string): void {
    this.loadRunSnapshot(runId, this.generation, true)
  }

  async createProject(name: string): Promise<void> {
    await this.switchProject(() => this.api.newProject(name))
  }

  async openProject(projectId: string): Promise<void> {
    await this.switchProject(() => this.api.openProject(projectId))
  }

  async interrupt(runId: string): Promise<void> {
    await this.api.interruptRun(runId)
  }

  async respondPermission(runId: string, requestId: string, decision: PermissionDecision): Promise<void> {
    await this.api.respondRunPermission(runId, requestId, decision)
  }

  async respondQuestion(runId: string, requestId: string, answer: string, via: string): Promise<void> {
    await this.api.respondRunQuestion(runId, requestId, answer, via)
  }

  private async switchProject(operation: () => Promise<unknown>): Promise<void> {
    if (!this.active) throw new Error('工作台尚未初始化')
    const generation = this.rotateGeneration()
    try {
      await operation()
    } catch (error) {
      await this.refreshWorkspace(generation)
      throw error
    }
    await this.refreshWorkspace(generation)
  }

  private rotateGeneration(): number {
    this.generation += 1
    const generation = this.generation
    this.unsubscribeAll()
    this.workspaceLoaded = false
    this.workspaceLoading = false
    this.workspaceLoadingGeneration = null
    this.workspaceRefreshPromise = null
    this.workspaceReloadRequested = false
    this.workspaceEvents = []
    this.runEvents = []
    this.runLoads.clear()
    useWorkspaceStore.getState().reset()
    useWorkspaceStore.getState().setLoading()
    useRunStore.getState().clear()
    this.bindEvents(generation)
    return generation
  }

  private bindEvents(generation: number): void {
    this.unsubscribe = [
      this.api.subscribeWorkspaceEvents((event) => {
        if (!this.isCurrent(generation)) return
        this.handleWorkspaceEvent(event, generation)
      }),
      this.api.subscribeRunEvents((event) => {
        if (!this.isCurrent(generation)) return
        this.handleRunEvent(event, generation)
      }),
    ]
  }

  private unsubscribeAll(): void {
    for (const unsubscribe of this.unsubscribe) unsubscribe()
    this.unsubscribe = []
  }

  private isCurrent(generation: number): boolean {
    return this.active && this.generation === generation
  }

  private handleWorkspaceEvent(event: WorkspaceChangedEvent, generation: number): void {
    if (!this.workspaceLoaded) {
      this.workspaceEvents = [...this.workspaceEvents, event].slice(-MAX_BUFFERED_EVENTS)
      this.workspaceReloadRequested = true
      if (!this.workspaceLoading) void this.refreshWorkspace(generation)
      return
    }

    const workspace = useWorkspaceStore.getState()
    const currentProjectId = workspace.snapshot?.project?.id ?? null
    if (event.reason === 'project' && event.revision > workspace.revision && event.projectId !== currentProjectId) {
      const nextGeneration = this.rotateGeneration()
      void this.refreshWorkspace(nextGeneration)
      return
    }
    if (event.projectId !== currentProjectId) return
    if (event.revision > workspace.revision) void this.refreshWorkspace(generation)
  }

  private handleRunEvent(event: RunEvent, generation: number): void {
    if (!this.workspaceLoaded) {
      this.runEvents = [...this.runEvents, event].slice(-MAX_BUFFERED_EVENTS)
      return
    }

    const currentProjectId = useWorkspaceStore.getState().snapshot?.project?.id ?? null
    if (!currentProjectId || event.projectId !== currentProjectId) return
    useRunStore.getState().receiveEvent(event)
    this.loadRequestedRuns(generation)
    if (event.type === 'status' || event.type === 'pending') {
      void this.refreshWorkspace(generation)
    }
  }

  private refreshWorkspace(generation: number): Promise<void> {
    if (!this.isCurrent(generation)) return Promise.resolve()
    if (this.workspaceLoading && this.workspaceLoadingGeneration === generation) {
      this.workspaceReloadRequested = true
      return this.workspaceRefreshPromise ?? Promise.resolve()
    }

    const refresh = this.performWorkspaceRefresh(generation)
    this.workspaceRefreshPromise = refresh
    void refresh.then(
      () => {
        if (this.workspaceRefreshPromise === refresh) this.workspaceRefreshPromise = null
      },
      () => {
        if (this.workspaceRefreshPromise === refresh) this.workspaceRefreshPromise = null
      },
    )
    return refresh
  }

  private async performWorkspaceRefresh(generation: number): Promise<void> {
    if (!this.isCurrent(generation)) return
    this.workspaceLoading = true
    this.workspaceLoadingGeneration = generation
    useWorkspaceStore.getState().setLoading()
    let attempts = 0
    try {
      do {
        this.workspaceReloadRequested = false
        const result = await this.api.getWorkspaceSnapshot()
        if (!this.isCurrent(generation)) return
        const accepted = useWorkspaceStore.getState().applySnapshot(result)
        if (!accepted) {
          this.workspaceReloadRequested = true
          attempts += 1
          continue
        }

        const projectId = result.snapshot.project?.id ?? null
        useRunStore.getState().setWorkspaceSnapshot(projectId, result.snapshot.runs ?? [])
        this.workspaceLoaded = true
        const queuedWorkspaceEvents = this.workspaceEvents
        this.workspaceEvents = []
        if (queuedWorkspaceEvents.some((event) =>
          event.revision > result.revision && (event.projectId === projectId || event.reason === 'project'),
        )) {
          this.workspaceReloadRequested = true
        }

        const queuedRunEvents = this.runEvents
        this.runEvents = []
        for (const event of queuedRunEvents) {
          if (event.projectId === projectId) useRunStore.getState().receiveEvent(event)
        }
        if (queuedRunEvents.some((event) => event.type === 'status' || event.type === 'pending')) {
          this.workspaceReloadRequested = true
        }
        this.loadRequestedRuns(generation)
        attempts += 1
      } while (this.workspaceReloadRequested && attempts < 3)
    } catch (error) {
      if (this.isCurrent(generation)) useWorkspaceStore.getState().setError(messageOf(error))
    } finally {
      if (this.workspaceLoadingGeneration === generation) {
        this.workspaceLoading = false
        this.workspaceLoadingGeneration = null
        if (this.isCurrent(generation) && this.workspaceReloadRequested) {
          this.workspaceReloadRequested = false
          await this.refreshWorkspace(generation)
        }
      }
    }
  }

  private loadRequestedRuns(generation: number): void {
    const store = useRunStore.getState()
    const ids = Object.entries(store.runs)
      .filter(([, view]) => view.needsSnapshot && !view.loading)
      .map(([runId]) => runId)
    for (const runId of ids) this.loadRunSnapshot(runId, generation)
  }

  private loadRunSnapshot(runId: string, generation: number, force = false): Promise<void> {
    if (!this.isCurrent(generation)) return Promise.resolve()
    const existingLoad = this.runLoads.get(runId)
    if (existingLoad) return existingLoad
    const view = useRunStore.getState().runs[runId]
    if (!force && view?.snapshot && !view.needsSnapshot) return Promise.resolve()

    const projectId = useRunStore.getState().projectId
    if (!projectId) return Promise.resolve()
    const previousSeq = view?.lastSeq ?? 0
    useRunStore.getState().beginSnapshot(runId)

    const load = this.api.getRunSnapshot(runId)
      .then((snapshot) => {
        if (!this.isCurrent(generation)) return
        if (snapshot.projectId !== projectId) {
          useRunStore.getState().failSnapshot(runId, '运行快照所属工程与当前工程不一致，已忽略。')
          return
        }
        useRunStore.getState().applySnapshot(snapshot)
        const refreshed = useRunStore.getState().runs[runId]
        if (refreshed?.needsSnapshot) {
          if (snapshot.lastSeq > previousSeq) {
            queueMicrotask(() => this.loadRunSnapshot(runId, generation, true))
          } else {
            useRunStore.getState().failSnapshot(runId, '运行事件仍有序号缺口，请重试同步。')
          }
        }
      })
      .catch((error: unknown) => {
        if (this.isCurrent(generation)) useRunStore.getState().failSnapshot(runId, messageOf(error))
      })
      .finally(() => {
        if (this.runLoads.get(runId) === load) this.runLoads.delete(runId)
        if (this.isCurrent(generation)) this.loadRequestedRuns(generation)
      })
    this.runLoads.set(runId, load)
    return load
  }
}
