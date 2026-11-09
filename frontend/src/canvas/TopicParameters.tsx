import { useEffect, useRef, useState } from 'react'
import type { Node, NodeParams, RunStartError, RunStartResult } from '../../bindings/audio-ui/internal/domain/entity'
import { formatRunStartError } from './runStartError'

const PARAM_SAVE_DEBOUNCE_MS = 320

type TopicDraft = {
  title: string
  idea: string
  direction: string
  candidateCount: string
}

type TopicDraftErrors = {
  title: string
  candidateCount: string
}

type SaveState = 'saved' | 'dirty' | 'saving' | 'error'

type Props = {
  node: Node
  projectId: string
  disabled: boolean
  runBlocked?: boolean
  globalRunBusy: boolean
  onSave: (nodeId: string, title: string, params: NodeParams, projectId: string) => Promise<boolean | void>
  onRun: (nodeId: string, projectId: string) => Promise<RunStartResult>
  onRegisterFlush?: (nodeId: string, projectId: string, flush: () => Promise<boolean>) => () => void
}

function draftFromNode(node: Node): TopicDraft {
  const candidateCount = node.params?.candidateCount ?? 0
  return {
    title: node.title,
    idea: node.params?.idea ?? '',
    direction: node.params?.direction ?? '',
    candidateCount: String(candidateCount >= 3 && candidateCount <= 10 ? candidateCount : 3),
  }
}

function needsInitialSave(node: Node): boolean {
  const count = node.params?.candidateCount ?? 0
  return count < 3 || count > 10
}

function validateDraft(draft: TopicDraft): TopicDraftErrors {
  return {
    title: draft.title.trim() ? '' : '选题标题不能为空。',
    candidateCount: /^(?:[3-9]|10)$/.test(draft.candidateCount)
      ? ''
      : '候选数必须是 3–10 之间的整数。',
  }
}

function hasErrors(errors: TopicDraftErrors): boolean {
  return Boolean(errors.title || errors.candidateCount)
}

function paramsFromDraft(draft: TopicDraft): NodeParams {
  return {
    idea: draft.idea,
    direction: draft.direction,
    candidateCount: Number(draft.candidateCount),
  }
}

function errorMessage(error: unknown): string {
  if (error instanceof Error) return error.message
  if (typeof error === 'string') return error
  if (error && typeof error === 'object' && 'message' in error) return String((error as { message: unknown }).message)
  return String(error)
}

function localStartFailure(error: unknown): RunStartError {
  return { code: 'RUN_START_FAILED', message: errorMessage(error) }
}

export function TopicParameters({ node, projectId, disabled, runBlocked = false, globalRunBusy, onSave, onRun, onRegisterFlush }: Props) {
  const initialDraft = draftFromNode(node)
  const [draft, setDraft] = useState<TopicDraft>(initialDraft)
  const [saveState, setSaveState] = useState<SaveState>(() => needsInitialSave(node) ? 'dirty' : 'saved')
  const [saveError, setSaveError] = useState('')
  const [syncPending, setSyncPending] = useState(false)
  const [runFailure, setRunFailure] = useState<RunStartError | null>(null)
  const [starting, setStarting] = useState(false)
  const latestDraft = useRef(initialDraft)
  const editVersion = useRef(needsInitialSave(node) ? 1 : 0)
  const savedVersion = useRef(0)
  const saveQueue = useRef(Promise.resolve())
  const activeSave = useRef<{ version: number; promise: Promise<void> } | null>(null)
  const syncPendingRef = useRef(false)
  const saveTimer = useRef<number | null>(null)
  const composing = useRef(false)
  const startInFlight = useRef(false)
  const identity = useRef({ projectId, nodeId: node.id })
  const currentProjectId = useRef(projectId)
  const currentNodeId = useRef(node.id)
  const isRunBusy = useRef(globalRunBusy)
  const saveCallback = useRef(onSave)
  const runCallback = useRef(onRun)
  const registerCallback = useRef(onRegisterFlush)
  const flushCallback = useRef<() => Promise<boolean>>(async () => true)
  currentProjectId.current = projectId
  currentNodeId.current = node.id
  isRunBusy.current = globalRunBusy
  saveCallback.current = onSave
  runCallback.current = onRun
  registerCallback.current = onRegisterFlush

  const validation = validateDraft(draft)
  const errors = hasErrors(validation)
  const titleErrorId = `topic-title-error-${node.id}`
  const countErrorId = `topic-count-error-${node.id}`
  const saveErrorId = `topic-save-error-${node.id}`
  const runErrorId = `topic-run-error-${node.id}`

  function clearSaveTimer() {
    if (saveTimer.current === null) return
    window.clearTimeout(saveTimer.current)
    saveTimer.current = null
  }

  function saveVersion(version: number, value: TopicDraft): Promise<void> {
    if (activeSave.current?.version === version) return activeSave.current.promise

    const operation = saveQueue.current.catch(() => undefined).then(async () => {
      if (currentProjectId.current !== projectId || currentNodeId.current !== node.id) {
        throw new Error('PROJECT_CHANGED: 工程已切换，未保存旧工程参数。')
      }
      if (version !== editVersion.current) return
      const currentErrors = validateDraft(value)
      if (hasErrors(currentErrors)) throw new Error(currentErrors.title || currentErrors.candidateCount)

      setSaveState('saving')
      setSaveError('')
      const synced = await saveCallback.current(node.id, value.title.trim(), paramsFromDraft(value), projectId)
      if (version === editVersion.current && currentProjectId.current === projectId && currentNodeId.current === node.id) {
        savedVersion.current = version
        syncPendingRef.current = synced === false
        setSyncPending(syncPendingRef.current)
        setSaveState('saved')
      }
    }).catch((failure: unknown) => {
      if (version === editVersion.current && currentProjectId.current === projectId && currentNodeId.current === node.id) {
        setSaveState('error')
        setSaveError(errorMessage(failure))
      }
      throw failure
    })

    saveQueue.current = operation.catch(() => undefined)
    activeSave.current = { version, promise: operation }
    void operation.finally(() => {
      if (activeSave.current?.version === version) activeSave.current = null
    }).catch(() => undefined)
    return operation
  }

  async function flushSave(): Promise<boolean> {
    clearSaveTimer()
    while (savedVersion.current < editVersion.current) {
      if (currentProjectId.current !== projectId || currentNodeId.current !== node.id) {
        throw new Error('PROJECT_CHANGED: 工程已切换，已取消旧工程参数保存。')
      }
      const version = editVersion.current
      const value = latestDraft.current
      const currentErrors = validateDraft(value)
      if (hasErrors(currentErrors)) throw new Error(currentErrors.title || currentErrors.candidateCount)
      await saveVersion(version, value)
      if (version === editVersion.current && savedVersion.current >= version) break
    }
    return true
  }

  function updateField(field: keyof TopicDraft, value: string) {
    const next = { ...latestDraft.current, [field]: value }
    latestDraft.current = next
    setDraft(next)
    editVersion.current += 1
    setSaveState('dirty')
    setSaveError('')
    setRunFailure(null)
    clearSaveTimer()
    if (composing.current || hasErrors(validateDraft(next))) return

    const version = editVersion.current
    saveTimer.current = window.setTimeout(() => {
      saveTimer.current = null
      void saveVersion(version, next).catch(() => undefined)
    }, PARAM_SAVE_DEBOUNCE_MS)
  }

  function startComposition() {
    composing.current = true
    clearSaveTimer()
  }

  function finishComposition(field: keyof TopicDraft, value: string) {
    composing.current = false
    updateField(field, value)
  }

  flushCallback.current = flushSave

  async function retrySave() {
    try {
      await flushSave()
    } catch {
      // The visible save error remains attached to the current draft.
    }
  }

  async function startRun() {
    if (startInFlight.current || disabled || runBlocked || globalRunBusy || errors || saveState === 'error') return
    startInFlight.current = true
    setStarting(true)
    setRunFailure(null)
    const ownerProjectId = projectId
    const ownerNodeId = node.id
    try {
      try {
        await flushSave()
      } catch {
        return
      }
      if (currentProjectId.current !== ownerProjectId || currentNodeId.current !== ownerNodeId) return
      if (syncPendingRef.current) {
        setRunFailure({ code: 'WORKSPACE_SYNC_STALE', message: '选题参数已提交，等待工程快照同步后再启动。' })
        return
      }
      if (isRunBusy.current) {
        setRunFailure({ code: 'BUSY', message: '已有运行正在进行，当前无法启动另一个运行。' })
        return
      }
      const result = await runCallback.current(ownerNodeId, ownerProjectId)
      if (currentProjectId.current !== ownerProjectId || currentNodeId.current !== ownerNodeId) return
      if (result.error) setRunFailure(result.error)
    } catch (failure) {
      if (currentProjectId.current === ownerProjectId && currentNodeId.current === ownerNodeId) {
        setRunFailure(localStartFailure(failure))
      }
    } finally {
      if (currentProjectId.current === ownerProjectId && currentNodeId.current === ownerNodeId) {
        startInFlight.current = false
        setStarting(false)
      }
    }
  }

  useEffect(() => {
    const unregister = registerCallback.current?.(node.id, projectId, () => flushCallback.current())
    return () => unregister?.()
  }, [node.id, projectId])

  useEffect(() => {
    const identityChanged = identity.current.projectId !== projectId || identity.current.nodeId !== node.id
    if (identityChanged) {
      identity.current = { projectId, nodeId: node.id }
      clearSaveTimer()
      composing.current = false
      const next = draftFromNode(node)
      const needsSave = needsInitialSave(node)
      latestDraft.current = next
      editVersion.current = needsSave ? 1 : 0
      savedVersion.current = 0
      saveQueue.current = Promise.resolve()
      activeSave.current = null
      startInFlight.current = false
      setDraft(next)
      setSaveState(needsSave ? 'dirty' : 'saved')
      setSaveError('')
      syncPendingRef.current = false
      setSyncPending(false)
      setRunFailure(null)
      setStarting(false)
      return
    }

    if (savedVersion.current >= editVersion.current) {
      const next = draftFromNode(node)
      const needsSave = needsInitialSave(node)
      latestDraft.current = next
      editVersion.current = savedVersion.current + (needsSave ? 1 : 0)
      setDraft(next)
      setSaveState(needsSave ? 'dirty' : 'saved')
    }
  }, [projectId, node.id, node.title, node.params])

  useEffect(() => {
    if (!syncPending || disabled) return
    syncPendingRef.current = false
    setSyncPending(false)
  }, [syncPending, disabled])

  useEffect(() => () => clearSaveTimer(), [])

  const runDisabled = disabled || runBlocked || globalRunBusy || starting || errors || saveState === 'error' || syncPending
  const saveStatusText = syncPending
    ? '已提交，等待快照同步…'
    : saveState === 'saving'
      ? '保存中…'
      : saveState === 'error'
        ? '保存失败，修改仍保留'
        : saveState === 'dirty'
          ? '未保存'
          : '已保存'

  return (
    <div className="topic-parameters">
      <label className="topic-field" htmlFor={`topic-title-${node.id}`}>
        <span>选题标题</span>
        <input
          id={`topic-title-${node.id}`}
          className="nodrag"
          type="text"
          value={draft.title}
          maxLength={80}
          required
          disabled={disabled || starting}
          aria-invalid={Boolean(validation.title)}
          aria-describedby={validation.title ? titleErrorId : undefined}
          onCompositionStart={startComposition}
          onCompositionEnd={(event) => finishComposition('title', event.currentTarget.value)}
          onChange={(event) => updateField('title', event.currentTarget.value)}
        />
        {validation.title ? <span className="topic-field-error" id={titleErrorId}>{validation.title}</span> : null}
      </label>
      <label className="topic-field" htmlFor={`topic-idea-${node.id}`}>
        <span>想法</span>
        <textarea
          id={`topic-idea-${node.id}`}
          className="nodrag nowheel"
          rows={3}
          value={draft.idea}
          disabled={disabled || starting}
          onCompositionStart={startComposition}
          onCompositionEnd={(event) => finishComposition('idea', event.currentTarget.value)}
          onChange={(event) => updateField('idea', event.currentTarget.value)}
        />
      </label>
      <label className="topic-field" htmlFor={`topic-direction-${node.id}`}>
        <span>方向</span>
        <input
          id={`topic-direction-${node.id}`}
          className="nodrag"
          type="text"
          value={draft.direction}
          disabled={disabled || starting}
          onCompositionStart={startComposition}
          onCompositionEnd={(event) => finishComposition('direction', event.currentTarget.value)}
          onChange={(event) => updateField('direction', event.currentTarget.value)}
        />
      </label>
      <label className="topic-field" htmlFor={`topic-count-${node.id}`}>
        <span>候选数</span>
        <input
          id={`topic-count-${node.id}`}
          className="nodrag"
          type="number"
          min={3}
          max={10}
          step={1}
          value={draft.candidateCount}
          disabled={disabled || starting}
          aria-invalid={Boolean(validation.candidateCount)}
          aria-describedby={validation.candidateCount ? countErrorId : undefined}
          onChange={(event) => updateField('candidateCount', event.currentTarget.value)}
        />
        {validation.candidateCount ? <span className="topic-field-error" id={countErrorId}>{validation.candidateCount}</span> : null}
      </label>
      <div className="topic-save-row">
        <span role="status" aria-live="polite">{saveStatusText}</span>
        {saveState === 'error' ? (
          <button className="topic-retry-save nodrag" type="button" onClick={() => void retrySave()} disabled={disabled || starting}>
            重试保存
          </button>
        ) : null}
      </div>
      {saveError ? <p className="topic-save-error" id={saveErrorId} role="alert">保存失败：{saveError}</p> : null}
      {runFailure ? <p className="topic-run-error" id={runErrorId} role="alert">{formatRunStartError(runFailure)}</p> : null}
      <button
        className="topic-run-button nodrag"
        type="button"
        disabled={runDisabled}
        aria-describedby={runFailure ? runErrorId : undefined}
        onClick={() => void startRun()}
      >
        {starting ? '正在启动…' : globalRunBusy ? '已有运行' : node.locked ? '已锁定' : '生成候选'}
      </button>
    </div>
  )
}
