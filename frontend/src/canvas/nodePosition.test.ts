import { describe, expect, it } from 'vitest'
import type { Node, Position } from '../../bindings/audio-ui/internal/domain/entity'
import { allocateNodePosition, NODE_LAYOUT_GAP } from './nodePosition'

const defaultSizes = {
  topic: { width: 280, height: 250 },
  candidate: { width: 235, height: 160 },
  prompt: { width: 280, height: 230 },
  agent: { width: 235, height: 165 },
  script: { width: 270, height: 205 },
} as const

type NodeKind = keyof typeof defaultSizes

function node(
  id: string,
  position: Position,
  type: NodeKind = 'topic',
  size: { width: number; height: number } = defaultSizes[type],
): Node {
  return {
    id, type: type as Node['type'], title: id, position, size, params: {},
    rounds: [], locked: false, stale: false,
  }
}

function isSeparated(position: Position, size: { width: number; height: number }, other: Node): boolean {
  return position.x >= other.position.x + other.size.width + NODE_LAYOUT_GAP
    || position.x + size.width + NODE_LAYOUT_GAP <= other.position.x
    || position.y >= other.position.y + other.size.height + NODE_LAYOUT_GAP
    || position.y + size.height + NODE_LAYOUT_GAP <= other.position.y
}

describe('allocateNodePosition', () => {
  it('uses the requested origin when it is clear', () => {
    expect(allocateNodePosition([], { x: 400, y: 300 })).toEqual({ x: 400, y: 300 })
  })

  it('uses persisted node sizes and the requested node type size', () => {
    const resized = node('resized', { x: 100, y: 100 }, 'script', { width: 1000, height: 850 })
    const position = allocateNodePosition([resized], resized.position, 'candidate')

    expect(position).not.toEqual(resized.position)
    expect(isSeparated(position, defaultSizes.candidate, resized)).toBe(true)
  })

  it('keeps automatically placed siblings apart using their default dimensions', () => {
    const topic = node('topic', { x: 0, y: 0 })
    const origin = { x: topic.position.x + topic.size.width + NODE_LAYOUT_GAP, y: topic.position.y }
    const placed = [topic]
    const positions: Position[] = []

    for (let index = 0; index < 4; index += 1) {
      const requested = { x: origin.x, y: origin.y + index * (defaultSizes.candidate.height + NODE_LAYOUT_GAP) }
      const position = allocateNodePosition(placed, requested, 'candidate')
      positions.push(position)
      placed.push(node(`candidate-${index}`, position, 'candidate'))
    }

    for (let index = 0; index < positions.length; index += 1) {
      expect(placed.slice(0, index + 1).every((other) => isSeparated(positions[index], defaultSizes.candidate, other))).toBe(true)
    }
    expect(positions).toEqual([
      { x: origin.x, y: origin.y },
      { x: origin.x, y: origin.y + defaultSizes.candidate.height + NODE_LAYOUT_GAP },
      { x: origin.x, y: origin.y + 2 * (defaultSizes.candidate.height + NODE_LAYOUT_GAP) },
      { x: origin.x, y: origin.y + 3 * (defaultSizes.candidate.height + NODE_LAYOUT_GAP) },
    ])
  })

  it('returns a deterministic non-overlapping position', () => {
    const existing = [node('one', { x: 535, y: 362 }), node('two', { x: 579, y: 381 })]
    const origin = { x: 535, y: 362 }
    const position = allocateNodePosition(existing, origin)

    expect(position).toEqual(allocateNodePosition(existing, origin))
    expect(existing.every((other) => isSeparated(position, defaultSizes.prompt, other))).toBe(true)
  })
})
