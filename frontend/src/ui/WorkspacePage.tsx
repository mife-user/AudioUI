import { useCallback, useEffect, useRef } from 'react'
import type { GraphMutationReceipt, RunStartResult, WorkspaceSnapshot } from '../../bindings/audio-ui/internal/domain/entity'
import type { WorkspaceStatus } from '../api/workspace'
import { useWorkspaceStore } from '../stores/workspaceStore'
import { graphAPI, type GraphAPI } from '../canvas/graphApi'
import { WorkspaceCanvas } from '../canvas/WorkspaceCanvas'

type Props = {
  status: WorkspaceStatus
  snapshot: WorkspaceSnapshot | null
  error: string
  onRetry: () => void
  onGraphMutation?: (receipt: GraphMutationReceipt) => void | Promise<void>
  onRegisterDraftFlush?: (flush: (projectId: string) => Promise<boolean>) => () => void
  onStartRun?: (nodeId: string, projectId: string) => Promise<RunStartResult>
  api?: GraphAPI
}

export function WorkspacePage({ status, snapshot, error, onRetry, onGraphMutation, onRegisterDraftFlush, onStartRun, api = graphAPI }: Props) {
  const projectId = snapshot?.project?.id ?? ''
  const currentProjectId = useRef(projectId)
  currentProjectId.current = projectId
  const selectionSequence = useRef(0)
  const selectionQueue = useRef(Promise.resolve())
  const confirmedSelection = useRef(snapshot?.project?.activeNodeId ?? '')

  useEffect(() => {
    selectionSequence.current += 1
    confirmedSelection.current = snapshot?.project?.activeNodeId ?? ''
  }, [projectId])

  const selectNode = useCallback(async (id: string) => {
    const requestId = ++selectionSequence.current
    const ownerProjectId = projectId
    const store = useWorkspaceStore.getState()
    store.setActiveNode(id)
    const operation = selectionQueue.current.catch(() => undefined).then(async () => {
      if (currentProjectId.current !== ownerProjectId || requestId !== selectionSequence.current) return
      try {
        await api.selectNode(id, ownerProjectId)
        if (currentProjectId.current === ownerProjectId) confirmedSelection.current = id
      } catch (failure) {
        if (currentProjectId.current !== ownerProjectId || requestId !== selectionSequence.current) return
        store.setActiveNode(confirmedSelection.current)
        throw failure
      }
    })
    selectionQueue.current = operation.catch(() => undefined)
    await operation
  }, [api, projectId])

  if (!snapshot && status === 'loading') {
    return (
      <main className="workspace-main" aria-label="工作台">
        <div className="workspace-state" role="status">正在读取工作台快照…</div>
      </main>
    )
  }

  if (!snapshot && status === 'error') {
    return (
      <main className="workspace-main" aria-label="工作台">
        <section className="workspace-state workspace-state-error" role="alert">
          <h1>工作台加载失败</h1>
          <p>{error || '读取工程快照失败。'}</p>
          <button className="primary-btn" type="button" onClick={onRetry}>重试</button>
        </section>
      </main>
    )
  }

  if (!snapshot?.project) {
    return (
      <main className="workspace-main" aria-label="工作台">
        {status === 'error' ? (
          <div className="workspace-sync-error" role="alert">
            <span>同步失败：{error || '未知错误'}</span>
            <button className="agent-mini-btn" type="button" onClick={onRetry}>重试</button>
          </div>
        ) : null}
        <section className="workspace-empty">
          <span className="workspace-kicker">LOCAL WORKSPACE</span>
          <h1>还没有打开的工程</h1>
          <p>从顶栏新建工程，或打开本机已有工程。工程数据保留在程序目录中。</p>
        </section>
      </main>
    )
  }

  return (
    <main className="workspace-main" aria-label="工作台">
      {status === 'loading' ? <div className="workspace-sync-notice" role="status">正在同步工程快照…</div> : null}
      {status === 'error' ? (
        <div className="workspace-sync-error" role="alert">
          <span>快照同步失败：{error || '未知错误'}</span>
          <button className="agent-mini-btn" type="button" onClick={onRetry}>重试</button>
        </div>
      ) : null}
      <WorkspaceCanvas snapshot={snapshot} onSelectNode={selectNode} onGraphMutation={onGraphMutation} onRegisterDraftFlush={onRegisterDraftFlush} onStartRun={onStartRun} api={api} />
    </main>
  )
}
