import { memo } from 'react'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import { nodeHandleLayoutKey, type CanvasFlowNode } from './flowModel'
import { PromptParameters } from './PromptParameters'
import { useMeasuredNodeHandles } from './useMeasuredNodeHandles'

export const PromptNode = memo(function PromptNode({ id, data }: NodeProps<CanvasFlowNode>) {
  const { node, runningLocked, pathHighlighted, mutationsDisabled, projectId, onAddChild } = data
  const requiredHandles = node.parent
    ? [{ id: 'prompt-input', type: 'target' as const }, { id: 'prompt-output', type: 'source' as const }]
    : [{ id: 'prompt-input', type: 'target' as const }]
  useMeasuredNodeHandles(id, nodeHandleLayoutKey('prompt', data), projectId ?? '', requiredHandles, data.onNodeHandlesMeasured)
  const isDraft = !node.parent
  const disabled = runningLocked || node.locked || mutationsDisabled

  return (
    <article className={`canvas-node prompt-node${pathHighlighted ? ' is-path' : ''}${runningLocked ? ' is-running-locked' : ''}${node.locked ? ' is-content-locked' : ''}${isDraft ? ' is-draft' : ''}`}>
      <Handle
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
      </header>
      <div className="canvas-node-content prompt-node-content">
        <PromptParameters
          node={node}
          projectId={projectId ?? ''}
          disabled={disabled}
          onSave={data.onUpdateNodeParams ?? (async () => undefined)}
          onRegisterFlush={data.onRegisterPromptFlush}
        />
        {!isDraft ? (
          <button
            type="button"
            className="node-add-child nodrag"
            disabled={runningLocked || mutationsDisabled}
            onClick={() => onAddChild(node.id, 'agent')}
            aria-label={`从 ${node.title} 新增 Agent`}
          >
            ＋ 新增 Agent
          </button>
        ) : (
          <p className="canvas-node-hint">连接到候选或脚本节点后可添加 Agent。</p>
        )}
      </div>
      {node.parent ? (
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
      ) : null}
    </article>
  )
})
