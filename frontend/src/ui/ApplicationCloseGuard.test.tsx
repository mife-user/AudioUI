import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApplicationCloseGuard } from './ApplicationCloseGuard'

const runtime = vi.hoisted(() => {
  const handlers = new Map<string, (event: unknown) => void>()
  return {
    handlers,
    on: vi.fn((name: string, handler: (event: unknown) => void) => {
      handlers.set(name, handler)
      return () => handlers.delete(name)
    }),
    emit: vi.fn<(name: string, data: null) => Promise<boolean>>(),
  }
})

vi.mock('@wailsio/runtime', () => ({
  Events: { On: runtime.on, Emit: runtime.emit },
}))

beforeEach(() => {
  runtime.handlers.clear()
  runtime.on.mockClear()
  runtime.emit.mockReset().mockResolvedValue(false)
  Object.defineProperty(HTMLDialogElement.prototype, 'showModal', {
    configurable: true,
    value() { this.setAttribute('open', '') },
  })
  Object.defineProperty(HTMLDialogElement.prototype, 'close', {
    configurable: true,
    value() { this.removeAttribute('open') },
  })
})

function requestClose() {
  const handler = runtime.handlers.get('app:close-requested')
  if (!handler) throw new Error('close listener not registered')
  act(() => handler({ name: 'app:close-requested' }))
}

describe('ApplicationCloseGuard', () => {
  it('flushes drafts before approving the application close', async () => {
    const onFlush = vi.fn().mockResolvedValue(true)
    render(<ApplicationCloseGuard onFlush={onFlush} />)
    requestClose()

    await waitFor(() => expect(onFlush).toHaveBeenCalledOnce())
    await waitFor(() => expect(runtime.emit).toHaveBeenCalledWith('app:close-approved', null))
  })

  it('offers retry, explicit discard, or stay after a failed flush', async () => {
    const onFlush = vi.fn().mockRejectedValueOnce(new Error('快照同步失败')).mockResolvedValue(true)
    render(<ApplicationCloseGuard onFlush={onFlush} />)
    requestClose()

    expect(await screen.findByRole('alert')).toHaveTextContent('快照同步失败')
    fireEvent.click(screen.getByRole('button', { name: '重试保存并关闭' }))
    await waitFor(() => expect(onFlush).toHaveBeenCalledTimes(2))
    await waitFor(() => expect(runtime.emit).toHaveBeenCalledWith('app:close-approved', null))
  })

  it('allows discarding only after the failed-save choice', async () => {
    const onFlush = vi.fn().mockResolvedValue(false)
    render(<ApplicationCloseGuard onFlush={onFlush} />)
    requestClose()

    expect(await screen.findByRole('alert')).toHaveTextContent('工程快照同步仍未完成。')
    fireEvent.click(screen.getByRole('button', { name: '放弃未保存草稿并关闭' }))
    await waitFor(() => expect(runtime.emit).toHaveBeenCalledWith('app:close-approved', null))
    expect(onFlush).toHaveBeenCalledOnce()
  })

  it('does not approve a close cancelled while a flush is in flight', async () => {
    let finishFlush!: (synced: boolean) => void
    const onFlush = vi.fn(() => new Promise<boolean>((resolve) => { finishFlush = resolve }))
    render(<ApplicationCloseGuard onFlush={onFlush} />)
    requestClose()
    expect(await screen.findByRole('status')).toHaveTextContent('正在保存草稿')
    fireEvent.click(screen.getByRole('button', { name: '取消关闭' }))

    await act(async () => {
      finishFlush(true)
      await Promise.resolve()
    })
    expect(runtime.emit).not.toHaveBeenCalled()
  })

  it('keeps the application open when the user declines to discard a failed draft', async () => {
    const onFlush = vi.fn().mockRejectedValue(new Error('磁盘不可用'))
    render(<ApplicationCloseGuard onFlush={onFlush} />)
    requestClose()
    expect(await screen.findByRole('alert')).toHaveTextContent('磁盘不可用')
    fireEvent.click(screen.getByRole('button', { name: '留在应用' }))

    expect(runtime.emit).not.toHaveBeenCalled()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })
})
