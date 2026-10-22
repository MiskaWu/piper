import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { RecoverableSession } from '../api'

interface Props {
  sessions: RecoverableSession[]
  onRestoreAll: () => Promise<void>
  onDismiss: () => void
  onShowDetails: () => void
}

export default function RecoveryBanner({ sessions, onRestoreAll, onDismiss, onShowDetails }: Props) {
  const { t } = useTranslation()
  const [restoring, setRestoring] = useState(false)

  const handleRestore = async () => {
    setRestoring(true)
    try { await onRestoreAll() } finally { setRestoring(false) }
  }

  return (
    <div className="flex items-center gap-3 px-4 py-2.5 bg-secondary border-b border-border text-[13px] text-foreground shrink-0">
      <span className="text-lg shrink-0">{'\uD83D\uDD04'}</span>
      <span className="flex-1 min-w-0">
        {t('recovery.bannerText', { count: sessions.length })}
      </span>
      <div className="flex gap-2 items-center shrink-0">
        <button
          className="text-xs text-primary bg-transparent border-none cursor-pointer underline p-0"
          onClick={onShowDetails}
        >
          {t('recovery.viewDetails')}
        </button>
        <button
          className="text-xs px-3.5 py-[5px] rounded bg-primary text-white border-none cursor-pointer font-medium flex items-center gap-1 hover:opacity-90 disabled:opacity-60 disabled:cursor-not-allowed"
          onClick={handleRestore}
          disabled={restoring}
        >
          {restoring && <span className="btn-spinner" />}
          {restoring ? t('recovery.restoring') : t('recovery.restoreAll', { count: sessions.length })}
        </button>
        <button
          className="bg-transparent border-none text-text-tertiary cursor-pointer p-0.5 flex items-center hover:text-foreground"
          onClick={onDismiss}
          title="Dismiss"
        >
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <line x1="18" y1="6" x2="6" y2="18" />
            <line x1="6" y1="6" x2="18" y2="18" />
          </svg>
        </button>
      </div>
    </div>
  )
}
