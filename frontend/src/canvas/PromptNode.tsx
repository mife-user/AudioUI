import { memo } from 'react'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { CanvasFlowNode } from './flowModel'

export const PromptNode = memo(function PromptNode({ data }: NodeProps<CanvasFlowNode>) {
  const { node, runningLocked, pathHighlighted, mutationsDisabled, onAddChild } = data
  const isDraft = !node.parent
  return (
    <article className={`canvas-node prompt-node${pathHighlighted ? ' is-path' : ''}${runningLocked ? ' is-running-locked' : ''}${node.locked ? ' is-content-locked' : ''}${isDraft ? ' is-draft' : ''}`}>
      <Handle
        id="prompt-input"
        type="target"
        position={Position.Top}
        isConnectable={!runningLocked}
        isConnectableStart={false}
        isConnectableEnd={!runningLocked}
        aria-label={`连接到 ${node.title}`}
        className="prompt-input-handle"
      />
      <header className="canvas-node-head">
        <span className="canvas-node-kind prompt-kind">提示词</span>
        <strong title={node.title}>{node.title}</strong>
        {isDraft ? <span className="canvas-node-status">草稿</span> : null}
        {runningLocked ? <span className="canvas-node-status">运行中 · 只读</span> : node.locked ? <span className="canvas-node-status">内容已锁定</span> : null}
      </header>
      <div className="canvas-node-content prompt-node-content">
        <p className="canvas-node-hint">
          {isDraft ? '未连接的草稿可作为连接目标。' : node.stale ? '上游已变化，当前内容基于旧上下文。' : '参数与执行将在后续任务接入。'}
        </p>
        {!isDraft ? (
          <button
            type="button"
            className="node-add-child nodrag"
            disabled={runningLocked || mutationsDisabled}
            onClick={() => onAddChild(node.id, null)}
            aria-label={`从 ${node.title} 新增提示词`}
          >
            ＋ 新增提示词
          </button>
        ) : null}
      </div>
      {!isDraft ? (
        <Handle
          id="prompt-output"
          type="source"
          position={Position.Bottom}
          isConnectable={!runningLocked}
          isConnectableStart={!runningLocked}
          isConnectableEnd={false}
          aria-label={`${node.title} 输出端口`}
          className="prompt-output-handle"
        />
      ) : null}
    </article>
  )
})
