import { describe, it, expect } from 'vitest'
import type { Session } from '../types'

/**
 * Pure logic extracted from App.handleDelete:
 * After deleting a session, pick the next session to select.
 */
function selectAfterDelete(sessions: Pick<Session, 'name' | 'status'>[], deletedName: string): string | null {
  const idx = sessions.findIndex((s) => s.name === deletedName)
  const remaining = sessions.filter((s) => s.name !== deletedName)
  if (remaining.length === 0) return null
  const nextIdx = Math.min(idx, remaining.length - 1)
  return remaining[nextIdx].name
}

describe('selectAfterDelete', () => {
  const sessions: Pick<Session, 'name' | 'status'>[] = [
    { name: 'a', status: 'idle' },
    { name: 'b', status: 'idle' },
    { name: 'c', status: 'idle' },
  ]

  it('selects next session when deleting from the middle', () => {
    expect(selectAfterDelete(sessions, 'b')).toBe('c')
  })

  it('selects previous session when deleting the last one', () => {
    expect(selectAfterDelete(sessions, 'c')).toBe('b')
  })

  it('selects next session when deleting the first one', () => {
    expect(selectAfterDelete(sessions, 'a')).toBe('b')
  })

  it('returns null when deleting the only session', () => {
    expect(selectAfterDelete([{ name: 'x', status: 'idle' }], 'x')).toBeNull()
  })

  it('selects the only remaining session', () => {
    const two: Pick<Session, 'name' | 'status'>[] = [
      { name: 'a', status: 'idle' },
      { name: 'b', status: 'idle' },
    ]
    expect(selectAfterDelete(two, 'a')).toBe('b')
    expect(selectAfterDelete(two, 'b')).toBe('a')
  })
})
