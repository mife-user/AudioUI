import { useCallback, useEffect, useState } from 'react'
import { fetchEnvironment, fetchRecentLogs, type EnvironmentDTO, type LogEntryDTO } from './api/system'
import type { GraphMutationReceipt } from '../bindings/audio-ui/internal/domain/entity'
import { useWorkspaceBridge } from './hooks/useWorkspaceBridge'
import { useRunStore } from './stores/runStore'
import { useWorkspaceStore } from './stores/workspaceStore'
import { AboutDialog } from './ui/AboutDialog'
import { AgentConsole } from './ui/AgentConsole'
import { CLIInstallBanner } from './ui/CLIInstallBanner'
import { AppShell } from './ui/AppShell'
import { SystemDrawer } from './ui/SystemDrawer'
import { WorkbenchTopbar } from './ui/WorkbenchTopbar'
import { WorkspacePage } from './ui/WorkspacePage'

function messageOf(error: unknown): string {
  if (error instanceof Error) return error.message
  if (typeof error === 'string') return error
  if (error && typeof error === 'object' && 'message' in error) return String((error as { message: unknown }).message)
  return String(error)
}

function App() {
  const bridge = useWorkspaceBridge()
  const workspaceStatus = useWorkspaceStore((store) => store.status)
  const workspaceSnapshot = useWorkspaceStore((store) => store.snapshot)
  const workspaceError = useWorkspaceStore((store) => store.error)
  const runViews = useRunStore((store) => store.runs)
  const runSummaries = useRunStore((store) => store.summaries)
  const selectedRunId = useRunStore((store) => store.selectedRunId)
  const selectedRun = useRunStore((store) => selectedRunId ? store.runs[selectedRunId] : undefined)
  const [env, setEnv] = useState<EnvironmentDTO | null>(null)
  const [logs, setLogs] = useState<LogEntryDTO[]>([])
  const [systemOpen, setSystemOpen] = useState(false)
  const [aboutOpen, setAboutOpen] = useState(false)
  const [consoleCollapsed, setConsoleCollapsed] = useState(false)
  const [cliBannerDismissed, setCliBannerDismissed] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const [logsLoading, setLogsLoading] = useState(false)
  const [environmentError, setEnvironmentError] = useState('')
  const [logError, setLogError] = useState('')
  const [consoleError, setConsoleError] = useState('')

  const loadEnv = useCallback(async (refresh: boolean) => {
    if (refresh) setRefreshing(true)
    try {
      setEnv(await fetchEnvironment(refresh))
      setEnvironmentError('')
    } catch (err) {
      setEnvironmentError(`环境检查失败：${messageOf(err)}`)
    } finally {
      setRefreshing(false)
    }
  }, [])

  const loadLogs = useCallback(async () => {
    setLogsLoading(true)
    try {
      setLogs(await fetchRecentLogs(150))
      setLogError('')
    } catch (err) {
      setLogError(`读取日志失败：${messageOf(err)}`)
    } finally {
      setLogsLoading(false)
    }
  }, [])

  useEffect(() => {
    void loadEnv(false)
  }, [loadEnv])

  useEffect(() => {
    if (!systemOpen) return
    void loadLogs()
    const timer = window.setInterval(() => {
      if (document.visibilityState === 'visible') void loadLogs()
    }, 5000)
    return () => window.clearInterval(timer)
  }, [systemOpen, loadLogs])

  const createProject = useCallback((name: string) => bridge.createProject(name), [bridge])
  const openProject = useCallback((projectId: string) => bridge.openProject(projectId), [bridge])
  const refreshGraph = useCallback((receipt: GraphMutationReceipt) => bridge.syncGraphMutation(receipt), [bridge])
  const startNodeRun = useCallback((nodeId: string, projectId: string) => bridge.startRun(nodeId, projectId), [bridge])
  const onSelectRun = useCallback((runId: string) => bridge.selectRun(runId), [bridge])
  const onRetryRun = useCallback((runId: string) => bridge.retryRun(runId), [bridge])

  const onPermission = useCallback(async (runId: string, requestId: string, decision: 'allow' | 'allow_session' | 'deny') => {
    setConsoleError('')
    try {
      await bridge.respondPermission(runId, requestId, decision)
    } catch (err) {
      setConsoleError(`权限响应失败：${messageOf(err)}`)
    }
  }, [bridge])

  const onQuestion = useCallback(async (runId: string, requestId: string, answer: string, via: string) => {
    setConsoleError('')
    try {
      await bridge.respondQuestion(runId, requestId, answer, via)
    } catch (err) {
      setConsoleError(`问题回答失败：${messageOf(err)}`)
    }
  }, [bridge])

  const onInterrupt = useCallback(async (runId: string) => {
    setConsoleError('')
    try {
      await bridge.interrupt(runId)
    } catch (err) {
      setConsoleError(`停止运行失败：${messageOf(err)}`)
    }
  }, [bridge])

  const isBusy = workspaceSnapshot?.runs?.some((run) => run.status === 'starting' || run.status === 'running') ?? false
  const version = env?.version || 'dev'

  return (
    <AppShell>
      <WorkbenchTopbar
        project={workspaceSnapshot?.project ?? null}
        environment={env}
        busy={isBusy}
        version={version}
        onNewProject={createProject}
        onOpenProject={openProject}
        onAbout={() => setAboutOpen(true)}
        onSystem={() => setSystemOpen(true)}
      />
      {environmentError ? <div className="workspace-banner" role="alert">{environmentError}</div> : null}
      {env && !env.cli.installed && !cliBannerDismissed ? <CLIInstallBanner onDismiss={() => setCliBannerDismissed(true)} /> : null}
      <div className="workspace-layout">
        <WorkspacePage
          status={workspaceStatus}
          snapshot={workspaceSnapshot}
          error={workspaceError}
          onRetry={() => bridge.retryWorkspace()}
          onGraphMutation={refreshGraph}
          onStartRun={startNodeRun}
        />
        <AgentConsole
          runs={runSummaries}
          runViews={runViews}
          selectedRunId={selectedRunId}
          selectedRun={selectedRun}
          actionError={consoleError}
          collapsed={consoleCollapsed}
          onToggle={() => setConsoleCollapsed((value) => !value)}
          onSelectRun={onSelectRun}
          onRetryRun={onRetryRun}
          onPermission={onPermission}
          onQuestion={onQuestion}
          onInterrupt={onInterrupt}
        />
      </div>
      <SystemDrawer
        open={systemOpen}
        env={env}
        logs={logs}
        refreshing={refreshing}
        logsLoading={logsLoading}
        error={environmentError}
        logError={logError}
        onRefreshEnv={() => void loadEnv(true)}
        onRefreshLogs={() => void loadLogs()}
        onClose={() => setSystemOpen(false)}
      />
      <AboutDialog open={aboutOpen} env={env} onClose={() => setAboutOpen(false)} />
    </AppShell>
  )
}

export default App
