import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { OutputKind } from '../../bindings/audio-ui/internal/domain/valueobject'
import type { Node, NodeParams, RunStartOptions, RunStartResult } from '../../bindings/audio-ui/internal/domain/entity'
import { TopicParameters } from './TopicParameters'

type SaveTopicParams = (nodeId: string, title: string, params: NodeParams, projectId: string) => Promise<boolean | void>
type StartTopicRun = (nodeId: string, projectId: string, options?: RunStartOptions) => Promise<RunStartResult>

function makeNode(overrides: Partial<Node> = {}): Node {
  return {
    id: 'n_topic',
    type: 'topic' as Node['type'],
    title: '选题标题',
    position: { x: 0, y: 0 },
    size: { width: 280, height: 230 },
    params: { idea: '初始想法', direction: '人物观察', candidateCount: 3 },
    rounds: [],
    locked: false,
    stale: false,
    ...overrides,
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

function renderTopic(options: {
  node?: Node
  disabled?: boolean
  globalRunBusy?: boolean
  allowLockedRerun?: boolean
  rerunPending?: boolean
  onSave?: SaveTopicParams
  onRun?: StartTopicRun
} = {}) {
  const onSave = options.onSave ?? vi.fn<SaveTopicParams>().mockResolvedValue(true)
  const onRun = options.onRun ?? vi.fn<StartTopicRun>().mockResolvedValue({} satisfies RunStartResult)
  render(
    <TopicParameters
      node={options.node ?? makeNode()}
      projectId="p_test"
      disabled={options.disabled ?? false}
      globalRunBusy={options.globalRunBusy ?? false}
      allowLockedRerun={options.allowLockedRerun}
      rerunPending={options.rerunPending}
      onSave={onSave}
      onRun={onRun}
    />,
  )
  return { onSave, onRun }
}

describe('TopicParameters', () => {
  it('validates the 3–10 integer range and saves valid topic changes', async () => {
    const { onSave, onRun } = renderTopic()
    const count = screen.getByRole('spinbutton', { name: '候选数' })

    fireEvent.change(count, { target: { value: '2' } })
    expect(screen.getByText('候选数必须是 3–10 之间的整数。')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '生成候选' })).toBeDisabled()

    fireEvent.change(count, { target: { value: '10' } })
    fireEvent.change(screen.getByRole('textbox', { name: '想法' }), { target: { value: '新的想法' } })
    await waitFor(() => expect(onSave).toHaveBeenCalledWith(
      'n_topic',
      '选题标题',
      { idea: '新的想法', direction: '人物观察', candidateCount: 10 },
      'p_test',
    ))
    expect(screen.getByRole('status')).toHaveTextContent('已保存')
    expect(onRun).not.toHaveBeenCalled()
  })

  it('waits for input composition to finish before saving the final topic draft', async () => {
    vi.useFakeTimers()
    try {
      const { onSave } = renderTopic()
      const idea = screen.getByRole('textbox', { name: '想法' })
      const finalValue = '中文输入😀\n第二行'

      fireEvent.compositionStart(idea)
      fireEvent.change(idea, { target: { value: finalValue } })
      await act(async () => { await vi.advanceTimersByTimeAsync(400) })
      expect(onSave).not.toHaveBeenCalled()

      fireEvent.compositionEnd(idea, { data: finalValue })
      await act(async () => { await vi.advanceTimersByTimeAsync(320) })
      expect(onSave).toHaveBeenCalledOnce()
      expect(onSave).toHaveBeenCalledWith(
        'n_topic',
        '选题标题',
        { idea: finalValue, direction: '人物观察', candidateCount: 3 },
        'p_test',
      )
    } finally {
      vi.useRealTimers()
    }
  })

  it('waits for pending parameter persistence before starting a run', async () => {
    const save = deferred<boolean | void>()
    const onSave = vi.fn(() => save.promise)
    const onRun = vi.fn().mockResolvedValue({} satisfies RunStartResult)
    renderTopic({ onSave, onRun })

    fireEvent.change(screen.getByRole('textbox', { name: '想法' }), { target: { value: '运行前的新想法' } })
    fireEvent.click(screen.getByRole('button', { name: '生成候选' }))
    await waitFor(() => expect(onSave).toHaveBeenCalledOnce())
    expect(onRun).not.toHaveBeenCalled()

    save.resolve(true)
    await waitFor(() => expect(onRun).toHaveBeenCalledWith('n_topic', 'p_test', undefined))
  })

  it('serializes consecutive edits so a delayed save cannot replace the latest draft', async () => {
    const firstSave = deferred<boolean | void>()
    const secondSave = deferred<boolean | void>()
    const onSave = vi.fn<SaveTopicParams>()
      .mockImplementationOnce(() => firstSave.promise)
      .mockImplementationOnce(() => secondSave.promise)
    const { onRun } = renderTopic({ onSave })

    fireEvent.change(screen.getByRole('textbox', { name: '想法' }), { target: { value: '先保存的想法' } })
    await waitFor(() => expect(onSave).toHaveBeenCalledOnce())
    fireEvent.change(screen.getByRole('textbox', { name: '方向' }), { target: { value: '最后输入的方向' } })

    firstSave.resolve(true)
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(2))
    expect(onSave).toHaveBeenLastCalledWith(
      'n_topic',
      '选题标题',
      { idea: '先保存的想法', direction: '最后输入的方向', candidateCount: 3 },
      'p_test',
    )
    secondSave.resolve(true)
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('已保存'))

    fireEvent.click(screen.getByRole('button', { name: '生成候选' }))
    await waitFor(() => expect(onRun).toHaveBeenCalledOnce())
  })

  it('retains edits and blocks execution after save failure until an explicit retry succeeds', async () => {
    const onSave = vi.fn()
      .mockRejectedValueOnce(new Error('disk full'))
      .mockResolvedValueOnce(true)
    const onRun = vi.fn().mockResolvedValue({} satisfies RunStartResult)
    renderTopic({ onSave, onRun })
    const idea = screen.getByRole('textbox', { name: '想法' })

    fireEvent.change(idea, { target: { value: '保留在输入框的新想法' } })
    await waitFor(() => expect(onSave).toHaveBeenCalledOnce())
    expect(idea).toHaveValue('保留在输入框的新想法')
    expect(screen.getByRole('alert')).toHaveTextContent('disk full')
    expect(screen.getByRole('button', { name: '生成候选' })).toBeDisabled()
    expect(onRun).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: '重试保存' }))
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(2))
    expect(screen.getByRole('button', { name: '生成候选' })).toBeEnabled()
    fireEvent.click(screen.getByRole('button', { name: '生成候选' }))
    await waitFor(() => expect(onRun).toHaveBeenCalledOnce())
  })

  it('prevents duplicate starts and presents the structured context-budget failure', async () => {
    const run = deferred<RunStartResult>()
    const onRun = vi.fn(() => run.promise)
    renderTopic({ onRun })
    const button = screen.getByRole('button', { name: '生成候选' })

    fireEvent.click(button)
    fireEvent.click(button)
    await waitFor(() => expect(onRun).toHaveBeenCalledOnce())
    run.resolve({ error: {
      code: 'CONTEXT_TOO_LARGE',
      message: 'CONTEXT_TOO_LARGE',
      estimatedTokens: 1_200_001,
      tokenBudget: 1_000_000,
    } })

    expect(await screen.findByRole('alert')).toHaveTextContent('估算 1,200,001 tokens，预算 1,000,000 tokens')
    expect(screen.getByRole('button', { name: '生成候选' })).toBeEnabled()
  })

  it.each([
    ['AGENT_UNAVAILABLE', 'Claude CLI 不可用'],
    ['BUSY', '已有运行正在进行'],
    ['NODE_LOCKED', '已应用候选并锁定'],
    ['RUNNING_CHAIN_LOCKED', '位于运行中的路径'],
  ])('renders the structured %s start failure', async (code, message) => {
    const onRun = vi.fn<StartTopicRun>().mockResolvedValue({
      error: { code, message: code },
    })
    renderTopic({ onRun })

    fireEvent.click(screen.getByRole('button', { name: '生成候选' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(message)
  })

  it('allows a locked Topic rerun with unchanged parameters and an explicit batch option', async () => {
    const onRun = vi.fn<StartTopicRun>().mockResolvedValue({} satisfies RunStartResult)
    const lockedTopic = makeNode({
      locked: true,
      rounds: [{ n: 1, kind: OutputKind.OutputKindTopics, artifactId: 'sha256:topics', pendingId: 'pending_topics', runId: 'r_topics' }],
    })
    renderTopic({ node: lockedTopic, disabled: true, allowLockedRerun: true, onRun })

    expect(screen.getByRole('textbox', { name: '想法' })).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: '按锁定参数重新生成' }))
    await waitFor(() => expect(onRun).toHaveBeenCalledWith('n_topic', 'p_test', { topicRerun: true }))
  })

  it('blocks a Topic rerun while its previous batch is pending', () => {
    const lockedTopic = makeNode({ locked: true })
    renderTopic({ node: lockedTopic, disabled: true, allowLockedRerun: true, rerunPending: true })

    expect(screen.getByRole('button', { name: '批次待处理' })).toBeDisabled()
  })

  it('disables parameter edits and execution when the caller reports a lock or global run', () => {
    const { onSave, onRun } = renderTopic({ disabled: true, globalRunBusy: true })
    expect(screen.getByRole('textbox', { name: '想法' })).toBeDisabled()
    expect(screen.getByRole('button', { name: '已有运行' })).toBeDisabled()
    expect(onSave).not.toHaveBeenCalled()
    expect(onRun).not.toHaveBeenCalled()
  })
})
