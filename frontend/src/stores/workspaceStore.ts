import { create } from 'zustand'
import type { WorkspaceSnapshot, WorkspaceSnapshotResult } from '../../bindings/audio-ui/internal/domain/entity'
import type { WorkspaceStatus } from '../api/workspace'

export type WorkspaceStoreState = {
  status: WorkspaceStatus
  revision: number
  snapshot: WorkspaceSnapshot | null
  error: string
  setLoading: () => void
  applySnapshot: (result: WorkspaceSnapshotResult) => boolean
  setError: (message: string) => void
  reset: () => void
}

export const useWorkspaceStore = create<WorkspaceStoreState>((set) => ({
  status: 'idle',
  revision: 0,
  snapshot: null,
  error: '',
  setLoading: () => set((current) => ({
    ...current,
    status: 'loading',
    error: '',
  })),
  applySnapshot: (result) => {
    let accepted = true
    set((current) => {
      const currentProjectId = current.snapshot?.project?.id ?? null
      const incomingProjectId = result.snapshot.project?.id ?? null
      if (currentProjectId === incomingProjectId && result.revision < current.revision) {
        accepted = false
        return current
      }
      return {
        status: 'ready',
        revision: result.revision,
        snapshot: result.snapshot,
        error: '',
        setLoading: current.setLoading,
        applySnapshot: current.applySnapshot,
        setError: current.setError,
        reset: current.reset,
      }
    })
    return accepted
  },
  setError: (message) => set((current) => ({
    ...current,
    status: 'error',
    error: message,
  })),
  reset: () => set((current) => ({
    ...current,
    status: 'idle',
    revision: 0,
    snapshot: null,
    error: '',
  })),
}))
