import { useEffect, useRef, useState, type FormEvent } from 'react'
import { listProjects } from '../api/workspace'
import type { ProjectSummary, WorkspaceProject } from '../../bindings/audio-ui/internal/domain/entity'

type Props = {
  project: WorkspaceProject | null
  blocked: boolean
  onNew: (name: string) => Promise<void>
  onOpen: (projectId: string) => Promise<void>
}

function messageOf(error: unknown): string {
  if (error instanceof Error) return error.message
  if (typeof error === 'string') return error
  if (error && typeof error === 'object' && 'message' in error) return String((error as { message: unknown }).message)
  return String(error)
}

function operationMessage(error: unknown): string {
  const message = messageOf(error)
  if (message.includes('BUSY')) return '已有运行在途，工程切换暂不可用（BUSY）。请等待运行结束后重试。'
  return `工程操作失败：${message}`
}

export function ProjectSwitcher({ project, blocked, onNew, onOpen }: Props) {
  const dialogRef = useRef<HTMLDialogElement>(null)
  const nameRef = useRef<HTMLInputElement>(null)
  const [open, setOpen] = useState(false)
  const [projects, setProjects] = useState<ProjectSummary[]>([])
  const [name, setName] = useState('')
  const [loadingList, setLoadingList] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    const dialog = dialogRef.current
    if (!dialog) return
    if (open && !dialog.open) dialog.showModal()
    if (!open && dialog.open) dialog.close()
    if (open) nameRef.current?.focus()
  }, [open])

  const reloadProjects = async () => {
    setLoadingList(true)
    setError('')
    try {
      setProjects(await listProjects())
    } catch (err) {
      setError(`读取工程列表失败：${messageOf(err)}`)
    } finally {
      setLoadingList(false)
    }
  }

  const showDialog = () => {
    if (blocked) return
    setOpen(true)
    void reloadProjects()
  }

  const close = () => {
    if (submitting) return
    setOpen(false)
    setError('')
  }

  const create = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const value = name.trim()
    if (!value || submitting || blocked) return
    setSubmitting(true)
    setError('')
    try {
      await onNew(value)
      setName('')
      setOpen(false)
    } catch (err) {
      setError(operationMessage(err))
    } finally {
      setSubmitting(false)
    }
  }

  const openProject = async (projectId: string) => {
    if (submitting || blocked || projectId === project?.id) return
    setSubmitting(true)
    setError('')
    try {
      await onOpen(projectId)
      setOpen(false)
    } catch (err) {
      setError(operationMessage(err))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <>
      <button
        className="project-switcher-trigger"
        type="button"
        disabled={blocked}
        onClick={showDialog}
        aria-haspopup="dialog"
        aria-expanded={open}
      >
        <span>{project?.name ?? '选择工程'}</span>
        <span aria-hidden="true">⌄</span>
      </button>
      <dialog
        ref={dialogRef}
        className="project-dialog"
        aria-labelledby="project-dialog-title"
        onCancel={(event) => {
          event.preventDefault()
          close()
        }}
        onClose={() => setOpen(false)}
        onClick={(event) => {
          if (event.target === event.currentTarget) close()
        }}
      >
        <div className="project-dialog-head">
          <div>
            <h2 id="project-dialog-title">工程</h2>
            <p>本机工程；运行在途时不能切换。</p>
          </div>
          <button type="button" className="agent-icon-btn" aria-label="关闭工程选择" onClick={close}>×</button>
        </div>
        <div className="project-dialog-body">
          {blocked ? <div className="workspace-inline-error" role="status">运行中，工程切换已禁用。</div> : null}
          {error ? <div className="workspace-inline-error" role="alert">{error}</div> : null}
          <div className="project-list-head">
            <span>最近工程</span>
            <button className="agent-mini-btn" type="button" disabled={loadingList || submitting} onClick={() => void reloadProjects()}>
              {loadingList ? '读取中…' : '刷新'}
            </button>
          </div>
          {loadingList ? <div className="agent-console-empty">正在读取工程列表…</div> : null}
          {!loadingList && projects.length === 0 ? <div className="agent-console-empty">还没有工程，请新建一个。</div> : null}
          <ul className="project-list">
            {projects.map((item) => (
              <li key={item.id}>
                <button
                  type="button"
                  className="project-list-item"
                  disabled={blocked || submitting || item.id === project?.id}
                  onClick={() => void openProject(item.id)}
                >
                  <span className="project-list-name">{item.name}</span>
                  <span className="project-list-meta">{item.id} · rev {item.revision}</span>
                  {item.id === project?.id ? <span className="project-current">当前</span> : null}
                </button>
              </li>
            ))}
          </ul>
          <form className="project-create-form" onSubmit={(event) => void create(event)}>
            <label htmlFor="project-name">新建工程</label>
            <div className="project-create-row">
              <input
                ref={nameRef}
                id="project-name"
                value={name}
                maxLength={80}
                placeholder="输入工程名称"
                disabled={blocked || submitting}
                onChange={(event) => setName(event.target.value)}
              />
              <button className="primary-btn" type="submit" disabled={blocked || submitting || !name.trim()}>
                {submitting ? '处理中…' : '新建'}
              </button>
            </div>
          </form>
        </div>
      </dialog>
    </>
  )
}
