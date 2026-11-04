import type { EnvironmentDTO } from '../api/system'
import type { WorkspaceProject } from '../../bindings/audio-ui/internal/domain/entity'
import { ProjectSwitcher } from './ProjectSwitcher'
import { copy } from './copy'

type Props = {
  project: WorkspaceProject | null
  environment: EnvironmentDTO | null
  busy: boolean
  version: string
  onNewProject: (name: string) => Promise<void>
  onOpenProject: (projectId: string) => Promise<void>
  onAbout: () => void
  onSystem: () => void
}

function statusOf(environment: EnvironmentDTO | null, busy: boolean) {
  if (!environment) return { tone: 'warn', label: '环境检查中' }
  if (!environment.cli.installed) return { tone: 'bad', label: 'Agent 不可用' }
  if (busy) return { tone: 'ok', label: '工作中' }
  return { tone: 'ok', label: 'Agent 空闲' }
}

export function WorkbenchTopbar({ project, environment, busy, version, onNewProject, onOpenProject, onAbout, onSystem }: Props) {
  const status = statusOf(environment, busy)
  const toneClass = status.tone === 'ok' ? '' : ` ${status.tone === 'warn' ? 'warn' : 'bad'}`
  return (
    <header className="topbar">
      <div className="brand">
        <span className="brand-mark" aria-hidden="true">AU</span>
        <span>{copy.brand}</span>
        <span className="brand-sub">{copy.brandSub}</span>
      </div>
      <ProjectSwitcher
        project={project}
        blocked={busy}
        onNew={onNewProject}
        onOpen={onOpenProject}
      />
      <div className="topbar-spacer" />
      <div className="env-state" role="status" aria-live="polite">
        <span className={`dot${toneClass}`} />
        <span>{status.label}</span>
      </div>
      <button className="version-badge" onClick={onSystem}>系统</button>
      <button className="version-badge" onClick={onAbout} aria-haspopup="dialog">
        v{version || 'dev'}
      </button>
    </header>
  )
}
