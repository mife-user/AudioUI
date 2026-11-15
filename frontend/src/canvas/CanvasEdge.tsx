import {
  BaseEdge,
  Position,
  useReactFlow,
  useStore,
  type Edge,
  type EdgeProps,
} from '@xyflow/react'
import { useCallback, useEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from 'react'
import type { Edge as ProjectEdge } from '../../bindings/audio-ui/internal/domain/entity'
import { routeCanvasEdge, type CanvasEdgeData } from './edgeRouting'
import { canvasNodeRects } from './nodeRects'
import type { CanvasFlowNode } from './flowModel'

export type WorkspaceEdge = Edge<CanvasEdgeData, 'workspace'>

type DragState = {
  projectId: string
  edge: ProjectEdge
  pointer: { x: number; y: number }
  targetNodeId: string | null
  valid: boolean
}

const GRIP_SIZE = 24
const GRIP_LANE_GAP = 24
const GRIP_ARROW_OFFSET = 8

function horizontalDirection(position: Position): -1 | 1 {
  return position === Position.Left ? -1 : 1
}

function inputNodeAt(clientX: number, clientY: number): string | null {
  const element = document.elementFromPoint(clientX, clientY)
  const directHandle = element?.closest<HTMLElement>('.react-flow__handle.target')
  const hitNode = directHandle?.closest<HTMLElement>('.react-flow__node') ?? element?.closest<HTMLElement>('.react-flow__node')
  const handles = hitNode
    ? [...hitNode.querySelectorAll<HTMLElement>('.react-flow__handle.target')]
    : [...document.querySelectorAll<HTMLElement>('.react-flow__handle.target')]
  const handle = directHandle ?? handles.find((candidate) => {
    const bounds = candidate.getBoundingClientRect()
    return clientX >= bounds.left && clientX <= bounds.right && clientY >= bounds.top && clientY <= bounds.bottom
  })
  return handle?.closest<HTMLElement>('.react-flow__node')?.dataset.id ?? null
}

export function CanvasEdge({
  id,
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  data,
  markerStart,
  markerEnd,
  interactionWidth,
  style,
}: EdgeProps<WorkspaceEdge>) {
  const flow = useReactFlow<CanvasFlowNode>()
  const zoom = useStore((state) => state.transform[2])
  const [drag, setDrag] = useState<DragState | null>(null)
  const [gripHovered, setGripHovered] = useState(false)
  const dragHover = useRef<{ targetNodeId: string | null; valid: boolean }>({ targetNodeId: null, valid: false })
  const dragProjectId = useRef(data?.projectId ?? '')

  const reportHover = useCallback((targetNodeId: string | null, valid: boolean) => {
    const current = dragHover.current
    if (current.targetNodeId === targetNodeId && current.valid === valid) return
    dragHover.current = { targetNodeId, valid }
    data?.onReconnectHoverTarget?.(targetNodeId, valid)
  }, [data?.onReconnectHoverTarget])

  const cancelDrag = useCallback(() => {
    setDrag(null)
    setGripHovered(false)
    reportHover(null, false)
  }, [reportHover])

  useEffect(() => {
    if (dragProjectId.current === data?.projectId) return
    dragProjectId.current = data?.projectId ?? ''
    cancelDrag()
  }, [cancelDrag, data?.projectId])

  useEffect(() => {
    if (!drag) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      event.preventDefault()
      cancelDrag()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [cancelDrag, drag])

  const onGripPointerDown = (event: ReactPointerEvent<SVGCircleElement>) => {
    if (event.button !== 0 || !data?.reconnectable || (event.pointerType && event.pointerType !== 'mouse')) return
    event.preventDefault()
    event.stopPropagation()
    event.currentTarget.setPointerCapture(event.pointerId)
    const pointer = flow.screenToFlowPosition({ x: event.clientX, y: event.clientY })
    setGripHovered(true)
    setDrag({ projectId: data.projectId, edge: { ...data.edge }, pointer, targetNodeId: null, valid: false })
  }

  const onGripPointerMove = (event: ReactPointerEvent<SVGCircleElement>) => {
    if (!drag) return
    event.preventDefault()
    event.stopPropagation()
    const pointer = flow.screenToFlowPosition({ x: event.clientX, y: event.clientY })
    const targetNodeId = inputNodeAt(event.clientX, event.clientY)
    const valid = Boolean(targetNodeId && data?.reconnectableTargetIds.includes(targetNodeId))
    reportHover(targetNodeId, valid)
    setDrag({ ...drag, pointer, targetNodeId, valid })
  }

  const onGripPointerUp = (event: ReactPointerEvent<SVGCircleElement>) => {
    if (!drag) return
    event.preventDefault()
    event.stopPropagation()
    const targetNodeId = inputNodeAt(event.clientX, event.clientY)
    const valid = Boolean(targetNodeId && data?.reconnectableTargetIds.includes(targetNodeId))
    const expectedProjectId = drag.projectId
    const expectedEdge = drag.edge
    cancelDrag()
    if (valid && targetNodeId && targetNodeId !== expectedEdge.targetNodeId) {
      data?.onReconnectTarget?.(expectedProjectId, expectedEdge, targetNodeId)
    }
  }

  const slotOffset = ((data?.reconnectSlot ?? 0) * GRIP_LANE_GAP) / zoom
  let gripX = targetX
  let gripY = targetY
  if (targetPosition === Position.Left) {
    gripX -= GRIP_ARROW_OFFSET / zoom
    gripY += slotOffset
  } else if (targetPosition === Position.Right) {
    gripX += GRIP_ARROW_OFFSET / zoom
    gripY += slotOffset
  } else if (targetPosition === Position.Top) {
    gripY -= GRIP_ARROW_OFFSET / zoom
    gripX += slotOffset
  } else {
    gripY += GRIP_ARROW_OFFSET / zoom
    gripX += slotOffset
  }

  const previewRoute = drag && data
    ? routeCanvasEdge(
        { x: sourceX, y: sourceY },
        drag.pointer,
        canvasNodeRects(flow.getNodes()),
        data.sourceNodeId,
        drag.valid ? drag.targetNodeId ?? '' : '',
        {
          sourceDirection: horizontalDirection(sourcePosition),
          targetDirection: horizontalDirection(targetPosition),
        },
      )
    : null

  return (
    <>
      <BaseEdge
        path={previewRoute?.path ?? routeCanvasEdge(
          { x: sourceX, y: sourceY },
          { x: targetX, y: targetY },
          data?.obstacles ?? [],
          data?.sourceNodeId ?? '',
          data?.targetNodeId ?? '',
        ).path}
        markerStart={markerStart}
        markerEnd={markerEnd}
        interactionWidth={drag ? 0 : interactionWidth ?? 20}
        style={{ ...style, visibility: drag ? 'hidden' : style?.visibility }}
        className={gripHovered ? 'workspace-edge-reconnect-hovered' : undefined}
      />
      {previewRoute ? <path className={`canvas-edge-reconnect-preview ${drag?.valid ? 'is-valid' : 'is-invalid'}`} d={previewRoute.path} /> : null}
      {data?.reconnectable ? (
        <circle
          className="canvas-edge-reconnect-grip"
          data-testid={`reconnect-grip-${id}`}
          aria-hidden="true"
          cx={gripX}
          cy={gripY}
          r={GRIP_SIZE / (2 * zoom)}
          onPointerDown={onGripPointerDown}
          onPointerMove={onGripPointerMove}
          onPointerUp={onGripPointerUp}
          onPointerCancel={cancelDrag}
          onLostPointerCapture={cancelDrag}
          onPointerEnter={() => setGripHovered(true)}
          onPointerLeave={() => { if (!drag) setGripHovered(false) }}
          onClick={(event) => event.stopPropagation()}
          onDoubleClick={(event) => event.stopPropagation()}
        />
      ) : null}
    </>
  )
}
