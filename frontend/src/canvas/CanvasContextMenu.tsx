import { createPortal } from 'react-dom'
import { useEffect, useId, useLayoutEffect, useRef, useState, type KeyboardEvent } from 'react'

export type CanvasCreateType = 'topic' | 'prompt' | 'agent' | 'script'
type Point = { x: number; y: number }
type SubmenuSide = 'left' | 'right'

type Props = {
  screenPosition: Point
  disabled: boolean
  onCreate: (type: CanvasCreateType) => void
  onClose: (returnFocus: boolean) => void
}

const options: { type: CanvasCreateType; label: string; description: string }[] = [
  { type: 'topic', label: '选题', description: '独立节点' },
  { type: 'prompt', label: '提示词', description: '未连接草稿' },
  { type: 'agent', label: 'Agent', description: '未连接草稿' },
  { type: 'script', label: '脚本', description: '未连接草稿' },
]

const viewportPadding = 8
const submenuGap = 4

export function CanvasContextMenu({ screenPosition, disabled, onCreate, onClose }: Props) {
  const menuRef = useRef<HTMLDivElement>(null)
  const submenuRef = useRef<HTMLDivElement>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const submenuId = useId()
  const [position, setPosition] = useState(screenPosition)
  const [submenuOpen, setSubmenuOpen] = useState(false)
  const [submenuSide, setSubmenuSide] = useState<SubmenuSide>('right')
  const [submenuTop, setSubmenuTop] = useState(0)
  const [initialSubmenuFocus, setInitialSubmenuFocus] = useState<'first' | 'last'>('first')

  useLayoutEffect(() => {
    const updatePosition = () => {
      const menu = menuRef.current
      if (!menu) return
      const { width, height } = menu.getBoundingClientRect()
      setPosition({
        x: Math.max(viewportPadding, Math.min(screenPosition.x, window.innerWidth - width - viewportPadding)),
        y: Math.max(viewportPadding, Math.min(screenPosition.y, window.innerHeight - height - viewportPadding)),
      })
    }
    updatePosition()
    window.addEventListener('resize', updatePosition)
    return () => window.removeEventListener('resize', updatePosition)
  }, [screenPosition.x, screenPosition.y])

  useLayoutEffect(() => {
    if (!submenuOpen) return
    const updateSubmenuPosition = () => {
      const menu = menuRef.current
      const submenu = submenuRef.current
      if (!menu || !submenu) return
      const menuRect = menu.getBoundingClientRect()
      const submenuRect = submenu.getBoundingClientRect()
      const roomRight = window.innerWidth - menuRect.right - viewportPadding
      const roomLeft = menuRect.left - viewportPadding
      let side: SubmenuSide
      let menuX = position.x

      if (roomRight >= submenuRect.width + submenuGap) {
        side = 'right'
      } else if (roomLeft >= submenuRect.width + submenuGap) {
        side = 'left'
      } else {
        side = roomRight >= roomLeft ? 'right' : 'left'
        if (menuRect.width + submenuRect.width + submenuGap <= window.innerWidth - viewportPadding * 2) {
          menuX = side === 'right'
            ? Math.max(viewportPadding, Math.min(position.x, window.innerWidth - menuRect.width - submenuRect.width - submenuGap - viewportPadding))
            : Math.max(submenuRect.width + submenuGap + viewportPadding, Math.min(position.x, window.innerWidth - menuRect.width - viewportPadding))
        }
      }

      const maxTop = Math.max(viewportPadding, window.innerHeight - submenuRect.height - viewportPadding)
      const top = Math.max(viewportPadding, Math.min(menuRect.top, maxTop))
      setSubmenuSide(side)
      setSubmenuTop(top - menuRect.top)
      if (menuX !== position.x) setPosition((current) => ({ ...current, x: menuX }))
    }
    updateSubmenuPosition()
    window.addEventListener('resize', updateSubmenuPosition)
    return () => window.removeEventListener('resize', updateSubmenuPosition)
  }, [position.x, position.y, submenuOpen])

  useLayoutEffect(() => {
    if (!submenuOpen) {
      triggerRef.current?.focus()
      return
    }
    const items = submenuRef.current?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]:not(:disabled)')
    if (!items?.length) return
    items[initialSubmenuFocus === 'first' ? 0 : items.length - 1].focus()
  }, [initialSubmenuFocus, submenuOpen])

  useLayoutEffect(() => {
    triggerRef.current?.focus()
  }, [])

  useEffect(() => {
    const dismissOutside = (event: PointerEvent) => {
      if (event.target instanceof Node && !menuRef.current?.contains(event.target)) onClose(false)
    }
    document.addEventListener('pointerdown', dismissOutside, true)
    return () => document.removeEventListener('pointerdown', dismissOutside, true)
  }, [onClose])

  function openSubmenu(initialFocus: 'first' | 'last' = 'first') {
    setInitialSubmenuFocus(initialFocus)
    setSubmenuOpen(true)
  }

  function closeSubmenu() {
    setSubmenuOpen(false)
    setInitialSubmenuFocus('first')
    triggerRef.current?.focus()
  }

  function handleKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    const target = event.target instanceof HTMLElement ? event.target : null
    if (event.key === 'Escape') {
      event.preventDefault()
      if (submenuOpen) closeSubmenu()
      else onClose(true)
      return
    }
    if (event.key === 'Tab') {
      event.preventDefault()
      onClose(true)
      return
    }
    if (target === triggerRef.current) {
      if (['ArrowRight', 'ArrowDown', 'Enter', ' '].includes(event.key)) {
        event.preventDefault()
        openSubmenu()
      } else if (event.key === 'ArrowUp') {
        event.preventDefault()
        openSubmenu('last')
      }
      return
    }
    if (submenuOpen && target?.closest('.canvas-context-menu-submenu')) {
      if (event.key === 'ArrowLeft') {
        event.preventDefault()
        closeSubmenu()
        return
      }
      if (event.key === 'Enter' || event.key === ' ') {
        const item = target.closest<HTMLButtonElement>('button[role="menuitem"]:not(:disabled)')
        if (item) {
          event.preventDefault()
          item.click()
          return
        }
      }
    }
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
    const activeMenu = target?.closest<HTMLElement>('[role="menu"]')
    const items = [...(activeMenu?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]:not(:disabled)') ?? [])]
    if (!items.length) return
    event.preventDefault()
    const currentIndex = items.indexOf(target?.closest('button[role="menuitem"]') as HTMLButtonElement)
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
      <button
        ref={triggerRef}
        type="button"
        role="menuitem"
        className="canvas-context-menu-item canvas-context-menu-trigger"
        aria-haspopup="menu"
        aria-controls={submenuId}
        aria-expanded={submenuOpen}
        onClick={() => submenuOpen ? closeSubmenu() : openSubmenu()}
      >
        <span>新增节点</span>
        <span className="canvas-context-menu-chevron" aria-hidden="true">›</span>
      </button>
      {submenuOpen ? (
        <div
          ref={submenuRef}
          id={submenuId}
          className="canvas-context-menu-submenu"
          role="menu"
          aria-label="选择节点类型"
          data-side={submenuSide}
          style={{ top: submenuTop }}
        >
          <strong className="canvas-context-menu-title">选择节点类型</strong>
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
            <div className="canvas-context-menu-item canvas-context-menu-item-info" role="presentation">
              <span>候选</span>
              <small>由选题生成</small>
            </div>
          </div>
          <span className="canvas-context-menu-hint">方向键选择 · Enter 确认 · Esc 返回</span>
        </div>
      ) : null}
    </div>,
    document.body,
  )
}
