import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { Node, NodeParams } from '../../bindings/audio-ui/internal/domain/entity'
import { PromptParameters } from './PromptParameters'

function makeNode(overrides: Partial<Node> = {}): Node {
  return {
    id: 'n_prompt',
    type: 'prompt' as Node['type'],
    title: '脚本任务',
    parent: 'n_candidate',
    position: { x: 0, y: 0 },
    params: { instruction: '写一个解说脚本。', constraints: { scenes: 3, duration: 90 } },
    rounds: [],
    locked: false,
    stale: false,
    ...overrides,
  }
}

type SaveParams = (nodeId: string, title: string, params: NodeParams, projectId: string) => Promise<boolean | void>

type Flush = () => Promise<boolean>

function renderPrompt(onSave = vi.fn<SaveParams>().mockResolvedValue(true)) {
  let flush: Flush | undefined
  const onRegisterFlush = vi.fn((_nodeId: string, _projectId: string, callback: Flush) => {
    flush = callback
    return () => { flush = undefined }
  })
  render(
    <PromptParameters
      node={makeNode()}
      projectId="p_test"
      disabled={false}
      onSave={onSave}
      onRegisterFlush={onRegisterFlush}
    />,
  )
  return { onSave, onRegisterFlush, getFlush: () => flush }
}

describe('PromptParameters', () => {
  it('persists task and optional constraints before the Agent starts', async () => {
    const { onSave, getFlush } = renderPrompt()
    fireEvent.change(screen.getByRole('textbox', { name: '任务' }), { target: { value: '按候选写一段旁白。' } })
    fireEvent.change(screen.getByRole('spinbutton', { name: '场景数约束' }), { target: { value: '5' } })

    await waitFor(() => expect(getFlush()).toBeTypeOf('function'))
    await act(async () => { await expect(getFlush()?.()).resolves.toBe(true) })
    expect(onSave).toHaveBeenCalledWith('n_prompt', '脚本任务', {
      instruction: '按候选写一段旁白。',
      constraints: { scenes: 5, duration: 90 },
    }, 'p_test')
    expect(screen.getByRole('status')).toHaveTextContent('已保存')
  })

  it('waits for input composition to finish before saving the final Prompt draft', async () => {
    vi.useFakeTimers()
    try {
      const onSave = vi.fn<SaveParams>().mockResolvedValue(true)
      renderPrompt(onSave)
      const task = screen.getByRole('textbox', { name: '任务' })
      const finalValue = '中文输入😀\n第二行'

      fireEvent.compositionStart(task)
      fireEvent.change(task, { target: { value: finalValue } })
      await act(async () => { await vi.advanceTimersByTimeAsync(400) })
      expect(onSave).not.toHaveBeenCalled()

      fireEvent.compositionEnd(task, { data: finalValue })
      await act(async () => { await vi.advanceTimersByTimeAsync(320) })
      expect(onSave).toHaveBeenCalledOnce()
      expect(onSave).toHaveBeenCalledWith(
        'n_prompt',
        '脚本任务',
        { instruction: finalValue, constraints: { scenes: 3, duration: 90 } },
        'p_test',
      )
    } finally {
      vi.useRealTimers()
    }
  })

  it('retains edits and reports a failed save so the run cannot use stale parameters', async () => {
    const onSave = vi.fn<SaveParams>()
      .mockRejectedValueOnce(new Error('disk full'))
      .mockResolvedValueOnce(true)
    const { getFlush } = renderPrompt(onSave)
    const task = screen.getByRole('textbox', { name: '任务' })
    fireEvent.change(task, { target: { value: '保存失败后仍保留的任务。' } })

    await waitFor(() => expect(getFlush()).toBeTypeOf('function'))
    await act(async () => { await expect(getFlush()?.()).rejects.toThrow('disk full') })
    expect(task).toHaveValue('保存失败后仍保留的任务。')
    expect(screen.getByRole('alert')).toHaveTextContent('disk full')

    fireEvent.click(screen.getByRole('button', { name: '重试保存' }))
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(2))
    expect(screen.getByRole('status')).toHaveTextContent('已保存')
  })

  it('does not save invalid constraint values', async () => {
    const { onSave, getFlush } = renderPrompt()
    fireEvent.change(screen.getByRole('spinbutton', { name: '场景数约束' }), { target: { value: '-1' } })

    await waitFor(() => expect(getFlush()).toBeTypeOf('function'))
    await expect(getFlush()?.()).rejects.toThrow('场景数必须是非负整数。')
    expect(onSave).not.toHaveBeenCalled()
  })
})
