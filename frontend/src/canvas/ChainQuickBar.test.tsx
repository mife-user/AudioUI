import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { Node, WorkspaceChain } from '../../bindings/audio-ui/internal/domain/entity'
import { ChainQuickBar } from './ChainQuickBar'

const nodes: Node[] = [
  { id: 'n_shared', type: 'prompt' as Node['type'], title: '共同提示词', parent: 'n_candidate', position: { x: 0, y: 0 }, size: { width: 280, height: 230 }, params: {}, rounds: [], locked: false, stale: false },
  { id: 'n_agent_a', type: 'agent' as Node['type'], title: 'Agent 甲', parent: 'n_shared', position: { x: 0, y: 0 }, size: { width: 280, height: 230 }, params: {}, rounds: [], locked: false, stale: false },
  { id: 'n_agent_b', type: 'agent' as Node['type'], title: 'Agent 乙', parent: 'n_shared', position: { x: 0, y: 0 }, size: { width: 280, height: 230 }, params: {}, rounds: [], locked: false, stale: false },
  { id: 'n_script_c', type: 'script' as Node['type'], title: '脚本丙', parent: 'n_shared', position: { x: 0, y: 0 }, size: { width: 280, height: 230 }, params: {}, rounds: [], locked: false, stale: false },
]

const chains: WorkspaceChain[] = [
  { id: 'c_n_agent_a', tailNodeId: 'n_agent_a', path: ['n_topic', 'n_candidate', 'n_shared', 'n_agent_a'], nodeIds: ['n_topic', 'n_candidate', 'n_shared', 'n_agent_a'], ambiguous: false, label: '分支甲', status: 'running' },
  { id: 'c_n_agent_b', tailNodeId: 'n_agent_b', path: ['n_topic', 'n_candidate', 'n_shared', 'n_agent_b'], nodeIds: ['n_topic', 'n_candidate', 'n_shared', 'n_agent_b'], ambiguous: false, label: '分支乙', status: 'pending' },
  { id: 'c_n_script_c', tailNodeId: 'n_script_c', path: ['n_topic', 'n_candidate', 'n_shared', 'n_script_c'], nodeIds: ['n_topic', 'n_candidate', 'n_shared', 'n_script_c'], ambiguous: false, label: '分支丙', status: 'idle' },
]

const renderQuickbar = (chainList = chains, activeNodeId = 'n_shared', onSelectNode = vi.fn()) =>
  render(<ChainQuickBar chains={chainList} nodes={nodes} activeNodeId={activeNodeId} onSelectNode={onSelectNode} />)

describe('ChainQuickBar', () => {
  it('shows only the arrow button until hovered or focused', () => {
    renderQuickbar()

    const toggle = screen.getByRole('button', { name: '显示节点链路' })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(document.getElementById('chain-quickbar-list')).toHaveAttribute('hidden')
    expect(screen.queryByRole('button', { name: /新建节点/ })).not.toBeInTheDocument()

    fireEvent.pointerEnter(screen.getByRole('navigation', { name: '链路选择' }))
    expect(screen.getAllByRole('button', { name: /选择链路/ })).toHaveLength(3)
    expect(screen.queryAllByText(/工作中|待处理/)).toHaveLength(0)
  })

  it('opens on hover, closes on pointer leave, and allows the list to be pinned', () => {
    renderQuickbar()
    const quickbar = screen.getByRole('navigation', { name: '链路选择' })
    const toggle = screen.getByRole('button', { name: '显示节点链路' })

    fireEvent.pointerEnter(quickbar)
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    fireEvent.pointerLeave(quickbar)
    expect(toggle).toHaveAttribute('aria-expanded', 'false')

    fireEvent.click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    expect(toggle).toHaveAttribute('aria-pressed', 'true')
    fireEvent.pointerLeave(quickbar)
    expect(toggle).toHaveAttribute('aria-expanded', 'true')

    fireEvent.click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(toggle).toHaveAttribute('aria-pressed', 'false')
  })

  it('closes on an outside pointer and opens again on re-entry', () => {
    renderQuickbar()
    const quickbar = screen.getByRole('navigation', { name: '链路选择' })
    const toggle = screen.getByRole('button', { name: '显示节点链路' })

    fireEvent.click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    fireEvent.pointerDown(document.body)
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(toggle).toHaveAttribute('aria-pressed', 'false')

    fireEvent.pointerEnter(quickbar)
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
  })

  it('keeps keyboard focus inside the open list and Escape closes and returns focus', () => {
    renderQuickbar()
    const toggle = screen.getByRole('button', { name: '显示节点链路' })
    fireEvent.focus(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'true')

    const item = screen.getByRole('button', { name: /选择链路 分支甲/ })
    fireEvent.focus(item)
    fireEvent.keyDown(item, { key: 'Escape' })

    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(document.activeElement).toBe(toggle)
  })

  it('uses snapshot chains and selects only the chosen chain tail', () => {
    const onSelectNode = vi.fn()
    renderQuickbar(chains, 'n_shared', onSelectNode)
    const quickbar = screen.getByRole('navigation', { name: '链路选择' })
    fireEvent.pointerEnter(quickbar)

    const branch = screen.getByRole('button', { name: /选择链路 分支乙/ })
    expect(branch).toHaveAttribute('aria-pressed', 'false')
    expect(branch).toHaveAttribute('title', 'n_topic → n_candidate → 共同提示词 → Agent 乙')
    fireEvent.click(branch)

    expect(onSelectNode).toHaveBeenCalledOnce()
    expect(onSelectNode).toHaveBeenCalledWith('n_agent_b')
    expect(document.getElementById('chain-quickbar-list')).toHaveAttribute('hidden')
  })

  it('shows a convergence as its real upstream set and keeps the full route accessible', () => {
    const convergence: WorkspaceChain = {
      id: 'c_merge', tailNodeId: 'n_agent_a', path: [],
      nodeIds: ['n_shared', 'n_script_c', 'n_agent_a'], ambiguous: true, label: '汇合', status: 'idle',
    }
    renderQuickbar([convergence], 'n_agent_a')
    fireEvent.pointerEnter(screen.getByRole('navigation', { name: '链路选择' }))

    const item = screen.getByRole('button', { name: '选择汇合节点 汇合：多上游：共同提示词、脚本丙、Agent 甲' })
    expect(item).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByText('多上游：共同提示词、脚本丙、Agent 甲')).toBeInTheDocument()
    expect(screen.queryByText(/共同提示词 → 脚本丙/)).not.toBeInTheDocument()
  })

  it('shows the empty state only while the list is open', () => {
    renderQuickbar([], '')

    const list = document.getElementById('chain-quickbar-list')
    expect(list).toHaveAttribute('hidden')
    fireEvent.pointerEnter(screen.getByRole('navigation', { name: '链路选择' }))
    expect(list).not.toHaveAttribute('hidden')
    expect(screen.getByText('暂无链路')).toBeInTheDocument()
  })
})
