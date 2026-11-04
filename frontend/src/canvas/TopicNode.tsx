import { memo } from 'react'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import { TopicParameters } from './TopicParameters'
import type { CanvasFlowNode } from './flowModel'

function runStatusLabel(status: string | undefined): string {
  switch (status) {
    case 'starting': return '启动中'
    case 'running': return '执行中'
    case 'succeeded': return '已完成'
    case 'failed': return '运行失败'
    case 'interrupted': return '已中断'
    case 'imported': return '历史导入'
    default: return ''
  }
}

export const TopicNode = memo(function TopicNode({ data }: NodeProps<CanvasFlowNode>) {
  const {
    node,
    projectId,
    runningLocked,
    pathHighlighted,
    candidates,
    mutationsDisabled,
    globalRunBusy,
    runSubmitting,
    runStatus,
    pendingCount = 0,
    onAddChild,
    onUpdateTopicParams,
    onStartTopicRun,
  } = data
  const runState = runSubmitting ? '正在启动' : runStatusLabel(runStatus)
  const status = runningLocked
    ? `${runStatus === 'starting' ? '启动中' : '运行中'} · 只读`
    : node.locked
      ? '内容已锁定'
      : pendingCount > 0
        ? runState ? `${runState} · 待处理 ${pendingCount}` : `待处理 · ${pendingCount}`
        : runState

  return (
    <article className={`canvas-node topic-node${pathHighlighted ? ' is-path' : ''}${runningLocked ? ' is-running-locked' : ''}${node.locked ? ' is-content-locked' : ''}`}>
      <header className="canvas-node-head">
        <span className="canvas-node-kind topic-kind">选题</span>
        <strong title={node.title}>{node.title}</strong>
        {status ? <span className="canvas-node-status">{status}</span> : null}
      </header>
      <div className="canvas-node-content">
        <TopicParameters
          node={node}
          projectId={projectId ?? ''}
          disabled={runningLocked || node.locked || mutationsDisabled}
          globalRunBusy={globalRunBusy ?? false}
          onSave={onUpdateTopicParams ?? (async () => undefined)}
          onRun={onStartTopicRun ?? (async () => ({}))}
        />
        {pendingCount > 0 ? (
          <p className="canvas-node-hint topic-pending-hint" role="status">
            候选结果待处理；预览与应用将在 M2-10B 接入。
          </p>
        ) : null}
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
          <p className="canvas-node-hint topic-candidate-hint">
            {node.rounds?.some((round) => round.kind === 'topics')
              ? '应用后的候选会成为可连接的输出端口。'
              : '生成结果待用户应用；应用候选后才会出现可连接的输出端口。'}
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
