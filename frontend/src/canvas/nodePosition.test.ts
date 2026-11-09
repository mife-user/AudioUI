import { describe, expect, it } from 'vitest'
import type { Node } from '../../bindings/audio-ui/internal/domain/entity'
import { allocateNodePosition } from './nodePosition'

function node(id: string, x: number, y: number): Node {
  return {
    id, type: 'topic' as Node['type'], title: id, position: { x, y }, size: { width: 280, height: 230 }, params: {},
    rounds: [], locked: false, stale: false,
  }
}

describe('allocateNodePosition', () => {
  it('uses the requested origin when it is clear', () => {
    expect(allocateNodePosition([], { x: 400, y: 300 })).toEqual({ x: 400, y: 300 })
  })

  it('moves later nodes to non-overlapping positions even when siblings share an origin', () => {
    const existing = [node('one', 400, 300)]
    const second = allocateNodePosition(existing, { x: 400, y: 300 })
    const third = allocateNodePosition([...existing, node('two', second.x, second.y)], { x: 400, y: 300 })

    expect(second).not.toEqual(existing[0].position)
    expect(third).not.toEqual(second)
    expect([second, third].every((position) => Math.abs(position.x - 400) >= 300 || Math.abs(position.y - 300) >= 220)).toBe(true)
  })

  it('keeps expanded topic cards separated when they share an origin', () => {
    const first = node('one', 400, 300)
    const secondPosition = allocateNodePosition([first], first.position, 'topic')
    const second = node('two', secondPosition.x, secondPosition.y)
    const overlaps = second.position.x < first.position.x + 320 + 36
      && second.position.x + 320 + 36 > first.position.x
      && second.position.y < first.position.y + 480 + 36
      && second.position.y + 480 + 36 > first.position.y

    expect(overlaps).toBe(false)
  })

  it('avoids old nodes that were placed close together', () => {
    const existing = [node('one', 535, 362), node('two', 579, 381)]
    const next = allocateNodePosition(existing, { x: 535, y: 362 })

    expect(next).not.toEqual(existing[0].position)
    expect(next).not.toEqual(existing[1].position)
  })
})
