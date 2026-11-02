import { copy } from './copy'

type Props = {
  onDismiss: () => void
}

export function CLIInstallBanner({ onDismiss }: Props) {
  return (
    <div className="banner" role="status">
      <span>
        <strong>{copy.cliMissingTitle}</strong> {copy.cliMissingBody}
      </span>
      <code>{copy.cliInstallCommand}</code>
      <span>{copy.cliDocsUrl}</span>
      <button className="icon-btn banner-action" title="关闭提示" aria-label="关闭提示" onClick={onDismiss}>
        ×
      </button>
    </div>
  )
}
