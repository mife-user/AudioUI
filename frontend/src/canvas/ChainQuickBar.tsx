import { useEffect, useRef, useState } from 'react'
import type { KeyboardEvent } from 'react'
import type { Node, WorkspaceChain } from '../../bindings/audio-ui/internal/domain/entity'

type Props = {
  chains: WorkspaceChain[]
  nodes: Node[]
  activeNodeId: string
  onSelectNode: (nodeId: string) => void
}

export function ChainQuickBar({ chains, nodes, activeNodeId, onSelectNode }: Props) {
  const [hovered, setHovered] = useState(false)
  const [focused, setFocused] = useState(false)
  const [pinned, setPinned] = useState(false)
  const [dismissed, setDismissed] = useState(false)
  const rootRef = useRef<HTMLElement>(null)
  const toggleRef = useRef<HTMLButtonElement>(null)
  const suppressFocusOpen = useRef(false)
  const isOpen = !dismissed && (hovered || focused || pinned)
  const titleByNodeId = new Map(nodes.map((node) => [node.id, node.title]))
  const selectedChain = chains.find((chain) => chain.tailNodeId === activeNodeId)

  useEffect(() => {
    if (!isOpen) return

    function closeOnOutsidePointer(event: PointerEvent) {
      if (event.target instanceof Element && rootRef.current?.contains(event.target)) return
      setPinned(false)
      setDismissed(true)
    }

    document.addEventListener('pointerdown', closeOnOutsidePointer)
    return () => document.removeEventListener('pointerdown', closeOnOutsidePointer)
  }, [isOpen])

  function togglePinned() {
    if (pinned) {
      setPinned(false)
      setDismissed(true)
      return
    }
    setPinned(true)
    setDismissed(false)
  }

  function handleEscape(event: KeyboardEvent<HTMLElement>) {
    if (event.key !== 'Escape' || !isOpen) return
    event.preventDefault()
    event.stopPropagation()
    setPinned(false)
    setDismissed(true)
    if (document.activeElement !== toggleRef.current) {
      suppressFocusOpen.current = true
      toggleRef.current?.focus()
    }
  }

  return (
    <nav
      ref={rootRef}
      className={`chain-quickbar nodrag nopan${isOpen ? ' is-open' : ''}`}
      aria-label="链路选择"
      onPointerEnter={() => {
        setHovered(true)
        setDismissed(false)
      }}
      onPointerLeave={() => setHovered(false)}
      onFocusCapture={(event) => {
        setFocused(true)
        if (suppressFocusOpen.current && event.target === toggleRef.current) {
          suppressFocusOpen.current = false
          return
        }
        setDismissed(false)
      }}
      onBlurCapture={(event) => {
        if (event.relatedTarget instanceof Element && rootRef.current?.contains(event.relatedTarget)) return
        setFocused(false)
      }}
      onKeyDown={handleEscape}
    >
      <button
        ref={toggleRef}
        type="button"
        className="chain-quickbar-toggle"
        aria-label={pinned ? '收起节点链路' : isOpen ? '固定节点链路' : '显示节点链路'}
        aria-expanded={isOpen}
        aria-pressed={pinned}
        aria-controls="chain-quickbar-list"
        onClick={togglePinned}
      >
        <span className="chain-quickbar-arrow" aria-hidden="true">⌃</span>
      </button>
      <ul className="chain-quickbar-list" id="chain-quickbar-list" hidden={!isOpen}>
        {chains.length ? chains.map((chain) => {
          const nodeLabel = chain.label || titleByNodeId.get(chain.tailNodeId) || chain.tailNodeId
          const pathLabel = chain.ambiguous
            ? `多上游：${(chain.nodeIds ?? []).map((nodeId) => titleByNodeId.get(nodeId) || nodeId).join('、')}`
            : (chain.path ?? []).map((nodeId) => titleByNodeId.get(nodeId) || nodeId).join(' → ')
          const selected = chain.id === selectedChain?.id
          return (
            <li key={chain.id}>
              <button
                type="button"
                className={`chain-quickbar-item${selected ? ' is-active' : ''}`}
                aria-pressed={selected}
                aria-label={`选择${chain.ambiguous ? '汇合节点' : '链路'} ${nodeLabel}：${pathLabel}`}
                title={pathLabel}
                onClick={() => {
                  setPinned(false)
                  setDismissed(true)
                  if (chain.tailNodeId !== activeNodeId) onSelectNode(chain.tailNodeId)
                }}
              >
                <strong>{nodeLabel}</strong>
                <span className="chain-quickbar-path">{pathLabel}</span>
              </button>
            </li>
          )
        }) : (
          <li className="chain-quickbar-empty">暂无链路</li>
        )}
      </ul>
    </nav>
  )
}
