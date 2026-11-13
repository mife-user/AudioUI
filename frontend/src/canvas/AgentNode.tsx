import { memo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { RunStartError } from '../../bindings/audio-ui/internal/domain/entity'
import { formatRunStartError } from './runStartError'
import { nodeHandleLayoutKey, type CanvasFlowNode, type RunInputOption } from './flowModel'
import { NodeCollapseControl, NodeResizeHandles, NodeSizeControl } from './NodeLayoutControls'
import { PendingNodeEntry } from '../ui/PendingNodeEntry'
import { NodeRunRequestEntry } from '../ui/NodeRunRequestEntry'
import { useMeasuredNodeHandles } from './useMeasuredNodeHandles'

function runStatusLabel(status: string | undefined): string {
  switch (status) {
    case 'starting': return '启动中'
    case 'running': return '执行中'
    case 'succeeded': return '已完成'
    case 'failed': return '运行失败'
    case 'interrupted': return '已中断'
    default: return '空闲'
  }
}

function runInputRoleLabel(role: RunInputOption['role']): string {
  switch (role) {
    case 'task': return '任务 · Prompt'
    case 'candidate': return '上下文 · Candidate'
    case 'artifact': return '上下文 · Script'
    default: return '不兼容输入'
  }
}

export const AgentNode = memo(function AgentNode({ id, data, selected }: NodeProps<CanvasFlowNode>) {
  const { node, projectId, runningLocked, pathHighlighted, mutationsDisabled, globalRunBusy, runSubmitting, runStatus, pendingCount = 0, outputScript, outputAmbiguous, runInputs = [], onStartRun, onAddChild } = data
  const requiredHandles = [
    { id: 'agent-input', type: 'target' as const },
    { id: 'agent-output', type: 'source' as const },
  ]
  useMeasuredNodeHandles(id, nodeHandleLayoutKey('agent', data), projectId ?? '', requiredHandles, data.onNodeHandlesMeasured)
  const [runFailure, setRunFailure] = useState<RunStartError | null>(null)
  const [starting, setStarting] = useState(false)
  const [inputDialogOpen, setInputDialogOpen] = useState(false)
  const [selectedInputIDs, setSelectedInputIDs] = useState<string[]>([])
  const startInFlight = useRef(false)
  const versions = outputScript?.scriptVersions ?? []
  const latestVersion = versions[versions.length - 1]
  const outputSummary = outputAmbiguous
    ? '已连接多个或共享的 Script，暂不能确定输出目标'
    : outputScript
      ? latestVersion ? `绑定 ${outputScript.title} · v${latestVersion.n}` : `绑定 ${outputScript.title} · 等待首个结果`
      : '未指定输出 · 合法结果自动创建 Script'
  const status = runningLocked
    ? `${runStatus === 'starting' ? '启动中' : '执行中'} · 只读`
    : starting || runSubmitting ? '正在启动' : runStatusLabel(runStatus)
  const statusWithPending = pendingCount > 0 ? `${status} · 待处理 ${pendingCount}` : status
  const runDisabled = runningLocked || node.locked || mutationsDisabled || globalRunBusy || starting
  const selectedSet = new Set(selectedInputIDs)
  const orderedInputOptions = [
    ...selectedInputIDs.flatMap((edgeID) => {
      const input = runInputs.find((candidate) => candidate.edgeId === edgeID)
      return input ? [input] : []
    }),
    ...runInputs.filter((input) => !selectedSet.has(input.edgeId)),
  ]
  const selectedOptions = selectedInputIDs.flatMap((edgeID) => {
    const input = runInputs.find((candidate) => candidate.edgeId === edgeID)
    return input ? [input] : []
  })
  const hasPromptTask = selectedOptions.some((input) => input.role === 'task')
  const hasUnsupportedInput = selectedOptions.some((input) => input.role === 'unsupported')

  function openRunInputDialog() {
    if (runDisabled || startInFlight.current) return
    setRunFailure(null)
    setSelectedInputIDs(runInputs.map((input) => input.edgeId))
    setInputDialogOpen(true)
  }

  async function startRun(inputEdgeIDs?: string[]) {
    if (startInFlight.current || runningLocked || node.locked || globalRunBusy || mutationsDisabled) return
    startInFlight.current = true
    setStarting(true)
    setRunFailure(null)
    try {
      const result = inputEdgeIDs === undefined
        ? await onStartRun?.(node.id, projectId ?? '')
        : await onStartRun?.(node.id, projectId ?? '', inputEdgeIDs)
      if (result?.error) setRunFailure(result.error)
    } catch (failure) {
      setRunFailure({ code: 'RUN_START_FAILED', message: failure instanceof Error ? failure.message : String(failure) })
    } finally {
      startInFlight.current = false
      setStarting(false)
    }
  }

  async function confirmRun() {
    const defaultOrder = runInputs.map((input) => input.edgeId)
    const unchanged = selectedInputIDs.length === defaultOrder.length
      && selectedInputIDs.every((edgeID, index) => edgeID === defaultOrder[index])
    setInputDialogOpen(false)
    await startRun(unchanged ? undefined : selectedInputIDs)
  }

  function toggleInput(input: RunInputOption, checked: boolean) {
    setSelectedInputIDs((current) => checked
      ? current.includes(input.edgeId) ? current : [...current, input.edgeId]
      : current.filter((edgeID) => edgeID !== input.edgeId))
  }

  function moveInput(edgeID: string, offset: -1 | 1) {
    setSelectedInputIDs((current) => {
      const index = current.indexOf(edgeID)
      const nextIndex = index + offset
      if (index < 0 || nextIndex < 0 || nextIndex >= current.length) return current
      const next = [...current]
      const previous = next[index]
      next[index] = next[nextIndex]
      next[nextIndex] = previous
      return next
    })
  }

  return (
    <>
      <NodeResizeHandles nodeId={id} visible={selected && !node.collapsed} disabled={mutationsDisabled} />
      <article className={`canvas-node agent-node${pathHighlighted ? ' is-path' : ''}${runningLocked ? ' is-running-locked' : ''}${node.collapsed ? ' is-collapsed' : ''}`}>      <Handle
        id="agent-input"
        type="target"
        position={Position.Left}
        isConnectable={!runningLocked}
        isConnectableStart={false}
        isConnectableEnd={!runningLocked}
        aria-label={`连接到 ${node.title}`}
        className="node-input-handle"
      />
      <header className="canvas-node-head" tabIndex={-1}>
        <span className="canvas-node-kind agent-kind">AGENT</span>
        <strong title={node.title}>{node.title}</strong>
        <span className="canvas-node-status" role="status">{statusWithPending}</span>
        <NodeRunRequestEntry nodeId={node.id} nodeTitle={node.title} />
        <PendingNodeEntry node={node} pendingIds={data.pendingResultIds ?? []} />
        {!node.collapsed ? <NodeSizeControl node={node} disabled={mutationsDisabled} onApply={data.onSetNodeLayout} /> : null}
        <NodeCollapseControl node={node} disabled={mutationsDisabled} onToggle={data.onSetNodeCollapsed} />
      </header>
      <div id={`node-content-${node.id}`} className="canvas-node-content agent-node-content nowheel" hidden={Boolean(node.collapsed)}>
        <p className="canvas-node-hint">运行前将校验输入与输出。</p>
        <p className="agent-output-summary"><span>输出契约</span>{outputSummary}</p>
        {!outputScript && !outputAmbiguous ? (
          <button
            type="button"
            className="node-add-child nodrag"
            disabled={runningLocked || mutationsDisabled || globalRunBusy}
            onClick={() => onAddChild(node.id, 'script')}
            aria-label={`为 ${node.title} 指定空 Script 输出`}
          >
            ＋ 指定空 Script 输出
          </button>
        ) : null}
        {runFailure ? <p className="agent-run-error" id={`agent-run-error-${node.id}`} role="alert">{formatRunStartError(runFailure)}</p> : null}
        <button
          type="button"
          className="agent-run-button nodrag"
          disabled={runDisabled}
          aria-describedby={runFailure ? `agent-run-error-${node.id}` : undefined}
          onClick={openRunInputDialog}
        >
          {starting ? '正在启动…' : globalRunBusy ? '已有运行' : node.locked ? '节点已锁定' : '运行本地 Agent'}
        </button>
      </div>
      <Handle
        id="agent-output"
        type="source"
        position={Position.Right}
        isConnectable={!runningLocked}
        isConnectableStart={!runningLocked}
        isConnectableEnd={false}
        aria-label={`${node.title} 输出端口`}
        className="node-output-handle"
      />
      </article>
      {inputDialogOpen ? createPortal(
        <div
          className="agent-run-input-backdrop"
          role="presentation"
          onMouseDown={(event) => {
            if (event.target === event.currentTarget) setInputDialogOpen(false)
          }}
        >
          <section
            className="agent-run-input-dialog"
            role="dialog"
            aria-modal="true"
            aria-labelledby={`agent-run-input-title-${node.id}`}
            aria-describedby={`agent-run-input-description-${node.id}`}
            onKeyDown={(event) => {
              if (event.key === 'Escape') {
                event.preventDefault()
                setInputDialogOpen(false)
              }
            }}
          >
            <header className="agent-run-input-head">
              <div>
                <p className="agent-run-input-kicker">RUN INPUTS</p>
                <h2 id={`agent-run-input-title-${node.id}`}>选择运行输入</h2>
              </div>
              <button type="button" className="agent-run-input-close" onClick={() => setInputDialogOpen(false)} aria-label="取消运行">
                ×
              </button>
            </header>
            <p id={`agent-run-input-description-${node.id}`} className="agent-run-input-description">
              默认包含所有直接输入。Prompt 是任务，Candidate 与 Script 是上下文；可取消输入并调整顺序。
            </p>
            {runInputs.length > 0 ? (
              <ol className="agent-run-input-list">
                {orderedInputOptions.map((input, index) => {
                  const selectedIndex = selectedInputIDs.indexOf(input.edgeId)
                  const checked = selectedIndex >= 0
                  return (
                    <li className={`agent-run-input-item${checked ? ' is-selected' : ''}${input.role === 'unsupported' ? ' is-unsupported' : ''}`} key={input.edgeId}>
                      <label className="agent-run-input-choice">
                        <input
                          type="checkbox"
                          checked={checked}
                          aria-label={`包含 ${input.title}，${runInputRoleLabel(input.role)}`}
                          autoFocus={index === 0}
                          onChange={(event) => toggleInput(input, event.target.checked)}
                        />
                        <span className="agent-run-input-copy">
                          <strong>{input.title}</strong>
                          <small>{runInputRoleLabel(input.role)}</small>
                          {input.role === 'task' && !input.taskReady ? <em>任务为空或有未保存草稿；确认时会先提交所选 Prompt。</em> : null}
                        </span>
                      </label>
                      <span className="agent-run-input-order">
                        {checked ? `第 ${selectedIndex + 1} 项` : '未选择'}
                      </span>
                      <div className="agent-run-input-move">
                        <button type="button" aria-label={`上移 ${input.title}`} disabled={!checked || selectedIndex === 0} onClick={() => moveInput(input.edgeId, -1)}>↑</button>
                        <button type="button" aria-label={`下移 ${input.title}`} disabled={!checked || selectedIndex === selectedInputIDs.length - 1} onClick={() => moveInput(input.edgeId, 1)}>↓</button>
                      </div>
                    </li>
                  )
                })}
              </ol>
            ) : <p className="agent-run-input-empty">此 Agent 没有直接输入，请先连接至少一个非空 Prompt 任务。</p>}
            <p className={`agent-run-input-note${hasUnsupportedInput ? ' is-warning' : ''}`} role="status">
              {!hasPromptTask
                ? '至少选择一个 Prompt 任务。'
                : hasUnsupportedInput
                  ? '存在不兼容输入，请取消选择；连接不会被删除。'
                  : selectedOptions.some((input) => input.role === 'task' && !input.taskReady)
                    ? '将先保存被选中的 Prompt 草稿，再冻结本次输入。'
                    : '运行输入会按上方顺序冻结；未选择的连线保持不变。'}
            </p>
            <footer className="agent-run-input-actions">
              <button type="button" className="agent-run-input-cancel" onClick={() => setInputDialogOpen(false)}>取消</button>
              <button
                type="button"
                className="agent-run-input-confirm"
                disabled={!hasPromptTask || hasUnsupportedInput || runDisabled}
                onClick={() => void confirmRun()}
              >
                {starting ? '正在启动…' : '确认并运行'}
              </button>
            </footer>
          </section>
        </div>,
        document.body,
      ) : null}
    </>
  )
})
