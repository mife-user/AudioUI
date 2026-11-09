import type { CanvasFlowNode } from './flowModel'

function hasCompleteMeasurement(node: CanvasFlowNode): boolean {
  return Number.isFinite(node.measured?.width)
    && Number(node.measured?.width) > 0
    && Number.isFinite(node.measured?.height)
    && Number(node.measured?.height) > 0
}

function sameNodeStyle(left: CanvasFlowNode['style'], right: CanvasFlowNode['style']): boolean {
  return left?.width === right?.width && left?.height === right?.height
}

function sameNodeData(left: CanvasFlowNode['data'], right: CanvasFlowNode['data']): boolean {
  const keys = Object.keys(left) as Array<keyof CanvasFlowNode['data']>
  return keys.length === Object.keys(right).length && keys.every((key) => Object.is(left[key], right[key]))
}

/**
 * Keeps React Flow's transient layout/interaction state when business snapshots
 * rebuild a node with the same identity. Internal React Flow internals are never
 * copied; project and node type changes start with a fresh measurement.
 */
export function preserveCanvasNodeViewState(
  nextNodes: CanvasFlowNode[],
  currentNodes: CanvasFlowNode[],
  projectId: string,
): CanvasFlowNode[] {
  const currentById = new Map(currentNodes.map((node) => [node.id, node]))

  return nextNodes.map((node) => {
    const previous = currentById.get(node.id)
    if (
      !previous
      || !projectId
      || previous.type !== node.type
      || previous.data.projectId !== projectId
      || node.data.projectId !== projectId
    ) return node

    let next = node
    if (previous.resizing) {
      next = {
        ...next,
        position: previous.position,
        style: previous.style,
        ...(hasCompleteMeasurement(previous) ? { measured: previous.measured } : {}),
        resizing: true,
      }
    } else if (sameNodeStyle(previous.style, node.style) && hasCompleteMeasurement(previous) && node.measured !== previous.measured) {
      next = { ...next, measured: previous.measured }
    }
    if (typeof previous.dragging === 'boolean' && next.dragging !== previous.dragging) {
      next = { ...next, dragging: previous.dragging }
    }
    if (!previous.resizing && typeof previous.resizing === 'boolean' && next.resizing !== previous.resizing) {
      next = { ...next, resizing: previous.resizing }
    }
    if (
      previous.position.x === next.position.x
      && previous.position.y === next.position.y
      && previous.selected === next.selected
      && previous.measured === next.measured
      && previous.dragging === next.dragging
      && previous.resizing === next.resizing
      && sameNodeStyle(previous.style, next.style)
      && sameNodeData(previous.data, next.data)
    ) return previous
    return next
  })
}
