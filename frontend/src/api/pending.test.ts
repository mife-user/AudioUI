import { describe, expect, it } from 'vitest'
import { decodePendingPayload } from './pending'

function encodeUtf8(value: string): string {
  return btoa(Array.from(new TextEncoder().encode(value), (byte) => String.fromCharCode(byte)).join(''))
}

describe('pending payload decoding', () => {
  it('decodes Wails byte slices from base64 as UTF-8 text', () => {
    const payload = '{"topics":[{"title":"凌晨面包房"}]}'
    expect(decodePendingPayload(encodeUtf8(payload))).toBe(payload)
  })

  it('returns an empty string for an absent payload', () => {
    expect(decodePendingPayload(null)).toBe('')
  })
})
