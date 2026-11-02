import { useEffect, useRef } from 'react'
import type { EnvironmentDTO } from '../api/system'
import { copy } from './copy'

type Props = {
  open: boolean
  env: EnvironmentDTO | null
  onClose: () => void
}

function valueOrDash(value: string | undefined) {
  return value && value.trim() !== '' ? value : '—'
}

export function AboutDialog({ open, env, onClose }: Props) {
  const dialogRef = useRef<HTMLDialogElement>(null)

  useEffect(() => {
    const dialog = dialogRef.current
    if (!dialog) return
    if (open && !dialog.open) dialog.showModal()
    if (!open && dialog.open) dialog.close()
  }, [open])

  return (
    <dialog
      ref={dialogRef}
      className="dialog"
      aria-labelledby="about-title"
      onCancel={(event) => {
        event.preventDefault()
        onClose()
      }}
      onClose={onClose}
    >
      <div className="dialog-head">
        <span className="dialog-title" id="about-title">{copy.aboutTitle}</span>
        <div className="card-actions">
          <button className="icon-btn" aria-label={copy.close} onClick={onClose}>×</button>
        </div>
      </div>
      <div className="dialog-body">
        <div className="dialog-row">
          <span className="dialog-key">版本</span>
          <span className="dialog-value">{valueOrDash(env?.version)}</span>
        </div>
        <div className="dialog-row">
          <span className="dialog-key">构建</span>
          <span className="dialog-value">
            wails {valueOrDash(env?.build.wailsVersion)} · {valueOrDash(env?.build.goVersion)} ·{' '}
            {valueOrDash(env?.build.os)}/{valueOrDash(env?.build.arch)}
          </span>
        </div>
        <div className="dialog-row">
          <span className="dialog-key">程序目录</span>
          <span className="dialog-value">{valueOrDash(env?.layout.root)}</span>
        </div>
        <div className="dialog-row">
          <span className="dialog-key">日志</span>
          <span className="dialog-value">{valueOrDash(env?.layout.logFile)}</span>
        </div>
        <div className="dialog-row">
          <span className="dialog-key">Claude CLI</span>
          <span className="dialog-value">
            {env?.cli.installed ? valueOrDash(env.cli.version) : '未安装'}
          </span>
        </div>
        <div className="dialog-row">
          <span className="dialog-key">WebView2</span>
          <span className="dialog-value">
            {env?.webview2.installed ? valueOrDash(env.webview2.version) : '未安装'}
          </span>
        </div>
      </div>
      <div className="dialog-foot">
        <button className="primary-btn" onClick={onClose}>{copy.close}</button>
      </div>
    </dialog>
  )
}
