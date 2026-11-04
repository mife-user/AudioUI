import type { Node, Position } from '../../bindings/audio-ui/internal/domain/entity'

const CARD_WIDTH = 300
const CARD_HEIGHT = 220
const GAP = 36
const STEP_X = CARD_WIDTH + GAP
const STEP_Y = CARD_HEIGHT + GAP

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

function overlaps(position: Position, node: Node): boolean {
  return position.x < node.position.x + CARD_WIDTH + GAP
    && position.x + CARD_WIDTH + GAP > node.position.x
    && position.y < node.position.y + CARD_HEIGHT + GAP
    && position.y + CARD_HEIGHT + GAP > node.position.y
}

export function allocateNodePosition(nodes: Node[], origin: Position): Position {
  const candidates = offsetsThrough(Math.max(2, nodes.length + 1))
  for (const offset of candidates) {
    const position = { x: origin.x + offset.x * STEP_X, y: origin.y + offset.y * STEP_Y }
    if (nodes.every((node) => !overlaps(position, node))) return position
  }
  return { x: origin.x + STEP_X * candidates.length, y: origin.y }
}
