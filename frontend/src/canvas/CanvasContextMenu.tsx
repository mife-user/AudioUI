import { createPortal } from 'react-dom'
import { useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent } from 'react'

export type CanvasCreateType = 'topic' | 'prompt' | 'agent' | 'script'
type Point = { x: number; y: number }

type Props = {
  screenPosition: Point
  disabled: boolean
  onCreate: (type: CanvasCreateType) => void
  onClose: (returnFocus: boolean) => void
}

const options: { type: CanvasCreateType; label: string; description: string }[] = [
  { type: 'topic', label: '选题', description: '新建独立选题根节点' },
  { type: 'prompt', label: '提示词', description: '新建未连接草稿' },
  { type: 'agent', label: 'Agent', description: '新建未连接草稿' },
  { type: 'script', label: '脚本', description: '新建未连接草稿' },
]

export function CanvasContextMenu({ screenPosition, disabled, onCreate, onClose }: Props) {
  const menuRef = useRef<HTMLDivElement>(null)
  const [position, setPosition] = useState(screenPosition)

  useLayoutEffect(() => {
    const updatePosition = () => {
      const menu = menuRef.current
      if (!menu) return
      const { width, height } = menu.getBoundingClientRect()
      setPosition({
        x: Math.max(8, Math.min(screenPosition.x, window.innerWidth - width - 8)),
        y: Math.max(8, Math.min(screenPosition.y, window.innerHeight - height - 8)),
      })
    }
    updatePosition()
    window.addEventListener('resize', updatePosition)
    return () => window.removeEventListener('resize', updatePosition)
  }, [screenPosition.x, screenPosition.y])

  useLayoutEffect(() => {
    const firstItem = menuRef.current?.querySelector<HTMLButtonElement>('[role="menuitem"]:not(:disabled)')
    const focusTarget = firstItem ?? menuRef.current
    focusTarget?.focus()
  }, [])

  useEffect(() => {
    const dismissOutside = (event: PointerEvent) => {
      if (event.target instanceof Node && !menuRef.current?.contains(event.target)) onClose(false)
    }
    document.addEventListener('pointerdown', dismissOutside, true)
    return () => document.removeEventListener('pointerdown', dismissOutside, true)
  }, [onClose])

  function handleKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === 'Escape' || event.key === 'Tab') {
      event.preventDefault()
      onClose(true)
      return
    }
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
    event.preventDefault()
    const items = [...(menuRef.current?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]:not(:disabled)') ?? [])]
    if (!items.length) return
    const currentIndex = items.indexOf(document.activeElement as HTMLButtonElement)
    const nextIndex = event.key === 'Home'
      ? 0
      : event.key === 'End'
        ? items.length - 1
        : currentIndex < 0
          ? (event.key === 'ArrowDown' ? 0 : items.length - 1)
          : (currentIndex + (event.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length
    items[nextIndex].focus()
  }

  return createPortal(
    <div
      ref={menuRef}
      className="canvas-context-menu"
      role="menu"
      aria-label="新建节点"
      tabIndex={-1}
      data-testid="canvas-context-menu"
      style={{ left: position.x, top: position.y }}
      onKeyDown={handleKeyDown}
    >
      <strong className="canvas-context-menu-title">新建节点</strong>
      <div className="canvas-context-menu-options">
        {options.map((option) => (
          <button
            key={option.type}
            type="button"
            role="menuitem"
            aria-label={option.label}
            className="canvas-context-menu-item"
            disabled={disabled}
            onClick={() => onCreate(option.type)}
          >
            <span>{option.label}</span>
            <small>{option.description}</small>
          </button>
        ))}
      </div>
      <p className="canvas-context-menu-note">候选节点由选题运行生成，不能手动创建。</p>
      <span className="canvas-context-menu-hint">方向键选择 · Enter 确认 · Esc 关闭</span>
    </div>,
    document.body,
  )
}
