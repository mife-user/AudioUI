import { memo } from 'react'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import { nodeHandleLayoutKey, type CanvasFlowNode } from './flowModel'
import { useMeasuredNodeHandles } from './useMeasuredNodeHandles'

export const ScriptNode = memo(function ScriptNode({ id, data }: NodeProps<CanvasFlowNode>) {
  const { node, projectId, runningLocked, pathHighlighted, mutationsDisabled, onAddChild } = data
  const requiredHandles = node.parent
    ? [{ id: 'script-input', type: 'target' as const }, { id: 'script-output', type: 'source' as const }]
    : [{ id: 'script-input', type: 'target' as const }]
  useMeasuredNodeHandles(id, nodeHandleLayoutKey('script', data), projectId ?? '', requiredHandles, data.onNodeHandlesMeasured)
  const versions = node.scriptVersions ?? []
  const latestVersion = versions[versions.length - 1]

  return (
    <article className={`canvas-node script-node${pathHighlighted ? ' is-path' : ''}${runningLocked ? ' is-running-locked' : ''}${node.stale ? ' is-stale' : ''}`}>
      <Handle
        id="script-input"
        type="target"
        position={Position.Left}
        isConnectable={!runningLocked}
        isConnectableStart={false}
        isConnectableEnd={!runningLocked}
        aria-hidden="true"
        className="node-input-handle"
      />
      <header className="canvas-node-head">
        <span className="canvas-node-kind script-kind">SCRIPT</span>
        <strong title={node.title}>{node.title}</strong>
        <span className="canvas-node-status">{latestVersion ? `v${latestVersion.n}` : '空输出'}</span>
      </header>
      <div className="canvas-node-content script-node-content">
        {latestVersion ? (
          <>
            <p className="script-version-summary">{versions.length} 个不可变版本 · 最新 v{latestVersion.n}</p>
            <p className="script-version-source">来源运行 <code>{latestVersion.runId}</code></p>
            <p className="script-version-source">Artifact <code>{latestVersion.artifactId}</code></p>
          </>
        ) : (
          <p className="canvas-node-hint">等待绑定 Agent 的合法 script 结果。</p>
        )}
        {node.parent ? (
          <button
            type="button"
            className="node-add-child nodrag"
            disabled={runningLocked || mutationsDisabled}
            onClick={() => onAddChild(node.id, 'prompt')}
            aria-label={`从 ${node.title} 新增提示词`}
          >
            ＋ 新增提示词
          </button>
        ) : null}
      </div>
      {node.parent ? (
        <Handle
          id="script-output"
          type="source"
          position={Position.Right}
          isConnectable={!runningLocked}
          isConnectableStart={!runningLocked}
          isConnectableEnd={false}
          aria-label={`${node.title} 输出端口`}
          className="node-output-handle"
        />
      ) : null}
    </article>
  )
})
