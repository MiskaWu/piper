import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '../lib/utils'
import type { RecoverableSession } from '../api'

function formatLastSeen(isoString: string, t: (key: string, opts?: Record<string, unknown>) => string): string {
  const diff = Math.floor((Date.now() - new Date(isoString).getTime()) / 1000)
  if (diff < 60) return t('recovery.justNow')
  const min = Math.floor(diff / 60)
  if (min < 60) return t('recovery.minutesAgo', { count: min })
  const hr = Math.floor(min / 60)
  if (hr < 24) return t('recovery.hoursAgo', { count: hr })
  const day = Math.floor(hr / 24)
  return t('recovery.daysAgo', { count: day })
}

function formatDate(isoString: string): string {
  const d = new Date(isoString)
  return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

function groupByDate(sessions: RecoverableSession[]): { date: string; items: RecoverableSession[] }[] {
  const groups = new Map<string, RecoverableSession[]>()
  for (const s of sessions) {
    const key = formatDate(s.lastSeenAt)
    const arr = groups.get(key) ?? []
    arr.push(s)
    groups.set(key, arr)
  }
  return [...groups.entries()].map(([date, items]) => ({ date, items }))
}

interface Props {
  sessions: RecoverableSession[]
  onSelect: (name: string) => void
  onReopen: (name: string) => Promise<unknown>
  onRemove: (name: string) => Promise<unknown>
}

export default function RecoveryPage({ sessions, onSelect, onReopen, onRemove }: Props) {
  const { t } = useTranslation()
  const groups = groupByDate(sessions)

  if (sessions.length === 0) {
    return (
      <div className="px-8 py-6 max-w-[720px] h-full overflow-y-auto">
        <div className="mb-5">
          <h2 className="text-xl font-semibold text-foreground m-0">{t('recovery.title')}</h2>
        </div>
        <div className="text-text-tertiary text-sm mt-10 text-center">{t('recovery.empty')}</div>
      </div>
    )
  }

  return (
    <div className="px-8 py-6 max-w-[720px] h-full overflow-y-auto">
      <div className="mb-5">
        <h2 className="text-xl font-semibold text-foreground m-0">{t('recovery.title')}</h2>
        <span className="text-[13px] text-text-tertiary">{t('recovery.subtitle', { count: sessions.length })}</span>
      </div>
      <div>
        {groups.map(({ date, items }) => (
          <div key={date} className="mb-4">
            <div className="text-xs font-semibold text-muted-foreground uppercase py-1 border-b border-border mb-1">
              {date}
            </div>
            {items.map((s) => (
              <RecoveryRow
                key={s.name}
                session={s}
                isActive={s.active}
                onSelect={onSelect}
                onReopen={onReopen}
                onRemove={onRemove}
              />
            ))}
          </div>
        ))}
      </div>
    </div>
  )
}

function RecoveryRow({ session, isActive, onSelect, onReopen, onRemove }: {
  session: RecoverableSession
  isActive: boolean
  onSelect: (name: string) => void
  onReopen: (name: string) => Promise<unknown>
  onRemove: (name: string) => Promise<unknown>
}) {
  const { t } = useTranslation()
  const [reopening, setReopening] = useState(false)
  const [removing, setRemoving] = useState(false)

  const handleReopen = async () => {
    setReopening(true)
    try { await onReopen(session.name) } finally { setReopening(false) }
  }

  const handleRemove = async () => {
    setRemoving(true)
    try { await onRemove(session.name) } finally { setRemoving(false) }
  }

  const busy = reopening || removing

  const btnBase = cn(
    "text-[11px] px-2.5 py-1 rounded border border-border",
    "bg-transparent text-muted-foreground cursor-pointer",
    "flex items-center gap-1",
    "hover:bg-accent disabled:opacity-50 disabled:cursor-not-allowed"
  )

  return (
    <div
      className={cn(
        "flex items-center gap-3 px-2 py-2.5 rounded-md",
        "hover:bg-accent",
        isActive && "border-l-[3px] border-l-primary"
      )}
      onClick={isActive ? () => onSelect(session.name) : undefined}
      style={isActive ? { cursor: 'pointer' } : undefined}
    >
      <div className="text-lg shrink-0 w-7 text-center">
        {session.isDevContainer ? '\uD83D\uDC33' : '\uD83D\uDCBB'}
      </div>
      <div className="flex-1 min-w-0">
        <div className="text-sm font-medium text-foreground whitespace-nowrap overflow-hidden text-ellipsis">
          {session.title || session.name}
          {isActive && (
            <span className={cn(
              "text-[10px] px-1.5 py-px rounded-sm bg-primary text-white ml-2 align-middle"
            )}>
              {t('recovery.active')}
            </span>
          )}
        </div>
        {session.description && (
          <div className="text-xs text-muted-foreground whitespace-nowrap overflow-hidden text-ellipsis mt-px">
            {session.description}
          </div>
        )}
        <div className="flex gap-2 text-[11px] text-text-tertiary mt-0.5">
          <span>{formatLastSeen(session.lastSeenAt, t)}</span>
          {session.projectPath && (
            <span className="font-mono max-w-[200px] overflow-hidden text-ellipsis whitespace-nowrap">
              {session.projectPath}
            </span>
          )}
        </div>
      </div>
      <div className="flex gap-1.5 shrink-0">
        {isActive ? (
          <button
            className={btnBase}
            onClick={(e) => { e.stopPropagation(); onSelect(session.name) }}
          >
            {t('recovery.goTo')}
          </button>
        ) : (
          <button
            className={cn(btnBase, "bg-primary text-white border-primary hover:opacity-90")}
            onClick={handleReopen}
            disabled={busy}
          >
            {reopening && <span className="btn-spinner" />}
            {t('recovery.reopen')}
          </button>
        )}
        {!isActive && (
          <button
            className={btnBase}
            onClick={(e) => { e.stopPropagation(); handleRemove() }}
            disabled={busy}
          >
            {t('recovery.remove')}
          </button>
        )}
      </div>
    </div>
  )
}
