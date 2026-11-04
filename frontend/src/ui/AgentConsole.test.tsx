import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { PermissionTier, RunStatus, TranscriptKind } from '../../bindings/audio-ui/internal/domain/valueobject'
import type { RunSnapshot, WorkspaceRunSummary } from '../../bindings/audio-ui/internal/domain/entity'
import type { RunView } from '../stores/runStore'
import { AgentConsole } from './AgentConsole'

const summary: WorkspaceRunSummary = {
  id: 'r_history',
  nodeId: 'n_topic',
  status: RunStatus.RunSucceeded,
  chainId: 'c_n_topic',
  chainPath: ['n_topic'],
  estimatedTokens: 120,
  contextHash: '1234567890abcdef',
  startedAt: '2026-11-03T12:00:00Z',
}

const runSnapshot: RunSnapshot = {
  runId: summary.id,
  projectId: 'p_demo',
  lastSeq: 3,
  run: {
    id: summary.id,
    treeId: 'n_topic',
    nodeId: summary.nodeId,
    chainId: summary.chainId,
    chainPath: ['n_topic'],
    status: RunStatus.RunSucceeded,
    contextVersion: 1,
    contextHash: summary.contextHash,
    estimatedTokens: summary.estimatedTokens,
    startedAt: summary.startedAt,
    transcript: [{
      id: 'm1', kind: TranscriptKind.TranscriptMessage, role: 'assistant', text: '已完成',
      streaming: false, createdAt: summary.startedAt ?? '',
    }],
  },
  permissions: [],
  questions: [],
}

const historyView: RunView = {
  summary,
  snapshot: runSnapshot,
  lastSeq: 3,
  bufferedEvents: [],
  loading: false,
  needsSnapshot: false,
  error: '',
}

function renderConsole(overrides: Partial<React.ComponentProps<typeof AgentConsole>> = {}) {
  const props: React.ComponentProps<typeof AgentConsole> = {
    runs: [summary],
    runViews: { [summary.id]: historyView },
    selectedRunId: summary.id,
    selectedRun: historyView,
    actionError: '',
    collapsed: false,
    onToggle: vi.fn(),
    onSelectRun: vi.fn(),
    onRetryRun: vi.fn(),
    onPermission: vi.fn().mockResolvedValue(undefined),
    onQuestion: vi.fn().mockResolvedValue(undefined),
    onInterrupt: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  }
  return { ...render(<AgentConsole {...props} />), props }
}

describe('AgentConsole', () => {
  it('shows run history and transcript without a free-chat input', () => {
    renderConsole()
    expect(screen.getByText('r_history')).toBeInTheDocument()
    expect(screen.getByText('已完成')).toBeInTheDocument()
    expect(screen.queryByRole('textbox', { name: '自由聊天' })).not.toBeInTheDocument()
    expect(document.querySelector('textarea')).toBeNull()
  })

  it('keeps terminal history read-only and does not invent unknown token usage', () => {
    renderConsole()
    expect(screen.queryByRole('button', { name: '停止运行' })).not.toBeInTheDocument()
    expect(screen.getByText('未知')).toBeInTheDocument()
  })

  it('does not expose pending artifact actions before the pending UI task', () => {
    renderConsole()
    fireEvent.click(screen.getByRole('tab', { name: /待处理/ }))
    expect(screen.getByText('待应用产物预览与操作将在 M2-10B 接入。')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '应用' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '丢弃' })).not.toBeInTheDocument()
  })

  it('routes permission responses with both run and request IDs', () => {
    const request = {
      id: 'permission-1', tool: 'Write', tier: PermissionTier.PermissionConfirm, risk: '写文件', summary: 'Write file',
      allowSession: false, expiresAt: new Date(Date.now() + 60_000).toISOString(),
    }
    const activeSnapshot: RunSnapshot = {
      ...runSnapshot,
      runId: 'r_active',
      run: { ...runSnapshot.run, id: 'r_active', status: RunStatus.RunRunning, transcript: [] },
      permissions: [request],
    }
    const activeSummary = { ...summary, id: 'r_active', status: RunStatus.RunRunning }
    const activeView: RunView = { ...historyView, summary: activeSummary, snapshot: activeSnapshot }
    const onPermission = vi.fn().mockResolvedValue(undefined)
    renderConsole({ runs: [activeSummary], runViews: { r_active: activeView }, selectedRunId: 'r_active', selectedRun: activeView, onPermission })

    fireEvent.click(screen.getByRole('button', { name: '允许一次' }))
    expect(onPermission).toHaveBeenCalledWith('r_active', 'permission-1', 'allow')
  })

  it('shows the collapsed rail with the selected run identity', () => {
    renderConsole({ collapsed: true })
    expect(screen.getByText('c_n_topic')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '展开控制台' })).toBeInTheDocument()
  })
})
