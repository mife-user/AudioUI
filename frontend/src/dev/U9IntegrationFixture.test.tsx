import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { WorkspaceSnapshot } from '../../bindings/audio-ui/internal/domain/entity'
import { U9IntegrationFixture } from './U9IntegrationFixture'

vi.mock('../canvas/WorkspaceCanvas', () => ({
  WorkspaceCanvas: ({ snapshot, workspaceSyncError }: { snapshot: WorkspaceSnapshot; workspaceSyncError?: { message: string; retry: () => void } }) => (
    <section aria-label="节点画布">
      <span>{snapshot.project?.name}</span>
      {(snapshot.nodes ?? []).map((node) => <span key={node.id}>{node.title}</span>)}
      {workspaceSyncError ? <div role="alert">{workspaceSyncError.message}</div> : null}
    </section>
  ),
}))

describe('U9IntegrationFixture', () => {
  it('provides a local project graph, Script pending, and an active permission request', async () => {
    render(<U9IntegrationFixture />)

    expect(await screen.findByText('U9 离线集成样例')).toBeInTheDocument()
    expect(screen.getByText('街巷里的修表铺')).toBeInTheDocument()
    expect(screen.getByText('人物短片任务')).toBeInTheDocument()
    expect(await screen.findByRole('button', { name: '请求 1' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '结果 1' })).toBeInTheDocument()
  })

  it('replays injected permission and failure events in the run store', async () => {
    render(<U9IntegrationFixture />)
    await screen.findByRole('button', { name: '请求 1' })

    fireEvent.click(screen.getByRole('button', { name: '注入权限事件' }))
    expect(await screen.findByRole('button', { name: '请求 2' })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '展开运行面板' }))
    fireEvent.click(screen.getByRole('button', { name: '注入运行失败' }))
    await waitFor(() => expect(screen.getByText('FIXTURE_RUN_FAILED: 演示运行失败，不涉及真实 Agent。')).toBeInTheDocument())
  })

  it('routes permission decisions back through a local request-resolved event', async () => {
    render(<U9IntegrationFixture />)
    await screen.findByRole('button', { name: '请求 1' })
    fireEvent.click(screen.getByRole('button', { name: '展开运行面板' }))
    fireEvent.click(screen.getByRole('button', { name: '请求 1' }))
    fireEvent.click(await screen.findByRole('button', { name: '定位节点' }))

    const permission = await screen.findByRole('region', { name: '权限确认' })
    fireEvent.click(within(permission).getByRole('button', { name: '允许一次' }))

    await waitFor(() => expect(screen.getByRole('button', { name: '请求 0' })).toBeInTheDocument())
  })

  it('loads a pending Script and surfaces a simulated append failure', async () => {
    render(<U9IntegrationFixture />)
    fireEvent.click(screen.getByRole('button', { name: '确认追加失败：关' }))
    fireEvent.click(screen.getByRole('button', { name: '打开待确认 Script' }))

    const preview = await screen.findByRole('complementary', { name: '待确认 Script 夹具预览' })
    expect(within(preview).getByText('街巷里的修表铺')).toBeInTheDocument()
    fireEvent.click(within(preview).getByRole('button', { name: '确认追加' }))
    expect(await screen.findByText('确认追加失败：FIXTURE_PENDING_APPLY_FAILED: 模拟追加冲突，原版本保持不变。')).toBeInTheDocument()
  })

  it('recovers a pending detail read and exposes the workspace sync failure state', async () => {
    render(<U9IntegrationFixture />)
    fireEvent.click(screen.getByRole('button', { name: '待确认读取失败：关' }))
    fireEvent.click(screen.getByRole('button', { name: '打开待确认 Script' }))

    const preview = await screen.findByRole('complementary', { name: '待确认 Script 夹具预览' })
    expect(await within(preview).findByText('正文读取失败：FIXTURE_PENDING_READ_FAILED: 模拟正文读取失败。')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '待确认读取失败：开' }))
    fireEvent.click(within(preview).getByRole('button', { name: '重试读取' }))
    expect(await within(preview).findByText('街巷里的修表铺')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '切换工作区失败' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('FIXTURE_WORKSPACE_SYNC_FAILED')
  })
})
