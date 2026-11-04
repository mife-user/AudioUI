import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { WorkspaceSnapshot } from '../../bindings/audio-ui/internal/domain/entity'
import { WorkspacePage } from './WorkspacePage'

const emptyProject: WorkspaceSnapshot = {
  project: { id: 'p_one', name: '测试工程', revision: 2, activeNodeId: '' },
  nodes: [], pending: [], runs: [], chains: [], lockedNodeIds: [],
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

  it('shows the placeholder canvas and records that node editing is deferred', () => {
    render(<WorkspacePage status="ready" snapshot={emptyProject} error="" onRetry={vi.fn()} />)
    expect(screen.getByRole('heading', { name: '测试工程' })).toBeInTheDocument()
    expect(screen.getByText('画布与节点交互将在 M2-09 接入。')).toBeInTheDocument()
  })
})
