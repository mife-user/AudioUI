import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ProjectSummary, WorkspaceProject } from '../../bindings/audio-ui/internal/domain/entity'
import { ProjectSwitcher } from './ProjectSwitcher'

const { listProjects } = vi.hoisted(() => ({ listProjects: vi.fn<() => Promise<ProjectSummary[]>>() }))
vi.mock('../api/workspace', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/workspace')>()
  return { ...actual, listProjects }
})

const project: WorkspaceProject = { id: 'p_current', name: '当前工程', revision: 1, activeNodeId: '' }

beforeEach(() => {
  listProjects.mockReset()
  Object.defineProperty(HTMLDialogElement.prototype, 'showModal', {
    configurable: true,
    value() { this.setAttribute('open', '') },
  })
  Object.defineProperty(HTMLDialogElement.prototype, 'close', {
    configurable: true,
    value() { this.removeAttribute('open') },
  })
})

describe('ProjectSwitcher', () => {
  it('disables project switching while a run is active', () => {
    render(<ProjectSwitcher project={project} blocked onNew={vi.fn()} onOpen={vi.fn()} />)
    expect(screen.getByRole('button', { name: /当前工程/ })).toBeDisabled()
  })

  it('shows a backend BUSY race as a visible retryable message', async () => {
    listProjects.mockResolvedValue([
      { id: 'p_current', name: '当前工程', revision: 1 },
      { id: 'p_other', name: '另一个工程', revision: 2 },
    ])
    const onOpen = vi.fn().mockRejectedValue(new Error('BUSY'))
    render(<ProjectSwitcher project={project} blocked={false} onNew={vi.fn()} onOpen={onOpen} />)
    fireEvent.click(screen.getByRole('button', { name: /当前工程/ }))
    fireEvent.click(await screen.findByRole('button', { name: /另一个工程/ }))

    expect(onOpen).toHaveBeenCalledWith('p_other')
    expect(await screen.findByRole('alert')).toHaveTextContent('已有运行在途，工程切换暂不可用（BUSY）。')
  })

  it('keeps the name and reports a failed project creation', async () => {
    listProjects.mockResolvedValue([])
    const onNew = vi.fn().mockRejectedValue(new Error('BUSY'))
    render(<ProjectSwitcher project={null} blocked={false} onNew={onNew} onOpen={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: /选择工程/ }))
    const input = screen.getByLabelText('新建工程')
    fireEvent.change(input, { target: { value: '本地工程' } })
    fireEvent.click(screen.getByRole('button', { name: '新建' }))

    expect(onNew).toHaveBeenCalledWith('本地工程')
    expect(await screen.findByRole('alert')).toHaveTextContent('已有运行在途，工程切换暂不可用（BUSY）。')
    expect(input).toHaveValue('本地工程')
  })

  it('offers save retry, explicit discard, or staying after draft save fails', async () => {
    listProjects.mockResolvedValue([
      { id: 'p_current', name: '当前工程', revision: 1 },
      { id: 'p_other', name: '另一个工程', revision: 2 },
    ])
    const onOpen = vi.fn()
      .mockRejectedValueOnce(new Error('DRAFT_SAVE_FAILED: 提示词保存失败'))
      .mockResolvedValue(undefined)
    render(<ProjectSwitcher project={project} blocked={false} onNew={vi.fn()} onOpen={onOpen} />)
    fireEvent.click(screen.getByRole('button', { name: /当前工程/ }))
    fireEvent.click(await screen.findByRole('button', { name: /另一个工程/ }))

    expect(await screen.findByRole('heading', { name: '当前工程有未保存草稿' })).toBeInTheDocument()
    expect(screen.getByRole('alert')).toHaveTextContent('提示词保存失败')
    fireEvent.click(screen.getByRole('button', { name: '留在当前工程' }))
    expect(onOpen).toHaveBeenCalledOnce()
    expect(screen.getByRole('button', { name: /当前工程/ })).toBeInTheDocument()
  })

  it('retries draft persistence before opening the selected project', async () => {
    listProjects.mockResolvedValue([
      { id: 'p_current', name: '当前工程', revision: 1 },
      { id: 'p_other', name: '另一个工程', revision: 2 },
    ])
    const onOpen = vi.fn()
      .mockRejectedValueOnce(new Error('DRAFT_SAVE_FAILED: 磁盘暂不可用'))
      .mockResolvedValue(undefined)
    render(<ProjectSwitcher project={project} blocked={false} onNew={vi.fn()} onOpen={onOpen} />)
    fireEvent.click(screen.getByRole('button', { name: /当前工程/ }))
    fireEvent.click(await screen.findByRole('button', { name: /另一个工程/ }))
    const retry = await screen.findByRole('button', { name: '重试保存并继续' })
    await waitFor(() => expect(retry).toBeEnabled())
    fireEvent.click(retry)

    await waitFor(() => expect(onOpen).toHaveBeenCalledTimes(2))
    expect(onOpen).toHaveBeenNthCalledWith(2, 'p_other', false)
  })

  it('only switches after an explicit draft discard choice', async () => {
    listProjects.mockResolvedValue([
      { id: 'p_current', name: '当前工程', revision: 1 },
      { id: 'p_other', name: '另一个工程', revision: 2 },
    ])
    const onOpen = vi.fn()
      .mockRejectedValueOnce(new Error('DRAFT_SAVE_FAILED: 标题不能为空'))
      .mockResolvedValue(undefined)
    render(<ProjectSwitcher project={project} blocked={false} onNew={vi.fn()} onOpen={onOpen} />)
    fireEvent.click(screen.getByRole('button', { name: /当前工程/ }))
    fireEvent.click(await screen.findByRole('button', { name: /另一个工程/ }))
    const discard = await screen.findByRole('button', { name: '放弃未保存草稿并继续' })
    await waitFor(() => expect(discard).toBeEnabled())
    fireEvent.click(discard)

    await waitFor(() => expect(onOpen).toHaveBeenCalledTimes(2))
    expect(onOpen).toHaveBeenNthCalledWith(2, 'p_other', true)
  })
})
