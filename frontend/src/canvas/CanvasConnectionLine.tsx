import { Position, useReactFlow, type ConnectionLineComponentProps } from '@xyflow/react'
import type { CanvasFlowNode } from './flowModel'
import { canvasNodeRects } from './nodeRects'
import { routeCanvasEdge } from './edgeRouting'

function horizontalDirection(position: Position): -1 | 1 {
  return position === Position.Left ? -1 : 1
}

export function CanvasConnectionLine({
  connectionLineStyle,
  connectionStatus,
  fromNode,
  fromPosition,
  fromX,
  fromY,
  toNode,
  toPosition,
  toX,
  toY,
}: ConnectionLineComponentProps<CanvasFlowNode>) {
  const flow = useReactFlow<CanvasFlowNode>()
  const route = routeCanvasEdge(
    { x: fromX, y: fromY },
    { x: toX, y: toY },
    canvasNodeRects(flow.getNodes()),
    fromNode.id,
    toNode?.id ?? '',
    {
      sourceDirection: horizontalDirection(fromPosition),
      targetDirection: horizontalDirection(toPosition),
    },
  )
  const statusClass = connectionStatus === 'invalid' ? 'canvas-connection-invalid' : 'canvas-connection-valid'

  return <path className={`react-flow__connection-path ${statusClass}`} d={route.path} style={connectionLineStyle} fill="none" />
}
