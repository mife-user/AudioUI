export type CanvasNodeRect = {
  nodeId: string
  x: number
  y: number
  width: number
  height: number
}

export type CanvasEdgeData = {
  sourceNodeId: string
  targetNodeId: string
  obstacles: CanvasNodeRect[]
}

type Point = { x: number; y: number }
type ExpandedRect = CanvasNodeRect

type RouteResult = {
  path: string
  kind: 'curve' | 'detour' | 'fallback'
}

const OBSTACLE_PADDING = 14
const ROUTE_GAP = 28
const MIN_FORWARD_GAP = 88
const EXIT_LENGTHS = [24, 40, 64]

function expandRect(rect: CanvasNodeRect): ExpandedRect {
  return {
    ...rect,
    x: rect.x - OBSTACLE_PADDING,
    y: rect.y - OBSTACLE_PADDING,
    width: rect.width + OBSTACLE_PADDING * 2,
    height: rect.height + OBSTACLE_PADDING * 2,
  }
}

function format(value: number): string {
  return Number(value.toFixed(2)).toString()
}

function curvePath(source: Point, target: Point): string {
  const distance = Math.max(48, Math.min(160, Math.abs(target.x - source.x) * 0.45))
  return `M ${format(source.x)} ${format(source.y)} C ${format(source.x + distance)} ${format(source.y)}, ${format(target.x - distance)} ${format(target.y)}, ${format(target.x)} ${format(target.y)}`
}

function pointInRect(point: Point, rect: ExpandedRect): boolean {
  return point.x > rect.x && point.x < rect.x + rect.width
    && point.y > rect.y && point.y < rect.y + rect.height
}

function curveIsClear(source: Point, target: Point, obstacles: ExpandedRect[]): boolean {
  const distance = Math.max(48, Math.min(160, Math.abs(target.x - source.x) * 0.45))
  const left = Math.min(source.x, target.x)
  const right = Math.max(source.x, target.x)
  const top = Math.min(source.y, target.y)
  const bottom = Math.max(source.y, target.y)
  const nearby = obstacles.filter((rect) => rect.x <= right && rect.x + rect.width >= left
    && rect.y <= bottom && rect.y + rect.height >= top)
  const sampleCount = Math.max(24, Math.ceil(Math.max(right - left, bottom - top) / 32))
  for (let sample = 1; sample < sampleCount; sample += 1) {
    const t = sample / sampleCount
    const inverse = 1 - t
    const point = {
      x: inverse ** 3 * source.x + 3 * inverse ** 2 * t * (source.x + distance)
        + 3 * inverse * t ** 2 * (target.x - distance) + t ** 3 * target.x,
      y: inverse ** 3 * source.y + 3 * inverse ** 2 * t * source.y
        + 3 * inverse * t ** 2 * target.y + t ** 3 * target.y,
    }
    if (nearby.some((rect) => pointInRect(point, rect))) return false
  }
  return true
}

function segmentIsClear(start: Point, end: Point, obstacles: ExpandedRect[]): boolean {
  return !obstacles.some((rect) => {
    if (start.y === end.y) {
      return start.y > rect.y && start.y < rect.y + rect.height
        && Math.min(start.x, end.x) < rect.x + rect.width
        && Math.max(start.x, end.x) > rect.x
    }
    if (start.x === end.x) {
      return start.x > rect.x && start.x < rect.x + rect.width
        && Math.min(start.y, end.y) < rect.y + rect.height
        && Math.max(start.y, end.y) > rect.y
    }
    const steps = 24
    return Array.from({ length: steps - 1 }, (_, index) => index + 1).some((step) => {
      const t = step / steps
      return pointInRect({ x: start.x + (end.x - start.x) * t, y: start.y + (end.y - start.y) * t }, rect)
    })
  })
}

function pointsAreClear(points: Point[], obstacles: ExpandedRect[]): boolean {
  return points.slice(1).every((point, index) => segmentIsClear(points[index], point, obstacles))
}

function pathFromPoints(points: Point[]): string {
  const unique = points.filter((point, index) => index === 0 || point.x !== points[index - 1].x || point.y !== points[index - 1].y)
  return unique.map((point, index) => `${index === 0 ? 'M' : 'L'} ${format(point.x)} ${format(point.y)}`).join(' ')
}

export function routeCanvasEdge(
  source: Point,
  target: Point,
  nodeRects: CanvasNodeRect[],
  sourceNodeId: string,
  targetNodeId: string,
): RouteResult {
  const obstacles = nodeRects
    .filter((rect) => rect.nodeId !== sourceNodeId && rect.nodeId !== targetNodeId)
    .map(expandRect)
  const curve = curvePath(source, target)
  if (target.x - source.x >= MIN_FORWARD_GAP && curveIsClear(source, target, obstacles)) {
    return { path: curve, kind: 'curve' }
  }

  const horizontalLeft = Math.min(source.x, target.x)
  const horizontalRight = Math.max(source.x, target.x)
  const nearby = obstacles.filter((rect) => rect.x <= horizontalRight + 80 && rect.x + rect.width >= horizontalLeft - 80)
  const top = Math.min(source.y, target.y, ...nearby.map((rect) => rect.y)) - ROUTE_GAP
  const bottom = Math.max(source.y, target.y, ...nearby.map((rect) => rect.y + rect.height)) + ROUTE_GAP

  for (const laneY of [top, bottom]) {
    for (const exitLength of EXIT_LENGTHS) {
      const sourceExit = { x: source.x + exitLength, y: source.y }
      const targetEntry = { x: target.x - exitLength, y: target.y }
      const points = [
        source,
        sourceExit,
        { x: sourceExit.x, y: laneY },
        { x: targetEntry.x, y: laneY },
        targetEntry,
        target,
      ]
      if (pointsAreClear(points, obstacles)) return { path: pathFromPoints(points), kind: 'detour' }
    }
  }

  const left = Math.min(source.x, target.x, ...obstacles.map((rect) => rect.x)) - ROUTE_GAP
  const right = Math.max(source.x, target.x, ...obstacles.map((rect) => rect.x + rect.width)) + ROUTE_GAP
  for (const laneX of [left, right]) {
    const points = [source, { x: laneX, y: source.y }, { x: laneX, y: target.y }, target]
    if (pointsAreClear(points, obstacles)) return { path: pathFromPoints(points), kind: 'detour' }
  }

  return { path: curve, kind: 'fallback' }
}
