import { vi, describe, expect, it } from 'vitest'
import type { HandleProps, NodeProps } from '@xyflow/react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { Node } from '../../bindings/audio-ui/internal/domain/entity'
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

function nodeProps(nodeValue: Node, data = nodeData(nodeValue)): NodeProps<CanvasFlowNode> {
  return {
    id: nodeValue.id,
    type: nodeValue.type,
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

describe('M2-12A canvas nodes', () => {
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

  it('renders the immutable Script version and allows creating a downstream prompt', () => {
    const script = node('n_script', 'script', 'n_agent', {
      scriptVersions: [{ n: 1, artifactId: 'sha256:script-v1', runId: 'r_one' }],
    })
    const onAddChild = vi.fn()
    render(<ScriptNode {...nodeProps(script, nodeData(script, { onAddChild }))} />)

    expect(screen.getByText('1 个不可变版本 · 最新 v1')).toBeInTheDocument()
    expect(screen.getByText('r_one')).toBeInTheDocument()
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
