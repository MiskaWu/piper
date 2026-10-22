import { useTranslation } from 'react-i18next'

export default function EmptyState() {
  const { t } = useTranslation()
  return (
    <div className="empty-state">
      <div className="empty-state-icon">
        <svg width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
          <polyline points="4 17 10 11 4 5" />
          <line x1="12" y1="19" x2="20" y2="19" />
        </svg>
      </div>
      <p className="empty-state-text">{t('emptyState.text')}</p>
    </div>
  )
}
