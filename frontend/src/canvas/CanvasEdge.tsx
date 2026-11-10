import { BaseEdge, type Edge, type EdgeProps } from '@xyflow/react'
import { routeCanvasEdge, type CanvasEdgeData } from './edgeRouting'

export type WorkspaceEdge = Edge<CanvasEdgeData, 'workspace'>

export function CanvasEdge({
  sourceX,
  sourceY,
  targetX,
  targetY,
  data,
  markerStart,
  markerEnd,
  interactionWidth,
  style,
}: EdgeProps<WorkspaceEdge>) {
  const route = routeCanvasEdge(
    { x: sourceX, y: sourceY },
    { x: targetX, y: targetY },
    data?.obstacles ?? [],
    data?.sourceNodeId ?? '',
    data?.targetNodeId ?? '',
  )

  return (
    <BaseEdge
      path={route.path}
      markerStart={markerStart}
      markerEnd={markerEnd}
      interactionWidth={interactionWidth ?? 20}
      style={style}
    />
  )
}
