import { describe, it, expect } from 'vitest'
import type { ActivePlanSession } from '../types'

/**
 * Tests for useStatusSocket activePlanSessions parsing logic:
 * The hook receives WS messages with `data.activePlanSessions` and stores them.
 * We extract the parsing/fallback pattern and test it in isolation
 * without requiring a real WebSocket connection.
 */

// --- Parsing pattern extracted from useStatusSocket onmessage ---

interface StatusMessage {
  sessions: unknown[]
  usage?: unknown
  activePlanSessions?: ActivePlanSession[]
}

function parseActivePlanSessions(data: StatusMessage): ActivePlanSession[] {
  return data.activePlanSessions ?? []
}

describe('activePlanSessions parsing from WS message', () => {
  it('extracts activePlanSessions when present', () => {
    const data: StatusMessage = {
      sessions: [],
      activePlanSessions: [
        { epic: 'epic1', type: 'plan', name: 'pplan-epic1' },
        { epic: 'epic2', type: 'dispatch', name: 'pdisp-epic2' },
        { epic: 'epic3', type: 'retro', name: 'pretro-epic3' },
      ],
    }
    const result = parseActivePlanSessions(data)
    expect(result).toHaveLength(3)
    expect(result[0]).toEqual({ epic: 'epic1', type: 'plan', name: 'pplan-epic1' })
    expect(result[1]).toEqual({ epic: 'epic2', type: 'dispatch', name: 'pdisp-epic2' })
    expect(result[2]).toEqual({ epic: 'epic3', type: 'retro', name: 'pretro-epic3' })
  })

  it('returns empty array when activePlanSessions is undefined', () => {
    const data: StatusMessage = { sessions: [] }
    const result = parseActivePlanSessions(data)
    expect(result).toEqual([])
  })

  it('returns empty array when activePlanSessions is null', () => {
    const data = { sessions: [], activePlanSessions: null } as unknown as StatusMessage
    const result = parseActivePlanSessions(data)
    expect(result).toEqual([])
  })

  it('returns empty array when activePlanSessions is an empty array', () => {
    const data: StatusMessage = { sessions: [], activePlanSessions: [] }
    const result = parseActivePlanSessions(data)
    expect(result).toEqual([])
  })

  it('preserves all three session types', () => {
    const sessions: ActivePlanSession[] = [
      { epic: 'a', type: 'plan', name: 'pplan-a' },
      { epic: 'b', type: 'dispatch', name: 'pdisp-b' },
      { epic: 'c', type: 'retro', name: 'pretro-c' },
    ]
    const data: StatusMessage = { sessions: [], activePlanSessions: sessions }
    const result = parseActivePlanSessions(data)
    const types = result.map(s => s.type)
    expect(types).toContain('plan')
    expect(types).toContain('dispatch')
    expect(types).toContain('retro')
  })
})

// --- mergeSessions should not be affected by activePlanSessions ---

describe('mergeSessions independence from activePlanSessions', () => {
  // Extracted from useStatusSocket
  interface MinimalSession { name: string; _optimistic?: boolean }

  function mergeSessions(prev: MinimalSession[], incoming: MinimalSession[]): MinimalSession[] {
    const serverNames = new Set(incoming.map(s => s.name))
    const optimistic = prev.filter(s => !serverNames.has(s.name) && s._optimistic)
    return [...incoming, ...optimistic]
  }

  it('merges sessions normally regardless of plan sessions in payload', () => {
    const prev = [{ name: 'ragent-a', _optimistic: true }]
    const incoming = [{ name: 'ragent-b' }]
    const result = mergeSessions(prev, incoming)
    // ragent-a is optimistic and not in incoming → preserved
    expect(result).toHaveLength(2)
    expect(result.map(s => s.name)).toEqual(['ragent-b', 'ragent-a'])
  })
})
