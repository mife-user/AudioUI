import { useState } from 'react'
import type { Node, WorkspaceChain } from '../../bindings/audio-ui/internal/domain/entity'

const COLLAPSED_STORAGE_KEY = 'audio-ui.chain-quickbar.collapsed'

type Props = {
  chains: WorkspaceChain[]
  nodes: Node[]
  activeNodeId: string
  onSelectNode: (nodeId: string) => void
  onCreateNode: () => void
  createNodeDisabled: boolean
}

function statusLabel(status: string): string {
  switch (status) {
    case 'running': return '工作中'
    case 'pending': return '待处理'
    case 'idle': return '空闲'
    default: return status
  }
}

export function ChainQuickBar({ chains, nodes, activeNodeId, onSelectNode, onCreateNode, createNodeDisabled }: Props) {
  const [collapsed, setCollapsed] = useState(() => window.localStorage.getItem(COLLAPSED_STORAGE_KEY) === 'true')
  const titleByNodeId = new Map(nodes.map((node) => [node.id, node.title]))
  const selectedChain = chains.find((chain) => chain.tailNodeId === activeNodeId)
  const runningCount = chains.filter((chain) => chain.status === 'running').length
  const pendingCount = chains.filter((chain) => chain.status === 'pending').length
  const summary = chains.length ? [
    `${chains.length} 条叶子链`,
    runningCount ? `${runningCount} 工作中` : '',
    pendingCount ? `${pendingCount} 待处理` : '',
  ].filter(Boolean).join(' · ') : '暂无叶子链'

  function toggleCollapsed() {
    const next = !collapsed
    window.localStorage.setItem(COLLAPSED_STORAGE_KEY, String(next))
    setCollapsed(next)
  }

  return (
    <nav className={`chain-quickbar nodrag nopan${collapsed ? ' is-collapsed' : ''}`} aria-label="叶子链快捷栏">
      <button
        type="button"
        className="chain-quickbar-toggle"
        aria-expanded={!collapsed}
        aria-controls="chain-quickbar-list"
        onClick={toggleCollapsed}
      >
        <span>叶子链</span>
        <strong>{collapsed ? summary || '暂无叶子链' : chains.length}</strong>
        <span className="chain-quickbar-toggle-label">{collapsed ? '展开' : '收起'}</span>
      </button>
      {chains.length ? (
        <ul className="chain-quickbar-list" id="chain-quickbar-list" hidden={collapsed}>
          {chains.map((chain) => {
            const label = chain.label || titleByNodeId.get(chain.tailNodeId) || chain.tailNodeId
            const pathLabel = (chain.path ?? []).map((nodeId) => titleByNodeId.get(nodeId) || nodeId).join(' → ')
            const state = statusLabel(chain.status)
            return (
              <li key={chain.id}>
                <button
                  type="button"
                  className={`chain-quickbar-item${chain.id === selectedChain?.id ? ' is-active' : ''}`}
                  aria-pressed={chain.id === selectedChain?.id}
                  aria-label={`选择叶子链 ${label}，${state}`}
                  title={pathLabel}
                  onClick={() => {
                    if (chain.tailNodeId !== activeNodeId) onSelectNode(chain.tailNodeId)
                  }}
                >
                  <strong>{label}</strong>
                  <span className="chain-quickbar-path">{pathLabel}</span>
                  <span className={`chain-quickbar-status is-${chain.status}`}>{state}</span>
                </button>
              </li>
            )
          })}
        </ul>
      ) : !collapsed ? (
        <p className="chain-quickbar-empty">暂无叶子链</p>
      ) : null}
      <button type="button" className="chain-quickbar-new" disabled={createNodeDisabled} onClick={onCreateNode}>
        ＋ 新建节点
      </button>
    </nav>
  )
}
