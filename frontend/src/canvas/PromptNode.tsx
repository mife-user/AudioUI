import { memo } from 'react'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import { nodeHandleLayoutKey, type CanvasFlowNode } from './flowModel'
import { PromptParameters } from './PromptParameters'
import { NodeCollapseControl, NodeResizeHandles, NodeSizeControl } from './NodeLayoutControls'
import { useMeasuredNodeHandles } from './useMeasuredNodeHandles'

export const PromptNode = memo(function PromptNode({ id, data, selected }: NodeProps<CanvasFlowNode>) {
  const { node, runningLocked, pathHighlighted, mutationsDisabled, projectId, onAddChild } = data
  const requiredHandles = [
    { id: 'prompt-input', type: 'target' as const },
    { id: 'prompt-output', type: 'source' as const },
  ]
  useMeasuredNodeHandles(id, nodeHandleLayoutKey('prompt', data), projectId ?? '', requiredHandles, data.onNodeHandlesMeasured)
  const isDraft = !data.hasInputs
  const disabled = runningLocked || node.locked || (data.paramsEditingDisabled ?? mutationsDisabled)

  return (
    <>
      <NodeResizeHandles nodeId={id} visible={selected && !node.collapsed} disabled={mutationsDisabled} />
      <article className={`canvas-node prompt-node${pathHighlighted ? ' is-path' : ''}${runningLocked ? ' is-running-locked' : ''}${node.locked ? ' is-content-locked' : ''}${isDraft ? ' is-draft' : ''}${node.collapsed ? ' is-collapsed' : ''}`}>      <Handle
        id="prompt-input"
        type="target"
        position={Position.Left}
        isConnectable={!runningLocked}
        isConnectableStart={false}
        isConnectableEnd={!runningLocked}
        aria-label={`连接到 ${node.title}`}
        className="node-input-handle"
      />
      <header className="canvas-node-head">
        <span className="canvas-node-kind prompt-kind">提示词</span>
        <strong title={node.title}>{node.title}</strong>
        {isDraft ? <span className="canvas-node-status">草稿</span> : null}
        {runningLocked ? <span className="canvas-node-status">运行中 · 只读</span> : node.stale ? <span className="canvas-node-status">上游已变化</span> : null}
        {!node.collapsed ? <NodeSizeControl node={node} disabled={mutationsDisabled} onApply={data.onSetNodeLayout} /> : null}
        <NodeCollapseControl node={node} disabled={mutationsDisabled} onToggle={data.onSetNodeCollapsed} />
      </header>
      <div id={`node-content-${node.id}`} className="canvas-node-content prompt-node-content nowheel" hidden={Boolean(node.collapsed)}>
        <PromptParameters
          node={node}
          projectId={projectId ?? ''}
          disabled={disabled}
          onSave={data.onUpdateNodeParams ?? (async () => undefined)}
          onRegisterFlush={data.onRegisterPromptFlush}
        />
        <button
          type="button"
          className="node-add-child nodrag"
          disabled={runningLocked || mutationsDisabled}
          onClick={() => onAddChild(node.id, 'agent')}
          aria-label={`从 ${node.title} 新增 Agent`}
        >
          ＋ 新增 Agent
        </button>
      </div>
      <Handle
        id="prompt-output"
        type="source"
        position={Position.Right}
        isConnectable={!runningLocked}
        isConnectableStart={!runningLocked}
        isConnectableEnd={false}
        aria-label={`${node.title} 输出端口`}
        className="node-output-handle prompt-output-handle"
      />
      </article>
    </>
  )
})
