import { memo } from 'react'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import { TopicParameters } from './TopicParameters'
import { nodeHandleLayoutKey, type CanvasFlowNode } from './flowModel'
import { useMeasuredNodeHandles } from './useMeasuredNodeHandles'

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

export const TopicNode = memo(function TopicNode({ id, data }: NodeProps<CanvasFlowNode>) {
  const {
    node,
    projectId,
    runningLocked,
    pathHighlighted,
    candidates,
    candidateError,
    candidateLoading = false,
    mutationsDisabled,
    globalRunBusy,
    runSubmitting,
    runStatus,
    pendingCount = 0,
    onAddChild,
    onUpdateTopicParams,
    onStartTopicRun,
  } = data
  const requiredHandles = candidates.length
    ? candidates.map((candidate, index) => ({ id: `candidate-${candidate.index ?? index}`, type: 'source' as const }))
    : data.hasUnmappedChildren ? [{ id: 'topic-connection-anchor', type: 'source' as const }] : []
  useMeasuredNodeHandles(id, nodeHandleLayoutKey('topic', data), projectId ?? '', requiredHandles, data.onNodeHandlesMeasured)
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
            候选结果待处理；请在控制台预览并应用。
          </p>
        ) : null}
        {candidateError ? (
          <p className="canvas-node-hint topic-candidate-error" role="alert">候选读取失败：{candidateError}</p>
        ) : candidateLoading ? (
          <p className="canvas-node-hint topic-candidate-hint" role="status">正在读取已应用候选…</p>
        ) : candidates.length ? (
          <div className="candidate-port-list" aria-label={`${node.title} 的候选输出端口`}>
            {candidates.map((candidate, index) => {
              const candidateIndex = candidate.index ?? index
              return (
                <div className="candidate-port-row" key={`${node.id}-${candidateIndex}`}>
                  <div className="candidate-port-copy">
                    <strong>{candidateIndex + 1}. {candidate.title}</strong>
                    {candidate.angle ? <span>{candidate.angle}</span> : null}
                    {candidate.hook ? <span>{candidate.hook}</span> : null}
                  </div>
                  <Handle
                    id={`candidate-${candidateIndex}`}
                    type="source"
                    position={Position.Right}
                    isConnectable={!runningLocked && !candidateError && !candidateLoading}
                    isConnectableStart={!runningLocked && !candidateError && !candidateLoading}
                    isConnectableEnd={false}
                    aria-label={`候选 ${candidateIndex + 1}：${candidate.title} 输出端口`}
                    className="candidate-handle"
                  />
                  <button
                    type="button"
                    className="node-add-branch nodrag"
                    disabled={runningLocked || mutationsDisabled || candidateLoading || Boolean(candidateError)}
                    onClick={() => onAddChild(node.id, candidateIndex)}
                    aria-label={`从候选 ${candidateIndex + 1} 新增提示词`}
                  >
                    ＋
                  </button>
                </div>
              )
            })}
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
