import React from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '../lib/utils'
import { formatTimeUntil } from '../lib/format'
import type { Usage } from '../types'


function getBarColor(utilization: number): string {
  if (utilization >= 80) return 'var(--status-needs-input)'
  if (utilization >= 50) return 'var(--status-working)'
  return 'var(--status-idle)'
}

interface UsageBarProps {
  usage: Usage | null
}

export default function UsageBar({ usage }: UsageBarProps) {
  const { t } = useTranslation()

  if (!usage) return null

  const hasData = usage.fiveHour || usage.sevenDay

  if (!hasData) {
    const retryMin = usage.retryAfterSec ? Math.ceil(usage.retryAfterSec / 60) : 0
    return (
      <div className="flex flex-col gap-[5px] px-3 py-2 text-[13px] text-muted-foreground font-mono border-b border-border opacity-50">
        <span className="shrink-0 text-xs font-semibold text-text-tertiary">
          {t('usage.unavailable')}
          {retryMin > 0 && (
            <span className="ml-1.5">
              ({t('usage.retryIn', { time: `${retryMin}m` })})
            </span>
          )}
        </span>
      </div>
    )
  }

  const items: Array<{ label: string; utilization: number; resetsAt?: string }> = []
  if (usage.fiveHour) {
    items.push({ label: '5H', ...usage.fiveHour })
  }
  if (usage.sevenDay) {
    items.push({ label: '7D', ...usage.sevenDay })
  }

  const isStale = !!usage.stale

  return (
    <div
      className="flex flex-col gap-[5px] px-3 py-2 text-[13px] text-muted-foreground font-mono border-b border-border"
      title={isStale ? t('usage.staleTooltip') : undefined}
    >
      {items.map((item) => {
        const resetTime = formatTimeUntil(item.resetsAt)
        const barColor = getBarColor(item.utilization)
        return (
          <div key={item.label} className="flex items-center gap-1.5 min-w-0">
            <span className="shrink-0 text-xs font-semibold text-text-tertiary min-w-[22px]">{item.label}</span>
            <div className={cn(
              "flex-1 h-1.5 bg-accent rounded-sm min-w-[30px] relative",
              isStale && "border border-dashed border-text-tertiary"
            )}>
              <div
                className={cn(
                  "h-full rounded-sm transition-[width] duration-300 overflow-hidden",
                  isStale && "usage-fill--stale"
                )}
                style={{
                  width: `${Math.min(item.utilization, 100)}%`,
                  backgroundColor: barColor,
                  ...(isStale ? { '--fill-color': barColor } as React.CSSProperties : {}),
                }}
              />
            </div>
            <span className="shrink-0 text-[11px] font-semibold font-mono text-muted-foreground min-w-8 text-right">
              {isStale ? '~' : ''}{Math.round(item.utilization)}%
            </span>
            <span className="shrink-0 text-[10px] text-text-tertiary ml-0.5 min-w-[38px] text-right">
              {resetTime || ''}
            </span>
          </div>
        )
      })}
    </div>
  )
}
