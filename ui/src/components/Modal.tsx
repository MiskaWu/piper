import React, { useState, useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { createDevcontainerSession, setProjectAutoMode, updateProjectOrder } from '../api'
import { useProjects, useBrowseDirectory, useAddProject, useRemoveProject, useRebuildProject, useUpgradeProject, useUpdateProjectClaude } from '../hooks/useProjects'
import { Button } from './ui/button'
import { cn } from '../lib/utils'
import type { TFunction } from 'i18next'
import type { Project, Session } from '../types'

interface DirPickerProps {
  value: string
  onChange: (path: string) => void
  label?: string
  t: TFunction
}

function DirPicker({ value, onChange, label, t }: DirPickerProps) {
  const [browsePath, setBrowsePath] = useState('')
  const [manualPath, setManualPath] = useState(false)

  const { data: browseResult, isLoading: loading } = useBrowseDirectory(browsePath)

  // 初次載入或 browsePath 變更時，自動將 current 回填給 parent
  const prevCurrentRef = useRef<string | undefined>(undefined)
  useEffect(() => {
    if (browseResult && browseResult.current !== prevCurrentRef.current) {
      prevCurrentRef.current = browseResult.current
      // 初次載入時（value 為空），或使用者透過 navigateTo 瀏覽時，同步選取值
      if (!value || browsePath) {
        onChange(browseResult.current)
      }
    }
  }, [browseResult]) // eslint-disable-line react-hooks/exhaustive-deps

  const navigateTo = (path: string) => {
    setBrowsePath(path)
  }

  const breadcrumbs = browseResult
    ? browseResult.current.split('/').filter(Boolean)
    : []

  return (
    <div className="dir-picker">
      <div className="dir-picker-header">
        <label className="dir-picker-label">{label || t('modal.workDir')}</label>
        <button
          type="button"
          className="dir-picker-toggle"
          onClick={() => setManualPath(!manualPath)}
        >
          {manualPath ? t('modal.browse') : t('modal.customPath')}
        </button>
      </div>

      {manualPath ? (
        <input
          type="text"
          className="modal-input"
          placeholder={t('modal.workDirPlaceholder') as string}
          value={value}
          onChange={(e) => onChange(e.target.value)}
        />
      ) : (
        <div className="dir-browser">
          <div className="dir-breadcrumb">
            <button
              type="button"
              className="dir-crumb"
              onClick={() => navigateTo('/')}
            >/</button>
            {breadcrumbs.map((part, i) => {
              const path = '/' + breadcrumbs.slice(0, i + 1).join('/')
              const isLast = i === breadcrumbs.length - 1
              return (
                <span key={path}>
                  <button
                    type="button"
                    className={`dir-crumb${isLast ? ' dir-crumb--active' : ''}`}
                    onClick={() => !isLast && navigateTo(path)}
                  >
                    {part}
                  </button>
                  {!isLast && <span className="dir-crumb-sep">/</span>}
                </span>
              )
            })}
          </div>

          <div className="dir-list">
            {loading && <div className="dir-list-loading">{t('modal.loading')}</div>}
            {!loading && browseResult && (
              <>
                {browseResult.current !== browseResult.parent && (
                  <button
                    type="button"
                    className="dir-item dir-item--parent"
                    onClick={() => navigateTo(browseResult.parent)}
                  >
                    ..
                  </button>
                )}
                {browseResult.entries.length === 0 && (
                  <div className="dir-list-empty">{t('modal.noSubDirs')}</div>
                )}
                {browseResult.entries.map((entry) => (
                  <button
                    key={entry.path}
                    type="button"
                    className={`dir-item${entry.path === value ? ' dir-item--selected' : ''}`}
                    onClick={() => onChange(entry.path)}
                    onDoubleClick={() => navigateTo(entry.path)}
                    title={t('modal.doubleClickToOpen') as string}
                  >
                    <span className="dir-item-icon">{'\uD83D\uDCC1'}</span>
                    <span className="dir-item-name">{entry.name}</span>
                  </button>
                ))}
              </>
            )}
          </div>
        </div>
      )}

      <div className="dir-selected">
        <span className="dir-selected-label">{t('modal.selected')}:</span>
        <code className="dir-selected-path">{value || '~'}</code>
      </div>
    </div>
  )
}

interface LocalTabProps {
  workDir: string
  setWorkDir: (dir: string) => void
  onSubmit: (workDir: string) => void
  onClose: () => void
  t: TFunction
  initialDescription?: string
}

function LocalTab({ workDir, setWorkDir, onSubmit, onClose, t, initialDescription }: LocalTabProps) {
  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    onSubmit(workDir)
  }

  return (
    <form onSubmit={handleSubmit}>
      <div className="mb-3 px-3 py-2 rounded-md bg-success/10 border border-success text-xs text-foreground">
        {t('modal.sandboxHint')}
      </div>

      <DirPicker value={workDir} onChange={setWorkDir} t={t} />

      {initialDescription && (
        <div className="inbox-item-enriched mt-3 whitespace-pre-wrap text-xs">
          <div className="text-[11px] font-semibold mb-1 text-muted-foreground">{t('inbox.enrichedDesc')}</div>
          {initialDescription}
        </div>
      )}

      <div className="flex justify-end gap-2 mt-4">
        <Button type="button" variant="outline" onClick={onClose}>
          {t('modal.cancel')}
        </Button>
        <Button type="submit">
          {t('modal.create')}
        </Button>
      </div>
    </form>
  )
}

interface StateConfig {
  btnKey: string | null
  shortKey: string
  icon: string
}

const stateConfig: Record<string, StateConfig> = {
  running: { btnKey: 'openSession', shortKey: 'openShort', icon: '\u25B6' },
  stopped: { btnKey: 'startAndOpen', shortKey: 'startShort', icon: '\u23FB' },
  none: { btnKey: 'createAndOpen', shortKey: 'createShort', icon: '+' },
  error: { btnKey: null, shortKey: 'rebuildShort', icon: '\u21BB' },
}

function shortenPath(p: string): string {
  const home = typeof window !== 'undefined' && (window as unknown as Record<string, string>).__HOME_DIR__
  if (home && p.startsWith(home)) return '~' + p.slice(home.length)
  return p
}

interface DevcontainerTabProps {
  onSessionCreated: (session: Session) => void
  onClose: () => void
  onNewProject?: () => void
  t: TFunction
}

function DevcontainerTab({ onSessionCreated, onClose, onNewProject, t }: DevcontainerTabProps) {
  const { data: projectsData, isLoading: loading, error: queryError } = useProjects()
  const addProjectMutation = useAddProject()
  const removeProjectMutation = useRemoveProject()
  const rebuildProjectMutation = useRebuildProject()
  const upgradeProjectMutation = useUpgradeProject()
  const updateClaudeMutation = useUpdateProjectClaude()

  const rawList: Project[] = projectsData?.projects || []
  const cliAvailable = projectsData?.devcontainerCLI !== false

  // 本地排序狀態（拖曳時即時反映）
  const [localOrder, setLocalOrder] = useState<Project[] | null>(null)
  const projects = localOrder || rawList

  // 當 query 資料更新時，重設本地排序
  useEffect(() => {
    setLocalOrder(null)
  }, [projectsData])

  const error = queryError ? (queryError as Error).message : null

  const [actionLoading, setActionLoading] = useState<Set<string>>(new Set())
  const [selectedPath, setSelectedPath] = useState('')
  const [addError, setAddError] = useState<string | null>(null)
  const [showPicker, setShowPicker] = useState(false)
  const [autoModeMap, setAutoModeMap] = useState<Record<string, boolean>>({})
  const [dragIndex, setDragIndex] = useState<number | null>(null)
  const [dropIndex, setDropIndex] = useState<number | null>(null)
  const dragNode = useRef<EventTarget | null>(null)
  const [removeConfirm, setRemoveConfirm] = useState<Project | null>(null)
  const [cleanupContainers, setCleanupContainers] = useState(true)
  const [cleanupImages, setCleanupImages] = useState(false)

  // 從 API 資料初始化 autoMode map
  useEffect(() => {
    if (rawList.length > 0) {
      const map: Record<string, boolean> = {}
      rawList.forEach((p: Project) => {
        map[p.name] = p.autoMode ?? false
      })
      setAutoModeMap(map)
    }
  }, [projectsData]) // eslint-disable-line react-hooks/exhaustive-deps

  const handleToggleAutoMode = async (project: Project) => {
    const current = autoModeMap[project.name] ?? false
    const next = !current
    setAutoModeMap((prev) => ({ ...prev, [project.name]: next }))
    try {
      await setProjectAutoMode(project.slug, next)
    } catch {
      // Revert on failure
      setAutoModeMap((prev) => ({ ...prev, [project.name]: current }))
    }
  }

  const handleAddProject = async (): Promise<boolean> => {
    const trimmed = selectedPath.trim()
    if (!trimmed) return false
    setAddError(null)
    try {
      await addProjectMutation.mutateAsync(trimmed)
      setSelectedPath('')
      return true
    } catch (err) {
      const code = (err as Error).message.replace(/^\d+:\s*/, '').trim()
      const errorMsg = (t(`modal.addProjectError.${code}`, '') as string) || (t('modal.addProjectError.default') as string)
      setAddError(errorMsg)
      return false
    }
  }

  const handleRemoveProject = (project: Project) => {
    const hasContainer = project.containerState !== 'none'
    setCleanupContainers(hasContainer)
    setCleanupImages(false)
    setRemoveConfirm(project)
  }

  const confirmRemoveProject = async () => {
    if (!removeConfirm) return
    const project = removeConfirm
    setRemoveConfirm(null)
    setActionLoading((prev) => new Set(prev).add(project.name))
    try {
      const cleanup = (cleanupContainers || cleanupImages)
        ? { containers: cleanupContainers, images: cleanupImages }
        : undefined
      await removeProjectMutation.mutateAsync({ name: project.slug, cleanup })
    } catch (err) {
      alert((err as Error).message)
    } finally {
      setActionLoading((prev) => { const next = new Set(prev); next.delete(project.name); return next })
    }
  }

  const handleOpen = async (project: Project) => {
    setActionLoading((prev) => new Set(prev).add(project.name))
    try {
      const result = await createDevcontainerSession(project.path)
      onSessionCreated(result)
    } catch (err) {
      alert(t('app.createFailed', { message: (err as Error).message }))
    } finally {
      setActionLoading((prev) => { const next = new Set(prev); next.delete(project.name); return next })
    }
  }

  const handleRebuild = async (project: Project) => {
    setActionLoading((prev) => new Set(prev).add(project.name))
    try {
      await rebuildProjectMutation.mutateAsync(project.slug)
    } catch (err) {
      alert((err as Error).message)
    } finally {
      setActionLoading((prev) => { const next = new Set(prev); next.delete(project.name); return next })
    }
  }

  const handleUpgrade = async (project: Project) => {
    setActionLoading((prev) => new Set(prev).add(project.name))
    try {
      const result = await upgradeProjectMutation.mutateAsync(project.slug)
      if (result.languageExtracted && !result.extractionSuccess) {
        alert(t('modal.upgradeExtractFailed', { error: result.extractionError }))
      }
    } catch (err) {
      alert((err as Error).message)
    } finally {
      setActionLoading((prev) => { const next = new Set(prev); next.delete(project.name); return next })
    }
  }

  const handleUpdateClaude = async (project: Project) => {
    setActionLoading((prev) => new Set(prev).add(project.name))
    try {
      const result = await updateClaudeMutation.mutateAsync(project.slug)
      if (result.status === 'up_to_date') {
        alert(t('modal.claudeUpToDate', { version: result.current }))
      } else {
        alert(t('modal.claudeUpdated', { previous: result.previous, current: result.current }))
      }
    } catch (err) {
      alert((err as Error).message)
    } finally {
      setActionLoading((prev) => { const next = new Set(prev); next.delete(project.name); return next })
    }
  }

  const handleConfirmAdd = async () => {
    const success = await handleAddProject()
    if (success) {
      setShowPicker(false)
      setSelectedPath('')
    }
  }

  const handleCancelPicker = () => {
    setShowPicker(false)
    setSelectedPath('')
    setAddError(null)
  }

  const handleProjectDragStart = (e: React.DragEvent, index: number) => {
    setDragIndex(index)
    dragNode.current = e.target
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', '')
  }

  const handleProjectDragOver = (e: React.DragEvent, index: number) => {
    e.preventDefault()
    e.dataTransfer.dropEffect = 'move'
    if (index !== dropIndex) {
      setDropIndex(index)
    }
  }

  const handleProjectDragEnd = () => {
    if (projects && dragIndex !== null && dropIndex !== null && dragIndex !== dropIndex) {
      const reordered = [...projects]
      const [removed] = reordered.splice(dragIndex, 1)
      reordered.splice(dropIndex, 0, removed)
      setLocalOrder(reordered)
      const order = reordered.map((p) => p.path)
      updateProjectOrder(order).catch((err: Error) => {
        console.error(`Failed to update project order: ${err.message}`)
      })
    }
    setDragIndex(null)
    setDropIndex(null)
    dragNode.current = null
  }

  const cliDisabled = !cliAvailable

  return (
    <div className="project-list">
      {loading && <div className="project-list-empty">{t('modal.loadingProjects')}</div>}

      {error && <div className="project-list-error">{t('modal.dockerUnavailable')}</div>}

      {!loading && !error && cliDisabled && (
        <div className="project-cli-warning">
          <strong>{t('modal.cliUnavailable')}</strong>
          <code>npm install -g @devcontainers/cli</code>
        </div>
      )}

      {!loading && !error && projects.length === 0 && (
        <div className="project-list-empty">{t('modal.noProjects')}</div>
      )}

      {!loading && !error && projects.map((project, index) => {
        const config = stateConfig[project.containerState] || stateConfig.none
        const isLoading = actionLoading.has(project.name)
        const isDragging = dragIndex === index
        const isOver = dropIndex === index && dragIndex !== index
        return (
          <div
            key={project.name}
            className={`project-item${isDragging ? ' project-item--dragging' : ''}${isOver ? ' project-item--drag-over' : ''}`}
            draggable
            onDragStart={(e) => handleProjectDragStart(e, index)}
            onDragOver={(e) => handleProjectDragOver(e, index)}
            onDragEnd={handleProjectDragEnd}
          >
            <div className="project-item-left">
              <div className="project-item-row">
                <span
                  className={`project-item-dot project-item-dot--${project.containerState}`}
                  title={t(`modal.containerState.${project.containerState}`) as string}
                />
                <span className="project-item-name">{project.name}</span>
                {project.updateAvailable && (
                  <span className="project-update-badge" title={t('modal.updateAvailable') as string}>
                    {t('modal.updateBadge')}
                  </span>
                )}
                <label className="auto-mode-label" title={(autoModeMap[project.name] ? t('modal.autoModeOn') : t('modal.autoModeOff')) as string}>
                  <button
                    type="button"
                    className={`auto-mode-toggle${autoModeMap[project.name] ? ' auto-mode-toggle--on' : ''}`}
                    onClick={() => handleToggleAutoMode(project)}
                    aria-label={(autoModeMap[project.name] ? t('modal.autoModeOn') : t('modal.autoModeOff')) as string}
                  />
                  <span className="auto-mode-label-text">{t('modal.autoMode')}</span>
                </label>
              </div>
              <div className="project-item-row project-item-row--sub">
                <span className="project-item-path">{shortenPath(project.path)}</span>
                {project.containerState === 'error' && project.errorMessage && (
                  <span className="project-item-error">{project.errorMessage}</span>
                )}
              </div>
            </div>
            <div className="project-item-right">
              {project.containerState === 'error' ? (
                <button
                  className="btn btn--small btn--compact btn--secondary"
                  onClick={() => handleRebuild(project)}
                  disabled={isLoading || cliDisabled}
                  title={(isLoading ? t('modal.rebuilding') : t('modal.rebuild')) as string}
                >
                  <span className="btn-icon">{isLoading ? '\u23F3' : config.icon}</span>
                  {isLoading ? t('modal.loadingShort') : t(`modal.${config.shortKey}`)}
                </button>
              ) : (
                <>
                  {project.updateAvailable && (
                    <button
                      className="btn btn--small btn--compact btn--upgrade"
                      onClick={() => handleUpgrade(project)}
                      disabled={isLoading || cliDisabled}
                      title={t('modal.upgradeTooltip') as string}
                    >
                      {isLoading ? <span className="btn-spinner" /> : <span className="btn-icon">{'\u2B06'}</span>}
                      {t('modal.upgradeShort')}
                    </button>
                  )}
                  {project.containerState === 'running' && project.claudeUpdateAvailable && (
                    <button
                      className="btn btn--small btn--compact btn--secondary"
                      onClick={() => handleUpdateClaude(project)}
                      disabled={isLoading || cliDisabled}
                      title={t('modal.updateClaudeTooltip') as string}
                    >
                      {isLoading ? <span className="btn-spinner" /> : <span className="btn-icon">{'\u2B06'}</span>}
                      {t('modal.updateClaudeShort')}
                    </button>
                  )}
                  {(project.containerState === 'running' || project.containerState === 'stopped') && !project.updateAvailable && (
                    <button
                      className="btn btn--small btn--compact btn--secondary"
                      onClick={() => handleRebuild(project)}
                      disabled={isLoading || cliDisabled}
                      title={t('modal.rebuild') as string}
                    >
                      <span className="btn-icon">{'\u21BB'}</span>
                      {t('modal.rebuildShort')}
                    </button>
                  )}
                  <button
                    className="btn btn--small btn--compact btn--primary"
                    onClick={() => handleOpen(project)}
                    disabled={isLoading || cliDisabled}
                    title={(isLoading ? t('modal.loading') : t(`modal.${config.btnKey}`)) as string}
                  >
                    <span className="btn-icon">{isLoading ? '\u23F3' : config.icon}</span>
                    {isLoading ? t('modal.loadingShort') : t(`modal.${config.shortKey}`)}
                  </button>
                </>
              )}
              <button
                className="icon-btn project-remove-btn"
                onClick={() => handleRemoveProject(project)}
                disabled={isLoading}
                title={t('modal.removeProject') as string}
              >
                &times;
              </button>
            </div>
          </div>
        )
      })}

      {removeConfirm && (
        <div className="remove-confirm-overlay">
          <div className="remove-confirm-dialog">
            <p>{t('modal.removeConfirmTitle', { name: removeConfirm.name })}</p>
            {removeConfirm.containerState !== 'none' && (
              <div className="remove-confirm-options">
                <label>
                  <input
                    type="checkbox"
                    checked={cleanupContainers}
                    onChange={(e) => setCleanupContainers(e.target.checked)}
                  />
                  {t('modal.cleanupContainers')}
                </label>
                <label>
                  <input
                    type="checkbox"
                    checked={cleanupImages}
                    onChange={(e) => setCleanupImages(e.target.checked)}
                  />
                  {t('modal.cleanupImages')}
                </label>
              </div>
            )}
            <div className="remove-confirm-actions">
              <Button variant="outline" onClick={() => setRemoveConfirm(null)}>
                {t('modal.cancel')}
              </Button>
              <Button variant="destructive" onClick={confirmRemoveProject}>
                {t('modal.confirmRemove')}
              </Button>
            </div>
          </div>
        </div>
      )}

      <hr className="project-divider" />

      {!showPicker ? (
        <div className="project-add-actions">
          <Button variant="outline" onClick={() => setShowPicker(true)}>
            {t('modal.registerProject')}
          </Button>
          {onNewProject && (
            <Button onClick={onNewProject}>
              {t('modal.createProject')}
            </Button>
          )}
        </div>
      ) : (
        <>
          <DirPicker
            value={selectedPath}
            onChange={setSelectedPath}
            label={(t('modal.addProjectLabel') as string) || 'Project path'}
            t={t}
          />
          {addError && <div className="project-add-error">{addError}</div>}
          <div className="project-add-actions">
            <Button variant="outline" onClick={handleCancelPicker}>
              {t('modal.cancel')}
            </Button>
            <Button
              disabled={!selectedPath.trim() || addProjectMutation.isPending}
              onClick={handleConfirmAdd}
            >
              {addProjectMutation.isPending
                ? t('modal.loading')
                : t('modal.confirmAdd')}
            </Button>
          </div>
        </>
      )}

      <div className="modal-actions">
        <Button variant="outline" onClick={onClose}>
          {t('modal.cancel')}
        </Button>
      </div>
    </div>
  )
}

type TabType = 'local' | 'devcontainer'

interface ModalProps {
  isOpen: boolean
  onClose: () => void
  onSubmit: (workDir: string) => void
  onDevcontainerSession: (session: Session) => void
  onNewProject?: () => void
  initialDescription?: string
}

export default function Modal({ isOpen, onClose, onSubmit, onDevcontainerSession, onNewProject, initialDescription }: ModalProps) {
  const { t } = useTranslation()
  const [activeTab, setActiveTab] = useState<TabType>('local')
  const [workDir, setWorkDir] = useState('')

  useEffect(() => {
    if (isOpen) {
      setActiveTab('local')
      setWorkDir('')
    }
  }, [isOpen])

  // Escape key disabled — modal must be closed via Cancel button to prevent accidental state loss

  if (!isOpen) return null

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-[200]">
      <div className={cn(
        "bg-secondary border border-border rounded-[10px] p-6",
        "w-[520px] max-w-[90vw] shadow-[0_8px_24px_rgba(0,0,0,0.4)]"
      )} onClick={(e) => e.stopPropagation()}>
        <h2 className="text-lg font-semibold mb-4 text-foreground">{t('modal.title')}</h2>

        <div className="modal-tabs">
          <button
            type="button"
            className={`modal-tab${activeTab === 'local' ? ' modal-tab--active' : ''}`}
            onClick={() => setActiveTab('local')}
          >
            {t('modal.tabLocal')}
          </button>
          <button
            type="button"
            className={`modal-tab${activeTab === 'devcontainer' ? ' modal-tab--active' : ''}`}
            onClick={() => setActiveTab('devcontainer')}
          >
            {t('modal.tabDevcontainer')}
          </button>
        </div>

        {activeTab === 'local' && (
          <LocalTab
            workDir={workDir}
            setWorkDir={setWorkDir}
            onSubmit={onSubmit}
            onClose={onClose}
            t={t}
            initialDescription={initialDescription}
          />
        )}
        {activeTab === 'devcontainer' && (
          <DevcontainerTab
            onSessionCreated={onDevcontainerSession}
            onClose={onClose}
            onNewProject={onNewProject}
            t={t}
          />
        )}
      </div>
    </div>
  )
}
