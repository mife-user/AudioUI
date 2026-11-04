import { useEffect, useState } from 'react'
import { WorkspaceBridge } from './workspaceBridge'

export function useWorkspaceBridge(): WorkspaceBridge {
  const [bridge] = useState(() => new WorkspaceBridge())

  useEffect(() => {
    bridge.start()
    return () => bridge.stop()
  }, [bridge])

  return bridge
}
