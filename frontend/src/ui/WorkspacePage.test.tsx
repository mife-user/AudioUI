import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { WorkspaceSnapshot } from '../../bindings/audio-ui/internal/domain/entity'
import type { GraphAPI } from '../canvas/graphApi'
import { useWorkspaceStore } from '../stores/workspaceStore'
import { WorkspacePage } from './WorkspacePage'

vi.mock('../canvas/WorkspaceCanvas', () => ({
  WorkspaceCanvas: ({ onSelectNode }: { onSelectNode: (id: string) => Promise<void> }) => (
    <section aria-label="节点画布">
      可创建选题和提示词草稿。
      <button type="button" onClick={() => void onSelectNode('n_selected').catch(() => undefined)}>选择节点</button>
    </section>
  ),
}))

const emptyProject: WorkspaceSnapshot = {
  project: { id: 'p_one', name: '测试工程', revision: 2, activeNodeId: '' },
  nodes: [], pending: [], runs: [], chains: [], lockedNodeIds: [],
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((resolvePromise) => { resolve = resolvePromise })
  return { promise, resolve }
}

describe('WorkspacePage', () => {
  it('shows loading and retryable error states', () => {
    const onRetry = vi.fn()
    const { rerender } = render(<WorkspacePage status="loading" snapshot={null} error="" onRetry={onRetry} />)
    expect(screen.getByRole('status')).toHaveTextContent('正在读取工作台快照')

    rerender(<WorkspacePage status="error" snapshot={null} error="WORKSPACE_CHANGED" onRetry={onRetry} />)
    expect(screen.getByRole('alert')).toHaveTextContent('WORKSPACE_CHANGED')
    fireEvent.click(screen.getByRole('button', { name: '重试' }))
    expect(onRetry).toHaveBeenCalledOnce()
  })

  it('shows an empty project state without fabricating a project', () => {
    render(<WorkspacePage status="ready" snapshot={{ ...emptyProject, project: null }} error="" onRetry={vi.fn()} />)
    expect(screen.getByText('还没有打开的工程')).toBeInTheDocument()
  })

  it('shows the node canvas for an empty project', () => {
    render(<WorkspacePage status="ready" snapshot={emptyProject} error="" onRetry={vi.fn()} />)
    expect(screen.getByRole('region', { name: '节点画布' })).toBeInTheDocument()
    expect(screen.getByText('可创建选题和提示词草稿。')).toBeInTheDocument()
  })

  it('keeps a successful local selection when an older snapshot arrives during SelectNode', async () => {
    const projectSnapshot: WorkspaceSnapshot = {
      ...emptyProject,
      project: { id: 'p_one', name: '测试工程', revision: 4, activeNodeId: 'n_initial' },
    }
    useWorkspaceStore.getState().applySnapshot({ revision: 12, snapshot: projectSnapshot })
    const selected = deferred<{ projectId: string; projectRevision: number; nodeId: string }>()
    const api: GraphAPI = {
      addNode: vi.fn().mockResolvedValue({ projectId: 'p_one', projectRevision: 4, nodeId: 'n_created' }),
      connect: vi.fn().mockResolvedValue({ projectId: 'p_one', projectRevision: 4, nodeId: 'n_selected' }),
      moveNode: vi.fn().mockResolvedValue({ projectId: 'p_one', projectRevision: 4, nodeId: 'n_selected' }),
      setNodeLayout: vi.fn().mockResolvedValue({ projectId: 'p_one', projectRevision: 4, nodeId: 'n_selected' }),
      setNodeCollapsed: vi.fn().mockResolvedValue({ projectId: 'p_one', projectRevision: 4, nodeId: 'n_selected' }),
      selectNode: vi.fn(() => selected.promise),
      updateNodeParams: vi.fn().mockResolvedValue({ projectId: 'p_one', projectRevision: 4, nodeId: 'n_selected' }),
      getTopicCandidates: vi.fn().mockResolvedValue([]),
      getScriptVersion: vi.fn().mockResolvedValue({ nodeId: 'n_script', version: { n: 1, artifactId: 'sha256:script', runId: 'r_script' }, payload: '{"title":"脚本","scenes":[{"narration":"正文","duration":1}]}' }),
    }
    render(<WorkspacePage status="ready" snapshot={projectSnapshot} error="" onRetry={vi.fn()} api={api} />)

    fireEvent.click(screen.getByRole('button', { name: '选择节点' }))
    await waitFor(() => expect(api.selectNode).toHaveBeenCalledWith('n_selected', 'p_one'))
    useWorkspaceStore.getState().applySnapshot({ revision: 12, snapshot: projectSnapshot })
    selected.resolve({ projectId: 'p_one', projectRevision: 4, nodeId: 'n_selected' })

    await waitFor(() => expect(useWorkspaceStore.getState().snapshot?.project?.activeNodeId).toBe('n_selected'))
  })

  it('persists selection locally and restores the previous node when SelectNode fails', async () => {
    const projectSnapshot: WorkspaceSnapshot = {
      ...emptyProject,
      project: { id: 'p_one', name: '测试工程', revision: 4, activeNodeId: 'n_initial' },
    }
    useWorkspaceStore.getState().applySnapshot({ revision: 12, snapshot: projectSnapshot })
    const api: GraphAPI = {
      addNode: vi.fn().mockResolvedValue({ projectId: 'p_one', projectRevision: 4, nodeId: 'n_created' }),
      connect: vi.fn().mockResolvedValue({ projectId: 'p_one', projectRevision: 4, nodeId: 'n_selected' }),
      moveNode: vi.fn().mockResolvedValue({ projectId: 'p_one', projectRevision: 4, nodeId: 'n_selected' }),
      setNodeLayout: vi.fn().mockResolvedValue({ projectId: 'p_one', projectRevision: 4, nodeId: 'n_selected' }),
      setNodeCollapsed: vi.fn().mockResolvedValue({ projectId: 'p_one', projectRevision: 4, nodeId: 'n_selected' }),
      selectNode: vi.fn().mockRejectedValue(new Error('select failed')),
      updateNodeParams: vi.fn().mockResolvedValue({ projectId: 'p_one', projectRevision: 4, nodeId: 'n_selected' }),
      getTopicCandidates: vi.fn().mockResolvedValue([]),
      getScriptVersion: vi.fn().mockResolvedValue({ nodeId: 'n_script', version: { n: 1, artifactId: 'sha256:script', runId: 'r_script' }, payload: '{"title":"脚本","scenes":[{"narration":"正文","duration":1}]}' }),
    }
    render(<WorkspacePage status="ready" snapshot={projectSnapshot} error="" onRetry={vi.fn()} api={api} />)

    fireEvent.click(screen.getByRole('button', { name: '选择节点' }))
    await waitFor(() => expect(api.selectNode).toHaveBeenCalledWith('n_selected', 'p_one'))
    await waitFor(() => expect(useWorkspaceStore.getState().snapshot?.project?.activeNodeId).toBe('n_initial'))
    expect(useWorkspaceStore.getState().revision).toBe(12)
  })
})
