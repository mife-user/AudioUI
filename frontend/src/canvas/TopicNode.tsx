import { memo } from 'react'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import { TopicParameters } from './TopicParameters'
import { candidateAt, nodeHandleLayoutKey, type CanvasFlowNode } from './flowModel'
import { NodeResizeHandles, NodeSizeControl } from './NodeLayoutControls'
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
    case 'imported': return '历史导入'
    default: return ''
  }
}

export const TopicNode = memo(function TopicNode({ id, data, selected }: NodeProps<CanvasFlowNode>) {
  const {
    node,
    projectId,
    runningLocked,
    pathHighlighted,
    topicCandidateIndexes,
    candidates,
    candidateError,
    candidateLoading = false,
    mutationsDisabled,
    globalRunBusy,
    runSubmitting,
    runStatus,
    pendingCount = 0,
    pendingResultIds = [],
    onUpdateTopicParams,
    onStartRun,
  } = data
  const requiredHandles = topicCandidateIndexes.map((index) => ({ id: `topic-candidate-${index}`, type: 'source' as const }))
  useMeasuredNodeHandles(id, nodeHandleLayoutKey('topic', data), projectId ?? '', requiredHandles, data.onNodeHandlesMeasured)
  const runState = runSubmitting ? '正在启动' : runStatusLabel(runStatus)
  const status = runningLocked
    ? `${runStatus === 'starting' ? '启动中' : '运行中'} · 只读`
    : node.locked
      ? '内容与执行已锁定'
      : pendingCount > 0
        ? runState ? `${runState} · 待处理 ${pendingCount}` : `待处理 · ${pendingCount}`
        : runState

  return (
    <>
      <NodeResizeHandles nodeId={id} visible={selected} disabled={mutationsDisabled} />
      <article className={`canvas-node topic-node${pathHighlighted ? ' is-path' : ''}${runningLocked ? ' is-running-locked' : ''}${node.locked ? ' is-content-locked' : ''}`}>
      <header className="canvas-node-head" tabIndex={-1}>
        <span className="canvas-node-kind topic-kind">选题</span>
        <strong title={node.title}>{node.title}</strong>
        {status ? <span className="canvas-node-status">{status}</span> : null}
        <NodeRunRequestEntry nodeId={node.id} nodeTitle={node.title} />
        <NodeSizeControl node={node} disabled={mutationsDisabled} onApply={data.onSetNodeLayout} />
      </header>
      <div className="canvas-node-content nowheel">
        <TopicParameters
          node={node}
          projectId={projectId ?? ''}
          disabled={runningLocked || node.locked || (data.paramsEditingDisabled ?? mutationsDisabled)}
          runBlocked={mutationsDisabled}
          globalRunBusy={globalRunBusy ?? false}
          onSave={onUpdateTopicParams ?? (async () => undefined)}
          onRun={onStartRun ?? (async () => ({}))}
          onRegisterFlush={data.onRegisterTopicFlush}
        />
        <PendingNodeEntry node={node} pendingIds={pendingResultIds} />
        {candidateError ? <p className="canvas-node-hint topic-candidate-error" role="alert">候选读取失败：{candidateError}</p> : null}
        {candidateLoading ? <p className="canvas-node-hint topic-candidate-hint" role="status">正在读取候选内容…</p> : null}
        {topicCandidateIndexes.length ? (
          <div className="topic-candidate-list" aria-label={`${node.title} 的候选节点`}>
            {topicCandidateIndexes.map((index) => {
              const candidate = candidateAt(candidates, index)
              return (
                <div className="topic-candidate-row" key={`${node.id}-${index}`}>
                  <span className="topic-candidate-index">{index + 1}</span>
                  <span className="topic-candidate-title">{candidate?.title ?? `候选 ${index + 1}`}</span>
                  <Handle
                    id={`topic-candidate-${index}`}
                    type="source"
                    position={Position.Right}
                    isConnectable={false}
                    aria-hidden="true"
                    className="topic-candidate-handle"
                  />
                </div>
              )
            })}
          </div>
        ) : !candidateError && !candidateLoading ? (
          <p className="canvas-node-hint topic-candidate-hint">
            {node.rounds?.some((round) => round.kind === 'topics')
              ? '结果已保存；正在等待候选节点同步。'
              : '运行选题后，应用会校验结果并生成候选节点。'}
          </p>
        ) : null}
      </div>
      </article>
    </>
  )
})
