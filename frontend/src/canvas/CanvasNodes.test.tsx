import { vi, describe, expect, it } from 'vitest'
import { useMemo, type ReactElement } from 'react'
import type { HandleProps, NodeProps, NodeResizerProps } from '@xyflow/react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { Node, PendingDetail, PendingSummary, ScriptVersionDetail } from '../../bindings/audio-ui/internal/domain/entity'
import { NodeType, OutputKind, PendingStatus } from '../../bindings/audio-ui/internal/domain/valueobject'
import { PendingArtifactCard } from '../ui/PendingArtifactCard'
import { PendingControllerProvider, usePendingController } from '../hooks/usePendingController'
import type { CanvasFlowNode, RunInputOption } from './flowModel'
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
      'data-line-border-color': props.lineStyle?.borderColor,
    }),
    Position: { Left: 'left', Right: 'right', Top: 'top', Bottom: 'bottom' },
    MarkerType: { ArrowClosed: 'arrowclosed' },
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
    hasInputs: Boolean(nodeValue.parent),
    topicCandidates: [],
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

type PendingScriptHarnessProps = {
  script: Node
  pending: PendingSummary
  onGetPending: (id: string, projectId: string) => Promise<PendingDetail>
  onApplyPending: (id: string, force: boolean, projectId: string) => Promise<void>
  onDiscardPending: (id: string, projectId: string) => Promise<void>
  onRefreshWorkspace: () => Promise<void>
}

function PendingScriptHarness(props: PendingScriptHarnessProps) {
  const nodes = useMemo(() => [props.script], [props.script])
  const pending = useMemo(() => [props.pending], [props.pending])
  const controller = usePendingController({
    projectId: 'p_demo',
    nodes,
    pending,
    onGetPending: props.onGetPending,
    onApplyPending: props.onApplyPending,
    onDiscardPending: props.onDiscardPending,
    onRefreshWorkspace: props.onRefreshWorkspace,
  })
  const data = nodeData(props.script, {
    projectId: 'p_demo',
    pendingOutputCount: 1,
    pendingOutputIds: [props.pending.id],
  })
  return (
    <PendingControllerProvider controller={controller}>
      <ScriptNode {...nodeProps(props.script, data)} />
    </PendingControllerProvider>
  )
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
    expect(screen.getByTestId('resize-n_size')).toHaveAttribute('data-line-border-color', 'transparent')
    expect(screen.getByTestId('resize-n_size')).toHaveAttribute('data-min-width', '200')
    expect(screen.getByTestId('resize-n_size')).toHaveAttribute('data-min-height', '140')
    expect(screen.getByTestId('resize-n_size')).toHaveAttribute('data-max-width', '1200')
    expect(screen.getByTestId('resize-n_size')).toHaveAttribute('data-max-height', '900')
    fireEvent.click(screen.getByRole('button', { name: `调整 n_size 卡片尺寸` }))
    fireEvent.click(screen.getByRole('button', { name: '恢复默认尺寸' }))

    expect(onSetNodeLayout).toHaveBeenCalledWith('n_size', defaultSize.width, defaultSize.height)
  })

  it.each(canvasNodeViews)('collapses the body but preserves the header and expanded size for $label nodes', ({ type, renderNode }) => {
    const nodeValue = node('n_fold', type)
    const onSetNodeCollapsed = vi.fn()
    const { container, rerender } = render(renderNode(nodeValue, nodeData(nodeValue, { onSetNodeCollapsed }), true))

    expect(container.querySelector('.canvas-node-content')).not.toHaveAttribute('hidden')
    fireEvent.click(screen.getByRole('button', { name: '折叠 n_fold 正文' }))
    expect(onSetNodeCollapsed).toHaveBeenCalledWith('n_fold', true)

    const collapsedNode = { ...nodeValue, collapsed: true }
    rerender(renderNode(collapsedNode, nodeData(collapsedNode, { onSetNodeCollapsed }), true))
    expect(screen.getByRole('button', { name: '展开 n_fold 正文' })).toHaveAttribute('aria-expanded', 'false')
    expect(container.querySelector('.canvas-node-content')).toHaveAttribute('hidden')
    expect(screen.queryByRole('button', { name: '调整 n_fold 卡片尺寸' })).not.toBeInTheDocument()
    expect(screen.getByTestId('resize-n_fold')).toHaveAttribute('data-visible', 'false')
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

  it('renders protected candidate anchors alongside manual Topic and Candidate ports', () => {
    const topic = node('n_topic', 'topic')
    const candidate = node('n_candidate', 'candidate', topic.id, {
      candidateSource: { topicNodeId: topic.id, round: 1, artifactId: 'sha256:topics', index: 1, runId: 'r_topics' },
    })
    const onLocateNode = vi.fn()
    const { container } = render(
      <>
        <TopicNode {...nodeProps(topic, nodeData(topic, {
          topicCandidates: [{ nodeId: candidate.id, round: 1, artifactId: 'sha256:topics', index: 1, runId: 'r_topics' }],
          candidates: [{ nodeId: candidate.id, round: 1, runId: 'r_topics', index: 1, title: '谱面演示', angle: '转场', hook: '先看谱面' }],
          onLocateNode,
        }))} />
        <CandidateNode {...nodeProps(candidate, nodeData(candidate, {
          candidate: { nodeId: candidate.id, round: 1, runId: 'r_topics', index: 1, title: '谱面演示', angle: '转场', hook: '先看谱面' },
        }))} />
      </>,
    )

    expect(screen.getAllByText('谱面演示')).toHaveLength(2)
    expect(screen.getByText('转场')).toBeInTheDocument()
    expect(screen.getByLabelText('谱面演示 输出端口')).toHaveAttribute('data-handle', 'candidate-output')
    expect(screen.getByLabelText('谱面演示 输出端口')).toHaveAttribute('data-connectable', 'true')
    expect(screen.getByLabelText('n_topic 输出端口')).toHaveAttribute('data-connectable', 'true')
    expect(screen.getByLabelText('连接到 n_topic')).toHaveAttribute('data-connectable', 'true')
    expect(screen.getByLabelText('连接到 谱面演示')).toHaveAttribute('data-connectable', 'true')
    expect(screen.getByLabelText('n_topic 的候选节点')).toBeInTheDocument()
    expect(container.querySelector('.topic-candidate-index')).toHaveTextContent('2')
    expect(container.querySelector('.canvas-node-head [data-handle="topic-candidate-n_candidate"]')).toBeInTheDocument()
    expect(container.querySelector('.topic-candidate-row [data-handle="topic-candidate-n_candidate"]')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: '定位第 1 批候选 2' }))
    expect(onLocateNode).toHaveBeenCalledWith(candidate.id)
    expect(screen.getByLabelText('从 谱面演示 新增提示词')).toBeEnabled()
  })

  it('keeps all candidate anchors on the Topic header for a ten-candidate list', () => {
    const topic = node('n_topic', 'topic', '', { size: { width: 200, height: 140 } })
    const indexes = Array.from({ length: 10 }, (_value, index) => index)
    const topicCandidates = indexes.map((index) => ({
      nodeId: `n_candidate_${index}`, round: 1, artifactId: 'sha256:topics', index, runId: 'r_topics',
    }))
    const candidates = topicCandidates.map((source) => ({ ...source, title: `候选标题 ${source.index + 1}` }))
    const { container } = render(<TopicNode {...nodeProps(topic, nodeData(topic, {
      topicCandidates,
      candidates,
    }))} />)

    expect(screen.getByText('候选 10')).toBeInTheDocument()
    expect(container.querySelectorAll('.canvas-node-head [data-kind="source"]')).toHaveLength(10)
    expect(container.querySelectorAll('.topic-candidate-row')).toHaveLength(10)
    expect(container.querySelectorAll('.topic-candidate-row [data-kind="source"]')).toHaveLength(0)
  })

  it('keeps a draft prompt editable as a connection target and does not show a run button', () => {
    const prompt = node('n_draft', 'prompt')
    render(<PromptNode {...nodeProps(prompt)} />)

    expect(screen.getByLabelText('连接到 n_draft')).toHaveAttribute('data-handle', 'prompt-input')
    expect(screen.getByLabelText('n_draft 输出端口')).toHaveAttribute('data-handle', 'prompt-output')
    expect(screen.getByRole('button', { name: '从 n_draft 新增 Agent' })).toBeEnabled()
    expect(screen.queryByRole('button', { name: /运行/ })).not.toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: '提示词内容' })).toBeInTheDocument()
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

  it('starts local execution after confirming the default direct inputs', async () => {
    const agent = node('n_agent', 'agent', 'n_prompt')
    const onStartRun = vi.fn().mockResolvedValue({})
    const runInputs: RunInputOption[] = [{
      edgeId: 'e_prompt_agent', sourceNodeId: 'n_prompt', sourceType: NodeType.NodeTypePrompt,
      title: '任务', role: 'task', taskReady: true,
    }]
    render(<AgentNode {...nodeProps(agent, nodeData(agent, { onStartRun, projectId: 'p_test', runInputs }))} />)

    expect(screen.getByText('无现有输出 · 本次运行预留新 Script')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '运行本地 Agent' }))
    expect(screen.getByRole('dialog', { name: '选择运行输入' })).toBeInTheDocument()
    expect(screen.getByText('本次运行将预留新 Script；节点与连接仅在合法结果提交时创建。')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '确认并运行' }))
    await waitFor(() => expect(onStartRun).toHaveBeenCalledWith('n_agent', 'p_test', {
      inputEdgeIds: undefined, outputNodeId: undefined, createOutput: true,
    }))
  })

  it('defaults to the only existing Script but lets the run create a parallel output', async () => {
    const agent = node('n_agent', 'agent', 'n_prompt')
    const script = node('n_script', 'script', agent.id, {
      scriptVersions: [{ n: 2, artifactId: 'sha256:script-v2', runId: 'r_script' }],
    })
    const onStartRun = vi.fn().mockResolvedValue({})
    const runInputs: RunInputOption[] = [{
      edgeId: 'e_prompt_agent', sourceNodeId: 'n_prompt', sourceType: NodeType.NodeTypePrompt,
      title: '任务', role: 'task', taskReady: true,
    }]
    render(<AgentNode {...nodeProps(agent, nodeData(agent, {
      outputScript: script, outputScripts: [script], runInputs, onStartRun, projectId: 'p_test',
    }))} />)

    fireEvent.click(screen.getByRole('button', { name: '运行本地 Agent' }))
    const output = screen.getByRole('combobox', { name: '本次运行 Script 输出目标' })
    expect(output).toHaveValue(script.id)
    fireEvent.change(output, { target: { value: '__create_output__' } })
    fireEvent.click(screen.getByRole('button', { name: '确认并运行' }))
    await waitFor(() => expect(onStartRun).toHaveBeenCalledWith('n_agent', 'p_test', {
      inputEdgeIds: undefined, outputNodeId: undefined, createOutput: true,
    }))
  })

  it('shows backend run rejection after input confirmation for a rootless Agent', async () => {
    const agent = node('n_agent_draft', 'agent')
    const onStartRun = vi.fn().mockResolvedValue({ error: { code: 'RUN_GRAPH_UNSUPPORTED', message: '独立子图暂不可运行' } })
    const runInputs: RunInputOption[] = [{
      edgeId: 'e_prompt_agent', sourceNodeId: 'n_prompt', sourceType: NodeType.NodeTypePrompt,
      title: '草稿任务', role: 'task', taskReady: true,
    }]
    render(<AgentNode {...nodeProps(agent, nodeData(agent, { onStartRun, projectId: 'p_test', runInputs }))} />)

    expect(screen.getByLabelText('n_agent_draft 输出端口')).toHaveAttribute('data-handle', 'agent-output')
    fireEvent.click(screen.getByRole('button', { name: '运行本地 Agent' }))
    fireEvent.click(screen.getByRole('button', { name: '确认并运行' }))
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('独立子图暂不可运行'))
  })

  it('shows a clear prompt-content error when a selected task is empty', async () => {
    const agent = node('n_agent', 'agent')
    const onStartRun = vi.fn().mockResolvedValue({
      error: { code: 'PROMPT_TASK_EMPTY', message: 'PROMPT_TASK_EMPTY: Prompt n_prompt has no task' },
    })
    const runInputs: RunInputOption[] = [{
      edgeId: 'e_prompt_agent', sourceNodeId: 'n_prompt', sourceType: NodeType.NodeTypePrompt,
      title: '任务', role: 'task', taskReady: false,
    }]
    render(<AgentNode {...nodeProps(agent, nodeData(agent, { onStartRun, projectId: 'p_test', runInputs }))} />)

    fireEvent.click(screen.getByRole('button', { name: '运行本地 Agent' }))
    fireEvent.click(screen.getByRole('button', { name: '确认并运行' }))
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('请先填写提示词内容，再启动 Agent。'))
  })

  it('requires incompatible inputs to be deselected and preserves the chosen input order', async () => {
    const agent = node('n_agent', 'agent')
    const onStartRun = vi.fn().mockResolvedValue({})
    const runInputs: RunInputOption[] = [
      { edgeId: 'e_task', sourceNodeId: 'n_prompt', sourceType: NodeType.NodeTypePrompt, title: '任务', role: 'task', taskReady: true },
      { edgeId: 'e_candidate', sourceNodeId: 'n_candidate', sourceType: NodeType.NodeTypeCandidate, title: '候选', role: 'candidate', taskReady: false },
      { edgeId: 'e_script', sourceNodeId: 'n_script', sourceType: NodeType.NodeTypeScript, title: '旧稿', role: 'artifact', taskReady: false },
      { edgeId: 'e_incompatible', sourceNodeId: 'n_other', sourceType: NodeType.NodeTypeAgent, title: '不兼容边', role: 'unsupported', taskReady: false },
    ]
    render(<AgentNode {...nodeProps(agent, nodeData(agent, { onStartRun, projectId: 'p_test', runInputs }))} />)

    fireEvent.click(screen.getByRole('button', { name: '运行本地 Agent' }))
    const confirm = screen.getByRole('button', { name: '确认并运行' })
    expect(confirm).toBeDisabled()
    fireEvent.click(screen.getByRole('checkbox', { name: '包含 不兼容边，不兼容输入' }))
    expect(confirm).toBeEnabled()
    fireEvent.click(screen.getByRole('checkbox', { name: '包含 候选，上下文 · Candidate' }))
    fireEvent.click(screen.getByRole('button', { name: '上移 旧稿' }))
    fireEvent.click(confirm)

    await waitFor(() => expect(onStartRun).toHaveBeenCalledWith('n_agent', 'p_test', {
      inputEdgeIds: ['e_script', 'e_task'], outputNodeId: undefined, createOutput: true,
    }))
  })

  it('requires explicit output selection when an Agent has multiple Script targets', async () => {
    const agent = node('n_agent', 'agent', 'n_prompt')
    const first = node('n_script_a', 'script', agent.id)
    const second = node('n_script_b', 'script', agent.id)
    const onStartRun = vi.fn().mockResolvedValue({})
    const runInputs: RunInputOption[] = [{
      edgeId: 'e_prompt_agent', sourceNodeId: 'n_prompt', sourceType: NodeType.NodeTypePrompt,
      title: '任务', role: 'task', taskReady: true,
    }]
    render(<AgentNode {...nodeProps(agent, nodeData(agent, {
      outputScripts: [first, second], outputAmbiguous: true, runInputs, onStartRun, projectId: 'p_test',
    }))} />)

    expect(screen.getByText('存在多个或共享的 Script 输出；运行前必须确认本次目标')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '运行本地 Agent' }))
    const output = screen.getByRole('combobox', { name: '本次运行 Script 输出目标' })
    expect(output).toHaveValue('')
    expect(screen.getByRole('button', { name: '确认并运行' })).toBeDisabled()
    fireEvent.change(output, { target: { value: second.id } })
    fireEvent.click(screen.getByRole('button', { name: '确认并运行' }))
    await waitFor(() => expect(onStartRun).toHaveBeenCalledWith('n_agent', 'p_test', {
      inputEdgeIds: undefined, outputNodeId: second.id, createOutput: false,
    }))
  })

  it('surfaces a backend BUSY error on the Agent node', async () => {
    const agent = node('n_agent', 'agent', 'n_prompt')
    const onStartRun = vi.fn().mockResolvedValue({ error: { code: 'BUSY', message: 'run already active' } })
    const runInputs: RunInputOption[] = [{
      edgeId: 'e_prompt_agent', sourceNodeId: 'n_prompt', sourceType: NodeType.NodeTypePrompt,
      title: '任务', role: 'task', taskReady: true,
    }]
    render(<AgentNode {...nodeProps(agent, nodeData(agent, { onStartRun, projectId: 'p_test', runInputs }))} />)

    fireEvent.click(screen.getByRole('button', { name: '运行本地 Agent' }))
    fireEvent.click(screen.getByRole('button', { name: '确认并运行' }))
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
    expect(screen.getByRole('button', { name: '已有运行' })).toBeDisabled()
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
    render(<ScriptNode {...nodeProps(script, nodeData(script, { onAddChild, pendingOutputCount: 1, pendingOutputIds: ['pending_one'] }))} />)

    expect(screen.getByText('2 个不可变版本 · 最新 v2')).toBeInTheDocument()
    expect(screen.getAllByText('r_two')).toHaveLength(2)
    expect(screen.getByText('待处理 1')).toBeInTheDocument()
    fireEvent.click(screen.getByText('版本历史 · 2'))
    expect(screen.getByText('r_one')).toBeInTheDocument()
    expect(screen.getByText('sha256:script-v1')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '从 n_script 新增提示词' }))
    expect(onAddChild).toHaveBeenCalledWith('n_script', 'prompt')
  })

  it('reads the latest Script body and fetches historical versions only when selected', async () => {
    const script = node('n_script', 'script', 'n_agent', {
      scriptVersions: [
        { n: 1, artifactId: 'sha256:script-v1', runId: 'r_one' },
        { n: 2, artifactId: 'sha256:script-v2', runId: 'r_two' },
      ],
    })
    const onGetScriptVersion = vi.fn(async (_nodeId: string, version: number, _projectId: string) => ({
      nodeId: script.id,
      version: script.scriptVersions?.find((item) => item.n === version) ?? { n: version, artifactId: '', runId: '' },
      payload: JSON.stringify({
        title: version === 2 ? '最新标题' : '历史标题',
        scenes: [{ narration: version === 2 ? '第一行\\n<img src=x onerror=alert(1)>' : '历史正文', duration: 12 }],
      }),
    }))
    render(<ScriptNode {...nodeProps(script, nodeData(script, { projectId: 'p_demo', onGetScriptVersion }))} />)

    expect(await screen.findByText('最新标题')).toBeInTheDocument()
    expect(onGetScriptVersion).toHaveBeenCalledWith(script.id, 2, 'p_demo')
    fireEvent.click(screen.getByRole('button', { name: '查看完整正文' }))
    const latestBody = screen.getByLabelText('脚本 v2 正文')
    expect(latestBody.querySelector('pre')?.textContent).toBe('第一行\\n<img src=x onerror=alert(1)>')
    expect(latestBody.querySelector('img')).toBeNull()

    fireEvent.click(screen.getByText('版本历史 · 2'))
    fireEvent.click(screen.getByRole('button', { name: '查看 v1 正文' }))
    expect(await screen.findByText('历史标题')).toBeInTheDocument()
    expect(onGetScriptVersion).toHaveBeenCalledWith(script.id, 1, 'p_demo')
    expect(screen.getByLabelText('脚本 v1 正文').querySelector('pre')).toHaveTextContent('历史正文')
  })

  it('ignores a late Script body response after the project changes', async () => {
    const script = node('n_script', 'script', 'n_agent', {
      scriptVersions: [{ n: 1, artifactId: 'sha256:script-v1', runId: 'r_one' }],
    })
    let resolveOld!: (detail: ScriptVersionDetail) => void
    const oldRequest = new Promise<ScriptVersionDetail>((resolve) => { resolveOld = resolve })
    const detailFor = (nodeId: string, title: string): ScriptVersionDetail => ({
      nodeId,
      version: { n: 1, artifactId: 'sha256:script-v1', runId: 'r_one' },
      payload: JSON.stringify({ title, scenes: [{ narration: title, duration: 1 }] }),
    })
    const onGetScriptVersion = vi.fn((_nodeId: string, _version: number, ownerProjectId: string) => (
      ownerProjectId === 'p_one' ? oldRequest : Promise.resolve(detailFor(script.id, '新工程脚本'))
    ))
    const { rerender } = render(<ScriptNode {...nodeProps(script, nodeData(script, { projectId: 'p_one', onGetScriptVersion }))} />)
    await waitFor(() => expect(onGetScriptVersion).toHaveBeenCalledWith(script.id, 1, 'p_one'))

    rerender(<ScriptNode {...nodeProps(script, nodeData(script, { projectId: 'p_two', onGetScriptVersion }))} />)
    expect(await screen.findByText('新工程脚本', { selector: 'strong' })).toBeInTheDocument()
    resolveOld(detailFor(script.id, '旧工程脚本'))
    await waitFor(() => expect(screen.getByText('新工程脚本', { selector: 'strong' })).toBeInTheDocument())
    expect(screen.queryByText('旧工程脚本')).not.toBeInTheDocument()
  })

  it('opens and applies a frozen Script result through the shared project-scoped controller', async () => {
    const script = node('n_script', 'script', 'n_agent', {
      scriptVersions: [{ n: 1, artifactId: 'sha256:script-v1', runId: 'r_old' }],
    })
    const pending: PendingSummary = {
      id: 'pending_script',
      nodeId: 'n_agent',
      runId: 'r_new',
      chainId: 'c_n_agent',
      kind: OutputKind.OutputKindScript,
      status: PendingStatus.PendingPending,
      outputBinding: { bound: true, nodeId: script.id, parentNodeId: 'n_agent', baselineVersion: 1, baselineArtifactId: 'sha256:script-v1' },
      resultNodeId: script.id,
    }
    const payload = JSON.stringify({ title: '待追加脚本', scenes: [{ narration: '新增旁白。', duration: 8 }] })
    const rawPayload = btoa(Array.from(new TextEncoder().encode(payload), (byte) => String.fromCharCode(byte)).join(''))
    const onGetPending = vi.fn().mockResolvedValue({
      pending: {
        ...pending,
        payload: 'pending/pending_script.json',
        chainPath: ['n_agent'],
        outputSchemaVersion: 1,
        createdAt: '2026-11-09T10:00:00Z',
      },
      rawPayload,
    })
    const onApplyPending = vi.fn().mockResolvedValue(undefined)
    const onRefreshWorkspace = vi.fn().mockResolvedValue(undefined)
    render(<PendingScriptHarness
      script={script}
      pending={pending}
      onGetPending={onGetPending}
      onApplyPending={onApplyPending}
      onDiscardPending={vi.fn().mockResolvedValue(undefined)}
      onRefreshWorkspace={onRefreshWorkspace}
    />)

    fireEvent.click(screen.getByRole('button', { name: '查看待确认结果' }))
    expect(await screen.findByText('待追加脚本')).toBeInTheDocument()
    expect(screen.getByRole('dialog', { name: '脚本结果 pending_script' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '确认追加' }))
    await waitFor(() => expect(onApplyPending).toHaveBeenCalledWith('pending_script', false, 'p_demo'))
    expect(onGetPending).toHaveBeenCalledWith('pending_script', 'p_demo')
    expect(onRefreshWorkspace).toHaveBeenCalledOnce()
  })

  it('previews a rerun batch with its round and exposes confirm and cancel actions', () => {
    const summary: PendingSummary = {
      id: 'pending_topic_batch', nodeId: 'n_topic', runId: 'r_topic_2', chainId: '',
      kind: OutputKind.OutputKindTopics, status: PendingStatus.PendingPending,
    }
    const detail = {
      pending: {
        ...summary,
        chainPath: [], payload: '', createdAt: '',
        executionSnapshot: { topicBatch: { baselineRound: 1 } },
      },
      rawPayload: btoa([...new TextEncoder().encode(JSON.stringify({ topics: [
        { title: '新候选一', angle: '方向一', hook: '开场一' },
        { title: '新候选二', angle: '方向二', hook: '开场二' },
      ] }))].map((byte) => String.fromCharCode(byte)).join('')),
    } as unknown as PendingDetail
    const onApply = vi.fn()
    const onDiscard = vi.fn()
    render(<PendingArtifactCard
      summary={summary}
      detail={detail}
      loading={false}
      loadError=""
      actionError=""
      forceConfirmation={false}
      actionBusy={false}
      resolved={false}
      syncError=""
      onRetryDetail={vi.fn()}
      onApply={onApply}
      onDiscard={onDiscard}
      onConfirmForce={vi.fn()}
      onCancelForce={vi.fn()}
      onRetryWorkspace={vi.fn()}
    />)

    expect(screen.getByLabelText('第 2 批候选预览')).toHaveTextContent('新候选一')
    expect(screen.getByText('第 1 批；已应用候选保持不变')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '确认并追加第 2 批' }))
    fireEvent.click(screen.getByRole('button', { name: '取消本次重跑' }))
    expect(onApply).toHaveBeenCalledOnce()
    expect(onDiscard).toHaveBeenCalledOnce()
  })

  it('disables path operations while allowing content-locked topic topology to remain separate', () => {
    const topic = node('n_topic', 'topic', '', { locked: true })
    const candidate = node('n_candidate', 'candidate', topic.id, {
      candidateSource: { topicNodeId: topic.id, round: 1, artifactId: 'sha256:topics', index: 0, runId: 'r_topics' },
    })
    render(
      <>
        <TopicNode {...nodeProps(topic, nodeData(topic, { topicCandidates: [{ nodeId: candidate.id, round: 1, artifactId: 'sha256:topics', index: 0, runId: 'r_topics' }], candidates: [{ nodeId: candidate.id, round: 1, runId: 'r_topics', index: 0, title: '候选甲' }] }))} />
        <CandidateNode {...nodeProps(candidate, nodeData(candidate, { runningLocked: true, candidates: [], candidate: { nodeId: candidate.id, round: 1, runId: 'r_topics', index: 0, title: '候选甲' } }))} />
      </>,
    )

    expect(screen.getByRole('button', { name: '按锁定参数重新生成' })).toBeEnabled()
    expect(screen.getByRole('button', { name: '从 候选甲 新增提示词' })).toBeDisabled()
    expect(screen.getByLabelText('候选甲 输出端口')).toHaveAttribute('data-connectable', 'false')
  })
})
