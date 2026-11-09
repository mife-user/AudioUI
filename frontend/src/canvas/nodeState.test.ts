import { describe, expect, it } from 'vitest'
import type { Node } from '../../bindings/audio-ui/internal/domain/entity'
import type { CanvasFlowNode } from './flowModel'
import { preserveCanvasNodeViewState } from './nodeState'

function graphNode(id: string, type: 'topic' | 'prompt', title = id): Node {
  return {
    id,
    type: type as Node['type'],
    title,
    parent: '',
    position: { x: 10, y: 20 },
    size: { width: 280, height: 230 },
    params: {},
    rounds: [],
    locked: false,
    stale: false,
  }
}

function flowNode(
  id: string,
  type: 'topic' | 'prompt',
  projectId: string,
  overrides: Partial<CanvasFlowNode> = {},
): CanvasFlowNode {
  return {
    id,
    type,
    position: { x: 10, y: 20 },
    style: { width: 280, height: 230 },
    data: {
      node: graphNode(id, type),
      runningLocked: false,
      pathHighlighted: false,
      topicCandidateIndexes: [],
      candidates: [],
      mutationsDisabled: false,
      projectId,
      onAddChild: () => undefined,
    },
    ...overrides,
  }
}

describe('preserveCanvasNodeViewState', () => {
  it('carries measured and in-progress interaction state for the same project node', () => {
    const previous = flowNode('n_prompt', 'prompt', 'p_one', {
      measured: { width: 276, height: 132 },
      dragging: true,
      resizing: false,
    })
    const next = flowNode('n_prompt', 'prompt', 'p_one', {
      position: { x: 40, y: 60 },
      selected: true,
      data: { ...flowNode('n_prompt', 'prompt', 'p_one').data, pathHighlighted: true },
    })

    const [result] = preserveCanvasNodeViewState([next], [previous], 'p_one')

    expect(result).toMatchObject({
      measured: { width: 276, height: 132 },
      dragging: true,
      resizing: false,
      position: { x: 40, y: 60 },
      selected: true,
      data: { pathHighlighted: true },
    })
    expect(result.data).toBe(next.data)
    expect(result).not.toHaveProperty('internals')
  })

  it('reuses unchanged node objects when only the containing snapshot was refreshed', () => {
    const previous = flowNode('n_prompt', 'prompt', 'p_one', {
      measured: { width: 276, height: 132 },
      dragging: false,
      resizing: false,
    })
    const next = flowNode('n_prompt', 'prompt', 'p_one', {
      measured: { width: 276, height: 132 },
      dragging: false,
      resizing: false,
      data: { ...previous.data },
    })

    expect(preserveCanvasNodeViewState([next], [previous], 'p_one')[0]).toBe(previous)
  })

  it('does not reuse stale measurements when the persisted node size changes', () => {
    const previous = flowNode('n_prompt', 'prompt', 'p_one', {
      measured: { width: 280, height: 230 },
    })
    const next = flowNode('n_prompt', 'prompt', 'p_one', {
      style: { width: 340, height: 280 },
      data: { ...flowNode('n_prompt', 'prompt', 'p_one').data },
    })

    const [result] = preserveCanvasNodeViewState([next], [previous], 'p_one')

    expect(result.style).toEqual({ width: 340, height: 280 })
    expect(result.measured).toBeUndefined()
  })

  it('does not carry measurements across projects, node ids, or node types', () => {
    const previous = flowNode('n_prompt', 'prompt', 'p_one', { measured: { width: 276, height: 132 } })
    const next = [
      flowNode('n_prompt', 'prompt', 'p_two'),
      flowNode('n_other', 'prompt', 'p_one'),
      flowNode('n_prompt', 'topic', 'p_one'),
    ]

    expect(preserveCanvasNodeViewState(next, [previous], 'p_two').map((node) => node.measured)).toEqual([undefined, undefined, undefined])
    expect(preserveCanvasNodeViewState(next, [previous], 'p_one').map((node) => node.measured)).toEqual([undefined, undefined, undefined])
  })

  it('does not preserve incomplete or zero-size measurements as initialized state', () => {
    const previousNodes = [
      flowNode('n_width_only', 'prompt', 'p_one', { measured: { width: 276 } }),
      flowNode('n_zero', 'prompt', 'p_one', { measured: { width: 0, height: 132 } }),
    ]
    const nextNodes = [flowNode('n_width_only', 'prompt', 'p_one'), flowNode('n_zero', 'prompt', 'p_one')]

    expect(preserveCanvasNodeViewState(nextNodes, previousNodes, 'p_one').map((node) => node.measured)).toEqual([undefined, undefined])
  })
})
