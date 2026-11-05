import { useLayoutEffect } from 'react'
import { useReactFlow, useStoreApi, useUpdateNodeInternals } from '@xyflow/react'
import type { ReactFlowInstance } from '@xyflow/react'
import type { CanvasFlowNode } from './flowModel'

function scheduleFrame(callback: FrameRequestCallback): number {
  if (typeof window.requestAnimationFrame === 'function') return window.requestAnimationFrame(callback)
  return window.setTimeout(() => callback(Date.now()), 0)
}

function cancelFrame(handle: number): void {
  if (typeof window.cancelAnimationFrame === 'function') window.cancelAnimationFrame(handle)
  else window.clearTimeout(handle)
}

function sameIds(left: Set<string>, right: Set<string>): boolean {
  return left.size === right.size && [...left].every((id) => right.has(id))
}

type RequiredHandle = { id: string; type: 'source' | 'target' }

function measuredHandleKeys(nodeId: string, flow: ReactFlowInstance<CanvasFlowNode>): Set<string> | null {
  const node = flow.getInternalNode(nodeId)
  const width = node?.measured.width
  const height = node?.measured.height
  const bounds = node?.internals.handleBounds
  if (!node || !bounds || typeof width !== 'number' || width <= 0 || typeof height !== 'number' || height <= 0) return null

  const wrapper = [...document.querySelectorAll<HTMLElement>('.react-flow__node[data-id]')]
    .find((element) => element.dataset.id === nodeId)
  if (!wrapper) return null

  const actual = new Set([...wrapper.querySelectorAll<HTMLElement>('.react-flow__handle')]
    .map((handle) => `${handle.classList.contains('source') ? 'source' : 'target'}:${handle.dataset.handleid ?? ''}`))
  const measured = new Set([
    ...(bounds.source ?? []).map((handle) => `source:${handle.id ?? ''}`),
    ...(bounds.target ?? []).map((handle) => `target:${handle.id ?? ''}`),
  ])
  return sameIds(actual, measured) ? measured : null
}

export function useMeasuredNodeHandles(
  nodeId: string,
  layoutKey: string,
  projectId: string,
  requiredHandles: RequiredHandle[],
  onMeasured?: (nodeId: string, layoutKey: string, projectId: string) => void,
): void {
  const updateNodeInternals = useUpdateNodeInternals()
  const flow = useReactFlow<CanvasFlowNode>()
  const store = useStoreApi<CanvasFlowNode>()
  const requiredHandleKey = JSON.stringify(requiredHandles)

  useLayoutEffect(() => {
    if (!onMeasured || !projectId) return undefined

    const expected = JSON.parse(requiredHandleKey) as RequiredHandle[]
    let ready = false
    let reported = false
    let checkFrame = 0
    let currentUnsubscribe: () => void = () => undefined
    const check = () => {
      const actual = measuredHandleKeys(nodeId, flow)
      if (!ready || !actual || !expected.every(({ id, type }) => actual.has(`${type}:${id}`)) || reported) return
      reported = true
      currentUnsubscribe()
      onMeasured(nodeId, layoutKey, projectId)
    }

    currentUnsubscribe = store.subscribe(check)
    updateNodeInternals(nodeId)
    // updateNodeInternals queues React Flow's forced DOM measurement first. Check
    // on the next frame, and subscribe for later ResizeObserver size changes.
    checkFrame = scheduleFrame(() => {
      ready = true
      check()
    })

    return () => {
      cancelFrame(checkFrame)
      currentUnsubscribe()
    }
  }, [flow, layoutKey, nodeId, onMeasured, projectId, requiredHandleKey, store, updateNodeInternals])
}
