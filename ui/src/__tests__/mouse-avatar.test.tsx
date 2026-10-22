import { describe, it, expect } from 'vitest'
import { generateMouseConfig } from '../components/MouseAvatar'

describe('generateMouseConfig', () => {
  it('同一 seed 產生相同結果', () => {
    const a = generateMouseConfig('test-session-12345')
    const b = generateMouseConfig('test-session-12345')
    expect(a).toEqual(b)
  })

  it('不同 seed 產生不同結果', () => {
    const a = generateMouseConfig('session-alpha')
    const b = generateMouseConfig('session-beta')
    // 至少有一個欄位不同
    const same = a.topIdx === b.topIdx && a.midIdx === b.midIdx && a.botIdx === b.botIdx && a.color === b.color
    expect(same).toBe(false)
  })

  it('裝飾索引在合法範圍內', () => {
    for (let seed = 1; seed < 200; seed++) {
      const cfg = generateMouseConfig(`session-${seed}`)
      if (cfg.topIdx !== null) {
        expect(cfg.topIdx).toBeGreaterThanOrEqual(0)
        expect(cfg.topIdx).toBeLessThan(10)
      }
      if (cfg.midIdx !== null) {
        expect(cfg.midIdx).toBeGreaterThanOrEqual(0)
        expect(cfg.midIdx).toBeLessThan(10)
      }
      if (cfg.botIdx !== null) {
        expect(cfg.botIdx).toBeGreaterThanOrEqual(0)
        expect(cfg.botIdx).toBeLessThan(10)
      }
    }
  })

  it('color 和 bodyColor 來自預定義色盤', () => {
    const COLORS = [
      '#FF6B6B', '#FFB347', '#FFD93D', '#6BCB77', '#4D96FF',
      '#9B59B6', '#FF69B4', '#00CED1', '#FF8C42', '#A8D8EA',
    ]
    const BODY_COLORS = [
      '#F8C8C8', '#FADCB0', '#FFF2B0', '#C8E6C9', '#BBDEFB',
      '#E1BEE7', '#F8BBD0', '#B2EBF2', '#FFD0A8', '#D6EAF8',
    ]
    for (let seed = 1; seed < 100; seed++) {
      const cfg = generateMouseConfig(`session-${seed}`)
      expect(COLORS).toContain(cfg.color)
      expect(BODY_COLORS).toContain(cfg.bodyColor)
    }
  })

  it('forceSSR 強制產生 SSR 且無裝飾', () => {
    const cfg = generateMouseConfig('any-session', true)
    expect(cfg.isSSR).toBe(true)
    expect(cfg.topIdx).toBeNull()
    expect(cfg.midIdx).toBeNull()
    expect(cfg.botIdx).toBeNull()
  })

  it('SSR 機率約 1%（統計測試）', () => {
    let ssrCount = 0
    const total = 10000
    for (let seed = 1; seed <= total; seed++) {
      if (generateMouseConfig(`ssr-test-${seed}`).isSSR) ssrCount++
    }
    // 容許 0.3% ~ 2.5% 範圍
    expect(ssrCount / total).toBeGreaterThan(0.003)
    expect(ssrCount / total).toBeLessThan(0.025)
  })

  it('有裝飾和無裝飾都會出現', () => {
    let hasTop = false, hasNone = false
    for (let seed = 1; seed < 200; seed++) {
      const cfg = generateMouseConfig(`session-${seed}`)
      if (cfg.topIdx !== null) hasTop = true
      if (cfg.topIdx === null && cfg.midIdx === null && cfg.botIdx === null) hasNone = true
    }
    expect(hasTop).toBe(true)
    expect(hasNone).toBe(true)
  })
})
