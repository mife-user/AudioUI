import type { Node, Position } from '../../bindings/audio-ui/internal/domain/entity'

const GAP = 36
const CARD_SIZE = {
  topic: { width: 320, height: 480 },
  candidate: { width: 276, height: 235 },
  prompt: { width: 276, height: 360 },
  agent: { width: 276, height: 220 },
  script: { width: 276, height: 230 },
} as const

type NodeKind = keyof typeof CARD_SIZE

type Offset = { x: number; y: number }

function offsetsThrough(ringCount: number): Offset[] {
  const offsets: Offset[] = [{ x: 0, y: 0 }]
  for (let ring = 1; ring <= ringCount; ring += 1) {
    for (let x = -ring; x <= ring; x += 1) offsets.push({ x, y: -ring })
    for (let y = -ring + 1; y <= ring; y += 1) offsets.push({ x: ring, y })
    for (let x = ring - 1; x >= -ring; x -= 1) offsets.push({ x, y: ring })
    for (let y = ring - 1; y > -ring; y -= 1) offsets.push({ x: -ring, y })
  }
  return offsets
}

function overlaps(position: Position, candidateSize: (typeof CARD_SIZE)[NodeKind], node: Node): boolean {
  const existingSize = CARD_SIZE[node.type as NodeKind]
  return position.x < node.position.x + existingSize.width + GAP
    && position.x + candidateSize.width + GAP > node.position.x
    && position.y < node.position.y + existingSize.height + GAP
    && position.y + candidateSize.height + GAP > node.position.y
}

export function allocateNodePosition(nodes: Node[], origin: Position, nodeType: NodeKind = 'prompt'): Position {
  const candidateSize = CARD_SIZE[nodeType]
  const stepX = candidateSize.width + GAP
  const stepY = candidateSize.height + GAP
  const candidates = offsetsThrough(Math.max(2, nodes.length + 1))
  for (const offset of candidates) {
    const position = { x: origin.x + offset.x * stepX, y: origin.y + offset.y * stepY }
    if (nodes.every((node) => !overlaps(position, candidateSize, node))) return position
  }
  return { x: origin.x + stepX * candidates.length, y: origin.y }
}
