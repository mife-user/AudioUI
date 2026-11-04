import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { PermissionCard } from './PermissionCard'
import type { PermissionRequest } from '../api/runs'
import { PermissionTier } from '../../bindings/audio-ui/internal/domain/valueobject'

const request: PermissionRequest = {
  id: 'p1', tool: 'Write', tier: PermissionTier.PermissionConfirm, risk: '写文件', summary: 'Write notes.md',
  paths: ['notes.md'], allowSession: true, expiresAt: new Date(Date.now() + 60000).toISOString(),
}

describe('PermissionCard', () => {
  it('returns allow decisions', async () => {
    const onRespond = vi.fn().mockResolvedValue(undefined)
    render(<PermissionCard request={request} onRespond={onRespond} />)
    fireEvent.click(screen.getByRole('button', { name: '允许一次' }))
    expect(onRespond).toHaveBeenCalledWith('p1', 'allow')
  })
})
