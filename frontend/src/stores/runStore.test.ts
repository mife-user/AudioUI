import { beforeEach, describe, expect, it } from 'vitest'
import { PermissionTier, RunStatus, TranscriptKind } from '../../bindings/audio-ui/internal/domain/valueobject'
import type { RunSnapshot, WorkspaceRunSummary } from '../../bindings/audio-ui/internal/domain/entity'
import type { PermissionRequest, QuestionRequest, RunEvent, TranscriptItem } from '../api/runs'
import { useRunStore } from './runStore'

function summary(id: string, status: RunStatus = RunStatus.RunRunning): WorkspaceRunSummary {
  return {
    id, nodeId: `node_${id}`, status, chainId: `c_node_${id}`, chainPath: [`node_${id}`], estimatedTokens: 42,
  }
}

function transcript(id: string, text: string, streaming = false): TranscriptItem {
  return { id, kind: TranscriptKind.TranscriptMessage, role: 'assistant', text, streaming, createdAt: '' }
}

function snapshot(
  id: string,
  lastSeq: number,
  status: RunStatus = RunStatus.RunRunning,
  permissions: PermissionRequest[] = [],
  questions: QuestionRequest[] = [],
  items: TranscriptItem[] = [],
): RunSnapshot {
  return {
    runId: id,
    projectId: 'p_one',
    lastSeq,
    permissions,
    questions,
    run: {
      id,
      treeId: 'tree_one',
      nodeId: `node_${id}`,
      chainId: `c_node_${id}`,
      chainPath: ['tree_one', `node_${id}`],
      status,
      contextVersion: 1,
      transcript: items,
    },
  }
}

function event(runId: string, seq: number, value: Partial<RunEvent> = {}): RunEvent {
  return {
    v: 1, seq, runId, projectId: 'p_one', treeId: 'tree_one', chainId: `c_node_${runId}`,
    nodeId: `node_${runId}`, chainPath: ['tree_one', `node_${runId}`], type: 'message',
    message: transcript(`m${seq}`, `m${seq}`), ...value,
  }
}

const permission: PermissionRequest = {
  id: 'perm_1', tool: 'Write', tier: PermissionTier.PermissionConfirm, risk: '写文件', summary: '写当前节点文件',
  allowSession: false, expiresAt: '2026-11-03T14:00:00Z',
}
const question: QuestionRequest = {
  id: 'question_1', question: '选择方向', options: ['A', 'B'], allowOther: true, expiresAt: '2026-11-03T14:00:00Z',
}

beforeEach(() => useRunStore.getState().clear())

describe('runStore', () => {
  it('restores transcript and active requests from a run snapshot', () => {
    useRunStore.getState().setWorkspaceSnapshot('p_one', [summary('r1')])
    useRunStore.getState().applySnapshot(snapshot('r1', 5, RunStatus.RunRunning, [permission], [question]))

    const view = useRunStore.getState().runs.r1
    expect(view.lastSeq).toBe(5)
    expect(view.snapshot?.permissions).toEqual([permission])
    expect(view.snapshot?.questions).toEqual([question])
  })

  it('deduplicates old events and appends stream deltas before replacing the final message', () => {
    useRunStore.getState().setWorkspaceSnapshot('p_one', [summary('r1')])
    useRunStore.getState().applySnapshot(snapshot('r1', 1))
    useRunStore.getState().receiveEvent(event('r1', 2, { message: transcript('assistant', 'Hel', true) }))
    useRunStore.getState().receiveEvent(event('r1', 2, { message: transcript('assistant', 'duplicate', true) }))
    useRunStore.getState().receiveEvent(event('r1', 3, { message: transcript('assistant', 'lo', true) }))
    useRunStore.getState().receiveEvent(event('r1', 4, { message: transcript('assistant', 'Hello.', false) }))

    const view = useRunStore.getState().runs.r1
    expect(view.lastSeq).toBe(4)
    expect(view.snapshot?.run.transcript?.[0].text).toBe('Hello.')
    expect(view.needsSnapshot).toBe(false)
  })

  it('pauses on a sequence gap, then replays buffered events against the refreshed snapshot', () => {
    useRunStore.getState().setWorkspaceSnapshot('p_one', [summary('r1')])
    useRunStore.getState().applySnapshot(snapshot('r1', 1))
    useRunStore.getState().receiveEvent(event('r1', 3, { message: transcript('m3', 'C') }))
    expect(useRunStore.getState().runs.r1.needsSnapshot).toBe(true)
    expect(useRunStore.getState().runs.r1.lastSeq).toBe(1)

    useRunStore.getState().receiveEvent(event('r1', 2, { message: transcript('m2', 'B') }))
    useRunStore.getState().beginSnapshot('r1')
    useRunStore.getState().applySnapshot(snapshot('r1', 1))

    const view = useRunStore.getState().runs.r1
    expect(view.lastSeq).toBe(3)
    expect(view.snapshot?.run.transcript?.map((item) => item.text)).toEqual(['B', 'C'])
    expect(view.needsSnapshot).toBe(false)
  })

  it('isolates updates by project and run ID', () => {
    useRunStore.getState().setWorkspaceSnapshot('p_one', [summary('r1'), summary('r2')])
    useRunStore.getState().applySnapshot(snapshot('r1', 1))
    useRunStore.getState().applySnapshot(snapshot('r2', 1))

    useRunStore.getState().receiveEvent({ ...event('r1', 2), projectId: 'p_other' })
    useRunStore.getState().receiveEvent(event('r2', 2, { message: transcript('m2', '仅 r2') }))

    expect(useRunStore.getState().runs.r1.lastSeq).toBe(1)
    expect(useRunStore.getState().runs.r2.snapshot?.run.transcript?.[0].text).toBe('仅 r2')
  })

  it('removes the exact permission or question resolved for the run', () => {
    useRunStore.getState().setWorkspaceSnapshot('p_one', [summary('r1')])
    useRunStore.getState().applySnapshot(snapshot('r1', 1, RunStatus.RunRunning, [permission], [question]))
    useRunStore.getState().receiveEvent(event('r1', 2, { type: 'request_resolved', resolvedKind: 'permission', resolvedId: permission.id }))
    useRunStore.getState().receiveEvent(event('r1', 3, { type: 'request_resolved', resolvedKind: 'question', resolvedId: question.id }))

    expect(useRunStore.getState().runs.r1.snapshot?.permissions).toEqual([])
    expect(useRunStore.getState().runs.r1.snapshot?.questions).toEqual([])
  })

  it('buffers run events received before the run snapshot is loaded', () => {
    useRunStore.getState().setWorkspaceSnapshot('p_one', [summary('r1')])
    useRunStore.getState().receiveEvent(event('r1', 1, { type: 'status', status: RunStatus.RunRunning }))

    expect(useRunStore.getState().runs.r1.needsSnapshot).toBe(true)
    expect(useRunStore.getState().runs.r1.bufferedEvents).toHaveLength(1)
    useRunStore.getState().applySnapshot(snapshot('r1', 0))
    expect(useRunStore.getState().runs.r1.lastSeq).toBe(1)
  })
})
