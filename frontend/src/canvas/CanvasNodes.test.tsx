import { vi, describe, expect, it } from 'vitest'
import type { ReactElement } from 'react'
import type { HandleProps, NodeProps, NodeResizerProps } from '@xyflow/react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { Node } from '../../bindings/audio-ui/internal/domain/entity'
import { NodeType } from '../../bindings/audio-ui/internal/domain/valueobject'
import type { CanvasFlowNode } from './flowModel'
import { AgentNode } from './AgentNode'
import { CandidateNode } from './CandidateNode'
import { PromptNode } from './PromptNode'
import { ScriptNode } from './ScriptNode'
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
    NodeResizer: (props: NodeResizerProps) => React.createElement('div', {
      'data-testid': `resize-${props.nodeId}`,
      'data-visible': String(props.isVisible),
      'data-min-width': props.minWidth,
      'data-min-height': props.minHeight,
      'data-max-width': props.maxWidth,
      'data-max-height': props.maxHeight,
    }),
    Position: { Left: 'left', Right: 'right', Top: 'top', Bottom: 'bottom' },
    useReactFlow: () => ({ getInternalNode: vi.fn() }),
    useStoreApi: () => ({ subscribe: () => () => undefined }),
    useUpdateNodeInternals: () => vi.fn(),
  }
})

function node(id: string, type: string, parent = '', overrides: Partial<Node> = {}): Node {
  return {
    id,
    type: type as Node['type'],
    title: id,
    parent,
    position: { x: 0, y: 0 },
    size: { width: 280, height: 230 },
    params: {},
    rounds: [],
    locked: false,
    stale: false,
    ...overrides,
  }
}

function nodeData(nodeValue: Node, overrides: Partial<CanvasFlowNode['data']> = {}): CanvasFlowNode['data'] {
  return {
    node: nodeValue,
    runningLocked: false,
    pathHighlighted: false,
    topicCandidateIndexes: [],
    candidates: [],
    mutationsDisabled: false,
    onAddChild: vi.fn(),
    ...overrides,
  }
}

function nodeProps(nodeValue: Node, data = nodeData(nodeValue), selected = false): NodeProps<CanvasFlowNode> {
  return {
    id: nodeValue.id,
    type: nodeValue.type,
    data,
    dragging: false,
    zIndex: 0,
    selectable: true,
    deletable: false,
    selected,
    draggable: true,
    isConnectable: true,
    positionAbsoluteX: 0,
    positionAbsoluteY: 0,
  } as NodeProps<CanvasFlowNode>
}

type CanvasNodeView = {
  label: string
  type: Node['type']
  defaultSize: { width: number; height: number }
  renderNode: (nodeValue: Node, data: CanvasFlowNode['data'], selected: boolean) => ReactElement
}

const canvasNodeViews: CanvasNodeView[] = [
  { label: '选题', type: NodeType.NodeTypeTopic, defaultSize: { width: 280, height: 250 }, renderNode: (value, data, selected) => <TopicNode {...nodeProps(value, data, selected)} /> },
  { label: '候选', type: NodeType.NodeTypeCandidate, defaultSize: { width: 235, height: 160 }, renderNode: (value, data, selected) => <CandidateNode {...nodeProps(value, data, selected)} /> },
  { label: '提示词', type: NodeType.NodeTypePrompt, defaultSize: { width: 280, height: 230 }, renderNode: (value, data, selected) => <PromptNode {...nodeProps(value, data, selected)} /> },
  { label: 'Agent', type: NodeType.NodeTypeAgent, defaultSize: { width: 235, height: 165 }, renderNode: (value, data, selected) => <AgentNode {...nodeProps(value, data, selected)} /> },
  { label: '脚本', type: NodeType.NodeTypeScript, defaultSize: { width: 270, height: 205 }, renderNode: (value, data, selected) => <ScriptNode {...nodeProps(value, data, selected)} /> },
]

describe('M2-12A canvas nodes', () => {
  it.each(canvasNodeViews)('adds bounded resize and keyboard size controls to $label nodes', ({ type, defaultSize, renderNode }) => {
    const nodeValue = node('n_size', type, '', {
      size: { width: defaultSize.width + 20, height: defaultSize.height + 20 },
    })
    const onSetNodeLayout = vi.fn()
    render(renderNode(nodeValue, nodeData(nodeValue, { onSetNodeLayout }), true))

    expect(screen.getByTestId('resize-n_size')).toHaveAttribute('data-visible', 'true')
    expect(screen.getByTestId('resize-n_size')).toHaveAttribute('data-min-width', '200')
    expect(screen.getByTestId('resize-n_size')).toHaveAttribute('data-min-height', '140')
    expect(screen.getByTestId('resize-n_size')).toHaveAttribute('data-max-width', '1200')
    expect(screen.getByTestId('resize-n_size')).toHaveAttribute('data-max-height', '900')
    fireEvent.click(screen.getByRole('button', { name: `调整 n_size 卡片尺寸` }))
    fireEvent.click(screen.getByRole('button', { name: '恢复默认尺寸' }))

    expect(onSetNodeLayout).toHaveBeenCalledWith('n_size', defaultSize.width, defaultSize.height)
  })

  it('applies numeric size changes and returns focus to the card control', () => {
    const prompt = node('n_prompt', 'prompt')
    const onSetNodeLayout = vi.fn()
    render(<PromptNode {...nodeProps(prompt, nodeData(prompt, { onSetNodeLayout }), true)} />)
    const trigger = screen.getByRole('button', { name: '调整 n_prompt 卡片尺寸' })

    fireEvent.click(trigger)
    fireEvent.change(screen.getByRole('spinbutton', { name: '宽度' }), { target: { value: '420' } })
    fireEvent.change(screen.getByRole('spinbutton', { name: '高度' }), { target: { value: '360' } })
    fireEvent.click(screen.getByRole('button', { name: '应用' }))

    expect(onSetNodeLayout).toHaveBeenCalledWith('n_prompt', 420, 360)
    expect(trigger).toHaveFocus()
  })

  it('keeps layout controls available on a running-locked path', () => {
    const prompt = node('n_locked_prompt', 'prompt', 'n_candidate')
    render(<PromptNode {...nodeProps(prompt, nodeData(prompt, {
      runningLocked: true,
      onSetNodeLayout: vi.fn(),
    }), true)} />)

    expect(screen.getByTestId('resize-n_locked_prompt')).toHaveAttribute('data-visible', 'true')
    expect(screen.getByRole('button', { name: '调整 n_locked_prompt 卡片尺寸' })).toBeEnabled()
  })

  it('renders application-generated candidate links without exposing topic output ports for manual connection', () => {
    const topic = node('n_topic', 'topic')
    const candidate = node('n_candidate', 'candidate', topic.id, {
      candidateSource: { topicNodeId: topic.id, artifactId: 'sha256:topics', index: 1 },
    })
    const { container } = render(
      <>
        <TopicNode {...nodeProps(topic, nodeData(topic, {
          topicCandidateIndexes: [1],
          candidates: [{ index: 1, title: '谱面演示', angle: '转场', hook: '先看谱面' }],
        }))} />
        <CandidateNode {...nodeProps(candidate, nodeData(candidate, {
          candidate: { index: 1, title: '谱面演示', angle: '转场', hook: '先看谱面' },
        }))} />
      </>,
    )

    expect(screen.getAllByText('谱面演示')).toHaveLength(2)
    expect(screen.getByText('转场')).toBeInTheDocument()
    expect(screen.getByLabelText('谱面演示 输出端口')).toHaveAttribute('data-handle', 'candidate-output')
    expect(screen.getByLabelText('谱面演示 输出端口')).toHaveAttribute('data-connectable', 'true')
    expect(screen.getByLabelText('n_topic 的候选节点')).toBeInTheDocument()
    expect(container.querySelector('.topic-candidate-index')).toHaveTextContent('2')
    expect(screen.getByLabelText('从 谱面演示 新增提示词')).toBeEnabled()
  })

  it('keeps a draft prompt editable as a connection target and does not show a run button', () => {
    const prompt = node('n_draft', 'prompt')
    render(<PromptNode {...nodeProps(prompt)} />)

    expect(screen.getByLabelText('连接到 n_draft')).toHaveAttribute('data-handle', 'prompt-input')
    expect(screen.queryByLabelText('n_draft 输出端口')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /运行/ })).not.toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: '任务' })).toBeInTheDocument()
  })

  it('shows connected prompt parameters and exposes Agent creation instead of execution', () => {
    const prompt = node('n_prompt', 'prompt', 'n_candidate')
    const onAddChild = vi.fn()
    render(<PromptNode {...nodeProps(prompt, nodeData(prompt, { onAddChild }))} />)

    expect(screen.getByLabelText('连接到 n_prompt')).toHaveAttribute('data-handle', 'prompt-input')
    expect(screen.getByLabelText('n_prompt 输出端口')).toHaveAttribute('data-handle', 'prompt-output')
    fireEvent.click(screen.getByRole('button', { name: '从 n_prompt 新增 Agent' }))
    expect(onAddChild).toHaveBeenCalledWith('n_prompt', 'agent')
  })

  it('starts local execution only from an Agent node and offers an empty Script binding', async () => {
    const agent = node('n_agent', 'agent', 'n_prompt')
    const onAddChild = vi.fn()
    const onStartRun = vi.fn().mockResolvedValue({})
    render(<AgentNode {...nodeProps(agent, nodeData(agent, { onAddChild, onStartRun, projectId: 'p_test' }))} />)

    expect(screen.getByText('未指定输出 · 合法结果自动创建 Script')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '为 n_agent 指定空 Script 输出' }))
    expect(onAddChild).toHaveBeenCalledWith('n_agent', 'script')
    fireEvent.click(screen.getByRole('button', { name: '运行本地 Agent' }))
    await waitFor(() => expect(onStartRun).toHaveBeenCalledWith('n_agent', 'p_test'))
  })

  it('surfaces a backend BUSY error on the Agent node', async () => {
    const agent = node('n_agent', 'agent', 'n_prompt')
    const onStartRun = vi.fn().mockResolvedValue({ error: { code: 'BUSY', message: 'run already active' } })
    render(<AgentNode {...nodeProps(agent, nodeData(agent, { onStartRun, projectId: 'p_test' }))} />)

    fireEvent.click(screen.getByRole('button', { name: '运行本地 Agent' }))
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('已有运行正在进行'))
  })

  it('blocks Agent execution and output changes for a locked path or global BUSY state', () => {
    const lockedAgent = node('n_locked_agent', 'agent', 'n_prompt')
    const busyAgent = node('n_busy_agent', 'agent', 'n_prompt')
    const onStartRun = vi.fn().mockResolvedValue({})
    render(
      <>
        <AgentNode {...nodeProps(lockedAgent, nodeData(lockedAgent, { runningLocked: true, onStartRun, projectId: 'p_test' }))} />
        <AgentNode {...nodeProps(busyAgent, nodeData(busyAgent, { globalRunBusy: true, runStatus: 'running', onStartRun, projectId: 'p_test' }))} />
      </>,
    )

    expect(screen.getByRole('button', { name: '运行本地 Agent' })).toBeDisabled()
    expect(screen.getByRole('button', { name: '为 n_locked_agent 指定空 Script 输出' })).toBeDisabled()
    expect(screen.getByRole('button', { name: '已有运行' })).toBeDisabled()
    expect(screen.getByRole('button', { name: '为 n_busy_agent 指定空 Script 输出' })).toBeDisabled()
    expect(onStartRun).not.toHaveBeenCalled()
  })

  it('renders immutable Script version history and pending output status', () => {
    const script = node('n_script', 'script', 'n_agent', {
      scriptVersions: [
        { n: 1, artifactId: 'sha256:script-v1', runId: 'r_one' },
        { n: 2, artifactId: 'sha256:script-v2', runId: 'r_two' },
      ],
    })
    const onAddChild = vi.fn()
    render(<ScriptNode {...nodeProps(script, nodeData(script, { onAddChild, pendingOutputCount: 1 }))} />)

    expect(screen.getByText('2 个不可变版本 · 最新 v2')).toBeInTheDocument()
    expect(screen.getAllByText('r_two')).toHaveLength(2)
    expect(screen.getByText('1 个结果待处理，可在运行控制台查看。')).toBeInTheDocument()
    fireEvent.click(screen.getByText('版本历史 · 2'))
    expect(screen.getByText('r_one')).toBeInTheDocument()
    expect(screen.getByText('sha256:script-v1')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '从 n_script 新增提示词' }))
    expect(onAddChild).toHaveBeenCalledWith('n_script', 'prompt')
  })

  it('disables path operations while allowing content-locked topic topology to remain separate', () => {
    const topic = node('n_topic', 'topic', '', { locked: true })
    const candidate = node('n_candidate', 'candidate', topic.id, {
      candidateSource: { topicNodeId: topic.id, artifactId: 'sha256:topics', index: 0 },
    })
    render(
      <>
        <TopicNode {...nodeProps(topic, nodeData(topic, { topicCandidateIndexes: [0], candidates: [{ index: 0, title: '候选甲' }] }))} />
        <CandidateNode {...nodeProps(candidate, nodeData(candidate, { runningLocked: true, candidates: [], candidate: { index: 0, title: '候选甲' } }))} />
      </>,
    )

    expect(screen.getByRole('button', { name: '已锁定' })).toBeDisabled()
    expect(screen.getByRole('button', { name: '从 候选甲 新增提示词' })).toBeDisabled()
    expect(screen.getByLabelText('候选甲 输出端口')).toHaveAttribute('data-connectable', 'false')
  })
})
