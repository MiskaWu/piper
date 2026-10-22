import type { TrendEntry } from '../../api'
import './RetroTrends.css'

interface RetroTrendsProps {
  releases: TrendEntry[]
}

/** 四個指標的配置：標題 + 取值 fn + 更好的方向（higher/lower is better）+ 格式化 */
const METRICS: Array<{
  key: string
  label: string
  pick: (e: TrendEntry) => number
  format: (v: number) => string
  better: 'higher' | 'lower'
}> = [
  { key: 'firstPass', label: '一次通過率', pick: (e) => e.firstPassRate, format: (v) => `${v.toFixed(0)}%`, better: 'higher' },
  { key: 'gap', label: 'Gap 總數', pick: (e) => e.gapTotal, format: (v) => `${v}`, better: 'lower' },
  { key: 'rework', label: '平均 fix loop', pick: (e) => e.avgLoopCount, format: (v) => v.toFixed(1), better: 'lower' },
  { key: 'aanDelta', label: 'A/A/N 差距', pick: (e) => e.aanPassRate - e.nonAanPassRate, format: (v) => `${v >= 0 ? '+' : ''}${v.toFixed(0)}`, better: 'higher' },
]

export function RetroTrends({ releases }: RetroTrendsProps) {
  const withSnapshot = releases.filter((r) => r.hasSnapshot)
  if (withSnapshot.length === 0) {
    return (
      <div className="retro-trends">
        <p className="retro-trends-empty">尚無含指標的 retro 報告（只有 legacy markdown 報告）。</p>
      </div>
    )
  }

  const latest = withSnapshot[withSnapshot.length - 1]
  const previous = withSnapshot.length > 1 ? withSnapshot[withSnapshot.length - 2] : null

  return (
    <div className="retro-trends">
      <div className="retro-metric-grid">
        {METRICS.map((m) => {
          const current = m.pick(latest)
          const prev = previous ? m.pick(previous) : null
          const delta = prev === null ? null : current - prev
          const deltaClass = delta === null
            ? ''
            : delta === 0
              ? 'retro-metric-delta--flat'
              : (m.better === 'higher' ? delta > 0 : delta < 0)
                ? 'retro-metric-delta--good'
                : 'retro-metric-delta--bad'
          return (
            <div key={m.key} className="retro-metric-card">
              <div className="retro-metric-label">{m.label}</div>
              <div className="retro-metric-value">{m.format(current)}</div>
              {delta !== null && (
                <div className={`retro-metric-delta ${deltaClass}`}>
                  {delta > 0 ? '▲' : delta < 0 ? '▼' : '─'} {m.format(Math.abs(delta))}
                </div>
              )}
            </div>
          )
        })}
      </div>

      <div className="retro-sparklines">
        {METRICS.map((m) => (
          <div key={m.key} className="retro-sparkline-row">
            <span className="retro-sparkline-label">{m.label}</span>
            <Sparkline values={withSnapshot.map(m.pick)} labels={withSnapshot.map((e) => e.releaseLabel)} />
          </div>
        ))}
      </div>
    </div>
  )
}

interface SparklineProps {
  values: number[]
  labels: string[]
  width?: number
  height?: number
}

function Sparkline({ values, labels, width = 140, height = 28 }: SparklineProps) {
  if (values.length === 0) return null
  if (values.length === 1) {
    return (
      <svg width={width} height={height} className="retro-sparkline">
        <circle cx={width / 2} cy={height / 2} r={2.5} fill="currentColor" />
      </svg>
    )
  }
  const min = Math.min(...values)
  const max = Math.max(...values)
  const range = max - min || 1
  const step = width / (values.length - 1)
  const points = values.map((v, i) => {
    const x = i * step
    const y = height - ((v - min) / range) * (height - 4) - 2
    return `${x.toFixed(1)},${y.toFixed(1)}`
  })
  return (
    <svg width={width} height={height} className="retro-sparkline">
      <polyline
        fill="none"
        stroke="currentColor"
        strokeWidth={1.5}
        strokeLinecap="round"
        strokeLinejoin="round"
        points={points.join(' ')}
      />
      {values.map((v, i) => (
        <circle
          key={i}
          cx={(i * step).toFixed(1)}
          cy={(height - ((v - min) / range) * (height - 4) - 2).toFixed(1)}
          r={i === values.length - 1 ? 2.5 : 1.5}
          fill="currentColor"
        >
          <title>{`${labels[i]}: ${v}`}</title>
        </circle>
      ))}
    </svg>
  )
}
