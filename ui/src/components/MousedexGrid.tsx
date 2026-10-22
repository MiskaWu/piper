import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { useMousedex } from '../hooks/useDashboardStats'
import MouseAvatar, { enumerateAllSpeciesKeys, parseSpeciesKey, type MouseConfig } from './MouseAvatar'

interface MousedexGridProps {
  onClose: () => void
}

export default function MousedexGrid({ onClose }: MousedexGridProps) {
  const { t } = useTranslation()
  const { data, isLoading: loading } = useMousedex()

  const allKeys = useMemo(() => enumerateAllSpeciesKeys(), [])

  if (loading) {
    return (
      <div className="fixed inset-0 bg-black/60 z-[1000] flex items-center justify-center" onClick={onClose}>
        <div className="bg-[var(--bg-primary)] border border-[var(--border)] rounded-xl w-[90vw] max-w-[720px] max-h-[85vh] flex flex-col overflow-hidden" onClick={(e) => e.stopPropagation()}>
          <div className="p-10 text-center text-[var(--text-secondary)]">{t('stats.loading')}</div>
        </div>
      </div>
    )
  }

  const collected = data?.collected || {}
  const count = data?.count || 0
  const total = data?.total || 1332
  const pct = total > 0 ? ((count / total) * 100).toFixed(1) : '0'

  return (
    <div className="fixed inset-0 bg-black/60 z-[1000] flex items-center justify-center" onClick={onClose}>
      <div className="bg-[var(--bg-primary)] border border-[var(--border)] rounded-xl w-[90vw] max-w-[720px] max-h-[85vh] flex flex-col overflow-hidden" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between px-4 py-3 border-b border-[var(--border)]">
          <span className="text-base font-bold text-[var(--text-primary)]">{t('mousedex.title')}</span>
          <button className="bg-transparent border-none text-[var(--text-secondary)] text-xl cursor-pointer px-1 leading-none hover:text-[var(--text-primary)]" onClick={onClose}>&times;</button>
        </div>

        {/* 進度條 */}
        <div className="px-4 py-3 flex items-center gap-3 border-b border-[var(--border)]">
          <div className="flex-1 h-1.5 bg-[var(--border)] rounded-sm overflow-hidden">
            <div
              className="h-full bg-[var(--accent)] rounded-sm transition-[width] duration-300"
              style={{ width: `${Math.min((count / total) * 100, 100)}%` }}
            />
          </div>
          <span className="text-xs text-[var(--text-secondary)] font-mono tabular-nums whitespace-nowrap">
            {count} / {total} ({pct}%)
          </span>
        </div>

        {/* 圖鑑格子 */}
        <div className="mousedex-grid">
          {allKeys.map((key) => {
            const entry = collected[key]
            const isCollected = !!entry
            const parts = parseSpeciesKey(key)
            const config: MouseConfig = isCollected
              ? {
                  isSSR: entry.isSSR,
                  bodyColor: entry.bodyColor,
                  color: entry.color,
                  topIdx: entry.topIdx,
                  midIdx: entry.midIdx,
                  botIdx: entry.botIdx,
                }
              : {
                  isSSR: parts.isSSR,
                  bodyColor: '#888',
                  color: '#666',
                  topIdx: parts.topIdx,
                  midIdx: parts.midIdx,
                  botIdx: parts.botIdx,
                }
            const tooltip = isCollected
              ? `${key}\n${t('mousedex.unlocked')}: ${entry.unlockedAt}\n${t('mousedex.unlockCount')}: ${entry.unlockCount}\n${entry.sessionName}`
              : key
            return (
              <div
                key={key}
                className={`mousedex-cell ${isCollected ? 'mousedex-cell--collected' : ''}`}
                title={tooltip}
              >
                <MouseAvatar config={config} size={36} grayscale={!isCollected} />
                {isCollected && entry.unlockCount > 1 && (
                  <span className="mousedex-cell-badge">×{entry.unlockCount}</span>
                )}
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
}
