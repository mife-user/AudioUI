import { createPortal } from 'react-dom'
import { useCallback, useEffect, useRef } from 'react'
import { OutputKind, PendingStatus } from '../../bindings/audio-ui/internal/domain/valueobject'
import type { Node } from '../../bindings/audio-ui/internal/domain/entity'
import { usePendingControllerContext } from '../hooks/usePendingController'
import { PendingArtifactCard } from './PendingArtifactCard'

function label(status: string, type: string): string {
  if (status === PendingStatus.PendingInvalid) return '处理无效结果'
  return type === OutputKind.OutputKindTopics ? '查看选题结果' : '查看待确认结果'
}

export function PendingNodeEntry({ node, pendingIds }: { node: Node; pendingIds: string[] }) {
  const controller = usePendingControllerContext()
  const triggerRefs = useRef(new Map<string, HTMLButtonElement>())
  const dialogRef = useRef<HTMLElement>(null)
  const fallbackFocusRef = useRef<HTMLElement | null>(null)
  const activePending = controller?.surface === 'node' && controller.surfaceNodeId === node.id
    ? controller.selectedPending
    : undefined
  const pendingItems = controller?.pendingItems.filter((item) => pendingIds.includes(item.id)) ?? []

  const close = useCallback(() => {
    const pendingId = activePending?.id
    const trigger = pendingId ? triggerRefs.current.get(pendingId) : undefined
    const fallback = fallbackFocusRef.current
    controller?.closeNodeSurface()
    window.setTimeout(() => {
      if (trigger?.isConnected) trigger.focus()
      else fallback?.focus()
    }, 0)
  }, [activePending?.id, controller])

  useEffect(() => {
    if (activePending) dialogRef.current?.focus()
  }, [activePending?.id])

  useEffect(() => {
    if (!activePending) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      event.preventDefault()
      close()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [activePending, close])

  if (!controller || (!pendingItems.length && !activePending)) return null

  return (
    <>
      {pendingItems.length ? (
        <div className="node-pending-actions" aria-label={`${node.title} 的待处理结果`}>
          {pendingItems.map((item, index) => (
            <button
              key={item.id}
              ref={(element) => {
                if (element) triggerRefs.current.set(item.id, element)
                else triggerRefs.current.delete(item.id)
              }}
              type="button"
              className="node-pending-trigger nodrag"
              onClick={() => {
                const trigger = triggerRefs.current.get(item.id)
                fallbackFocusRef.current = trigger?.closest('.react-flow__node')?.querySelector<HTMLElement>('.canvas-node-head')
                  ?? trigger?.closest('article')?.querySelector<HTMLElement>('header')
                  ?? null
                void controller.openFromNode(item.id, node.id)
              }}
            >
              {label(item.status, item.kind)}{pendingItems.length > 1 ? ` ${index + 1}` : ''}
            </button>
          ))}
        </div>
      ) : null}
      {activePending ? createPortal(
        <div className="node-pending-backdrop" role="presentation" onMouseDown={(event) => {
          if (event.target === event.currentTarget) close()
        }}>
          <section
            ref={dialogRef}
            className="node-pending-dialog"
            role="dialog"
            aria-modal="true"
            aria-label={`结果 ${activePending.id}`}
            tabIndex={-1}
          >
            <header className="node-pending-dialog-head">
              <strong>{activePending.kind === OutputKind.OutputKindTopics ? '选题结果待处理' : '运行结果待确认'}</strong>
              <button type="button" className="agent-icon-btn" aria-label="关闭结果预览" onClick={close}>×</button>
            </header>
            <PendingArtifactCard
              summary={activePending}
              detail={controller.selectedPendingDetail}
              outputNode={node.type === 'script' ? node : controller.selectedOutputNode}
              loading={controller.loading}
              loadError={controller.loadError}
              actionError={controller.actionError}
              forceConfirmation={controller.forceConfirmation}
              actionBusy={controller.actionBusy}
              resolved={controller.resolved}
              syncError={controller.syncError}
              onRetryDetail={controller.retryDetail}
              onApply={() => void controller.apply()}
              onDiscard={() => void controller.discard()}
              onConfirmForce={() => void controller.apply(true)}
              onCancelForce={controller.cancelForce}
              onRetryWorkspace={() => void controller.retryWorkspace()}
            />
          </section>
        </div>,
        document.body,
      ) : null}
    </>
  )
}
