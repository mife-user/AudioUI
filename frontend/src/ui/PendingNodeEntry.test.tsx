import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { Node, PendingDetail, PendingSummary } from '../../bindings/audio-ui/internal/domain/entity'
import { OutputKind, PendingStatus } from '../../bindings/audio-ui/internal/domain/valueobject'
import { PendingControllerProvider, usePendingController } from '../hooks/usePendingController'
import { PendingNodeEntry } from './PendingNodeEntry'

const topic: Node = {
  id: 'n_topic',
  type: 'topic' as Node['type'],
  title: '选题节点',
  parent: '',
  position: { x: 0, y: 0 },
  size: { width: 280, height: 250 },
  params: {},
  rounds: [],
  locked: false,
  stale: false,
}

const pending: PendingSummary = {
  id: 'pending_topics',
  nodeId: topic.id,
  runId: 'r_topics',
  chainId: 'c_topic',
  kind: OutputKind.OutputKindTopics,
  status: PendingStatus.PendingInvalid,
  summary: '选题结果校验失败',
  error: 'SCHEMA_INVALID: topics is empty',
}

const detail: PendingDetail = {
  pending: {
    ...pending,
    payload: 'pending/pending_topics.json',
    chainPath: [topic.id],
    createdAt: '2026-11-09T10:00:00Z',
  },
  rawPayload: btoa('{"topics":[]}'),
}
const topicNodes = [topic]
const pendingItems = [pending]
const pendingIds = [pending.id]
const applyPending = vi.fn().mockResolvedValue(undefined)

type Props = {
  onGetPending: (id: string, projectId: string) => Promise<PendingDetail>
  onDiscardPending: (id: string, projectId: string) => Promise<void>
  onRefreshWorkspace: () => Promise<void>
}

function Harness({ onGetPending, onDiscardPending, onRefreshWorkspace }: Props) {
  const controller = usePendingController({
    projectId: 'p_demo',
    nodes: topicNodes,
    pending: pendingItems,
    onGetPending,
    onApplyPending: applyPending,
    onDiscardPending,
    onRefreshWorkspace,
  })
  return (
    <PendingControllerProvider controller={controller}>
      <PendingNodeEntry node={topic} pendingIds={pendingIds} />
    </PendingControllerProvider>
  )
}

describe('PendingNodeEntry', () => {
  it('opens the Topic-owned result and preserves the invalid-output recovery actions', async () => {
    const onGetPending = vi.fn().mockResolvedValue(detail)
    const onDiscardPending = vi.fn().mockResolvedValue(undefined)
    const onRefreshWorkspace = vi.fn().mockResolvedValue(undefined)
    render(<Harness onGetPending={onGetPending} onDiscardPending={onDiscardPending} onRefreshWorkspace={onRefreshWorkspace} />)

    fireEvent.click(screen.getByRole('button', { name: '处理无效结果' }))
    expect(await screen.findByRole('dialog', { name: '结果 pending_topics' })).toBeInTheDocument()
    expect(screen.getByText('{"topics":[]}')).toBeInTheDocument()
    expect(onGetPending).toHaveBeenCalledWith('pending_topics', 'p_demo')

    fireEvent.click(screen.getByRole('button', { name: '丢弃' }))
    await waitFor(() => expect(onDiscardPending).toHaveBeenCalledWith('pending_topics', 'p_demo'))
    expect(onRefreshWorkspace).toHaveBeenCalledOnce()
  })
})
