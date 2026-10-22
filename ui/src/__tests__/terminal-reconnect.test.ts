import { describe, it, expect, vi, beforeEach } from 'vitest'

/**
 * Tests for Terminal.jsx reconnection logic:
 * 1. Generation guard: stale callbacks become no-ops
 * 2. Input buffering: buffer during disconnect, flush on reconnect
 * 3. Quiet reconnect: brief disconnections produce no visible messages
 *
 * We extract the core logic patterns and test them in isolation
 * without requiring xterm.js or real WebSocket.
 */

// --- Generation guard pattern ---

describe('generation guard (isStale)', () => {
  it('returns false when generation matches', () => {
    const generationRef = { current: 0 }
    const gen = ++generationRef.current
    const isStale = () => generationRef.current !== gen
    expect(isStale()).toBe(false)
  })

  it('returns true after generation increments (effect re-run)', () => {
    const generationRef = { current: 0 }
    const gen1 = ++generationRef.current // effect 1
    const isStale1 = () => generationRef.current !== gen1

    // Simulate effect re-run (session switch)
    ++generationRef.current // effect 2
    expect(isStale1()).toBe(true) // old callbacks are now stale
  })

  it('handles rapid session switches (multiple generations)', () => {
    const generationRef = { current: 0 }
    const gen1 = ++generationRef.current
    const gen2 = ++generationRef.current
    const gen3 = ++generationRef.current

    expect(generationRef.current !== gen1).toBe(true)
    expect(generationRef.current !== gen2).toBe(true)
    expect(generationRef.current !== gen3).toBe(false) // only latest is valid
  })

  it('local cancelled + generation provides double protection', () => {
    const generationRef = { current: 0 }

    // Effect 1 starts
    let cancelled1 = false
    const gen1 = ++generationRef.current
    const isStale1 = () => cancelled1 || generationRef.current !== gen1

    // Effect 1 cleanup
    cancelled1 = true

    // Effect 2 starts (new session)
    let cancelled2 = false
    const gen2 = ++generationRef.current
    const isStale2 = () => cancelled2 || generationRef.current !== gen2

    // Old effect's async callback fires — both guards catch it
    expect(isStale1()).toBe(true) // cancelled1 = true
    // Even without cancelled, generation catches it
    cancelled1 = false // hypothetically if it were reset (like unmountedRef bug)
    expect(generationRef.current !== gen1).toBe(true) // generation still protects

    // New effect is valid
    expect(isStale2()).toBe(false)
  })
})

// --- Input buffer pattern ---

interface MockWs {
  readyState: number
  send: ReturnType<typeof vi.fn> & ((data: Uint8Array) => void)
}

describe('input buffer (sendOrBuffer + flushBuffer)', () => {
  let buffer: string[]
  let sent: Uint8Array[]
  let mockWs: MockWs | null

  function createMockWs(open = true): MockWs {
    return {
      readyState: open ? 1 : 3, // OPEN = 1, CLOSED = 3
      send: vi.fn((data: Uint8Array) => sent.push(data)),
    }
  }

  function sendOrBuffer(data: string, ws: MockWs | null) {
    if (ws && ws.readyState === 1) {
      ws.send(new TextEncoder().encode(data))
    } else {
      buffer.push(data)
    }
  }

  function flushBuffer(ws: MockWs | null) {
    if (!ws || ws.readyState !== 1) return
    const encoder = new TextEncoder()
    const items = buffer.splice(0)
    for (const data of items) {
      ws.send(encoder.encode(data))
    }
  }

  beforeEach(() => {
    buffer = []
    sent = []
    mockWs = null
  })

  it('sends directly when WS is open', () => {
    mockWs = createMockWs(true)
    sendOrBuffer('hello', mockWs)
    expect(buffer).toHaveLength(0)
    expect(mockWs.send).toHaveBeenCalledTimes(1)
  })

  it('buffers when WS is null', () => {
    sendOrBuffer('hello', null)
    expect(buffer).toEqual(['hello'])
  })

  it('buffers when WS is closed', () => {
    mockWs = createMockWs(false)
    sendOrBuffer('hello', mockWs)
    expect(buffer).toEqual(['hello'])
    expect(mockWs.send).not.toHaveBeenCalled()
  })

  it('buffers multiple inputs and flushes in order', () => {
    sendOrBuffer('a', null)
    sendOrBuffer('b', null)
    sendOrBuffer('c', null)
    expect(buffer).toEqual(['a', 'b', 'c'])

    mockWs = createMockWs(true)
    flushBuffer(mockWs)

    expect(buffer).toHaveLength(0) // cleared
    expect(mockWs.send).toHaveBeenCalledTimes(3)
    // Verify order
    const decoded = Array.from({ length: 3 }, (_, i) =>
      new TextDecoder().decode(mockWs!.send.mock.calls[i][0])
    )
    expect(decoded).toEqual(['a', 'b', 'c'])
  })

  it('flush is no-op when buffer is empty', () => {
    mockWs = createMockWs(true)
    flushBuffer(mockWs)
    expect(mockWs.send).not.toHaveBeenCalled()
  })

  it('flush is no-op when WS is not open', () => {
    buffer.push('data')
    flushBuffer(null)
    expect(buffer).toEqual(['data']) // not cleared
  })
})

// --- Quiet reconnect threshold ---

describe('quiet reconnect (message visibility)', () => {
  const QUIET_THRESHOLD = 2

  function shouldShowMessage(attempt: number, threshold = QUIET_THRESHOLD): boolean {
    return attempt >= threshold
  }

  it('hides reconnect message on 1st attempt', () => {
    expect(shouldShowMessage(1)).toBe(false)
  })

  it('shows reconnect message from 2nd attempt onwards', () => {
    expect(shouldShowMessage(2)).toBe(true)
    expect(shouldShowMessage(3)).toBe(true)
    expect(shouldShowMessage(5)).toBe(true)
  })

  it('hides reconnected message when reconnect was quick (attempt < threshold)', () => {
    // Simulates: disconnected, 1 reconnect attempt, succeeded
    const attemptWhenReconnected = 1
    expect(shouldShowMessage(attemptWhenReconnected)).toBe(false)
  })

  it('shows reconnected message when reconnect took multiple attempts', () => {
    const attemptWhenReconnected = 3
    expect(shouldShowMessage(attemptWhenReconnected)).toBe(true)
  })
})

// --- Reconnect delay calculation ---

describe('reconnect delay (exponential backoff)', () => {
  const BASE = 1000
  const MAX = 10000

  function calcDelay(attempt: number): number {
    return Math.min(BASE * Math.pow(2, attempt - 1), MAX)
  }

  it('1st attempt: 1s', () => {
    expect(calcDelay(1)).toBe(1000)
  })

  it('2nd attempt: 2s', () => {
    expect(calcDelay(2)).toBe(2000)
  })

  it('3rd attempt: 4s', () => {
    expect(calcDelay(3)).toBe(4000)
  })

  it('4th attempt: 8s', () => {
    expect(calcDelay(4)).toBe(8000)
  })

  it('caps at MAX_DELAY', () => {
    expect(calcDelay(5)).toBe(10000) // 16000 capped to 10000
    expect(calcDelay(10)).toBe(10000)
  })
})
