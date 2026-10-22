import { useTranslation } from 'react-i18next'
import { useEpicClaudeVersion, useStartEpicClaudeUpdate } from '../hooks/useEpicClaude'

export function ClaudeVersionIndicator() {
  const { t } = useTranslation()
  const { data, isLoading } = useEpicClaudeVersion()
  const startUpdate = useStartEpicClaudeUpdate()

  if (isLoading || !data) return null
  if (!data.dockerAvailable) return null
  if (!data.imageExists) return null

  const updating = data.update?.inProgress === true
  const errorMsg = data.update?.error
  const needsUpdate = data.needsUpdate === true

  if (updating) {
    return (
      <button
        className="claude-version-badge claude-version-badge--updating"
        disabled
        title={t('plan.claudeUpdateBlocking')}
      >
        {t('plan.claudeUpdating')}
      </button>
    )
  }

  if (errorMsg) {
    return (
      <button
        className="claude-version-badge claude-version-badge--error"
        onClick={() => startUpdate.mutate()}
        disabled={startUpdate.isPending}
        title={t('plan.claudeUpdateFailed', { error: errorMsg })}
      >
        {t('plan.claudeUpdate')}
      </button>
    )
  }

  if (needsUpdate && data.current && data.latest) {
    return (
      <button
        className="claude-version-badge claude-version-badge--needs-update"
        onClick={() => startUpdate.mutate()}
        disabled={startUpdate.isPending}
        title={t('plan.claudeNeedsUpdate', { current: data.current, latest: data.latest })}
      >
        {t('plan.claudeNeedsUpdate', { current: data.current, latest: data.latest })}
      </button>
    )
  }

  // up-to-date or unknown latest
  const version = data.current ?? ''
  return (
    <span
      className="claude-version-badge claude-version-badge--ok"
      title={t('plan.claudeUpToDate', { version })}
    >
      {t('plan.claudeVersion', { version })}
    </span>
  )
}
