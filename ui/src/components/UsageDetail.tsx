import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useDashboardStats, useMousedex } from '../hooks/useDashboardStats'
import { formatTimeUntil } from '../lib/format'
import type { Usage, DashboardStats, DailyUsage, DailyWork, DailySegmentedWork, ProjectWork, DailyCompletion, MousedexStats } from '../types'
import MousedexGrid from './MousedexGrid'

function formatDuration(sec: number): string {
  if (sec < 60) return `${sec}s`
  if (sec < 3600) return `${Math.round(sec / 60)}m`
  const h = Math.floor(sec / 3600)
  const m = Math.round((sec % 3600) / 60)
  return m > 0 ? `${h}h${m}m` : `${h}h`
}

function getBarColor(value: number): string {
  if (value >= 80) return 'var(--danger)'
  if (value >= 50) return 'var(--warning)'
  return 'var(--success)'
}

interface UsageDetailProps {
  usage: Usage | null
}

export default function UsageDetail({ usage }: UsageDetailProps) {
  const { t } = useTranslation()
  const now = new Date()
  const [year, setYear] = useState(now.getFullYear())
  const [month, setMonth] = useState(now.getMonth() + 1)
  const [showMousedex, setShowMousedex] = useState(false)

  const { data: stats, isLoading: loading } = useDashboardStats(year, month)
  const { data: mousedexData } = useMousedex()
  const mousedexStats: MousedexStats | null = mousedexData
    ? { collected: mousedexData.count, total: mousedexData.total }
    : null

  const goMonth = (delta: number) => {
    let m = month + delta
    let y = year
    if (m < 1) { m = 12; y-- }
    if (m > 12) { m = 1; y++ }
    setYear(y)
    setMonth(m)
  }

  const monthLabel = `${year}-${String(month).padStart(2, '0')}`

  return (
    <div className="w-full h-full bg-[var(--bg-secondary)] px-8 py-6 overflow-y-auto flex flex-col gap-5 max-w-[800px] mx-auto">
      {/* Current Usage — Real-time from API */}
      <CurrentUsageSection usage={usage} />

      {/* Mousedex Summary */}
      {mousedexStats && (
        <>
          <div className="h-px bg-[var(--border)] my-1" />
          <MousedexSummaryCard stats={mousedexStats} onClick={() => setShowMousedex(true)} />
        </>
      )}
      {showMousedex && <MousedexGrid onClose={() => setShowMousedex(false)} />}

      {/* Monthly Stats */}
      <div className="h-px bg-[var(--border)] my-1" />
      <div className="stats-header" style={{ justifyContent: 'center' }}>
        <button className="icon-btn" onClick={() => goMonth(-1)}>&lt;</button>
        <span className="stats-month">{monthLabel}</span>
        <button className="icon-btn" onClick={() => goMonth(1)}>&gt;</button>
      </div>

      {loading ? (
        <div className="stats-loading">{t('stats.loading')}</div>
      ) : stats ? (
        <MonthlyStatsSection stats={stats} year={year} month={month} />
      ) : (
        <div className="stats-loading">{t('stats.noData')}</div>
      )}
    </div>
  )
}

/* ── Current Usage (real-time from API via WebSocket) ── */

function CurrentUsageSection({ usage }: { usage: Usage | null }) {
  const { t } = useTranslation()

  if (!usage?.fiveHour) {
    return (
      <div className="flex flex-col gap-3">
        <div className="usage-detail-section-title">{t('stats.currentWindow')}</div>
        <div className="text-[var(--text-tertiary)] text-[13px] py-4">{t('stats.noWindowActivity')}</div>
      </div>
    )
  }

  const utilization = usage.fiveHour.utilization
  const resetTime = formatTimeUntil(usage.fiveHour.resetsAt)

  return (
    <div className="flex flex-col gap-3">
      <div className="usage-detail-section-title">{t('stats.currentWindow')}</div>

      {/* Big gauge */}
      <div className="flex justify-center py-2">
        <svg viewBox="0 0 200 120" className="w-[200px] h-[120px]">
          <path
            d="M 20 110 A 80 80 0 0 1 180 110"
            fill="none"
            stroke="var(--border)"
            strokeWidth="12"
            strokeLinecap="round"
          />
          <path
            d="M 20 110 A 80 80 0 0 1 180 110"
            fill="none"
            stroke={getBarColor(utilization)}
            strokeWidth="12"
            strokeLinecap="round"
            strokeDasharray={`${Math.min(utilization, 100) / 100 * 251.3} 251.3`}
          />
          <text x="100" y="85" textAnchor="middle" className="usage-detail-gauge-value">
            {Math.round(utilization)}%
          </text>
          <text x="100" y="105" textAnchor="middle" className="usage-detail-gauge-label">
            5H
          </text>
        </svg>
      </div>

      {/* Reset time */}
      {resetTime && (
        <div className="flex gap-4 flex-wrap">
          <span className="flex items-center gap-1 text-xs text-[var(--text-secondary)]">
            <span className="text-sm">&#9202;</span>
            {t('usage.resetsIn', { time: resetTime })}
          </span>
        </div>
      )}

      {/* SevenDay */}
      {usage.sevenDay && (
        <div className="flex items-center gap-2 py-2">
          <div className="text-xs text-[var(--text-secondary)] whitespace-nowrap min-w-[110px]">7D</div>
          <div className="flex-1 h-2 bg-[var(--bg-tertiary)] rounded overflow-hidden">
            <div
              className="h-full rounded transition-[width] duration-300"
              style={{
                width: `${Math.min(usage.sevenDay.utilization, 100)}%`,
                backgroundColor: getBarColor(usage.sevenDay.utilization),
              }}
            />
          </div>
          <span className="text-xs font-semibold font-mono text-[var(--text-primary)] min-w-[32px] text-right">
            {Math.round(usage.sevenDay.utilization)}%
          </span>
        </div>
      )}
    </div>
  )
}

/* ── Monthly Stats ── */

function MonthlyStatsSection({ stats, year, month }: { stats: DashboardStats; year: number; month: number }) {
  const { t } = useTranslation()
  const usage = stats.usage || {}
  const worktime = stats.worktime || {}
  const cost = stats.summaryCost || {}
  const dailyUsage: DailyUsage[] = usage.daily || []
  const dailyWork: DailyWork[] = worktime.daily || []
  const dailyCompletions: DailyCompletion[] = stats.completions?.daily || []
  const dailySegmentedWork: DailySegmentedWork[] = worktime.dailySegmented || []
  const projects: ProjectWork[] = worktime.projects || []

  const maxUtil = Math.max(100, ...dailyUsage.map(d => d.maxFiveHourUtil || 0))
  const maxWorkSec = Math.max(1, ...dailyWork.map(d => d.workSec || 0))

  return (
    <>
      {/* Burn Calendar — daily completion heatmap */}
      <BurnHeatmap dailyCompletions={dailyCompletions} dailyWork={dailySegmentedWork} year={year} month={month} />

      {/* Summary Cards */}
      <div className="grid grid-cols-3 gap-2">
        <SummaryCard
          label={t('stats.avgUtil5h')}
          value={`${(usage.avgFiveHourUtil || 0).toFixed(1)}%`}
          color={getBarColor(usage.avgFiveHourUtil || 0)}
        />
        <SummaryCard
          label={t('stats.peakUtil')}
          value={`${(usage.maxFiveHourUtil || 0).toFixed(1)}%`}
          color={getBarColor(usage.maxFiveHourUtil || 0)}
        />
        <SummaryCard
          label={t('stats.daysAbove80')}
          value={`${usage.daysAbove80Pct || 0} / ${usage.totalDays || 0}`}
          color={(usage.daysAbove80Pct || 0) > 0 ? 'var(--danger)' : 'var(--success)'}
        />
        <SummaryCard
          label={t('stats.totalWork')}
          value={formatDuration(worktime.totalDurationSec || 0)}
          color="var(--text-primary)"
        />
        <SummaryCard
          label={t('stats.daysAbove50')}
          value={`${usage.daysAbove50Pct || 0} / ${usage.totalDays || 0}`}
          color={(usage.daysAbove50Pct || 0) > 0 ? 'var(--warning)' : 'var(--success)'}
        />
        <SummaryCard
          label={t('stats.haikuCalls')}
          value={String(cost.count || 0)}
          color="var(--text-primary)"
        />
      </div>

      {/* Daily Utilization Chart */}
      {dailyUsage.length > 0 && (
        <div className="flex flex-col gap-3">
          <div className="usage-detail-section-title">{t('stats.dailyUtil')}</div>
          <div className="stats-chart">
            <div className="stats-chart-threshold" style={{ bottom: `${(80 / maxUtil) * 100}%` }}>
              <span className="stats-chart-threshold-label">80%</span>
            </div>
            <div className="stats-chart-bars">
              {dailyUsage.map((d) => {
                const day = parseInt(d.date.split('-')[2], 10)
                const avgH = (d.avgFiveHourUtil / maxUtil) * 100
                const maxH = (d.maxFiveHourUtil / maxUtil) * 100
                return (
                  <div key={d.date} className="stats-bar-group" title={`${d.date}\navg: ${d.avgFiveHourUtil.toFixed(1)}%\nmax: ${d.maxFiveHourUtil.toFixed(1)}%`}>
                    <div className="stats-bar-stack">
                      <div
                        className="stats-bar stats-bar--max"
                        style={{ height: `${maxH}%`, background: getBarColor(d.maxFiveHourUtil) }}
                      />
                      <div
                        className="stats-bar stats-bar--avg"
                        style={{ height: `${avgH}%`, background: getBarColor(d.avgFiveHourUtil), opacity: 0.6 }}
                      />
                    </div>
                    {day % 5 === 1 && <span className="stats-bar-label">{day}</span>}
                  </div>
                )
              })}
            </div>
          </div>
        </div>
      )}

      {/* Daily Work Time Chart */}
      {dailyWork.length > 0 && (
        <div className="flex flex-col gap-3">
          <div className="usage-detail-section-title">{t('stats.dailyWork')}</div>
          <div className="stats-chart">
            <div className="stats-chart-bars">
              {dailyWork.map((d) => {
                const day = parseInt(d.date.split('-')[2], 10)
                const h = (d.workSec / maxWorkSec) * 100
                return (
                  <div key={d.date} className="stats-bar-group" title={`${d.date}\n${formatDuration(d.workSec)}`}>
                    <div className="stats-bar-stack">
                      <div
                        className="stats-bar"
                        style={{ height: `${h}%`, background: 'var(--accent)' }}
                      />
                    </div>
                    {day % 5 === 1 && <span className="stats-bar-label">{day}</span>}
                  </div>
                )
              })}
            </div>
          </div>
        </div>
      )}

      {/* Project Breakdown */}
      {projects.length > 0 && (
        <div className="flex flex-col gap-3">
          <div className="usage-detail-section-title">{t('stats.projectBreakdown')}</div>
          <div className="stats-session-list">
            {[...projects]
              .sort((a, b) => b.totalDurationSec - a.totalDurationSec)
              .map((p) => {
                const maxSec = projects.reduce((m, x) => Math.max(m, x.totalDurationSec), 1)
                const pct = (p.totalDurationSec / maxSec) * 100
                const displayName = p.projectPath === '(unknown)' ? p.projectPath : p.projectPath.split('/').pop() || p.projectPath
                return (
                  <div key={p.projectPath} className="stats-session-row">
                    <span className="stats-session-name" title={p.projectPath}>{displayName}</span>
                    <div className="stats-session-bar-track">
                      <div className="stats-session-bar-fill" style={{ width: `${pct}%` }} />
                    </div>
                    <span className="stats-session-value">{formatDuration(p.totalDurationSec)}</span>
                  </div>
                )
              })}
          </div>
        </div>
      )}
    </>
  )
}

/* ── Burn Heatmap — time-segment heatmap with mode toggle ── */

const SEGMENT_COUNT = 5
const SEGMENT_LABELS = ['00-05', '05-10', '10-15', '15-20', '20-24']

type HeatmapMode = 'worktime' | 'completions'

function BurnHeatmap({ dailyCompletions, dailyWork, year, month }: {
  dailyCompletions: DailyCompletion[]; dailyWork: DailySegmentedWork[]; year: number; month: number
}) {
  const { t } = useTranslation()
  const [mode, setMode] = useState<HeatmapMode>('worktime')

  // Build segment maps for both modes
  const compSegMap = new Map<number, number[]>()
  for (const d of dailyCompletions) {
    compSegMap.set(parseInt(d.date.split('-')[2], 10), d.segments || [])
  }
  const workSegMap = new Map<number, number[]>()
  for (const d of dailyWork) {
    workSegMap.set(parseInt(d.date.split('-')[2], 10), d.segments || [])
  }

  const segMap = mode === 'worktime' ? workSegMap : compSegMap
  const daysInMonth = new Date(year, month, 0).getDate()

  // Max value for color scaling
  const source = mode === 'worktime' ? dailyWork : dailyCompletions
  const allVals: number[] = []
  for (const d of source) {
    for (const c of (d.segments || [])) allVals.push(c)
  }
  const maxVal = Math.max(1, ...allVals)

  // Max streak
  let maxStreak = 0
  let streak = 0
  for (let d = 1; d <= daysInMonth; d++) {
    const segs = segMap.get(d)
    if (segs && segs.some(c => c > 0)) {
      streak++
      if (streak > maxStreak) maxStreak = streak
    } else {
      streak = 0
    }
  }

  const isWork = mode === 'worktime'

  return (
    <div className="burn-strip">
      <div className="burn-strip-header">
        <span className="burn-strip-title">{t('stats.burnCalendar')}</span>
        <div className="burn-mode-toggle">
          <button
            className={`burn-mode-btn${mode === 'worktime' ? ' burn-mode-btn--active' : ''}`}
            onClick={() => setMode('worktime')}
          >{t('stats.heatmapWork')}</button>
          <button
            className={`burn-mode-btn${mode === 'completions' ? ' burn-mode-btn--active' : ''}`}
            onClick={() => setMode('completions')}
          >{t('stats.heatmapDone')}</button>
        </div>
        {maxStreak > 1 && (
          <span className="burn-streak">
            <span className="burn-streak-flame">&#x1F525;</span>
            {t('stats.maxStreak', { count: maxStreak })}
          </span>
        )}
      </div>
      <div className="burn-heatmap">
        <div className="burn-heatmap-labels">
          {SEGMENT_LABELS.map(label => (
            <span key={label} className="burn-seg-label">{label}</span>
          ))}
        </div>
        <div className="burn-heatmap-grid">
          {Array.from({ length: daysInMonth }, (_, i) => {
            const day = i + 1
            const segs = segMap.get(day) || []
            const total = segs.reduce((a, b) => a + b, 0)
            const tipDetail = SEGMENT_LABELS.map((l, s) => {
              const v = segs[s]
              if (!v) return ''
              return `  ${l}: ${isWork ? formatDuration(v) : v}`
            }).filter(Boolean).join('\n')
            const tipTotal = isWork ? formatDuration(total) : `${total} ${t('stats.missions')}`
            return (
              <div
                key={day}
                className="burn-col"
                title={total > 0 ? `${day}: ${tipTotal}\n${tipDetail}` : `${day}`}
              >
                {Array.from({ length: SEGMENT_COUNT }, (_, seg) => {
                  const val = segs[seg] || 0
                  const intensity = val > 0
                    ? Math.min(Math.ceil((val / maxVal) * 4), 4)
                    : 0
                  const blazing = val > maxVal * 0.8 && val > (isWork ? 60 : 1)
                  return (
                    <div
                      key={seg}
                      className={[
                        'burn-dot',
                        isWork ? `burn-dot--wk${intensity}` : `burn-dot--lv${intensity}`,
                        blazing ? 'burn-dot--blaze' : '',
                      ].join(' ')}
                    />
                  )
                })}
              </div>
            )
          })}
        </div>
      </div>
      <div className="burn-strip-footer">
        <div className="burn-strip-labels">
          <span>1</span>
          <span>{Math.ceil(daysInMonth / 2)}</span>
          <span>{daysInMonth}</span>
        </div>
        <div className="burn-strip-legend">
          <span className="burn-legend-label">{t('stats.noActivity')}</span>
          {[0, 1, 2, 3, 4].map(lv => (
            <div key={lv} className={`burn-dot ${isWork ? `burn-dot--wk${lv}` : `burn-dot--lv${lv}`} burn-dot--legend`} />
          ))}
          <span className="burn-legend-label">{t('stats.highActivity')}</span>
        </div>
      </div>
    </div>
  )
}

function SummaryCard({ label, value, color }: { label: string; value: string; color: string }) {
  return (
    <div className="bg-[var(--bg-tertiary)] rounded-lg p-3 text-center">
      <div className="text-[11px] text-[var(--text-tertiary)] mb-1">{label}</div>
      <div className="text-lg font-bold font-mono" style={{ color }}>{value}</div>
    </div>
  )
}

/* ── Mousedex Summary Card ── */

function MousedexSummaryCard({ stats, onClick }: { stats: MousedexStats; onClick: () => void }) {
  const { t } = useTranslation()
  const pct = stats.total > 0 ? (stats.collected / stats.total) * 100 : 0

  return (
    <div
      className="px-3 py-2 cursor-pointer rounded-md transition-colors hover:bg-[var(--hover)]"
      onClick={onClick}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => e.key === 'Enter' && onClick()}
    >
      <div className="flex items-center gap-1.5 mb-1.5">
        <span className="text-base">🐭</span>
        <span className="text-[13px] font-semibold text-[var(--text-primary)] flex-1">{t('mousedex.title')}</span>
        <span className="text-xs text-[var(--text-secondary)] font-mono tabular-nums">{stats.collected} / {stats.total}</span>
      </div>
      <div className="h-1 bg-[var(--border)] rounded-sm overflow-hidden">
        <div
          className="h-full bg-[var(--accent)] rounded-sm transition-[width] duration-300"
          style={{ width: `${Math.min(pct, 100)}%` }}
        />
      </div>
    </div>
  )
}
