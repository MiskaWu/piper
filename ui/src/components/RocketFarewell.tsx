import { useEffect, useState } from 'react'

export interface FarewellInfo {
  name: string
  rect: DOMRect
}

interface RocketFarewellProps {
  farewell: FarewellInfo
  onComplete: () => void
}

/**
 * 老鼠坐火箭飛走動畫 — 從指定 session card 位置發射
 */
export default function RocketFarewell({ farewell, onComplete }: RocketFarewellProps) {
  const [phase, setPhase] = useState<'launch' | 'done'>('launch')

  useEffect(() => {
    const t = setTimeout(() => {
      setPhase('done')
      onComplete()
    }, 600)
    return () => clearTimeout(t)
  }, [onComplete])

  if (phase === 'done') return null

  const { rect } = farewell
  // 定位：以 card 的中心為起點
  const startX = rect.left + rect.width / 2 - 30 // SVG 寬 60
  const startY = rect.top + rect.height / 2 - 40  // SVG 高 80

  return (
    <div
      className={`rocket-farewell rocket-farewell--${phase}`}
      style={{
        position: 'fixed',
        left: startX,
        top: startY,
        zIndex: 9999,
        pointerEvents: 'none',
      }}
    >
      <svg width="60" height="90" viewBox="0 0 60 90" fill="none" xmlns="http://www.w3.org/2000/svg">
        {/* 火焰 */}
        <g className="rocket-flame">
          <ellipse cx="30" cy="82" rx="7" ry="9" fill="#ff6b35" opacity="0.9" />
          <ellipse cx="30" cy="84" rx="5" ry="7" fill="#ffd23f" opacity="0.9" />
          <ellipse cx="30" cy="85" rx="2.5" ry="5" fill="#ffffff" opacity="0.8" />
        </g>

        {/* 火箭本體 */}
        <path d="M30 22 C30 22 19 34 19 52 L19 68 L41 68 L41 52 C41 34 30 22 30 22Z" fill="var(--accent)" />
        {/* 火箭尖端 */}
        <ellipse cx="30" cy="24" rx="3.5" ry="5" fill="var(--danger, #f85149)" />
        {/* 翅膀/鰭 */}
        <path d="M19 58 L10 72 L19 67Z" fill="var(--accent)" opacity="0.8" />
        <path d="M41 58 L50 72 L41 67Z" fill="var(--accent)" opacity="0.8" />
        {/* 窗戶（裝飾用） */}
        <circle cx="30" cy="55" r="4" fill="var(--bg-secondary)" stroke="var(--border)" strokeWidth="1" opacity="0.5" />

        {/* 老鼠探出火箭頂部 — 半身露出 */}
        <g transform="translate(30, 16)">
          {/* 身體（從火箭口露出） */}
          <ellipse cx="0" cy="6" rx="6" ry="5" fill="#c8a87c" />
          {/* 頭 */}
          <ellipse cx="0" cy="-2" rx="7" ry="6" fill="#d4b896" />
          {/* 大耳朵 — 明顯的圓耳 */}
          <circle cx="-6.5" cy="-8" r="4" fill="#d4b896" stroke="#b8956a" strokeWidth="0.8" />
          <circle cx="-6.5" cy="-8" r="2.2" fill="#f0c0a0" />
          <circle cx="6.5" cy="-8" r="4" fill="#d4b896" stroke="#b8956a" strokeWidth="0.8" />
          <circle cx="6.5" cy="-8" r="2.2" fill="#f0c0a0" />
          {/* 眼睛 — 大而亮 */}
          <ellipse cx="-2.8" cy="-2.5" rx="1.6" ry="1.8" fill="#1a1a2e" />
          <circle cx="-2.2" cy="-3.2" r="0.6" fill="#ffffff" />
          <ellipse cx="2.8" cy="-2.5" rx="1.6" ry="1.8" fill="#1a1a2e" />
          <circle cx="3.4" cy="-3.2" r="0.6" fill="#ffffff" />
          {/* 鼻子 */}
          <ellipse cx="0" cy="0.5" rx="1.2" ry="0.9" fill="#ff8fa3" />
          {/* 笑臉 */}
          <path d="M-2 2 Q0 4 2 2" stroke="#1a1a2e" strokeWidth="0.8" strokeLinecap="round" fill="none" />
          {/* 腮紅 */}
          <circle cx="-4.5" cy="0" r="1.5" fill="#ffb3c1" opacity="0.5" />
          <circle cx="4.5" cy="0" r="1.5" fill="#ffb3c1" opacity="0.5" />
          {/* 舉起的手 — 揮手告別 */}
          <g className="rocket-mouse-wave">
            <line x1="-6" y1="4" x2="-10" y2="-2" stroke="#d4b896" strokeWidth="2" strokeLinecap="round" />
            <circle cx="-10.5" cy="-3" r="1.5" fill="#d4b896" />
          </g>
          <line x1="6" y1="4" x2="9" y2="0" stroke="#d4b896" strokeWidth="2" strokeLinecap="round" />
          <circle cx="9.5" cy="-0.5" r="1.5" fill="#d4b896" />
        </g>
      </svg>

      {/* 星星粒子 */}
      <div className="rocket-particles">
        {[0, 1, 2, 3, 4].map(i => (
          <span key={i} className={`rocket-star rocket-star--${i}`}>✦</span>
        ))}
      </div>
    </div>
  )
}
