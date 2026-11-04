import { memo } from 'react'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { CanvasFlowNode } from './flowModel'

export const TopicNode = memo(function TopicNode({ data }: NodeProps<CanvasFlowNode>) {
  const { node, runningLocked, pathHighlighted, candidates, mutationsDisabled, onAddChild } = data
  return (
    <article className={`canvas-node topic-node${pathHighlighted ? ' is-path' : ''}${runningLocked ? ' is-running-locked' : ''}${node.locked ? ' is-content-locked' : ''}`}>
      <header className="canvas-node-head">
        <span className="canvas-node-kind topic-kind">选题</span>
        <strong title={node.title}>{node.title}</strong>
        {runningLocked ? <span className="canvas-node-status">运行中 · 只读</span> : node.locked ? <span className="canvas-node-status">内容已锁定</span> : null}
      </header>
      <div className="canvas-node-content">
        {candidates.length ? (
          <div className="candidate-port-list" aria-label={`${node.title} 的候选输出端口`}>
            {candidates.map((candidate, index) => (
              <div className="candidate-port-row" key={`${node.id}-${index}`}>
                <div className="candidate-port-copy">
                  <strong>{index + 1}. {candidate.title}</strong>
                  {candidate.angle ? <span>{candidate.angle}</span> : null}
                </div>
                <Handle
                  id={`candidate-${index}`}
                  type="source"
                  position={Position.Right}
                  isConnectable={!runningLocked}
                  isConnectableStart={!runningLocked}
                  isConnectableEnd={false}
                  aria-label={`候选 ${index + 1}：${candidate.title} 输出端口`}
                  className="candidate-handle"
                />
                <button
                  type="button"
                  className="node-add-branch nodrag"
                  disabled={runningLocked || mutationsDisabled}
                  onClick={() => onAddChild(node.id, index)}
                  aria-label={`从候选 ${index + 1} 新增提示词`}
                >
                  ＋
                </button>
              </div>
            ))}
          </div>
        ) : (
          <p className="canvas-node-hint">
            {node.rounds?.some((round) => round.kind === 'topics')
              ? '真实候选端口将在 M2-10B 接入；现有连线仍以工程图为准。'
              : '应用候选后才会出现可连接的候选端口。'}
          </p>
        )}
      </div>
      {data.hasUnmappedChildren ? (
        <Handle
          id="topic-connection-anchor"
          type="source"
          position={Position.Right}
          isConnectable={false}
          className="topic-edge-anchor"
          aria-hidden="true"
        />
      ) : null}
    </article>
  )
})
