import { useEffect } from 'react'
import { getAgentState, subscribeAgentEvents } from '../api/agent'
import { useAgentStore } from '../stores/agentStore'

export function useAgentBridge() {
  const setSnapshot = useAgentStore((store) => store.setSnapshot)
  const applyEvent = useAgentStore((store) => store.applyEvent)
  const needsResync = useAgentStore((store) => store.needsResync)

  useEffect(() => {
    let active = true
    const refresh = () => {
      void getAgentState()
        .then((state) => {
          if (active) setSnapshot(state)
        })
        .catch(() => undefined)
    }
    refresh()
    const unsubscribe = subscribeAgentEvents((event) => {
      if (!active) return
      applyEvent(event)
    })
    return () => {
      active = false
      unsubscribe()
    }
  }, [applyEvent, setSnapshot])

  useEffect(() => {
    if (!needsResync) return
    void getAgentState()
      .then((state) => setSnapshot(state))
      .catch(() => undefined)
  }, [needsResync, setSnapshot])
}
