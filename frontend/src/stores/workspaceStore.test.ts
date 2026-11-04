import { beforeEach, describe, expect, it } from 'vitest'
import { useWorkspaceStore } from './workspaceStore'

function result(revision: number, projectId: string | null) {
  return {
    revision,
    snapshot: {
      project: projectId ? { id: projectId, name: projectId, revision, activeNodeId: '' } : null,
      nodes: [], pending: [], runs: [], chains: [], lockedNodeIds: [],
    },
  }
}

beforeEach(() => useWorkspaceStore.getState().reset())

describe('workspaceStore', () => {
  it('accepts a consistent snapshot and treats its revision as process-local', () => {
    expect(useWorkspaceStore.getState().applySnapshot(result(8, 'p_one'))).toBe(true)
    expect(useWorkspaceStore.getState().revision).toBe(8)
    expect(useWorkspaceStore.getState().snapshot?.project?.revision).toBe(8)
  })

  it('rejects an older snapshot for the same project', () => {
    useWorkspaceStore.getState().applySnapshot(result(8, 'p_one'))
    expect(useWorkspaceStore.getState().applySnapshot(result(7, 'p_one'))).toBe(false)
    expect(useWorkspaceStore.getState().revision).toBe(8)
  })

  it('supports the no-active-project empty snapshot', () => {
    useWorkspaceStore.getState().applySnapshot(result(0, null))
    expect(useWorkspaceStore.getState().status).toBe('ready')
    expect(useWorkspaceStore.getState().snapshot?.project).toBeNull()
    expect(useWorkspaceStore.getState().snapshot?.nodes).toEqual([])
  })
})
