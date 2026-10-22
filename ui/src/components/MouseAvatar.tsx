import { useMemo } from 'react'

// ── 色盤 ──
// 裝飾用色（鮮豔）
const COLORS = [
  '#FF6B6B', '#FFB347', '#FFD93D', '#6BCB77', '#4D96FF',
  '#9B59B6', '#FF69B4', '#00CED1', '#FF8C42', '#A8D8EA',
]
// 鼠身用色（柔和粉彩）
const BODY_COLORS = [
  '#F8C8C8', '#FADCB0', '#FFF2B0', '#C8E6C9', '#BBDEFB',
  '#E1BEE7', '#F8BBD0', '#B2EBF2', '#FFD0A8', '#D6EAF8',
]

const SSR_CHANCE = 0.01

// ── 裝飾定義 ──

type DecoRenderer = (color: string) => React.ReactElement

// 上部裝飾（耳朵上方）— 以 (32, 8) 為中心 scale(2)
const TOP_DECORATIONS: DecoRenderer[] = [
  // 0: 蝴蝶結（左耳上）
  (c) => (
    <g transform="translate(12,4) scale(2)" fill={c}>
      <path d="M0,4 Q-6,0 0,0 Q6,0 0,4" />
      <path d="M0,4 Q-6,8 0,8 Q6,8 0,4" />
      <circle cx="0" cy="4" r="1.5" fill={c} opacity={0.7} />
    </g>
  ),
  // 1: 皇冠
  (c) => (
    <g transform="translate(32,8) scale(2) translate(-32,-8)">
      <polygon points="20,6 24,0 28,8 32,0 36,8 40,0 44,6 44,12 20,12" fill={c} />
      <rect x="20" y="10" width="24" height="4" fill={c} rx="1" opacity={0.8} />
    </g>
  ),
  // 2: 小花（右耳上）
  (c) => (
    <g transform="translate(48,4) scale(2)" fill={c}>
      <circle cx="0" cy="-3" r="2.5" />
      <circle cx="2.8" cy="-1" r="2.5" />
      <circle cx="1.7" cy="2" r="2.5" />
      <circle cx="-1.7" cy="2" r="2.5" />
      <circle cx="-2.8" cy="-1" r="2.5" />
      <circle cx="0" cy="0" r="2" fill="#FFD93D" />
    </g>
  ),
  // 3: 愛心髮夾（左耳上）
  (c) => (
    <g transform="translate(14,2) scale(2)">
      <path d="M0,2 a2,2 0 0,1 2.5,0 a2,2 0 0,1 2.5,0 Q5,5.5 2.5,7 Q0,5.5 0,2" fill={c} />
      <rect x="1.5" y="6" width="2" height="4" fill={c} rx="0.5" />
    </g>
  ),
  // 4: 廚師帽
  (c) => (
    <g transform="translate(32,8) scale(2) translate(-32,-8)">
      <ellipse cx="32" cy="4" rx="12" ry="8" fill={c} opacity={0.9} />
      <rect x="22" y="8" width="20" height="6" fill={c} />
      <ellipse cx="32" cy="4" rx="8" ry="5" fill="white" opacity={0.3} />
    </g>
  ),
  // 5: 派對帽
  (c) => (
    <g transform="translate(32,8) scale(2) translate(-32,-8)">
      <polygon points="32,-4 22,12 42,12" fill={c} />
      <circle cx="32" cy="-5" r="2.5" fill="#FFD93D" />
      <rect x="22" y="10" width="20" height="3" rx="1" fill={c} opacity={0.7} />
    </g>
  ),
  // 6: 天使光環
  (c) => (
    <g transform="translate(32,2) scale(2) translate(-32,-2)">
      <ellipse cx="32" cy="2" rx="10" ry="3" fill="none" stroke={c} strokeWidth="1.5" />
      <ellipse cx="32" cy="2" rx="10" ry="3" fill={c} opacity={0.15} />
    </g>
  ),
  // 7: 貓耳頭帶
  (c) => (
    <g transform="translate(32,8) scale(2) translate(-32,-8)" fill={c}>
      <polygon points="14,14 10,0 22,8" />
      <polygon points="50,14 54,0 42,8" />
      <rect x="14" y="12" width="36" height="2" rx="1" />
    </g>
  ),
  // 8: 耳機
  (c) => (
    <g transform="translate(32,10) scale(2) translate(-32,-10)">
      <path d="M12,16 Q12,2 32,2 Q52,2 52,16" stroke={c} strokeWidth="2" fill="none" />
      <rect x="8" y="14" width="6" height="8" rx="2" fill={c} />
      <rect x="50" y="14" width="6" height="8" rx="2" fill={c} />
    </g>
  ),
  // 9: 小星星（頭頂）
  (c) => (
    <g transform="translate(32,2) scale(2) translate(-32,-2)" fill={c}>
      <g transform="translate(32,2)">
        <path d="M0,-5 L1.2,-1.5 L5,-1.5 L2,0.8 L3.1,4.3 L0,2 L-3.1,4.3 L-2,0.8 L-5,-1.5 L-1.2,-1.5 Z" />
      </g>
    </g>
  ),
]

// 中部裝飾（臉部/眼睛）— 取代或覆蓋在眼睛上
const MID_DECORATIONS: DecoRenderer[] = [
  // 0: 圓眼鏡
  (c) => (
    <g>
      <circle cx="24" cy="32" r="6" fill="none" stroke={c} strokeWidth="1.5" />
      <circle cx="40" cy="32" r="6" fill="none" stroke={c} strokeWidth="1.5" />
      <line x1="30" y1="32" x2="34" y2="32" stroke={c} strokeWidth="1.5" />
      <circle cx="24" cy="32" r="3.5" fill="#333" />
      <circle cx="25.2" cy="30.8" r="1.2" fill="white" />
      <circle cx="40" cy="32" r="3.5" fill="#333" />
      <circle cx="41.2" cy="30.8" r="1.2" fill="white" />
    </g>
  ),
  // 1: 墨鏡
  (c) => (
    <g>
      <rect x="16" y="28" width="14" height="10" rx="3" fill={c} opacity={0.85} />
      <rect x="34" y="28" width="14" height="10" rx="3" fill={c} opacity={0.85} />
      <line x1="30" y1="33" x2="34" y2="33" stroke={c} strokeWidth="2" />
      <line x1="12" y1="33" x2="16" y2="33" stroke={c} strokeWidth="1.5" />
      <line x1="48" y1="33" x2="52" y2="33" stroke={c} strokeWidth="1.5" />
    </g>
  ),
  // 2: 海盜眼罩
  (c) => (
    <g>
      <ellipse cx="24" cy="32" rx="7" ry="6" fill={c} />
      <line x1="30" y1="28" x2="46" y2="18" stroke={c} strokeWidth="1.5" />
      <circle cx="40" cy="32" r="4.5" fill="#333" />
      <circle cx="41.5" cy="30.5" r="1.5" fill="white" />
    </g>
  ),
  // 3: 紅鼻子（大）
  (c) => (
    <g>
      <circle cx="24" cy="32" r="4.5" fill="#333" />
      <circle cx="25.5" cy="30.5" r="1.5" fill="white" />
      <circle cx="40" cy="32" r="4.5" fill="#333" />
      <circle cx="41.5" cy="30.5" r="1.5" fill="white" />
      <circle cx="32" cy="38" r="4.5" fill={c} />
      <ellipse cx="33" cy="37" rx="1.5" ry="1" fill="white" opacity={0.3} />
    </g>
  ),
  // 4: 腮紅
  (c) => (
    <g>
      <circle cx="24" cy="32" r="4.5" fill="#333" />
      <circle cx="25.5" cy="30.5" r="1.5" fill="white" />
      <circle cx="40" cy="32" r="4.5" fill="#333" />
      <circle cx="41.5" cy="30.5" r="1.5" fill="white" />
      <ellipse cx="16" cy="38" rx="5" ry="3" fill={c} opacity={0.35} />
      <ellipse cx="48" cy="38" rx="5" ry="3" fill={c} opacity={0.35} />
    </g>
  ),
  // 5: 威尼斯面具
  (c) => (
    <g>
      <path d="M14,30 Q20,24 32,26 Q44,24 50,30 Q44,36 32,34 Q20,36 14,30" fill={c} opacity={0.8} />
      <circle cx="24" cy="30" r="4" fill="white" opacity={0.6} />
      <circle cx="40" cy="30" r="4" fill="white" opacity={0.6} />
      <circle cx="24" cy="30" r="2.5" fill="#333" />
      <circle cx="40" cy="30" r="2.5" fill="#333" />
    </g>
  ),
  // 6: OK繃（斜貼在左臉）
  (c) => (
    <g>
      <circle cx="24" cy="32" r="4.5" fill="#333" />
      <circle cx="25.5" cy="30.5" r="1.5" fill="white" />
      <circle cx="40" cy="32" r="4.5" fill="#333" />
      <circle cx="41.5" cy="30.5" r="1.5" fill="white" />
      <g transform="translate(18,34) rotate(-30)">
        <rect x="-6" y="-2" width="12" height="4" rx="1" fill={c} opacity={0.85} />
        <rect x="-2" y="-2" width="4" height="4" fill={c} opacity={0.6} />
      </g>
    </g>
  ),
  // 7: 單片眼鏡（右眼）
  (c) => (
    <g>
      <circle cx="24" cy="32" r="4.5" fill="#333" />
      <circle cx="25.5" cy="30.5" r="1.5" fill="white" />
      <circle cx="40" cy="32" r="7" fill="none" stroke={c} strokeWidth="1.5" />
      <circle cx="40" cy="32" r="3.5" fill="#333" />
      <circle cx="41.5" cy="30.5" r="1.2" fill="white" />
      <line x1="46" y1="34" x2="50" y2="48" stroke={c} strokeWidth="1" />
    </g>
  ),
  // 8: 愛心眼
  (c) => (
    <g fill={c}>
      <path d="M20,29 a3,3 0 0,1 4,0 a3,3 0 0,1 4,0 Q28,34 24,37 Q20,34 20,29" />
      <path d="M36,29 a3,3 0 0,1 4,0 a3,3 0 0,1 4,0 Q44,34 40,37 Q36,34 36,29" />
    </g>
  ),
  // 9: 瞇瞇眼
  (c) => (
    <g stroke={c} strokeWidth="2" strokeLinecap="round">
      <path d="M20,32 Q24,35 28,32" fill="none" />
      <path d="M36,32 Q40,35 44,32" fill="none" />
    </g>
  ),
]

// 下部裝飾（嘴巴/下巴以下）
const BOTTOM_DECORATIONS: DecoRenderer[] = [
  // 0: 領結
  (c) => (
    <g transform="translate(32,52)">
      <path d="M0,0 L-8,-4 L-8,4 Z" fill={c} />
      <path d="M0,0 L8,-4 L8,4 Z" fill={c} />
      <circle cx="0" cy="0" r="2" fill={c} opacity={0.7} />
    </g>
  ),
  // 1: 圍巾
  (c) => (
    <g>
      <path d="M16,50 Q32,56 48,50 Q48,54 32,58 Q16,54 16,50" fill={c} opacity={0.9} />
      <rect x="28" y="54" width="6" height="10" rx="2" fill={c} opacity={0.8} />
    </g>
  ),
  // 2: 珍珠項鏈
  (c) => (
    <g>
      <path d="M16,50 Q32,56 48,50" stroke={c} strokeWidth="1" fill="none" />
      {[0, 1, 2, 3, 4, 5, 6].map((i) => {
        const t = i / 6
        const x = 16 + t * 32
        const y = 50 + Math.sin(t * Math.PI) * 6
        return <circle key={i} cx={x} cy={y} r="2" fill={c} opacity={0.8} />
      })}
    </g>
  ),
  // 3: 奶嘴
  (c) => (
    <g transform="translate(32,44)">
      <circle cx="0" cy="2" r="4" fill={c} opacity={0.8} />
      <ellipse cx="0" cy="-1" rx="2.5" ry="2" fill={c} />
      <circle cx="0" cy="2" r="2" fill={c} opacity={0.5} />
    </g>
  ),
  // 4: 吐舌頭
  (c) => (
    <g>
      <ellipse cx="32" cy="46" rx="4" ry="5" fill={c} opacity={0.8} />
      <ellipse cx="32" cy="45" rx="3" ry="3.5" fill={c} opacity={0.5} />
    </g>
  ),
  // 5: 叼玫瑰
  (c) => (
    <g transform="translate(38,43)">
      <line x1="0" y1="0" x2="10" y2="3" stroke="#4a7" strokeWidth="1.5" />
      <circle cx="10" cy="3" r="4" fill={c} />
      <path d="M8,1 Q10,-1 12,1 Q10,0 8,1" fill={c} opacity={0.7} />
    </g>
  ),
  // 6: 鈴鐺項圈
  (c) => (
    <g>
      <path d="M16,50 Q32,56 48,50" stroke={c} strokeWidth="2.5" fill="none" />
      <circle cx="32" cy="54" r="3.5" fill="#FFD93D" />
      <line x1="32" y1="52" x2="32" y2="56" stroke="#b8860b" strokeWidth="0.8" />
    </g>
  ),
  // 7: 口罩
  (c) => (
    <g>
      <path d="M16,36 Q16,50 32,52 Q48,50 48,36 Z" fill={c} opacity={0.7} />
      <line x1="20" y1="42" x2="44" y2="42" stroke="white" strokeWidth="0.5" opacity={0.4} />
      <line x1="20" y1="45" x2="44" y2="45" stroke="white" strokeWidth="0.5" opacity={0.4} />
      <line x1="12" y1="38" x2="16" y2="38" stroke={c} strokeWidth="1.5" />
      <line x1="48" y1="38" x2="52" y2="38" stroke={c} strokeWidth="1.5" />
    </g>
  ),
  // 8: 鬍子
  (c) => (
    <g>
      <path d="M24,42 Q20,46 16,44" stroke={c} strokeWidth="1.5" fill="none" />
      <path d="M24,43 Q20,48 14,47" stroke={c} strokeWidth="1.5" fill="none" />
      <path d="M40,42 Q44,46 48,44" stroke={c} strokeWidth="1.5" fill="none" />
      <path d="M40,43 Q44,48 50,47" stroke={c} strokeWidth="1.5" fill="none" />
    </g>
  ),
  // 9: 領帶
  (c) => (
    <g transform="translate(32,50)" fill={c}>
      <polygon points="0,0 -4,3 0,6 4,3" />
      <polygon points="-3,6 0,16 3,6" />
    </g>
  ),
]

// ── 隨機種子 ──

// 字串 → 數字 hash（djb2）
function hashString(str: string): number {
  let hash = 5381
  for (let i = 0; i < str.length; i++) {
    hash = ((hash << 5) + hash + str.charCodeAt(i)) | 0
  }
  return hash >>> 0  // 轉正數
}

// 簡易 mulberry32 PRNG
function mulberry32(seed: number) {
  let s = seed | 0
  return () => {
    s = (s + 0x6d2b79f5) | 0
    let t = Math.imul(s ^ (s >>> 15), 1 | s)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

export interface MouseConfig {
  isSSR: boolean
  bodyColor: string   // 鼠身顏色（柔和）
  color: string       // 裝飾顏色（鮮豔）
  topIdx: number | null
  midIdx: number | null
  botIdx: number | null
}

export function generateMouseConfig(seed: number | string, forceSSR = false): MouseConfig {
  const s = typeof seed === 'string' ? hashString(seed) : seed
  const rand = mulberry32(s)

  const isSSR = forceSSR || rand() < SSR_CHANCE
  const bodyIdx = Math.floor(rand() * BODY_COLORS.length)
  const bodyColor = BODY_COLORS[bodyIdx]
  const color = COLORS[bodyIdx]  // 裝飾色與身體色同系

  // SSR 鼠：整隻彩虹，無裝飾
  const topIdx = isSSR ? null : (rand() < 0.5 ? null : Math.floor(rand() * TOP_DECORATIONS.length))
  const midIdx = isSSR ? null : (rand() < 0.5 ? null : Math.floor(rand() * MID_DECORATIONS.length))
  const botIdx = isSSR ? null : (rand() < 0.5 ? null : Math.floor(rand() * BOTTOM_DECORATIONS.length))

  return { isSSR, bodyColor, color, topIdx, midIdx, botIdx }
}

// ── 元件 ──

// speciesKeyString generates a unique species identifier (ignoring colors).
export function speciesKeyString(cfg: MouseConfig): string {
  if (cfg.isSSR) return 'ssr'
  const f = (v: number | null) => v === null ? 'x' : String(v)
  return `${f(cfg.topIdx)}-${f(cfg.midIdx)}-${f(cfg.botIdx)}`
}

// enumerateAllSpeciesKeys returns all 1332 species keys in deterministic order.
export function enumerateAllSpeciesKeys(): string[] {
  const keys: string[] = ['ssr']
  const f = (v: number) => v < 0 ? 'x' : String(v)
  for (let t = -1; t <= 9; t++) {
    for (let m = -1; m <= 9; m++) {
      for (let b = -1; b <= 9; b++) {
        keys.push(`${f(t)}-${f(m)}-${f(b)}`)
      }
    }
  }
  return keys
}

// parseSpeciesKey converts a species key string back to config parts.
export function parseSpeciesKey(key: string): { isSSR: boolean; topIdx: number | null; midIdx: number | null; botIdx: number | null } {
  if (key === 'ssr') return { isSSR: true, topIdx: null, midIdx: null, botIdx: null }
  const parts = key.split('-')
  const parse = (s: string) => s === 'x' ? null : parseInt(s, 10)
  return { isSSR: false, topIdx: parse(parts[0]), midIdx: parse(parts[1]), botIdx: parse(parts[2]) }
}

interface MouseAvatarProps {
  name?: string           // session name，作為隨機種子
  config?: MouseConfig    // 直接提供 config（跳過 seed 生成）
  size?: number
  forceSSR?: boolean
  grayscale?: boolean     // 灰色顯示（未收集的物種）
}

export default function MouseAvatar({ name, config: configProp, size = 20, forceSSR = false, grayscale = false }: MouseAvatarProps) {
  const config = useMemo(() => {
    if (configProp) return configProp
    return generateMouseConfig(name || '', forceSSR)
  }, [name, configProp, forceSSR])
  const gradientId = `mouse-rainbow-${(name || 'cfg').replace(/[^a-zA-Z0-9]/g, '')}`
  const svgStyle: React.CSSProperties = {
    verticalAlign: 'middle',
    marginRight: 4,
    ...(grayscale ? { filter: 'grayscale(1)', opacity: 0.25 } : {}),
  }

  const needsExtraTop = config.topIdx !== null
  const viewH = needsExtraTop ? 88 : 64
  const offsetY = needsExtraTop ? 24 : 0

  const midNeedsDefaultEyes = config.midIdx === null
  const rainbow = `url(#${gradientId})`

  // SSR：整隻炫彩，無裝飾
  if (config.isSSR) {
    return (
      <svg
        width={size}
        height={size}
        viewBox="0 0 64 64"
        className="mouse-avatar mouse-avatar--ssr"
        style={svgStyle}
      >
        <defs>
          <linearGradient id={gradientId} x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" stopColor="#FF6B6B" />
            <stop offset="20%" stopColor="#FF69B4" />
            <stop offset="40%" stopColor="#FFD93D" />
            <stop offset="60%" stopColor="#6BCB77" />
            <stop offset="80%" stopColor="#4D96FF" />
            <stop offset="100%" stopColor="#9B59B6" />
          </linearGradient>
        </defs>
        {/* 耳朵 */}
        <ellipse cx="18" cy="16" rx="10" ry="12" fill={rainbow} />
        <ellipse cx="46" cy="16" rx="10" ry="12" fill={rainbow} />
        <ellipse cx="18" cy="16" rx="7" ry="8" fill={rainbow} opacity={0.6} />
        <ellipse cx="46" cy="16" rx="7" ry="8" fill={rainbow} opacity={0.6} />
        {/* 臉 */}
        <ellipse cx="32" cy="36" rx="20" ry="18" fill={rainbow} />
        {/* 閃光粒子 */}
        <circle className="mouse-sparkle mouse-sparkle--1" cx="8" cy="8" r="2" fill="white" />
        <circle className="mouse-sparkle mouse-sparkle--2" cx="56" cy="12" r="1.5" fill="white" />
        <circle className="mouse-sparkle mouse-sparkle--3" cx="52" cy="50" r="1.8" fill="white" />
        <circle className="mouse-sparkle mouse-sparkle--4" cx="10" cy="48" r="1.2" fill="white" />
      </svg>
    )
  }

  // 一般鼠：帶隨機裝飾
  return (
    <svg
      width={size}
      height={size * (viewH / 64)}
      viewBox={`0 0 64 ${viewH}`}
      className="mouse-avatar"
      style={svgStyle}
    >
      <g transform={`translate(0,${offsetY})`}>
        {/* 上部裝飾 */}
        {config.topIdx !== null && TOP_DECORATIONS[config.topIdx](config.color)}

        {/* 耳朵 */}
        <ellipse cx="18" cy="16" rx="10" ry="12" fill={config.bodyColor} />
        <ellipse cx="46" cy="16" rx="10" ry="12" fill={config.bodyColor} />
        <ellipse cx="18" cy="16" rx="7" ry="8" fill={config.color} opacity={0.35} />
        <ellipse cx="46" cy="16" rx="7" ry="8" fill={config.color} opacity={0.35} />

        {/* 臉 */}
        <ellipse cx="32" cy="36" rx="20" ry="18" fill={config.bodyColor} />

        {/* 預設眼睛（大眼 + 高光） */}
        {midNeedsDefaultEyes && (
          <>
            <circle cx="24" cy="32" r="4.5" fill="#333" />
            <circle cx="40" cy="32" r="4.5" fill="#333" />
            <circle cx="25.5" cy="30.5" r="1.5" fill="white" />
            <circle cx="41.5" cy="30.5" r="1.5" fill="white" />
          </>
        )}

        {/* 中部裝飾 */}
        {config.midIdx !== null && MID_DECORATIONS[config.midIdx](config.color)}

        {/* 鼻子（紅鼻子裝飾已自帶） */}
        {config.midIdx !== 3 && (
          <ellipse cx="32" cy="38" rx="3" ry="2" fill="#ffb6c1" />
        )}

        {/* 嘴巴（口罩時隱藏） */}
        {config.botIdx !== 7 && (
          <path d="M28 42 Q32 46 36 42" stroke="#999" strokeWidth="1.5" fill="none" />
        )}

        {/* 鬍鬚 */}
        <line x1="8" y1="36" x2="22" y2="38" stroke="#999" strokeWidth="1" />
        <line x1="8" y1="40" x2="22" y2="40" stroke="#999" strokeWidth="1" />
        <line x1="42" y1="38" x2="56" y2="36" stroke="#999" strokeWidth="1" />
        <line x1="42" y1="40" x2="56" y2="40" stroke="#999" strokeWidth="1" />

        {/* 下部裝飾 */}
        {config.botIdx !== null && BOTTOM_DECORATIONS[config.botIdx](config.color)}
      </g>
    </svg>
  )
}
