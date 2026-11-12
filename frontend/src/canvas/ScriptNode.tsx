import { memo, useCallback, useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { ScriptVersion, ScriptVersionDetail } from '../../bindings/audio-ui/internal/domain/entity'
import { PendingStatus } from '../../bindings/audio-ui/internal/domain/valueobject'
import { usePendingControllerContext } from '../hooks/usePendingController'
import { PendingArtifactCard } from '../ui/PendingArtifactCard'
import { nodeHandleLayoutKey, type CanvasFlowNode } from './flowModel'
import { NodeCollapseControl, NodeResizeHandles, NodeSizeControl } from './NodeLayoutControls'
import { useMeasuredNodeHandles } from './useMeasuredNodeHandles'

type ScriptDocument = {
  title: string
  scenes: Array<{ narration: string; duration: number }>
}

type ScriptReadState = {
  status: 'loading' | 'ready' | 'error'
  detail?: ScriptVersionDetail
  error?: string
}

function readScriptDocument(payload: string): ScriptDocument | null {
  try {
    const value: unknown = JSON.parse(payload)
    if (!value || typeof value !== 'object' || Array.isArray(value)) return null
    const script = value as Record<string, unknown>
    if (typeof script.title !== 'string' || !Array.isArray(script.scenes)) return null
    const scenes: ScriptDocument['scenes'] = []
    for (const value of script.scenes) {
      if (!value || typeof value !== 'object' || Array.isArray(value)) return null
      const scene = value as Record<string, unknown>
      if (typeof scene.narration !== 'string' || typeof scene.duration !== 'number') return null
      scenes.push({ narration: scene.narration, duration: scene.duration })
    }
    return scenes.length ? { title: script.title, scenes } : null
  } catch {
    return null
  }
}

function messageOf(error: unknown): string {
  if (error instanceof Error) return error.message
  if (typeof error === 'string') return error
  return String(error)
}

function versionKey(projectId: string, nodeId: string, version: ScriptVersion): string {
  return `${projectId}:${nodeId}:${version.n}:${version.artifactId}`
}

export const ScriptNode = memo(function ScriptNode({ id, data, selected }: NodeProps<CanvasFlowNode>) {
  const { node, projectId = '', runningLocked, pathHighlighted, mutationsDisabled, pendingOutputCount = 0, pendingOutputIds = [], onAddChild } = data
  const pendingController = usePendingControllerContext()
  const versions = node.scriptVersions ?? []
  const latestVersion = versions[versions.length - 1]
  const requiredHandles = [
    { id: 'script-input', type: 'target' as const },
    { id: 'script-output', type: 'source' as const },
  ]
  useMeasuredNodeHandles(id, nodeHandleLayoutKey('script', data), projectId, requiredHandles, data.onNodeHandlesMeasured)

  const [reads, setReads] = useState<Record<string, ScriptReadState>>({})
  const readsRef = useRef(reads)
  readsRef.current = reads
  const inFlight = useRef(new Set<string>())
  const scopeRef = useRef(`${projectId}:${id}`)
  scopeRef.current = `${projectId}:${id}`
  const [expandedVersion, setExpandedVersion] = useState<number | null>(null)
  const pendingTrigger = useRef<HTMLButtonElement | null>(null)
  const pendingDialog = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    setReads({})
    setExpandedVersion(null)
    inFlight.current.clear()
  }, [projectId, id])

  const loadVersion = useCallback((version: ScriptVersion, retry = false) => {
    const key = versionKey(projectId, id, version)
    if (!retry && (readsRef.current[key]?.status === 'ready' || readsRef.current[key]?.status === 'loading' || inFlight.current.has(key))) return
    if (!data.onGetScriptVersion) {
      setReads((current) => ({ ...current, [key]: { status: 'error', error: '脚本正文读取接口不可用。' } }))
      return
    }
    inFlight.current.add(key)
    setReads((current) => ({ ...current, [key]: { status: 'loading' } }))
    const requestScope = `${projectId}:${id}`
    void data.onGetScriptVersion(id, version.n, projectId).then((detail) => {
      if (scopeRef.current !== requestScope) return
      if (detail.nodeId !== id || detail.version.n !== version.n || detail.version.artifactId !== version.artifactId) {
        throw new Error('返回的脚本版本与请求不匹配。')
      }
      setReads((current) => ({ ...current, [key]: { status: 'ready', detail } }))
    }).catch((failure: unknown) => {
      if (scopeRef.current !== requestScope) return
      setReads((current) => ({ ...current, [key]: { status: 'error', error: messageOf(failure) } }))
    }).finally(() => inFlight.current.delete(key))
  }, [data.onGetScriptVersion, id, projectId])

  useEffect(() => {
    if (latestVersion) loadVersion(latestVersion)
  }, [latestVersion?.n, latestVersion?.artifactId, loadVersion])

  const expandedScriptVersion = expandedVersion === null
    ? undefined
    : versions.find((version) => version.n === expandedVersion)
  useEffect(() => {
    if (expandedScriptVersion) loadVersion(expandedScriptVersion)
  }, [expandedScriptVersion?.n, expandedScriptVersion?.artifactId, loadVersion])

  const latestKey = latestVersion ? versionKey(projectId, id, latestVersion) : ''
  const latestRead = latestKey ? reads[latestKey] : undefined
  const latestDocument = latestRead?.status === 'ready' && latestRead.detail ? readScriptDocument(latestRead.detail.payload) : null
  const expandedKey = expandedScriptVersion ? versionKey(projectId, id, expandedScriptVersion) : ''
  const expandedRead = expandedKey ? reads[expandedKey] : undefined
  const expandedDocument = expandedRead?.status === 'ready' && expandedRead.detail ? readScriptDocument(expandedRead.detail.payload) : null

  const activePending = pendingController?.surface === 'node'
    && pendingController.surfaceNodeId === node.id
    ? pendingController.selectedPending
    : undefined
  const pendingOpen = Boolean(activePending)

  useEffect(() => {
    if (pendingOpen) pendingDialog.current?.focus()
  }, [pendingOpen])

  useEffect(() => {
    if (!pendingOpen) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        pendingController?.closeNodeSurface()
        pendingTrigger.current?.focus()
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [pendingOpen, pendingController])

  const closePending = useCallback(() => {
    pendingController?.closeNodeSurface()
    pendingTrigger.current?.focus()
  }, [pendingController])

  function renderScriptBody(document: ScriptDocument | null, version: ScriptVersion) {
    if (!document) return <p className="script-version-read-error" role="alert">脚本正文无法解析。</p>
    return (
      <div className="script-version-full-body" aria-label={`v${version.n} 脚本正文`}>
        <strong>{document.title}</strong>
        <ol>
          {document.scenes.map((scene, index) => (
            <li key={`${version.n}-${index}`}>
              <pre>{scene.narration}</pre>
              <span>{scene.duration} 秒</span>
            </li>
          ))}
        </ol>
      </div>
    )
  }

  return (
    <>
      <NodeResizeHandles nodeId={id} visible={selected && !node.collapsed} disabled={mutationsDisabled} />
      <article className={`canvas-node script-node${pathHighlighted ? ' is-path' : ''}${runningLocked ? ' is-running-locked' : ''}${node.stale ? ' is-stale' : ''}${node.collapsed ? ' is-collapsed' : ''}`}>        <Handle
          id="script-input"
          type="target"
          position={Position.Left}
          isConnectable={!runningLocked}
          isConnectableStart={false}
          isConnectableEnd={!runningLocked}
          aria-label={`连接到 ${node.title}`}
          className="node-input-handle"
        />
        <header className="canvas-node-head">
          <span className="canvas-node-kind script-kind">SCRIPT</span>
          <strong title={node.title}>{node.title}</strong>
          <span className="canvas-node-status" role="status">
            {pendingOutputCount > 0 ? `待处理 ${pendingOutputCount}` : latestVersion ? `v${latestVersion.n}` : '空输出'}
          </span>
          {pendingOutputCount > 0 ? (
            <div className="script-pending-actions" aria-label="脚本待处理结果">
              {pendingOutputIds.map((pendingId, index) => {
                const summary = pendingController?.pendingItems.find((item) => item.id === pendingId)
                if (!summary || !pendingController) return null
                const label = summary.status === PendingStatus.PendingInvalid ? '处理无效结果' : '查看待确认结果'
                return (
                  <button
                    key={pendingId}
                    ref={(element) => { if (index === 0) pendingTrigger.current = element }}
                    type="button"
                    className="script-version-read nodrag"
                    onClick={() => void pendingController.openFromNode(pendingId, node.id)}
                  >
                    {label}{pendingOutputCount > 1 ? ` ${index + 1}` : ''}
                  </button>
                )
              })}
            </div>
          ) : null}
          {!node.collapsed ? <NodeSizeControl node={node} disabled={mutationsDisabled} onApply={data.onSetNodeLayout} /> : null}
          <NodeCollapseControl node={node} disabled={mutationsDisabled} onToggle={data.onSetNodeCollapsed} />
        </header>
        <div id={`node-content-${node.id}`} className="canvas-node-content script-node-content nowheel" hidden={Boolean(node.collapsed)}>
          {latestVersion ? (
            <>
              <p className="script-version-summary">{versions.length} 个不可变版本 · 最新 v{latestVersion.n}</p>
              <p className="script-version-source">来源运行 <code>{latestVersion.runId}</code></p>
              {latestRead?.status === 'loading' ? <p className="script-version-read-message" role="status">正在读取最新脚本正文…</p> : null}
              {latestRead?.status === 'error' ? (
                <div className="script-version-read-error" role="alert">
                  <span>正文读取失败：{latestRead.error}</span>
                  <button type="button" className="script-version-read" onClick={() => loadVersion(latestVersion, true)}>重试读取</button>
                </div>
              ) : null}
              {latestDocument ? (
                <div className="script-version-preview">
                  <strong>{latestDocument.title}</strong>
                  <p title={latestDocument.scenes[0]?.narration}>{latestDocument.scenes[0]?.narration || '正文为空。'}</p>
                  <button type="button" className="script-version-read" onClick={() => setExpandedVersion(expandedVersion === latestVersion.n ? null : latestVersion.n)}>
                    {expandedVersion === latestVersion.n ? '收起完整正文' : '查看完整正文'}
                  </button>
                </div>
              ) : null}
              <details className="script-version-history">
                <summary>版本历史 · {versions.length}</summary>
                <ol>
                  {[...versions].reverse().map((version) => (
                    <li key={version.n}>
                      <strong>v{version.n}</strong>
                      <span>来源运行 <code>{version.runId}</code></span>
                      <span>Artifact <code>{version.artifactId}</code></span>
                      <button type="button" className="script-version-read" onClick={() => setExpandedVersion(expandedVersion === version.n ? null : version.n)}>
                        {expandedVersion === version.n ? '收起正文' : `查看 v${version.n} 正文`}
                      </button>
                    </li>
                  ))}
                </ol>
              </details>
              {expandedVersion !== null && expandedScriptVersion ? (
                <section className="script-version-reader" aria-label={`脚本 v${expandedVersion} 正文`}>
                  {expandedRead?.status === 'loading' ? <p role="status">正在读取 v{expandedVersion} 正文…</p> : null}
                  {expandedRead?.status === 'error' ? (
                    <div className="script-version-read-error" role="alert">
                      <span>读取失败：{expandedRead.error}</span>
                      <button type="button" className="script-version-read" onClick={() => loadVersion(expandedScriptVersion, true)}>重试读取</button>
                    </div>
                  ) : null}
                  {expandedRead?.status === 'ready' ? renderScriptBody(expandedDocument, expandedScriptVersion) : null}
                </section>
              ) : null}
            </>
          ) : (
            <p className="canvas-node-hint">等待绑定 Agent 的合法 script 结果。</p>
          )}
          <button
            type="button"
            className="node-add-child nodrag"
            disabled={runningLocked || mutationsDisabled}
            onClick={() => onAddChild(node.id, 'prompt')}
            aria-label={`从 ${node.title} 新增提示词`}
          >
            ＋ 新增提示词
          </button>
        </div>
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
      </article>
      {pendingOpen && activePending && pendingController && typeof document !== 'undefined' ? createPortal(
        <div className="script-pending-backdrop" role="presentation" onMouseDown={(event) => {
          if (event.target === event.currentTarget) closePending()
        }}>
          <section
            className="script-pending-dialog"
            role="dialog"
            aria-modal="true"
            aria-label={`脚本结果 ${activePending.id}`}
            tabIndex={-1}
            ref={pendingDialog}
          >
            <header className="script-pending-dialog-head">
              <strong>脚本结果待确认</strong>
              <button type="button" className="agent-icon-btn" aria-label="关闭脚本结果预览" onClick={closePending}>×</button>
            </header>
            <PendingArtifactCard
              summary={activePending}
              detail={pendingController.selectedPendingDetail}
              outputNode={node}
              loading={pendingController.loading}
              loadError={pendingController.loadError}
              actionError={pendingController.actionError}
              forceConfirmation={pendingController.forceConfirmation}
              actionBusy={pendingController.actionBusy}
              resolved={pendingController.resolved}
              syncError={pendingController.syncError}
              onRetryDetail={pendingController.retryDetail}
              onApply={() => void pendingController.apply()}
              onDiscard={() => void pendingController.discard()}
              onConfirmForce={() => void pendingController.apply(true)}
              onCancelForce={pendingController.cancelForce}
              onRetryWorkspace={() => void pendingController.retryWorkspace()}
            />
          </section>
        </div>,
        document.body,
      ) : null}
    </>
  )
})
