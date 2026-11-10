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
type Direction = -1 | 1

type RouteSegment =
  | { type: 'line'; from: Point; to: Point }
  | { type: 'quadratic'; from: Point; control: Point; to: Point }
  | { type: 'cubic'; from: Point; control1: Point; control2: Point; to: Point }

type RouteResult = {
  path: string
  kind: 'curve' | 'detour' | 'fallback'
}

type RouteOptions = {
  sourceDirection?: Direction
  targetDirection?: Direction
}

const OBSTACLE_PADDING = 14
const ROUTE_GAP = 28
const CORNER_RADIUS = 24
const EXIT_LENGTHS = [24, 40, 64]
const LANE_OFFSETS = [0, 24, 56, 112]

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

function samePoint(left: Point, right: Point): boolean {
  return left.x === right.x && left.y === right.y
}

function distance(start: Point, end: Point): number {
  return Math.hypot(end.x - start.x, end.y - start.y)
}

function cubicPoint(segment: Extract<RouteSegment, { type: 'cubic' }>, t: number): Point {
  const inverse = 1 - t
  return {
    x: inverse ** 3 * segment.from.x + 3 * inverse ** 2 * t * segment.control1.x
      + 3 * inverse * t ** 2 * segment.control2.x + t ** 3 * segment.to.x,
    y: inverse ** 3 * segment.from.y + 3 * inverse ** 2 * t * segment.control1.y
      + 3 * inverse * t ** 2 * segment.control2.y + t ** 3 * segment.to.y,
  }
}

function quadraticPoint(segment: Extract<RouteSegment, { type: 'quadratic' }>, t: number): Point {
  const inverse = 1 - t
  return {
    x: inverse ** 2 * segment.from.x + 2 * inverse * t * segment.control.x + t ** 2 * segment.to.x,
    y: inverse ** 2 * segment.from.y + 2 * inverse * t * segment.control.y + t ** 2 * segment.to.y,
  }
}

function curveSegment(source: Point, target: Point, sourceDirection: Direction, targetDirection: Direction): Extract<RouteSegment, { type: 'cubic' }> {
  const distanceX = Math.abs(target.x - source.x)
  const handleLength = Math.min(160, distanceX * 0.45)
  return {
    type: 'cubic',
    from: source,
    control1: { x: source.x + sourceDirection * handleLength, y: source.y },
    control2: { x: target.x + targetDirection * handleLength, y: target.y },
    to: target,
  }
}

function cubicPath(segment: Extract<RouteSegment, { type: 'cubic' }>): string {
  return `M ${format(segment.from.x)} ${format(segment.from.y)} C ${format(segment.control1.x)} ${format(segment.control1.y)}, ${format(segment.control2.x)} ${format(segment.control2.y)}, ${format(segment.to.x)} ${format(segment.to.y)}`
}

function pointInRect(point: Point, rect: ExpandedRect): boolean {
  return point.x > rect.x && point.x < rect.x + rect.width
    && point.y > rect.y && point.y < rect.y + rect.height
}

function segmentBounds(segment: RouteSegment): ExpandedRect {
  const points = segment.type === 'line'
    ? [segment.from, segment.to]
    : segment.type === 'quadratic'
      ? [segment.from, segment.control, segment.to]
      : [segment.from, segment.control1, segment.control2, segment.to]
  const left = Math.min(...points.map((point) => point.x))
  const right = Math.max(...points.map((point) => point.x))
  const top = Math.min(...points.map((point) => point.y))
  const bottom = Math.max(...points.map((point) => point.y))
  return { nodeId: '', x: left, y: top, width: right - left, height: bottom - top }
}

function segmentSamples(segment: RouteSegment): Point[] {
  if (segment.type === 'line') {
    const count = Math.max(1, Math.ceil(distance(segment.from, segment.to) / 8))
    return Array.from({ length: count + 1 }, (_, index) => ({
      x: segment.from.x + (segment.to.x - segment.from.x) * index / count,
      y: segment.from.y + (segment.to.y - segment.from.y) * index / count,
    }))
  }
  const length = segment.type === 'quadratic'
    ? distance(segment.from, segment.control) + distance(segment.control, segment.to)
    : distance(segment.from, segment.control1) + distance(segment.control1, segment.control2) + distance(segment.control2, segment.to)
  const count = Math.max(8, Math.ceil(length / 8))
  return Array.from({ length: count + 1 }, (_, index) => {
    const t = index / count
    return segment.type === 'quadratic' ? quadraticPoint(segment, t) : cubicPoint(segment, t)
  })
}

function segmentIsClear(segment: RouteSegment, obstacles: ExpandedRect[]): boolean {
  if (segment.type === 'line' && segment.from.y === segment.to.y) {
    return !obstacles.some((rect) => segment.from.y > rect.y && segment.from.y < rect.y + rect.height
      && Math.min(segment.from.x, segment.to.x) < rect.x + rect.width
      && Math.max(segment.from.x, segment.to.x) > rect.x)
  }
  if (segment.type === 'line' && segment.from.x === segment.to.x) {
    return !obstacles.some((rect) => segment.from.x > rect.x && segment.from.x < rect.x + rect.width
      && Math.min(segment.from.y, segment.to.y) < rect.y + rect.height
      && Math.max(segment.from.y, segment.to.y) > rect.y)
  }

  const bounds = segmentBounds(segment)
  const nearby = obstacles.filter((rect) => rect.x <= bounds.x + bounds.width && rect.x + rect.width >= bounds.x
    && rect.y <= bounds.y + bounds.height && rect.y + rect.height >= bounds.y)
  return !segmentSamples(segment).some((point) => nearby.some((rect) => pointInRect(point, rect)))
}

function segmentsAreClear(segments: RouteSegment[], obstacles: ExpandedRect[]): boolean {
  return segments.every((segment) => segmentIsClear(segment, obstacles))
}

function simplifyPoints(points: Point[]): Point[] {
  const simplified: Point[] = []
  for (const point of points) {
    if (samePoint(point, simplified[simplified.length - 1] ?? { x: Number.NaN, y: Number.NaN })) continue
    while (simplified.length >= 2) {
      const previous = simplified[simplified.length - 2]
      const current = simplified[simplified.length - 1]
      const firstX = current.x - previous.x
      const firstY = current.y - previous.y
      const secondX = point.x - current.x
      const secondY = point.y - current.y
      if (firstX * secondY !== firstY * secondX || firstX * secondX + firstY * secondY < 0) break
      simplified.pop()
    }
    simplified.push(point)
  }
  return simplified
}

function roundedPath(points: Point[]): { path: string; segments: RouteSegment[] } {
  const routePoints = simplifyPoints(points)
  const first = routePoints[0]
  if (!first) return { path: '', segments: [] }

  let path = `M ${format(first.x)} ${format(first.y)}`
  let cursor = first
  const segments: RouteSegment[] = []
  const addLine = (to: Point) => {
    if (samePoint(cursor, to)) return
    segments.push({ type: 'line', from: cursor, to })
    path += ` L ${format(to.x)} ${format(to.y)}`
    cursor = to
  }
  const addQuadratic = (control: Point, to: Point) => {
    if (samePoint(cursor, to)) return
    segments.push({ type: 'quadratic', from: cursor, control, to })
    path += ` Q ${format(control.x)} ${format(control.y)} ${format(to.x)} ${format(to.y)}`
    cursor = to
  }

  for (let index = 1; index < routePoints.length - 1; index += 1) {
    const previous = routePoints[index - 1]
    const corner = routePoints[index]
    const next = routePoints[index + 1]
    const incomingLength = distance(previous, corner)
    const outgoingLength = distance(corner, next)
    const radius = Math.min(CORNER_RADIUS, incomingLength / 2, outgoingLength / 2)
    if (!radius) {
      addLine(corner)
      continue
    }
    const before = {
      x: corner.x + (previous.x - corner.x) / incomingLength * radius,
      y: corner.y + (previous.y - corner.y) / incomingLength * radius,
    }
    const after = {
      x: corner.x + (next.x - corner.x) / outgoingLength * radius,
      y: corner.y + (next.y - corner.y) / outgoingLength * radius,
    }
    addLine(before)
    addQuadratic(corner, after)
  }
  addLine(routePoints[routePoints.length - 1])
  return { path, segments }
}

function laneBounds(
  source: Point,
  target: Point,
  nodeRects: CanvasNodeRect[],
  obstacles: ExpandedRect[],
  sourceNodeId: string,
  targetNodeId: string,
  sourceDirection: Direction,
  targetDirection: Direction,
): { top: number; bottom: number } {
  const maxExit = EXIT_LENGTHS[EXIT_LENGTHS.length - 1]
  const left = Math.min(source.x, source.x + sourceDirection * maxExit, target.x, target.x + targetDirection * maxExit) - ROUTE_GAP
  const right = Math.max(source.x, source.x + sourceDirection * maxExit, target.x, target.x + targetDirection * maxExit) + ROUTE_GAP
  const endpoints = nodeRects
    .filter((rect) => rect.nodeId === sourceNodeId || rect.nodeId === targetNodeId)
    .map(expandRect)
  const nearby = obstacles.filter((rect) => rect.x <= right && rect.x + rect.width >= left)
  const bounds = [...endpoints, ...nearby]
  return {
    top: Math.min(source.y, target.y, ...bounds.map((rect) => rect.y)) - ROUTE_GAP,
    bottom: Math.max(source.y, target.y, ...bounds.map((rect) => rect.y + rect.height)) + ROUTE_GAP,
  }
}

export function routeCanvasEdge(
  source: Point,
  target: Point,
  nodeRects: CanvasNodeRect[],
  sourceNodeId: string,
  targetNodeId: string,
  options: RouteOptions = {},
): RouteResult {
  const sourceDirection = options.sourceDirection ?? 1
  const targetDirection = options.targetDirection ?? -1
  const obstacles = nodeRects
    .filter((rect) => rect.nodeId !== sourceNodeId && rect.nodeId !== targetNodeId)
    .map(expandRect)
  const curve = curveSegment(source, target, sourceDirection, targetDirection)
  if (sourceDirection === 1 && targetDirection === -1 && target.x > source.x
    && segmentsAreClear([curve], obstacles)) {
    return { path: cubicPath(curve), kind: 'curve' }
  }

  const { top, bottom } = laneBounds(
    source,
    target,
    nodeRects,
    obstacles,
    sourceNodeId,
    targetNodeId,
    sourceDirection,
    targetDirection,
  )
  for (const offset of LANE_OFFSETS) {
    for (const laneY of [top - offset, bottom + offset]) {
      for (const exitLength of EXIT_LENGTHS) {
        const sourceExit = { x: source.x + sourceDirection * exitLength, y: source.y }
        const targetEntry = { x: target.x + targetDirection * exitLength, y: target.y }
        const route = roundedPath([
          source,
          sourceExit,
          { x: sourceExit.x, y: laneY },
          { x: targetEntry.x, y: laneY },
          targetEntry,
          target,
        ])
        if (segmentsAreClear(route.segments, obstacles)) return { path: route.path, kind: 'detour' }
      }
    }
  }

  return { path: cubicPath(curve), kind: 'fallback' }
}
