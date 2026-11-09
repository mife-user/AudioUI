import { Events } from '@wailsio/runtime'
import { useCallback, useEffect, useRef, useState } from 'react'

type CloseState =
  | { phase: 'saving' }
  | { phase: 'closing' }
  | { phase: 'failed'; message: string }

type CloseAttempt = { cancelled: boolean }

type Props = {
  onFlush: () => Promise<boolean>
}

function errorMessage(error: unknown): string {
  if (error instanceof Error) return error.message
  if (typeof error === 'string') return error
  if (error && typeof error === 'object' && 'message' in error) return String((error as { message: unknown }).message)
  return String(error)
}

export function ApplicationCloseGuard({ onFlush }: Props) {
  const dialogRef = useRef<HTMLDialogElement>(null)
  const activeAttempt = useRef<CloseAttempt | null>(null)
  const [state, setState] = useState<CloseState | null>(null)

  useEffect(() => {
    const dialog = dialogRef.current
    if (!dialog) return
    if (state && !dialog.open) dialog.showModal()
    if (!state && dialog.open) dialog.close()
  }, [state])

  const approveClose = useCallback(async () => {
    if (await Events.Emit('app:close-approved', null)) throw new Error('应用关闭请求未获准。')
  }, [])

  const requestClose = useCallback(() => {
    if (activeAttempt.current) return
    const attempt = { cancelled: false }
    activeAttempt.current = attempt
    setState({ phase: 'saving' })
    void (async () => {
      try {
        if (!await onFlush()) throw new Error('工程快照同步仍未完成。')
        if (attempt.cancelled) return
        setState({ phase: 'closing' })
        await approveClose()
        if (!attempt.cancelled) setState(null)
      } catch (failure) {
        if (!attempt.cancelled) setState({ phase: 'failed', message: errorMessage(failure) })
      } finally {
        if (activeAttempt.current === attempt) activeAttempt.current = null
      }
    })()
  }, [approveClose, onFlush])

  useEffect(() => Events.On('app:close-requested', requestClose), [requestClose])

  const stayOpen = () => {
    if (activeAttempt.current) activeAttempt.current.cancelled = true
    setState(null)
  }

  const discardAndClose = () => {
    if (activeAttempt.current) return
    const attempt = { cancelled: false }
    activeAttempt.current = attempt
    setState({ phase: 'closing' })
    void approveClose().then(() => {
      if (!attempt.cancelled) setState(null)
    }).catch((failure: unknown) => {
      if (!attempt.cancelled) setState({ phase: 'failed', message: errorMessage(failure) })
    }).finally(() => {
      if (activeAttempt.current === attempt) activeAttempt.current = null
    })
  }

  return (
    <dialog
      ref={dialogRef}
      className="project-dialog"
      aria-labelledby="application-close-title"
      onCancel={(event) => {
        event.preventDefault()
        stayOpen()
      }}
    >
      <div className="project-dialog-head">
        <div>
          <h2 id="application-close-title">关闭 AudioUI</h2>
          <p>关闭前先保护当前工程的选题与提示词草稿。</p>
        </div>
      </div>
      <div className="project-dialog-body">
        {state?.phase === 'saving' ? (
          <section className="workspace-inline-error project-draft-warning" role="status">
            <div className="project-draft-message">
              <h3>正在保存草稿</h3>
              <p>保存和快照同步完成后会关闭应用。</p>
            </div>
            <div className="project-draft-actions">
              <button className="agent-mini-btn" type="button" onClick={stayOpen}>取消关闭</button>
            </div>
          </section>
        ) : null}
        {state?.phase === 'closing' ? (
          <div className="agent-console-empty" role="status">正在关闭应用…</div>
        ) : null}
        {state?.phase === 'failed' ? (
          <section className="workspace-inline-error project-draft-warning" aria-labelledby="application-close-warning">
            <div className="project-draft-message">
              <h3 id="application-close-warning">草稿尚未安全保存</h3>
              <p role="alert">{state.message}</p>
            </div>
            <div className="project-draft-actions">
              <button className="primary-btn" type="button" onClick={requestClose}>重试保存并关闭</button>
              <button className="agent-mini-btn" type="button" onClick={discardAndClose}>放弃未保存草稿并关闭</button>
              <button className="agent-mini-btn" type="button" onClick={stayOpen}>留在应用</button>
            </div>
          </section>
        ) : null}
      </div>
    </dialog>
  )
}
