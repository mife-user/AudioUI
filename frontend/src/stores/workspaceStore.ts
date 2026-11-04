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
  setActiveNode: (nodeId: string) => void
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
      const currentProject = current.snapshot?.project
      const incomingProject = result.snapshot.project
      const currentProjectId = currentProject?.id ?? null
      const incomingProjectId = incomingProject?.id ?? null
      if (currentProjectId === incomingProjectId && result.revision < current.revision) {
        accepted = false
        return current
      }
      let snapshot: WorkspaceSnapshot = result.snapshot
      if (currentProject && incomingProject && currentProject.id === incomingProject.id) {
        snapshot = {
          ...result.snapshot,
          project: {
            ...incomingProject,
            activeNodeId: currentProject.activeNodeId,
          },
        }
      }
      return {
        status: 'ready',
        revision: result.revision,
        snapshot,
        error: '',
        setLoading: current.setLoading,
        applySnapshot: current.applySnapshot,
        setActiveNode: current.setActiveNode,
        setError: current.setError,
        reset: current.reset,
      }
    })
    return accepted
  },
  setActiveNode: (nodeId) => set((current) => {
    if (!current.snapshot?.project || current.snapshot.project.activeNodeId === nodeId) return current
    return {
      ...current,
      snapshot: {
        ...current.snapshot,
        project: { ...current.snapshot.project, activeNodeId: nodeId },
      },
    }
  }),
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
