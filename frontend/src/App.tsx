import { useCallback, useEffect, useState } from 'react'
import {
  cancelResume,
  interrupt,
  newSession,
  respondPermission,
  respondQuestion,
  sendMessage,
  type AgentState,
} from './api/agent'
import {
  fetchEnvironment,
  fetchRecentLogs,
  type EnvironmentDTO,
  type LogEntryDTO,
} from './api/system'
import { useAgentBridge } from './hooks/useAgentBridge'
import { useAgentStore } from './stores/agentStore'
import { AboutDialog } from './ui/AboutDialog'
import { AppShell } from './ui/AppShell'
import { SessionPage } from './ui/SessionPage'
import { SystemDrawer } from './ui/SystemDrawer'
import { WorkbenchTopbar } from './ui/WorkbenchTopbar'

function messageOf(error: unknown) {
  return error instanceof Error ? error.message : String(error)
}

function App() {
  useAgentBridge()
  const agentState = useAgentStore((store) => store.state)
  const [env, setEnv] = useState<EnvironmentDTO | null>(null)
  const [logs, setLogs] = useState<LogEntryDTO[]>([])
  const [systemOpen, setSystemOpen] = useState(false)
  const [aboutOpen, setAboutOpen] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const [logsLoading, setLogsLoading] = useState(false)
  const [error, setError] = useState('')
  const [logError, setLogError] = useState('')

  const loadEnv = useCallback(async (refresh: boolean) => {
    if (refresh) setRefreshing(true)
    try {
      setEnv(await fetchEnvironment(refresh))
      setError('')
    } catch (err) {
      setError(`环境检查失败：${messageOf(err)}`)
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

  const send = useCallback(async (text: string) => {
    setError('')
    try {
      await sendMessage(text)
    } catch (err) {
      setError(`发送失败：${messageOf(err)}`)
      throw err
    }
  }, [])

  const onPermission = useCallback(async (id: string, decision: 'allow' | 'allow_session' | 'deny') => {
    setError('')
    try {
      await respondPermission(id, decision)
    } catch (err) {
      setError(`权限响应失败：${messageOf(err)}`)
    }
  }, [])

  const onQuestion = useCallback(async (id: string, answer: string, via: string) => {
    setError('')
    try {
      await respondQuestion(id, answer, via)
    } catch (err) {
      setError(`问题回答失败：${messageOf(err)}`)
    }
  }, [])

  const onNewSession = useCallback(async () => {
    setError('')
    try {
      await newSession()
    } catch (err) {
      setError(`新开会话失败：${messageOf(err)}`)
    }
  }, [])

  const onCancelResume = useCallback(() => {
    void cancelResume().catch((err) => setError(`取消失败：${messageOf(err)}`))
  }, [])

  const onInterrupt = useCallback(async () => {
    try {
      await interrupt()
    } catch (err) {
      setError(`停止失败：${messageOf(err)}`)
    }
  }, [])

  const version = env?.version || '0.1.0'
  const state: AgentState | null = agentState

  return (
    <AppShell>
      <WorkbenchTopbar
        state={state}
        version={version}
        onAbout={() => setAboutOpen(true)}
        onSystem={() => setSystemOpen(true)}
      />
      <SessionPage
        state={state}
        error={error}
        onSend={send}
        onPermission={onPermission}
        onQuestion={onQuestion}
        onNewSession={onNewSession}
        onCancelResume={onCancelResume}
        onInterrupt={onInterrupt}
      />
      <SystemDrawer
        open={systemOpen}
        env={env}
        logs={logs}
        refreshing={refreshing}
        logsLoading={logsLoading}
        error={error}
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
