import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Node, WorkspaceChain } from '../../bindings/audio-ui/internal/domain/entity'
import { ChainQuickBar } from './ChainQuickBar'

const nodes: Node[] = [
  { id: 'n_shared', type: 'prompt' as Node['type'], title: '共同提示词', parent: 'n_candidate', position: { x: 0, y: 0 }, size: { width: 280, height: 230 }, params: {}, rounds: [], locked: false, stale: false },
  { id: 'n_agent_a', type: 'agent' as Node['type'], title: 'Agent 甲', parent: 'n_shared', position: { x: 0, y: 0 }, size: { width: 280, height: 230 }, params: {}, rounds: [], locked: false, stale: false },
  { id: 'n_agent_b', type: 'agent' as Node['type'], title: 'Agent 乙', parent: 'n_shared', position: { x: 0, y: 0 }, size: { width: 280, height: 230 }, params: {}, rounds: [], locked: false, stale: false },
  { id: 'n_script_c', type: 'script' as Node['type'], title: '脚本丙', parent: 'n_shared', position: { x: 0, y: 0 }, size: { width: 280, height: 230 }, params: {}, rounds: [], locked: false, stale: false },
]

const chains: WorkspaceChain[] = [
  { id: 'c_n_agent_a', tailNodeId: 'n_agent_a', path: ['n_topic', 'n_candidate', 'n_shared', 'n_agent_a'], label: '分支甲', status: 'running' },
  { id: 'c_n_agent_b', tailNodeId: 'n_agent_b', path: ['n_topic', 'n_candidate', 'n_shared', 'n_agent_b'], label: '分支乙', status: 'pending' },
  { id: 'c_n_script_c', tailNodeId: 'n_script_c', path: ['n_topic', 'n_candidate', 'n_shared', 'n_script_c'], label: '分支丙', status: 'idle' },
]

describe('ChainQuickBar', () => {
  beforeEach(() => localStorage.removeItem('audio-ui.chain-quickbar.collapsed'))

  it('renders backend leaf chains and their run/pending state when branches share a prefix', () => {
    render(<ChainQuickBar chains={chains} nodes={nodes} activeNodeId="n_shared" onSelectNode={vi.fn()} onCreateNode={vi.fn()} createNodeDisabled={false} />)

    expect(screen.getAllByRole('button', { name: /选择叶子链/ })).toHaveLength(3)
    expect(screen.getByRole('button', { name: '选择叶子链 分支甲，工作中' })).toHaveAttribute('aria-pressed', 'false')
    expect(screen.getByRole('button', { name: '选择叶子链 分支乙，待处理' })).toHaveAttribute('aria-pressed', 'false')
    expect(screen.getByRole('button', { name: '选择叶子链 分支丙，空闲' })).toHaveAttribute('aria-pressed', 'false')
  })

  it('selects a chain tail through the supplied selection callback', () => {
    const onSelectNode = vi.fn()
    const { rerender } = render(<ChainQuickBar chains={chains} nodes={nodes} activeNodeId="n_shared" onSelectNode={onSelectNode} onCreateNode={vi.fn()} createNodeDisabled={false} />)

    fireEvent.click(screen.getByRole('button', { name: '选择叶子链 分支乙，待处理' }))
    expect(onSelectNode).toHaveBeenCalledOnce()
    expect(onSelectNode).toHaveBeenCalledWith('n_agent_b')

    rerender(<ChainQuickBar chains={chains} nodes={nodes} activeNodeId="n_agent_b" onSelectNode={onSelectNode} onCreateNode={vi.fn()} createNodeDisabled={false} />)
    expect(screen.getByRole('button', { name: '选择叶子链 分支乙，待处理' })).toHaveAttribute('aria-pressed', 'true')
  })

  it('persists collapse state and summarizes authoritative chain statuses', () => {
    const props = { chains, nodes, activeNodeId: 'n_shared', onSelectNode: vi.fn(), onCreateNode: vi.fn(), createNodeDisabled: false }
    const first = render(<ChainQuickBar {...props} />)
    const toggle = screen.getByRole('button', { name: '叶子链3收起' })

    fireEvent.click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(screen.getByText('3 条叶子链 · 1 工作中 · 1 待处理')).toBeInTheDocument()
    expect(localStorage.getItem('audio-ui.chain-quickbar.collapsed')).toBe('true')
    first.unmount()

    render(<ChainQuickBar {...props} />)
    expect(screen.getByRole('button', { name: '叶子链3 条叶子链 · 1 工作中 · 1 待处理展开' })).toHaveAttribute('aria-expanded', 'false')
    expect(screen.getByRole('button', { name: '选择叶子链 分支甲，工作中', hidden: true })).not.toBeVisible()
  })

  it('opens the shared node creation menu and respects its disabled state', () => {
    const onCreateNode = vi.fn()
    const { rerender } = render(
      <ChainQuickBar chains={chains} nodes={nodes} activeNodeId="n_shared" onSelectNode={vi.fn()} onCreateNode={onCreateNode} createNodeDisabled />,
    )
    const create = screen.getByRole('button', { name: '＋ 新建节点' })
    expect(create).toBeDisabled()
    fireEvent.click(create)
    expect(onCreateNode).not.toHaveBeenCalled()

    rerender(<ChainQuickBar chains={chains} nodes={nodes} activeNodeId="n_shared" onSelectNode={vi.fn()} onCreateNode={onCreateNode} createNodeDisabled={false} />)
    fireEvent.click(screen.getByRole('button', { name: '＋ 新建节点' }))
    expect(onCreateNode).toHaveBeenCalledOnce()
  })

  it('shows an empty state when the authoritative snapshot has no leaf chains', () => {
    render(<ChainQuickBar chains={[]} nodes={[]} activeNodeId="" onSelectNode={vi.fn()} onCreateNode={vi.fn()} createNodeDisabled={false} />)
    expect(screen.getByText('暂无叶子链')).toBeInTheDocument()
  })
})
