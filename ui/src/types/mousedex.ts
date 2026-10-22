// 老鼠圖鑑相關型別

export interface MousedexEntry {
  speciesKey: string
  isSSR: boolean
  topIdx: number | null
  midIdx: number | null
  botIdx: number | null
  bodyColor: string
  color: string
  unlockedAt: string
  sessionName: string
  unlockCount: number
}

export interface MousedexData {
  collected: Record<string, MousedexEntry>
  total: number
  count: number
}

export interface MousedexStats {
  collected: number
  total: number
}
