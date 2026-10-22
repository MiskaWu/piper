import { useState, useCallback, useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { useQueryClient } from '@tanstack/react-query'
import { useStoryMapEditor } from '../hooks/use-story-map-editor'
import { usePlanSession, useInfraStatus, planKeys } from '../hooks/usePlanQueries'
import { useEpicClaudeVersion } from '../hooks/useEpicClaude'
import { ClaudeVersionIndicator } from './ClaudeVersionIndicator'
import { extractAllTags } from '../lib/yaml-loader'
import { resolveReleaseColor } from '../lib/release-palette'
import { StoryMap } from './StoryMap'
import { StoryDetail } from './StoryDetail'
import { TagFilter } from './TagFilter'
import { createPlanSession, deletePlanSession, createDispatchSession, deleteDispatchSession, createRetroSession, deleteRetroSession, renameEpic, syncInfra } from '../../api'
import { RetroPanel } from './RetroPanel'
import { useProjects } from '../../hooks/useProjects'
import Terminal from '../../components/Terminal'
import type { ResolvedTheme } from '../types/theme'

/** 秘書鼠 — 戴圓眼鏡、打領結、持筆記板 */
function SecretaryMouse({ active }: { active: boolean }) {
  return (
    <svg
      className={`secretary-mouse${active ? ' secretary-mouse--active' : ''}`}
      viewBox="0 0 64 72"
      width="40"
      height="45"
      aria-hidden="true"
    >
      {/* 耳朵 */}
      <ellipse cx="18" cy="16" rx="10" ry="12" fill="#FADCB0" />
      <ellipse cx="18" cy="16" rx="7" ry="8" fill="#FFD0A8" />
      <ellipse cx="46" cy="16" rx="10" ry="12" fill="#FADCB0" />
      <ellipse cx="46" cy="16" rx="7" ry="8" fill="#FFD0A8" />
      {/* 臉 */}
      <ellipse cx="32" cy="36" rx="20" ry="18" fill="#FADCB0" />
      {/* 圓眼鏡 */}
      <circle cx="24" cy="33" r="7" fill="none" stroke="#4D96FF" strokeWidth="1.5" />
      <circle cx="40" cy="33" r="7" fill="none" stroke="#4D96FF" strokeWidth="1.5" />
      <line x1="31" y1="33" x2="33" y2="33" stroke="#4D96FF" strokeWidth="1.5" />
      <line x1="17" y1="32" x2="14" y2="28" stroke="#4D96FF" strokeWidth="1.2" />
      <line x1="47" y1="32" x2="50" y2="28" stroke="#4D96FF" strokeWidth="1.2" />
      {/* 眼睛（在鏡片後） */}
      <circle cx="24" cy="33" r="3" fill="#1a1a2e" />
      <circle cx="24" cy="32" r="1.2" fill="#fff" />
      <circle cx="40" cy="33" r="3" fill="#1a1a2e" />
      <circle cx="40" cy="32" r="1.2" fill="#fff" />
      {/* 鼻子 */}
      <ellipse cx="32" cy="39" rx="3" ry="2" fill="#ffb6c1" />
      {/* 嘴巴 */}
      <path d="M28,42 Q32,46 36,42" fill="none" stroke="#c97878" strokeWidth="1.2" strokeLinecap="round" />
      {/* 鬍鬚 */}
      <line x1="14" y1="37" x2="6" y2="35" stroke="#c9a87c" strokeWidth="0.8" opacity="0.5" />
      <line x1="14" y1="40" x2="6" y2="41" stroke="#c9a87c" strokeWidth="0.8" opacity="0.5" />
      <line x1="50" y1="37" x2="58" y2="35" stroke="#c9a87c" strokeWidth="0.8" opacity="0.5" />
      <line x1="50" y1="40" x2="58" y2="41" stroke="#c9a87c" strokeWidth="0.8" opacity="0.5" />
      {/* 領結 */}
      <g transform="translate(32,54)">
        <path d="M0,0 L-6,-4 L-6,4 Z" fill="#4D96FF" />
        <path d="M0,0 L6,-4 L6,4 Z" fill="#4D96FF" />
        <circle cx="0" cy="0" r="2" fill="#FFD93D" />
      </g>
      {/* 筆記板（右手持） */}
      <g transform="translate(52,48) rotate(15)">
        <rect x="-5" y="-8" width="10" height="14" rx="1.5" fill="#FFB347" />
        <rect x="-3.5" y="-5" width="7" height="1.5" rx="0.5" fill="#fff" opacity="0.7" />
        <rect x="-3.5" y="-2" width="7" height="1.5" rx="0.5" fill="#fff" opacity="0.7" />
        <rect x="-3.5" y="1" width="5" height="1.5" rx="0.5" fill="#fff" opacity="0.7" />
        {/* 夾子 */}
        <rect x="-2" y="-9" width="4" height="3" rx="0.8" fill="#e0a030" />
      </g>
    </svg>
  )
}

/** 派送鼠 — 戴鴨舌帽、背郵差包、持信封 */
function DispatchMouse({ active }: { active: boolean }) {
  return (
    <svg
      className={`dispatch-mouse${active ? ' dispatch-mouse--active' : ''}`}
      viewBox="0 0 64 72"
      width="40"
      height="45"
      aria-hidden="true"
    >
      {/* 耳朵 */}
      <ellipse cx="18" cy="16" rx="10" ry="12" fill="#FADCB0" />
      <ellipse cx="18" cy="16" rx="7" ry="8" fill="#FFD0A8" />
      <ellipse cx="46" cy="16" rx="10" ry="12" fill="#FADCB0" />
      <ellipse cx="46" cy="16" rx="7" ry="8" fill="#FFD0A8" />
      {/* 鴨舌帽 */}
      <ellipse cx="32" cy="20" rx="18" ry="8" fill="#4CAF50" />
      <ellipse cx="32" cy="20" rx="16" ry="6" fill="#66BB6A" />
      <path d="M14,20 Q10,18 8,22 L20,20 Z" fill="#4CAF50" />
      {/* 臉 */}
      <ellipse cx="32" cy="36" rx="20" ry="18" fill="#FADCB0" />
      {/* 眼睛 */}
      <circle cx="24" cy="33" r="3" fill="#1a1a2e" />
      <circle cx="24" cy="32" r="1.2" fill="#fff" />
      <circle cx="40" cy="33" r="3" fill="#1a1a2e" />
      <circle cx="40" cy="32" r="1.2" fill="#fff" />
      {/* 鼻子 */}
      <ellipse cx="32" cy="39" rx="3" ry="2" fill="#ffb6c1" />
      {/* 嘴巴 */}
      <path d="M28,42 Q32,46 36,42" fill="none" stroke="#c97878" strokeWidth="1.2" strokeLinecap="round" />
      {/* 鬍鬚 */}
      <line x1="14" y1="37" x2="6" y2="35" stroke="#c9a87c" strokeWidth="0.8" opacity="0.5" />
      <line x1="14" y1="40" x2="6" y2="41" stroke="#c9a87c" strokeWidth="0.8" opacity="0.5" />
      <line x1="50" y1="37" x2="58" y2="35" stroke="#c9a87c" strokeWidth="0.8" opacity="0.5" />
      <line x1="50" y1="40" x2="58" y2="41" stroke="#c9a87c" strokeWidth="0.8" opacity="0.5" />
      {/* 郵差包背帶 */}
      <line x1="22" y1="50" x2="18" y2="62" stroke="#8D6E63" strokeWidth="2.5" strokeLinecap="round" />
      <line x1="42" y1="50" x2="46" y2="62" stroke="#8D6E63" strokeWidth="2.5" strokeLinecap="round" />
      {/* 信封（右手持） */}
      <g transform="translate(52,50) rotate(10)">
        <rect x="-7" y="-5" width="14" height="10" rx="1.5" fill="#FFF9C4" stroke="#FFB300" strokeWidth="1" />
        <path d="M-7,-5 L0,1 L7,-5" fill="none" stroke="#FFB300" strokeWidth="1" />
      </g>
    </svg>
  )
}

/** 收割鼠 — 戴草帽、持鐮刀、背竹簍 */
function RetroMouse({ active }: { active: boolean }) {
  return (
    <svg
      className={`retro-mouse${active ? ' retro-mouse--active' : ''}`}
      viewBox="0 0 64 72"
      width="40"
      height="45"
      aria-hidden="true"
    >
      {/* 耳朵 */}
      <ellipse cx="18" cy="16" rx="10" ry="12" fill="#FADCB0" />
      <ellipse cx="18" cy="16" rx="7" ry="8" fill="#FFD0A8" />
      <ellipse cx="46" cy="16" rx="10" ry="12" fill="#FADCB0" />
      <ellipse cx="46" cy="16" rx="7" ry="8" fill="#FFD0A8" />
      {/* 草帽 */}
      <ellipse cx="32" cy="18" rx="22" ry="6" fill="#DEB887" />
      <ellipse cx="32" cy="16" rx="16" ry="8" fill="#F5DEB3" />
      <ellipse cx="32" cy="16" rx="14" ry="6" fill="#FAEBD7" />
      {/* 帽帶 */}
      <rect x="18" y="19" width="28" height="2.5" rx="1" fill="#CD853F" />
      {/* 臉 */}
      <ellipse cx="32" cy="36" rx="20" ry="18" fill="#FADCB0" />
      {/* 眼睛 */}
      <circle cx="24" cy="33" r="3" fill="#1a1a2e" />
      <circle cx="24" cy="32" r="1.2" fill="#fff" />
      <circle cx="40" cy="33" r="3" fill="#1a1a2e" />
      <circle cx="40" cy="32" r="1.2" fill="#fff" />
      {/* 鼻子 */}
      <ellipse cx="32" cy="39" rx="3" ry="2" fill="#ffb6c1" />
      {/* 嘴巴 — 開心 */}
      <path d="M26,42 Q32,48 38,42" fill="none" stroke="#c97878" strokeWidth="1.2" strokeLinecap="round" />
      {/* 鬍鬚 */}
      <line x1="14" y1="37" x2="6" y2="35" stroke="#c9a87c" strokeWidth="0.8" opacity="0.5" />
      <line x1="14" y1="40" x2="6" y2="41" stroke="#c9a87c" strokeWidth="0.8" opacity="0.5" />
      <line x1="50" y1="37" x2="58" y2="35" stroke="#c9a87c" strokeWidth="0.8" opacity="0.5" />
      <line x1="50" y1="40" x2="58" y2="41" stroke="#c9a87c" strokeWidth="0.8" opacity="0.5" />
      {/* 竹簍（背後） */}
      <g transform="translate(50,48)">
        <path d="M-4,-6 L4,-6 L5,6 L-5,6 Z" fill="#D2B48C" stroke="#8B7355" strokeWidth="0.8" />
        <line x1="-4" y1="-3" x2="4" y2="-3" stroke="#8B7355" strokeWidth="0.6" />
        <line x1="-4.5" y1="0" x2="4.5" y2="0" stroke="#8B7355" strokeWidth="0.6" />
        <line x1="-5" y1="3" x2="5" y2="3" stroke="#8B7355" strokeWidth="0.6" />
      </g>
      {/* 鐮刀（左手） */}
      <g transform="translate(10,52) rotate(-20)">
        <rect x="-1" y="-8" width="2.5" height="12" rx="1" fill="#8B7355" />
        <path d="M0,-8 Q8,-10 6,-4" fill="none" stroke="#A9A9A9" strokeWidth="2" strokeLinecap="round" />
      </g>
    </svg>
  )
}

interface PlanViewProps {
  epicName: string
  theme?: ResolvedTheme
}

export function PlanView({ epicName, theme = 'dark' }: PlanViewProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { data, error, selectedStory, handlers } = useStoryMapEditor(epicName)
  const [showRetro, setShowRetro] = useState(false)
  const [activeTags, setActiveTags] = useState<string[]>([])
  const { data: sessionData } = usePlanSession(epicName)
  const planSession = sessionData?.planSession ?? null
  const dispatchSession = sessionData?.dispatchSession ?? null
  const retroSession = sessionData?.retroSession ?? null
  const [sessionLoading, setSessionLoading] = useState(false)
  const [dispatchLoading, setDispatchLoading] = useState(false)
  const [retroLoading, setRetroLoading] = useState(false)
  const { data: projectsData } = useProjects()
  const projectOptions = projectsData?.projects?.map((p) => ({ slug: p.slug, name: p.name })) ?? []
  const [editingTitle, setEditingTitle] = useState(false)
  const [titleDraft, setTitleDraft] = useState('')
  const titleInputRef = useRef<HTMLInputElement>(null)
  const { data: infraStatus } = useInfraStatus(epicName)
  const [infraSyncing, setInfraSyncing] = useState(false)
  const { data: claudeVersion } = useEpicClaudeVersion()
  const claudeUpdating = claudeVersion?.update?.inProgress === true
  const blockNewSession = (sessionExists: boolean) => claudeUpdating && !sessionExists

  const invalidateSessions = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: planKeys.session(epicName) })
  }, [queryClient, epicName])

  const handleStartPlan = useCallback(async () => {
    setSessionLoading(true)
    try {
      await createPlanSession(epicName)
      invalidateSessions()
    } catch {
      // ignore
    } finally {
      setSessionLoading(false)
    }
  }, [epicName, invalidateSessions])

  const handleStopPlan = useCallback(async () => {
    setSessionLoading(true)
    try {
      await deletePlanSession(epicName)
      invalidateSessions()
    } catch {
      // ignore
    } finally {
      setSessionLoading(false)
    }
  }, [epicName, invalidateSessions])

  const handleStartDispatch = useCallback(async () => {
    setDispatchLoading(true)
    try {
      await createDispatchSession(epicName)
      invalidateSessions()
    } catch {
      // ignore
    } finally {
      setDispatchLoading(false)
    }
  }, [epicName, invalidateSessions])

  const handleStopDispatch = useCallback(async () => {
    setDispatchLoading(true)
    try {
      await deleteDispatchSession(epicName)
      invalidateSessions()
    } catch {
      // ignore
    } finally {
      setDispatchLoading(false)
    }
  }, [epicName, invalidateSessions])

  const handleStartRetro = useCallback(async () => {
    setRetroLoading(true)
    try {
      await createRetroSession(epicName)
      invalidateSessions()
    } catch {
      // ignore
    } finally {
      setRetroLoading(false)
    }
  }, [epicName, invalidateSessions])

  const handleStopRetro = useCallback(async () => {
    setRetroLoading(true)
    try {
      await deleteRetroSession(epicName)
      invalidateSessions()
    } catch {
      // ignore
    } finally {
      setRetroLoading(false)
    }
  }, [epicName, invalidateSessions])

  useEffect(() => {
    if (editingTitle && titleInputRef.current) {
      titleInputRef.current.focus()
      titleInputRef.current.select()
    }
  }, [editingTitle])

  const handleTitleClick = useCallback(() => {
    if (data) {
      setTitleDraft(data.meta.name)
      setEditingTitle(true)
    }
  }, [data])

  const handleTitleSave = useCallback(() => {
    setEditingTitle(false)
    const trimmed = titleDraft.trim()
    if (!trimmed || !data || trimmed === data.meta.name) return
    renameEpic(epicName, trimmed).catch(() => {})
  }, [epicName, titleDraft, data])

  const handleTitleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (e.key === 'Enter') handleTitleSave()
    else if (e.key === 'Escape') setEditingTitle(false)
  }, [handleTitleSave])

  const handleToggleTag = useCallback((tag: string) => {
    setActiveTags((prev) =>
      prev.includes(tag) ? prev.filter((t) => t !== tag) : [...prev, tag],
    )
  }, [])

  const outdatedCount = infraStatus?.projects?.filter((p) => p.updateAvailable).length ?? 0

  const handleSyncInfra = useCallback(async () => {
    setInfraSyncing(true)
    try {
      await syncInfra(epicName)
      queryClient.invalidateQueries({ queryKey: planKeys.infraStatus(epicName) })
    } catch {
      // ignore
    } finally {
      setInfraSyncing(false)
    }
  }, [epicName, queryClient])

  if (error) {
    return <div className="plan-view-error">Error: {error}</div>
  }

  if (!data) {
    return <div className="plan-view-loading">Loading story map...</div>
  }

  const allTags = extractAllTags(data)
  const activeSession = planSession || dispatchSession || retroSession

  const selectedReleaseColor = selectedStory
    ? (() => {
        const rc = data.meta.releases.find((r) => r.id === selectedStory.release)
        return rc ? resolveReleaseColor(rc.color, theme) : undefined
      })()
    : undefined

  return (
    <div className={`plan-view${activeSession || showRetro ? ' plan-view--split' : ''}`}>
      <div className="plan-view-main">
        <div className="plan-view-header">
          {editingTitle ? (
            <input
              ref={titleInputRef}
              className="plan-view-title plan-view-title--editing"
              value={titleDraft}
              onChange={(e) => setTitleDraft(e.target.value)}
              onBlur={handleTitleSave}
              onKeyDown={handleTitleKeyDown}
            />
          ) : (
            <h2
              className="plan-view-title"
              onClick={handleTitleClick}
              title={t('plan.clickToRename')}
            >
              {data.meta.name}
            </h2>
          )}
          <div className="plan-view-actions">
            <button
              className={`plan-session-btn${planSession ? ' plan-session-btn--active plan-session-btn--compact' : ' plan-session-btn--hero'}`}
              onClick={planSession ? handleStopPlan : handleStartPlan}
              disabled={sessionLoading || !!dispatchSession || !!retroSession || blockNewSession(!!planSession)}
              title={blockNewSession(!!planSession) ? t('plan.claudeUpdateBlocking') as string : undefined}
            >
              <SecretaryMouse active={!!planSession} />
              <span className="plan-session-btn__label">
                {sessionLoading
                  ? '...'
                  : planSession
                    ? t('plan.stop')
                    : t('plan.plan')}
              </span>
            </button>
            <button
              className={`plan-session-btn dispatch-btn${dispatchSession ? ' plan-session-btn--active plan-session-btn--compact' : ' plan-session-btn--hero'}`}
              onClick={dispatchSession ? handleStopDispatch : handleStartDispatch}
              disabled={dispatchLoading || !!planSession || !!retroSession || blockNewSession(!!dispatchSession)}
              title={blockNewSession(!!dispatchSession) ? t('plan.claudeUpdateBlocking') as string : undefined}
            >
              <DispatchMouse active={!!dispatchSession} />
              <span className="plan-session-btn__label">
                {dispatchLoading
                  ? '...'
                  : dispatchSession
                    ? t('plan.stopDispatch')
                    : t('plan.dispatch')}
              </span>
            </button>
            <button
              className={`plan-session-btn retro-btn${retroSession ? ' plan-session-btn--active plan-session-btn--compact' : ' plan-session-btn--hero'}`}
              onClick={retroSession ? handleStopRetro : handleStartRetro}
              disabled={retroLoading || !!planSession || !!dispatchSession || blockNewSession(!!retroSession)}
              title={blockNewSession(!!retroSession) ? t('plan.claudeUpdateBlocking') as string : undefined}
            >
              <RetroMouse active={!!retroSession} />
              <span className="plan-session-btn__label">
                {retroLoading
                  ? '...'
                  : retroSession
                    ? t('plan.stopRetro')
                    : t('plan.retro')}
              </span>
            </button>
            <button
              type="button"
              className={`plan-view-side-tab${showRetro ? ' plan-view-side-tab--active' : ''}`}
              onClick={() => setShowRetro((v) => !v)}
              title={showRetro ? t('plan.retroClose') as string : t('plan.retroOpen') as string}
            >
              📊 {showRetro ? t('plan.retroClose') : t('plan.retroOpen')}
            </button>
          </div>
          <ClaudeVersionIndicator />
          {(infraStatus?.projects?.length ?? 0) > 0 && (
            <button
              className={`infra-update-badge${outdatedCount === 0 ? ' infra-update-badge--ok' : ''}`}
              onClick={handleSyncInfra}
              disabled={infraSyncing || outdatedCount === 0}
              title={infraStatus?.projects
                ?.filter((p) => p.updateAvailable)
                .map((p) => p.name)
                .join(', ') || t('plan.infraUpToDate')}
            >
              {infraSyncing
                ? t('plan.infraUpdating')
                : outdatedCount > 0
                  ? t('plan.infraUpdate', { count: outdatedCount })
                  : t('plan.infraUpToDate')}
            </button>
          )}
        </div>
        <TagFilter
          tags={allTags}
          activeTags={activeTags}
          onToggle={handleToggleTag}
        />
        <StoryMap
          data={data}
          activeTags={activeTags}
          theme={theme}
          onMoveRelease={handlers.handleMoveRelease}
          onReorderStory={handlers.handleReorderStory}
          onSelectStory={handlers.handleSelectStory}
          onAddRelease={handlers.handleAddRelease}
          onRemoveRelease={handlers.handleRemoveRelease}
          onAddStory={handlers.handleAddStory}
          onAddStep={handlers.handleAddStep}
          onAddActivity={handlers.handleAddActivity}
        />
      </div>
      {selectedStory && (
        <StoryDetail
          story={selectedStory}
          releaseColor={selectedReleaseColor}
          projects={projectOptions}
          onClose={() => handlers.handleSelectStory(selectedStory)}
          onStatusChange={handlers.handleStatusChange}
          onDelete={handlers.handleDeleteStory}
          onAddTask={handlers.handleAddTask}
          onUpdateTask={handlers.handleUpdateTask}
          onDeleteTask={handlers.handleDeleteTask}
          onUpdateAcceptance={handlers.handleUpdateAcceptance}
        />
      )}
      {activeSession && (
        <div className="plan-view-terminal">
          <Terminal sessionName={activeSession} theme={theme} />
        </div>
      )}
      {showRetro && !activeSession && (
        <div className="plan-view-retro">
          <RetroPanel epicName={epicName} onClose={() => setShowRetro(false)} />
        </div>
      )}
    </div>
  )
}
