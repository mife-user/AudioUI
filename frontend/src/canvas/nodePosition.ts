import type { Node, Position } from '../../bindings/audio-ui/internal/domain/entity'

export const NODE_LAYOUT_GAP = 36

const DEFAULT_NODE_SIZE = {
  topic: { width: 280, height: 250 },
  candidate: { width: 235, height: 160 },
  prompt: { width: 280, height: 230 },
  agent: { width: 235, height: 165 },
  script: { width: 270, height: 205 },
} as const

type NodeKind = keyof typeof DEFAULT_NODE_SIZE

function overlaps(position: Position, candidateSize: (typeof DEFAULT_NODE_SIZE)[NodeKind], node: Node): boolean {
  return position.x < node.position.x + node.size.width + NODE_LAYOUT_GAP
    && position.x + candidateSize.width + NODE_LAYOUT_GAP > node.position.x
    && position.y < node.position.y + node.size.height + NODE_LAYOUT_GAP
    && position.y + candidateSize.height + NODE_LAYOUT_GAP > node.position.y
}

export function allocateNodePosition(nodes: Node[], origin: Position, nodeType: NodeKind = 'prompt'): Position {
  const candidateSize = DEFAULT_NODE_SIZE[nodeType]
  const stepX = candidateSize.width + NODE_LAYOUT_GAP
  const stepY = candidateSize.height + NODE_LAYOUT_GAP
  for (let column = 0; column <= nodes.length; column += 1) {
    for (let row = 0; row <= nodes.length; row += 1) {
      const position = { x: origin.x + column * stepX, y: origin.y + row * stepY }
      if (nodes.every((node) => !overlaps(position, candidateSize, node))) return position
    }
  }
  const rightmost = nodes.reduce((right, node) => Math.max(right, node.position.x + node.size.width), origin.x)
  return { x: rightmost + NODE_LAYOUT_GAP, y: origin.y }
}
