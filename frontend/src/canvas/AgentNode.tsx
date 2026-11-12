import { memo, useRef, useState } from 'react'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { RunStartError } from '../../bindings/audio-ui/internal/domain/entity'
import { formatRunStartError } from './runStartError'
import { nodeHandleLayoutKey, type CanvasFlowNode } from './flowModel'
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

export const AgentNode = memo(function AgentNode({ id, data, selected }: NodeProps<CanvasFlowNode>) {
  const { node, projectId, runningLocked, pathHighlighted, mutationsDisabled, globalRunBusy, runSubmitting, runStatus, pendingCount = 0, outputScript, outputAmbiguous, onStartRun, onAddChild } = data
  const requiredHandles = [
    { id: 'agent-input', type: 'target' as const },
    { id: 'agent-output', type: 'source' as const },
  ]
  useMeasuredNodeHandles(id, nodeHandleLayoutKey('agent', data), projectId ?? '', requiredHandles, data.onNodeHandlesMeasured)
  const [runFailure, setRunFailure] = useState<RunStartError | null>(null)
  const [starting, setStarting] = useState(false)
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

  async function startRun() {
    if (startInFlight.current || runningLocked || node.locked || globalRunBusy || mutationsDisabled) return
    startInFlight.current = true
    setStarting(true)
    setRunFailure(null)
    try {
      const result = await onStartRun?.(node.id, projectId ?? '')
      if (result?.error) setRunFailure(result.error)
    } catch (failure) {
      setRunFailure({ code: 'RUN_START_FAILED', message: failure instanceof Error ? failure.message : String(failure) })
    } finally {
      startInFlight.current = false
      setStarting(false)
    }
  }

  const runDisabled = runningLocked || node.locked || mutationsDisabled || globalRunBusy || starting

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
          onClick={() => void startRun()}
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
    </>
  )
})
