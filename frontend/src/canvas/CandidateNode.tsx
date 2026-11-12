import { memo } from 'react'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import { nodeHandleLayoutKey, type CanvasFlowNode } from './flowModel'
import { NodeCollapseControl, NodeResizeHandles, NodeSizeControl } from './NodeLayoutControls'
import { useMeasuredNodeHandles } from './useMeasuredNodeHandles'

export const CandidateNode = memo(function CandidateNode({ id, data, selected }: NodeProps<CanvasFlowNode>) {
  const { node, candidate, runningLocked, pathHighlighted, mutationsDisabled, candidateError, candidateLoading, onAddChild } = data
  const sourceIndex = node.candidateSource?.index ?? -1
  const requiredHandles = [
    { id: 'candidate-input', type: 'target' as const },
    { id: 'candidate-output', type: 'source' as const },
  ]
  useMeasuredNodeHandles(id, nodeHandleLayoutKey('candidate', data), data.projectId ?? '', requiredHandles, data.onNodeHandlesMeasured)

  return (
    <>
      <NodeResizeHandles nodeId={id} visible={selected && !node.collapsed} disabled={mutationsDisabled} />
      <article className={`canvas-node candidate-node${pathHighlighted ? ' is-path' : ''}${runningLocked ? ' is-running-locked' : ''}${node.stale ? ' is-stale' : ''}${node.collapsed ? ' is-collapsed' : ''}`}>      <Handle
        id="candidate-input"
        type="target"
        position={Position.Left}
        isConnectable={!runningLocked}
        isConnectableStart={false}
        isConnectableEnd={!runningLocked}
        aria-label={`连接到 ${candidate?.title ?? node.title}`}
        className="node-input-handle"
      />
      <header className="canvas-node-head">
        <span className="canvas-node-kind candidate-kind">候选</span>
        <strong title={candidate?.title ?? node.title}>{candidate?.title ?? node.title}</strong>
        <span className="canvas-node-status">来源 #{sourceIndex + 1}</span>
        {!node.collapsed ? <NodeSizeControl node={node} disabled={mutationsDisabled} onApply={data.onSetNodeLayout} /> : null}
        <NodeCollapseControl node={node} disabled={mutationsDisabled} onToggle={data.onSetNodeCollapsed} />
      </header>
      <div id={`node-content-${node.id}`} className="canvas-node-content candidate-node-content nowheel" hidden={Boolean(node.collapsed)}>
        {candidateError ? <p className="canvas-node-error" role="alert">候选读取失败：{candidateError}</p> : null}
        {candidateLoading ? <p className="canvas-node-hint" role="status">正在读取候选内容…</p> : null}
        {!candidateError && !candidateLoading && candidate ? (
          <>
            <p className="candidate-detail"><span>方向</span>{candidate.angle || '未提供'}</p>
            <p className="candidate-detail"><span>开场</span>{candidate.hook || '未提供'}</p>
          </>
        ) : null}
        <p className="candidate-source-id">{node.candidateSource?.artifactId ?? '候选来源缺失'}</p>
        <button
          type="button"
          className="node-add-child nodrag"
          disabled={runningLocked || mutationsDisabled}
          onClick={() => onAddChild(node.id, 'prompt')}
          aria-label={`从 ${candidate?.title ?? node.title} 新增提示词`}
        >
          ＋ 新增提示词
        </button>
      </div>
      <Handle
        id="candidate-output"
        type="source"
        position={Position.Right}
        isConnectable={!runningLocked}
        isConnectableStart={!runningLocked}
        isConnectableEnd={false}
        aria-label={`${candidate?.title ?? node.title} 输出端口`}
        className="node-output-handle candidate-output-handle"
      />
      </article>
    </>
  )
})
