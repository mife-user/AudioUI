import { memo } from 'react'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import { nodeHandleLayoutKey, type CanvasFlowNode } from './flowModel'
import { NodeResizeHandles, NodeSizeControl } from './NodeLayoutControls'
import { useMeasuredNodeHandles } from './useMeasuredNodeHandles'

export const ScriptNode = memo(function ScriptNode({ id, data, selected }: NodeProps<CanvasFlowNode>) {
  const { node, projectId, runningLocked, pathHighlighted, mutationsDisabled, pendingOutputCount = 0, onAddChild } = data
  const requiredHandles = node.parent
    ? [{ id: 'script-input', type: 'target' as const }, { id: 'script-output', type: 'source' as const }]
    : [{ id: 'script-input', type: 'target' as const }]
  useMeasuredNodeHandles(id, nodeHandleLayoutKey('script', data), projectId ?? '', requiredHandles, data.onNodeHandlesMeasured)
  const versions = node.scriptVersions ?? []
  const latestVersion = versions[versions.length - 1]

  return (
    <>
      <NodeResizeHandles nodeId={id} visible={selected} disabled={mutationsDisabled} />
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
        <span className="canvas-node-status" role="status">
          {pendingOutputCount > 0 ? `待处理 ${pendingOutputCount}` : latestVersion ? `v${latestVersion.n}` : '空输出'}
        </span>
        <NodeSizeControl node={node} disabled={mutationsDisabled} onApply={data.onSetNodeLayout} />
      </header>
      <div className="canvas-node-content script-node-content nowheel">
        {latestVersion ? (
          <>
            <p className="script-version-summary">{versions.length} 个不可变版本 · 最新 v{latestVersion.n}</p>
            <p className="script-version-source">来源运行 <code>{latestVersion.runId}</code></p>
            <p className="script-version-source">Artifact <code>{latestVersion.artifactId}</code></p>
            <details className="script-version-history">
              <summary>版本历史 · {versions.length}</summary>
              <ol>
                {[...versions].reverse().map((version) => (
                  <li key={version.n}>
                    <strong>v{version.n}</strong>
                    <span>来源运行 <code>{version.runId}</code></span>
                    <span>Artifact <code>{version.artifactId}</code></span>
                  </li>
                ))}
              </ol>
            </details>
            {pendingOutputCount > 0 ? (
              <p className="script-pending-hint" role="status">{pendingOutputCount} 个结果待处理，可在运行控制台查看。</p>
            ) : null}
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
    </>
  )
})
