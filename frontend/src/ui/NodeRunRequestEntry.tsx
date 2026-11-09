import { createPortal } from 'react-dom'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useRunRequestContext } from '../hooks/runRequests'
import { PermissionCard } from './PermissionCard'
import { QuestionCard } from './QuestionCard'

export function NodeRunRequestEntry({ nodeId, nodeTitle }: { nodeId: string; nodeTitle: string }) {
  const requestContext = useRunRequestContext()
  const groups = useMemo(() => requestContext?.groups.filter((group) => group.nodeId === nodeId) ?? [], [nodeId, requestContext?.groups])
  const requestCount = groups.reduce((total, group) => total + group.permissions.length + group.questions.length, 0)
  const [open, setOpen] = useState(false)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const dialogRef = useRef<HTMLElement>(null)
  const fallbackFocusRef = useRef<HTMLElement | null>(null)

  const close = useCallback(() => {
    const trigger = triggerRef.current
    const fallback = fallbackFocusRef.current
    setOpen(false)
    window.setTimeout(() => {
      if (trigger?.isConnected) trigger.focus()
      else fallback?.focus()
    }, 0)
  }, [])

  useEffect(() => {
    if (open) dialogRef.current?.focus()
  }, [open])

  useEffect(() => {
    if (!open) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        close()
        return
      }
      if (event.key !== 'Tab' || !dialogRef.current) return
      const focusable = [...dialogRef.current.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), [tabindex]:not([tabindex="-1"])')]
      if (!focusable.length) {
        event.preventDefault()
        return
      }
      if (event.shiftKey && document.activeElement === focusable[0]) {
        event.preventDefault()
        focusable[focusable.length - 1].focus()
      } else if (!event.shiftKey && document.activeElement === focusable[focusable.length - 1]) {
        event.preventDefault()
        focusable[0].focus()
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [close, open])

  if (!requestContext || (!requestCount && !open)) return null

  function openDialog() {
    const trigger = triggerRef.current
    fallbackFocusRef.current = trigger?.closest('.react-flow__node')?.querySelector<HTMLElement>('.canvas-node-head')
      ?? trigger?.closest('article')?.querySelector<HTMLElement>('header')
      ?? null
    setOpen(true)
  }

  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        className="node-run-request-trigger nodrag"
        aria-label={requestCount ? `查看 ${nodeTitle} 的 ${requestCount} 个运行请求` : `${nodeTitle} 的运行请求已处理`}
        aria-haspopup="dialog"
        aria-expanded={open}
        onClick={openDialog}
      >
        {requestCount ? `请求 ${requestCount}` : '请求已处理'}
      </button>
      {open && typeof document !== 'undefined' ? createPortal(
        <div className="node-run-request-backdrop" role="presentation" onMouseDown={(event) => {
          if (event.target === event.currentTarget) close()
        }}>
          <section
            ref={dialogRef}
            className="node-run-request-dialog"
            role="dialog"
            aria-modal="true"
            aria-label={`${nodeTitle} 的运行请求`}
            tabIndex={-1}
          >
            <header className="node-run-request-head">
              <div>
                <span className="workspace-kicker">运行请求</span>
                <strong>{nodeTitle}</strong>
              </div>
              <button type="button" className="agent-icon-btn" aria-label="关闭运行请求" onClick={close}>×</button>
            </header>
            <div className="node-run-request-body">
              {groups.length ? groups.map((group) => (
                <section className="run-request-group" key={group.runId}>
                  <div className="run-request-owner">运行 {group.runId}</div>
                  {group.permissions.map((request) => (
                    <PermissionCard
                      key={`${group.runId}-permission-${request.id}`}
                      request={request}
                      onRespond={(requestId, decision) => requestContext.respondPermission(group.runId, requestId, decision)}
                    />
                  ))}
                  {group.questions.map((request) => (
                    <QuestionCard
                      key={`${group.runId}-question-${request.id}`}
                      request={request}
                      onAnswer={(requestId, answer, via) => requestContext.respondQuestion(group.runId, requestId, answer, via)}
                    />
                  ))}
                </section>
              )) : <p className="node-run-request-empty" role="status">该节点的运行请求已全部处理。</p>}
            </div>
          </section>
        </div>,
        document.body,
      ) : null}
    </>
  )
}
