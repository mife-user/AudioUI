import type { WorkspaceSnapshot } from '../../bindings/audio-ui/internal/domain/entity'
import type { WorkspaceStatus } from '../api/workspace'

type Props = {
  status: WorkspaceStatus
  snapshot: WorkspaceSnapshot | null
  error: string
  onRetry: () => void
}

export function WorkspacePage({ status, snapshot, error, onRetry }: Props) {
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
      <section className="workspace-canvas-placeholder" aria-label="画布占位区">
        <div className="workspace-placeholder-header">
          <div>
            <span className="workspace-kicker">工作台</span>
            <h1>{snapshot.project.name}</h1>
          </div>
          <div className="workspace-revision">图版本 <code>{snapshot.project.revision}</code></div>
        </div>
        {snapshot.nodes?.length ? (
          <div className="workspace-placeholder-summary">
            已加载 {snapshot.nodes.length} 个节点 · {snapshot.runs?.length ?? 0} 条运行记录
          </div>
        ) : (
          <div className="workspace-empty-canvas">
            <span aria-hidden="true">＋</span>
            <strong>工程已就绪</strong>
            <p>画布与节点交互将在 M2-09 接入。</p>
          </div>
        )}
        <div className="workspace-placeholder-note">
          当前阶段可查看工程身份、运行历史与运行控制台；图编辑仍由后续任务接入。
        </div>
      </section>
    </main>
  )
}
