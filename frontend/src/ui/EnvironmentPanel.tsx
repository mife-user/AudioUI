import type { EnvironmentDTO } from '../api/system'
import { copy } from './copy'

type Props = {
  env: EnvironmentDTO
  refreshing: boolean
  onRefresh: () => void
}

function valueOrDash(value: string) {
  return value.trim() === '' ? '—' : value
}

function checkedAt(value: string) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return `最近检测 ${date.toLocaleTimeString('zh-CN', { hour12: false })}`
}

export function EnvironmentPanel({ env, refreshing, onRefresh }: Props) {
  const cliState = !env.cli.installed
    ? '未安装'
    : env.cli.error
      ? `已安装 · ${env.cli.error}`
      : `可用 · ${valueOrDash(env.cli.version)}`
  const webviewState = env.webview2.installed
    ? `可用 · ${valueOrDash(env.webview2.version)}`
    : '未安装'

  return (
    <section className="card" aria-labelledby="env-title">
      <div className="card-head">
        <span className="card-title" id="env-title">{copy.envTitle}</span>
        <span className="card-sub">{checkedAt(env.checkedAt)}</span>
        <div className="card-actions">
          <button className="version-badge" onClick={onRefresh} disabled={refreshing}>
            {refreshing ? copy.refreshing : copy.refresh}
          </button>
        </div>
      </div>
      <div className="meta">
        <div className="meta-row">
          <span className="meta-key">Claude CLI</span>
          <span className="meta-value mono">{valueOrDash(env.cli.path)}</span>
          <span className={`meta-state${!env.cli.installed || env.cli.error ? ' warn' : ''}`}>{cliState}</span>
        </div>
        <div className="meta-row">
          <span className="meta-key">WebView2</span>
          <span className="meta-value mono">{valueOrDash(env.webview2.path)}</span>
          <span className={`meta-state${env.webview2.installed ? '' : ' bad'}`}>{webviewState}</span>
        </div>
        <div className="meta-row">
          <span className="meta-key">程序目录</span>
          <span className="meta-value mono">{valueOrDash(env.layout.root)}</span>
          <span className="meta-state">{env.layout.portable ? '便携' : '开发覆盖'}</span>
        </div>
        <div className="meta-row">
          <span className="meta-key">数据目录</span>
          <span className="meta-value mono">{valueOrDash(env.layout.dataDir)}</span>
          <span className="meta-state">已创建</span>
        </div>
        <div className="meta-row">
          <span className="meta-key">日志</span>
          <span className="meta-value mono">{valueOrDash(env.layout.logFile)}</span>
          <span className="meta-state">已创建</span>
        </div>
        <div className="meta-row">
          <span className="meta-key">WebView2 数据</span>
          <span className="meta-value mono">{valueOrDash(env.layout.webview2Dir)}</span>
          <span className="meta-state">已重定向</span>
        </div>
      </div>
    </section>
  )
}
