import type { CanvasFlowNode } from './flowModel'
import type { CanvasNodeRect } from './edgeRouting'

export function canvasNodeRects(nodes: CanvasFlowNode[]): CanvasNodeRect[] {
  return nodes.map((node) => ({
    nodeId: node.id,
    x: node.position.x,
    y: node.position.y,
    width: node.measured?.width && node.measured.width > 0
      ? node.measured.width
      : typeof node.style?.width === 'number' ? node.style.width : node.data.node.size.width,
    height: node.measured?.height && node.measured.height > 0
      ? node.measured.height
      : typeof node.style?.height === 'number' ? node.style.height : node.data.node.size.height,
  }))
}
