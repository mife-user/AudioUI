import { describe, expect, it } from 'vitest'
import { routeCanvasEdge, type CanvasNodeRect } from './edgeRouting'

const empty: CanvasNodeRect[] = []

describe('routeCanvasEdge', () => {
  it('draws a clear forward connection as a smooth curve', () => {
    const route = routeCanvasEdge({ x: 100, y: 100 }, { x: 500, y: 160 }, empty, 'source', 'target')

    expect(route.kind).toBe('curve')
    expect(route.path).toContain(' C ')
  })

  it('routes around an intervening card with a bounded orthogonal detour', () => {
    const obstacle: CanvasNodeRect = { nodeId: 'blocker', x: 260, y: 40, width: 80, height: 120 }
    const route = routeCanvasEdge({ x: 100, y: 100 }, { x: 500, y: 100 }, [obstacle], 'source', 'target')

    expect(route.kind).toBe('detour')
    expect(route.path).not.toContain(' C ')
    expect(route.path).toContain('L')
  })

  it('does not skip cards along a long connection', () => {
    const obstacle: CanvasNodeRect = { nodeId: 'blocker', x: 5400, y: -50, width: 235, height: 160 }
    const route = routeCanvasEdge({ x: 0, y: 0 }, { x: 20000, y: 0 }, [obstacle], 'source', 'target')

    expect(route.kind).toBe('detour')
  })

  it('routes reverse connections around the endpoints instead of folding through them', () => {
    const route = routeCanvasEdge({ x: 500, y: 100 }, { x: 100, y: 160 }, empty, 'source', 'target')

    expect(route.kind).toBe('detour')
    expect(route.path).not.toContain(' C ')
  })

  it('handles routing across a 200-node layout', () => {
    const nodes = Array.from({ length: 200 }, (_, index) => ({
      nodeId: `node-${index}`,
      x: (index % 20) * 270,
      y: Math.floor(index / 20) * 220,
      width: 235,
      height: 160,
    }))
    const routes = nodes.slice(0, -1).map((source, index) => {
      const target = nodes[index + 1]
      return routeCanvasEdge(
        { x: source.x + source.width, y: source.y + source.height / 2 },
        { x: target.x, y: target.y + target.height / 2 },
        nodes,
        source.nodeId,
        target.nodeId,
      )
    })

    expect(routes).toHaveLength(199)
    expect(routes.every((route) => route.path.startsWith('M '))).toBe(true)
  })

  it('ignores the connected node rectangles when testing a forward curve', () => {
    const endpoints: CanvasNodeRect[] = [
      { nodeId: 'source', x: 80, y: 70, width: 40, height: 60 },
      { nodeId: 'target', x: 500, y: 130, width: 50, height: 60 },
    ]
    const route = routeCanvasEdge({ x: 120, y: 100 }, { x: 500, y: 160 }, endpoints, 'source', 'target')

    expect(route.kind).toBe('curve')
  })
})
