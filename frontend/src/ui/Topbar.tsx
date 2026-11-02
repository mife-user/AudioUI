import { copy } from './copy'

type Props = {
  version: string
  statusTone: 'ok' | 'warn' | 'bad'
  statusLabel: string
  refreshing: boolean
  onRefresh: () => void
  onAbout: () => void
}

export function Topbar({ version, statusTone, statusLabel, refreshing, onRefresh, onAbout }: Props) {
  const toneClass = statusTone === 'ok' ? '' : ` ${statusTone === 'warn' ? 'warn' : 'bad'}`
  return (
    <header className="topbar">
      <div className="brand">
        <span className="brand-mark" aria-hidden="true">AU</span>
        <span>{copy.brand}</span>
        <span className="brand-sub">{copy.brandSub}</span>
      </div>
      <div className="topbar-spacer" />
      <div className="env-state">
        <span className={`dot${toneClass}`} />
        <span>{statusLabel}</span>
      </div>
      <button className="version-badge" onClick={onAbout} aria-haspopup="dialog">
        {version && version !== '—' ? `v${version}` : 'dev'}
      </button>
      <button
        className="icon-btn"
        title={refreshing ? copy.refreshing : copy.refresh}
        aria-label={copy.refresh}
        onClick={onRefresh}
        disabled={refreshing}
      >
        <svg
          className={refreshing ? 'spin' : undefined}
          width="14"
          height="14"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.8"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
        >
          <path d="M21 12a9 9 0 1 1-2.64-6.36" />
          <path d="M21 3v6h-6" />
        </svg>
      </button>
    </header>
  )
}
