import { useEffect, useRef, useState } from 'react'
import type { Node, NodeParams } from '../../bindings/audio-ui/internal/domain/entity'

const PARAM_SAVE_DEBOUNCE_MS = 320

type PromptDraft = {
  title: string
  instruction: string
  scenes: string
  duration: string
}

type SaveState = 'saved' | 'dirty' | 'saving' | 'error' | 'sync'

type Props = {
  node: Node
  projectId: string
  disabled: boolean
  onSave: (nodeId: string, title: string, params: NodeParams, projectId: string) => Promise<boolean | void>
  onRegisterFlush?: (nodeId: string, projectId: string, flush: () => Promise<boolean>) => () => void
}

function draftFromNode(node: Node): PromptDraft {
  return {
    title: node.title,
    instruction: node.params?.instruction ?? '',
    scenes: node.params?.constraints?.scenes === undefined ? '' : String(node.params.constraints.scenes),
    duration: node.params?.constraints?.duration === undefined ? '' : String(node.params.constraints.duration),
  }
}

function draftError(draft: PromptDraft): string {
  if (!draft.title.trim()) return '提示词标题不能为空。'
  if (draft.scenes && !/^\d+$/.test(draft.scenes)) return '场景数必须是非负整数。'
  if (draft.duration && !/^\d+$/.test(draft.duration)) return '总时长必须是非负整数。'
  return ''
}

function paramsFromDraft(draft: PromptDraft): NodeParams {
  const constraints: NonNullable<NodeParams['constraints']> = {}
  if (draft.scenes) constraints.scenes = Number(draft.scenes)
  if (draft.duration) constraints.duration = Number(draft.duration)
  return {
    instruction: draft.instruction,
    ...(Object.keys(constraints).length ? { constraints } : {}),
  }
}

function errorMessage(error: unknown): string {
  if (error instanceof Error) return error.message
  if (typeof error === 'string') return error
  if (error && typeof error === 'object' && 'message' in error) return String((error as { message: unknown }).message)
  return String(error)
}

export function PromptParameters({ node, projectId, disabled, onSave, onRegisterFlush }: Props) {
  const initialDraft = draftFromNode(node)
  const [draft, setDraft] = useState(initialDraft)
  const [saveState, setSaveState] = useState<SaveState>('saved')
  const [saveError, setSaveError] = useState('')
  const latestDraft = useRef(initialDraft)
  const editVersion = useRef(0)
  const savedVersion = useRef(0)
  const saveQueue = useRef(Promise.resolve())
  const activeSave = useRef<{ version: number; promise: Promise<void> } | null>(null)
  const saveTimer = useRef<number | null>(null)
  const syncPending = useRef(false)
  const identity = useRef({ projectId, nodeId: node.id })
  const currentProjectId = useRef(projectId)
  const currentNodeId = useRef(node.id)
  const saveCallback = useRef(onSave)
  const registerCallback = useRef(onRegisterFlush)
  const flushCallback = useRef<() => Promise<boolean>>(async () => true)
  currentProjectId.current = projectId
  currentNodeId.current = node.id
  saveCallback.current = onSave
  registerCallback.current = onRegisterFlush

  const validationError = draftError(draft)
  const titleErrorId = `prompt-title-error-${node.id}`
  const taskErrorId = `prompt-task-error-${node.id}`
  const saveErrorId = `prompt-save-error-${node.id}`

  function clearSaveTimer() {
    if (saveTimer.current === null) return
    window.clearTimeout(saveTimer.current)
    saveTimer.current = null
  }

  function saveVersion(version: number, value: PromptDraft): Promise<void> {
    if (activeSave.current?.version === version) return activeSave.current.promise

    const operation = saveQueue.current.catch(() => undefined).then(async () => {
      if (currentProjectId.current !== projectId || currentNodeId.current !== node.id) {
        throw new Error('PROJECT_CHANGED: 工程已切换，未保存旧工程参数。')
      }
      if (version !== editVersion.current) return
      const error = draftError(value)
      if (error) throw new Error(error)

      setSaveState('saving')
      setSaveError('')
      const synced = await saveCallback.current(node.id, value.title.trim(), paramsFromDraft(value), projectId)
      if (version === editVersion.current && currentProjectId.current === projectId && currentNodeId.current === node.id) {
        savedVersion.current = version
        syncPending.current = synced === false
        setSaveState(syncPending.current ? 'sync' : 'saved')
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
      const error = draftError(value)
      if (error) throw new Error(error)
      await saveVersion(version, value)
    }
    return !syncPending.current
  }

  flushCallback.current = flushSave

  function updateField(field: keyof PromptDraft, value: string) {
    const next = { ...latestDraft.current, [field]: value }
    latestDraft.current = next
    setDraft(next)
    editVersion.current += 1
    setSaveState('dirty')
    setSaveError('')
    clearSaveTimer()
    if (draftError(next)) return

    const version = editVersion.current
    saveTimer.current = window.setTimeout(() => {
      saveTimer.current = null
      void saveVersion(version, next).catch(() => undefined)
    }, PARAM_SAVE_DEBOUNCE_MS)
  }

  async function retrySave() {
    try {
      const synced = await flushSave()
      if (!synced) setSaveState('sync')
    } catch {
      return
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
      const next = draftFromNode(node)
      latestDraft.current = next
      editVersion.current = 0
      savedVersion.current = 0
      saveQueue.current = Promise.resolve()
      activeSave.current = null
      syncPending.current = false
      setDraft(next)
      setSaveState('saved')
      setSaveError('')
      return
    }

    if (savedVersion.current >= editVersion.current) {
      const next = draftFromNode(node)
      latestDraft.current = next
      setDraft(next)
      setSaveState('saved')
    }
  }, [projectId, node.id, node.title, node.params])

  useEffect(() => {
    if (!syncPending.current || disabled) return
    syncPending.current = false
    setSaveState('saved')
  }, [disabled])

  useEffect(() => () => clearSaveTimer(), [])

  const statusText = saveState === 'sync'
    ? '已提交，等待快照同步…'
    : saveState === 'saving'
      ? '保存中…'
      : saveState === 'error'
        ? '保存失败，修改仍保留'
        : saveState === 'dirty'
          ? '未保存'
          : '已保存'

  return (
    <div className="prompt-parameters">
      <label className="topic-field" htmlFor={`prompt-title-${node.id}`}>
        <span>标题</span>
        <input
          id={`prompt-title-${node.id}`}
          className="nodrag"
          type="text"
          value={draft.title}
          maxLength={80}
          required
          disabled={disabled}
          aria-invalid={Boolean(draft.title.trim() === '')}
          aria-describedby={draft.title.trim() === '' ? titleErrorId : undefined}
          onChange={(event) => updateField('title', event.currentTarget.value)}
        />
        {draft.title.trim() === '' ? <span className="topic-field-error" id={titleErrorId}>提示词标题不能为空。</span> : null}
      </label>
      <label className="topic-field" htmlFor={`prompt-instruction-${node.id}`}>
        <span>任务</span>
        <textarea
          id={`prompt-instruction-${node.id}`}
          className="nodrag nowheel"
          rows={4}
          value={draft.instruction}
          disabled={disabled}
          aria-describedby={validationError && draft.title.trim() ? taskErrorId : undefined}
          onChange={(event) => updateField('instruction', event.currentTarget.value)}
        />
        {validationError && draft.title.trim() ? <span className="topic-field-error" id={taskErrorId}>{validationError}</span> : null}
      </label>
      <div className="prompt-constraints">
        <label className="topic-field" htmlFor={`prompt-scenes-${node.id}`}>
          <span>场景数约束</span>
          <input
            id={`prompt-scenes-${node.id}`}
            className="nodrag"
            type="number"
            min={0}
            step={1}
            value={draft.scenes}
            disabled={disabled}
            aria-invalid={Boolean(draft.scenes && !/^\d+$/.test(draft.scenes))}
            onChange={(event) => updateField('scenes', event.currentTarget.value)}
          />
        </label>
        <label className="topic-field" htmlFor={`prompt-duration-${node.id}`}>
          <span>总时长约束（秒）</span>
          <input
            id={`prompt-duration-${node.id}`}
            className="nodrag"
            type="number"
            min={0}
            step={1}
            value={draft.duration}
            disabled={disabled}
            aria-invalid={Boolean(draft.duration && !/^\d+$/.test(draft.duration))}
            onChange={(event) => updateField('duration', event.currentTarget.value)}
          />
        </label>
      </div>
      <div className="topic-save-row">
        <span role="status" aria-live="polite">{statusText}</span>
        {saveState === 'error' ? (
          <button className="topic-retry-save nodrag" type="button" onClick={() => void retrySave()} disabled={disabled}>重试保存</button>
        ) : null}
      </div>
      {saveError ? <p className="topic-save-error" id={saveErrorId} role="alert">保存失败：{saveError}</p> : null}
    </div>
  )
}
