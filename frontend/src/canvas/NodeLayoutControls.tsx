import { useEffect, useId, useRef, useState, type FormEvent } from 'react'
import { NodeResizer } from '@xyflow/react'
import type { Node } from '../../bindings/audio-ui/internal/domain/entity'
import { NodeType } from '../../bindings/audio-ui/internal/domain/valueobject'

const MIN_WIDTH = 200
const MIN_HEIGHT = 140
const MAX_WIDTH = 1200
const MAX_HEIGHT = 900

type SupportedNodeType = Exclude<Node['type'], NodeType.$zero>

const DEFAULT_NODE_SIZES: Record<SupportedNodeType, Node['size']> = {
  [NodeType.NodeTypeTopic]: { width: 280, height: 250 },
  [NodeType.NodeTypeCandidate]: { width: 235, height: 160 },
  [NodeType.NodeTypePrompt]: { width: 280, height: 230 },
  [NodeType.NodeTypeAgent]: { width: 235, height: 165 },
  [NodeType.NodeTypeScript]: { width: 270, height: 205 },
}

type NodeResizeHandlesProps = {
  nodeId: string
  visible: boolean
  disabled: boolean
}

export function NodeResizeHandles({ nodeId, visible, disabled }: NodeResizeHandlesProps) {
  return (
    <NodeResizer
      nodeId={nodeId}
      isVisible={visible && !disabled}
      minWidth={MIN_WIDTH}
      minHeight={MIN_HEIGHT}
      maxWidth={MAX_WIDTH}
      maxHeight={MAX_HEIGHT}
      handleClassName="canvas-node-resize-handle"
      lineClassName="canvas-node-resize-line"
      lineStyle={{ borderColor: 'transparent' }}
    />
  )
}

type NodeCollapseControlProps = {
  node: Node
  disabled: boolean
  onToggle?: (nodeId: string, collapsed: boolean) => void
}

export function NodeCollapseControl({ node, disabled, onToggle }: NodeCollapseControlProps) {
  const collapsed = Boolean(node.collapsed)
  return (
    <button
      type="button"
      className="canvas-node-collapse-trigger nodrag nopan nowheel"
      aria-label={`${collapsed ? '展开' : '折叠'} ${node.title} 正文`}
      aria-controls={`node-content-${node.id}`}
      aria-expanded={!collapsed}
      disabled={disabled || !onToggle}
      title={collapsed ? '展开节点正文' : '折叠节点正文'}
      onClick={() => onToggle?.(node.id, !collapsed)}
    >
      {collapsed ? '⌄' : '⌃'}
    </button>
  )
}

type NodeSizeControlProps = {
  node: Node
  disabled: boolean
  onApply?: (nodeId: string, width: number, height: number) => void
}

export function NodeSizeControl({ node, disabled, onApply }: NodeSizeControlProps) {
  const [open, setOpen] = useState(false)
  const [width, setWidth] = useState(String(node.size.width))
  const [height, setHeight] = useState(String(node.size.height))
  const triggerRef = useRef<HTMLButtonElement>(null)
  const widthRef = useRef<HTMLInputElement>(null)
  const dialogRef = useRef<HTMLDialogElement>(null)
  const titleId = useId()
  const helpId = useId()
  const isDisabled = disabled || !onApply

  useEffect(() => {
    if (open) widthRef.current?.focus()
  }, [open])

  function closeDialog() {
    const dialog = dialogRef.current
    if (dialog?.open) dialog.close()
    setOpen(false)
    triggerRef.current?.focus()
  }

  function showDialog() {
    if (isDisabled) return
    setWidth(String(node.size.width))
    setHeight(String(node.size.height))
    const dialog = dialogRef.current
    if (dialog && !dialog.open) dialog.showModal()
    setOpen(true)
  }

  function applySize(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const nextWidth = Number(width)
    const nextHeight = Number(height)
    if (!Number.isFinite(nextWidth) || !Number.isFinite(nextHeight) || !onApply) return
    onApply(
      node.id,
      Math.min(MAX_WIDTH, Math.max(MIN_WIDTH, nextWidth)),
      Math.min(MAX_HEIGHT, Math.max(MIN_HEIGHT, nextHeight)),
    )
    closeDialog()
  }

  function restoreDefaultSize() {
    const size = DEFAULT_NODE_SIZES[node.type as SupportedNodeType]
    onApply?.(node.id, size.width, size.height)
    closeDialog()
  }

  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        className="canvas-node-size-trigger nodrag nopan nowheel"
        aria-label={`调整 ${node.title} 卡片尺寸`}
        aria-haspopup="dialog"
        aria-expanded={open}
        disabled={isDisabled}
        title="调整卡片尺寸"
        onClick={showDialog}
      >
        <svg viewBox="0 0 16 16" aria-hidden="true">
          <path d="M3 7V3h4M9 3h4v4M13 9v4H9M7 13H3V9" />
        </svg>
      </button>
      <dialog
        ref={dialogRef}
        className="node-size-dialog"
        aria-labelledby={titleId}
        aria-describedby={helpId}
        onClose={() => setOpen(false)}
        onMouseDown={(event) => {
          if (event.target === event.currentTarget) closeDialog()
        }}
      >
        <form className="node-size-form" onSubmit={applySize}>
          <div className="node-size-dialog-heading">
            <div>
              <h2 id={titleId}>调整卡片尺寸</h2>
              <p>{node.title}</p>
            </div>
            <button type="button" className="node-size-close" aria-label="关闭" onClick={closeDialog}>×</button>
          </div>
          <p id={helpId} className="node-size-help">单位为画布像素，宽度 200–1200，高度 140–900。</p>
          <div className="node-size-fields">
            <label>
              宽度
              <input
                ref={widthRef}
                type="number"
                min={MIN_WIDTH}
                max={MAX_WIDTH}
                step="any"
                required
                value={width}
                onChange={(event) => setWidth(event.currentTarget.value)}
              />
            </label>
            <label>
              高度
              <input
                type="number"
                min={MIN_HEIGHT}
                max={MAX_HEIGHT}
                step="any"
                required
                value={height}
                onChange={(event) => setHeight(event.currentTarget.value)}
              />
            </label>
          </div>
          <div className="node-size-actions">
            <button type="button" className="node-size-reset" disabled={isDisabled} onClick={restoreDefaultSize}>恢复默认尺寸</button>
            <span />
            <button type="button" className="node-size-cancel" onClick={closeDialog}>取消</button>
            <button type="submit" className="node-size-apply" disabled={isDisabled}>应用</button>
          </div>
        </form>
      </dialog>
    </>
  )
}
