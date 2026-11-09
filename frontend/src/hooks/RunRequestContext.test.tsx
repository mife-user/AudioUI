import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { PermissionTier, RunStatus, TranscriptKind } from '../../bindings/audio-ui/internal/domain/valueobject'
import type { RunSnapshot, WorkspaceRunSummary } from '../../bindings/audio-ui/internal/domain/entity'
import type { RunView } from '../stores/runStore'
import { useRunStore } from '../stores/runStore'
import { NodeRunRequestEntry } from '../ui/NodeRunRequestEntry'
import { RunRequestProvider } from './RunRequestContext'
import { getRunRequestGroups, useRunRequestContext } from './runRequests'

const summary: WorkspaceRunSummary = {
  id: 'r_active',
  nodeId: 'n_agent',
  status: RunStatus.RunRunning,
  chainId: 'c_agent',
  chainPath: ['n_topic', 'n_agent'],
  estimatedTokens: 20,
}

const request = {
  id: 'permission-1',
  tool: 'Write',
  tier: PermissionTier.PermissionConfirm,
  risk: '写入文件',
  summary: '写入当前运行目录',
  allowSession: false,
  expiresAt: new Date(Date.now() + 60_000).toISOString(),
}

const snapshot: RunSnapshot = {
  runId: summary.id,
  projectId: 'p_demo',
  lastSeq: 1,
  run: {
    id: summary.id,
    treeId: 'n_topic',
    nodeId: summary.nodeId,
    chainId: summary.chainId,
    chainPath: summary.chainPath,
    status: RunStatus.RunRunning,
    contextVersion: 1,
    transcript: [{ id: 'm1', kind: TranscriptKind.TranscriptMessage, role: 'assistant', text: '处理中', streaming: true, createdAt: '' }],
  },
  permissions: [request],
  questions: [],
}

const view: RunView = {
  summary,
  snapshot,
  lastSeq: 1,
  bufferedEvents: [],
  loading: false,
  needsSnapshot: false,
  error: '',
}

function ScopeAction() {
  const context = useRunRequestContext()
  return <button type="button" onClick={() => void context?.respondPermission('r_active', 'permission-1', 'allow')}>响应请求</button>
}

describe('RunRequestContext', () => {
  it('routes only active request snapshots from the current project', () => {
    const terminalView = {
      ...view,
      summary: { ...summary, id: 'r_done', status: RunStatus.RunSucceeded },
      snapshot: { ...snapshot, runId: 'r_done', run: { ...snapshot.run, id: 'r_done', status: RunStatus.RunSucceeded } },
    }
    const otherProjectView = {
      ...view,
      summary: { ...summary, id: 'r_other' },
      snapshot: { ...snapshot, projectId: 'p_other', runId: 'r_other', run: { ...snapshot.run, id: 'r_other' } },
    }
    const groups = getRunRequestGroups({ r_active: view, r_done: terminalView, r_other: otherProjectView }, 'p_demo')

    expect(groups).toHaveLength(1)
    expect(groups[0]).toMatchObject({ projectId: 'p_demo', runId: 'r_active', nodeId: 'n_agent' })
    expect(groups[0]?.permissions.map((item) => item.id)).toEqual(['permission-1'])
  })

  it('does not send a response after the active project changes', async () => {
    useRunStore.getState().setWorkspaceSnapshot('p_demo', [summary])
    const onPermission = vi.fn().mockResolvedValue(undefined)
    render(
      <RunRequestProvider projectId="p_demo" runViews={{ r_active: view }} onPermission={onPermission} onQuestion={vi.fn()}>
        <ScopeAction />
      </RunRequestProvider>,
    )

    useRunStore.getState().setWorkspaceSnapshot('p_other', [])
    fireEvent.click(screen.getByRole('button', { name: '响应请求' }))
    await waitFor(() => expect(onPermission).not.toHaveBeenCalled())
  })

  it('shows the exact request on its originating node and responds with both IDs', async () => {
    useRunStore.getState().setWorkspaceSnapshot('p_demo', [summary])
    const onPermission = vi.fn().mockResolvedValue(undefined)
    render(
      <RunRequestProvider projectId="p_demo" runViews={{ r_active: view }} onPermission={onPermission} onQuestion={vi.fn()}>
        <NodeRunRequestEntry nodeId="n_agent" nodeTitle="Agent 主节点" />
      </RunRequestProvider>,
    )

    fireEvent.click(screen.getByRole('button', { name: '查看 Agent 主节点 的 1 个运行请求' }))
    expect(screen.getByRole('dialog', { name: 'Agent 主节点 的运行请求' })).toBeInTheDocument()
    expect(screen.getByText('运行 r_active')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '允许一次' }))
    await waitFor(() => expect(onPermission).toHaveBeenCalledWith('r_active', 'permission-1', 'allow'))
  })
})
