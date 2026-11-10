import { render } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ConnectionLineComponentProps, Position } from '@xyflow/react'
import type { CanvasFlowNode } from './flowModel'

const flowMocks = vi.hoisted(() => ({ getNodes: vi.fn((): unknown[] => []) }))

vi.mock('@xyflow/react', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@xyflow/react')>()
  return { ...actual, useReactFlow: () => ({ getNodes: flowMocks.getNodes }) }
})

import { CanvasConnectionLine } from './CanvasConnectionLine'

function makeFlowNode(id: string, x: number, y: number, width: number, height: number): CanvasFlowNode {
  return {
    id,
    type: 'agent',
    position: { x, y },
    style: { width, height },
    data: { node: { size: { width, height } } },
  } as CanvasFlowNode
}

describe('CanvasConnectionLine', () => {
  afterEach(() => vi.clearAllMocks())

  it('uses the shared obstacle route and keeps invalid status visible', () => {
    flowMocks.getNodes.mockReturnValue([
      makeFlowNode('source', 100, 50, 200, 100),
      makeFlowNode('target', 100, 120, 200, 100),
      makeFlowNode('blocker', 180, 40, 100, 180),
    ])
    const props = {
      connectionLineStyle: { strokeDasharray: '5 4' },
      connectionLineType: 'bezier',
      fromNode: { id: 'source' },
      fromHandle: {},
      fromX: 300,
      fromY: 100,
      fromPosition: 'right' as Position,
      toX: 100,
      toY: 160,
      toPosition: 'left' as Position,
      connectionStatus: 'invalid',
      toNode: { id: 'target' },
      toHandle: null,
      pointer: { x: 100, y: 160 },
    } as unknown as ConnectionLineComponentProps<CanvasFlowNode>

    const { container } = render(<svg><CanvasConnectionLine {...props} /></svg>)
    const path = container.querySelector('path')

    expect(path).toHaveAttribute('d', expect.stringContaining(' Q '))
    expect(path).toHaveClass('canvas-connection-invalid')
    expect(path).toHaveStyle({ strokeDasharray: '5 4' })
  })
})
