import { vi, describe, expect, it } from 'vitest'
import type { HandleProps, NodeProps } from '@xyflow/react'
import { render, screen } from '@testing-library/react'
import type { Node } from '../../bindings/audio-ui/internal/domain/entity'
import type { CanvasFlowNode } from './flowModel'
import { PromptNode } from './PromptNode'
import { TopicNode } from './TopicNode'

vi.mock('@xyflow/react', async () => {
  const React = await import('react')
  return {
    Handle: (props: HandleProps) => React.createElement('span', {
      'data-handle': props.id ?? '',
      'data-kind': props.type,
      'data-connectable': String(props.isConnectable),
      'aria-label': props['aria-label'],
      'aria-hidden': props['aria-hidden'],
    }),
    Position: { Right: 'right', Top: 'top', Bottom: 'bottom' },
    useReactFlow: () => ({ getInternalNode: vi.fn() }),
    useStoreApi: () => ({ subscribe: () => () => undefined }),
    useUpdateNodeInternals: () => vi.fn(),
  }
})

function node(id: string, type: 'topic' | 'prompt', parent = ''): Node {
  return {
    id,
    type: type as Node['type'],
    title: id,
    parent,
    position: { x: 0, y: 0 },
    params: {},
    rounds: [],
    locked: false,
    stale: false,
  }
}

function nodeProps(id: string, type: 'topic' | 'prompt', data: CanvasFlowNode['data']): NodeProps<CanvasFlowNode> {
  return {
    id,
    type,
    data,
    dragging: false,
    zIndex: 0,
    selectable: true,
    deletable: false,
    selected: false,
    draggable: true,
    isConnectable: true,
    positionAbsoluteX: 0,
    positionAbsoluteY: 0,
  } as NodeProps<CanvasFlowNode>
}

describe('canvas node handles', () => {
  it('renders one distinct candidate output handle for every available topic candidate', () => {
    const topic = node('n_topic', 'topic')
    const props = nodeProps(topic.id, 'topic', {
      node: topic,
      runningLocked: false,
      pathHighlighted: false,
      hasUnmappedChildren: false,
      mutationsDisabled: false,
      candidates: [{ title: '甲' }, { title: '乙' }],
      onAddChild: vi.fn(),
    })
    render(<TopicNode {...props} />)

    expect(screen.getByLabelText('候选 1：甲 输出端口')).toHaveAttribute('data-handle', 'candidate-0')
    expect(screen.getByLabelText('候选 2：乙 输出端口')).toHaveAttribute('data-handle', 'candidate-1')
  })

  it('keeps a draft prompt as a target without exposing an output port', () => {
    const prompt = node('n_draft', 'prompt')
    render(<PromptNode {...nodeProps(prompt.id, 'prompt', {
      node: prompt,
      runningLocked: false,
      pathHighlighted: false,
      hasUnmappedChildren: false,
      mutationsDisabled: false,
      candidates: [],
      onAddChild: vi.fn(),
    })} />)

    expect(screen.getByLabelText('连接到 n_draft')).toHaveAttribute('data-handle', 'prompt-input')
    expect(screen.queryByLabelText('n_draft 输出端口')).not.toBeInTheDocument()
  })

  it('renders input and output handles for a connected prompt', () => {
    const prompt = node('n_prompt', 'prompt', 'n_topic')
    render(<PromptNode {...nodeProps(prompt.id, 'prompt', {
      node: prompt,
      runningLocked: false,
      pathHighlighted: false,
      hasUnmappedChildren: false,
      mutationsDisabled: false,
      candidates: [],
      onAddChild: vi.fn(),
    })} />)

    expect(screen.getByLabelText('连接到 n_prompt')).toHaveAttribute('data-handle', 'prompt-input')
    expect(screen.getByLabelText('n_prompt 输出端口')).toHaveAttribute('data-handle', 'prompt-output')
  })

  it('keeps topology available for content-locked nodes but disables operations on a run-locked path', () => {
    const contentLockedTopic = { ...node('n_topic', 'topic'), locked: true }
    const runLockedPrompt = node('n_running', 'prompt', 'n_topic')
    render(
      <>
        <TopicNode {...nodeProps(contentLockedTopic.id, 'topic', {
          node: contentLockedTopic,
          runningLocked: false,
          pathHighlighted: false,
          hasUnmappedChildren: false,
      mutationsDisabled: false,
          candidates: [{ title: '候选甲' }],
          onAddChild: vi.fn(),
        })} />
        <PromptNode {...nodeProps(runLockedPrompt.id, 'prompt', {
          node: runLockedPrompt,
          runningLocked: true,
          pathHighlighted: true,
          hasUnmappedChildren: false,
      mutationsDisabled: false,
          candidates: [],
          onAddChild: vi.fn(),
        })} />
      </>,
    )

    expect(screen.getByRole('button', { name: '从候选 1 新增提示词' })).toBeEnabled()
    expect(screen.getByLabelText('候选 1：候选甲 输出端口')).toHaveAttribute('data-connectable', 'true')
    expect(screen.getByRole('button', { name: '从 n_running 新增提示词' })).toBeDisabled()
    expect(screen.getByLabelText('连接到 n_running')).toHaveAttribute('data-connectable', 'false')
  })
})
