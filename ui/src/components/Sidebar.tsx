import React, { useState, useRef, useCallback, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { Plus, Settings, Sun, Moon, RotateCcw, X, Pin } from 'lucide-react'
import SessionCard from './SessionCard'
import MouseAvatar from './MouseAvatar'
import UsageBar from './UsageBar'
import InboxCapture from './InboxCapture'
import CollapsibleSection from './CollapsibleSection'
import ItemLimitStepper from './ItemLimitStepper'
import SettingsModal from './SettingsModal'
import { Button } from './ui/button'
import { Separator } from './ui/separator'
import { Tooltip, TooltipContent, TooltipTrigger } from './ui/tooltip'
import { updateSessionOrder } from '../api'
import { cn } from '../lib/utils'
import type { Session, Usage, EpicInfo, Idea, ActivePlanSession } from '../types'

function loadPersistedNumber(key: string, defaultValue: number): number {
  try {
    const v = localStorage.getItem(key)
    if (v !== null) return parseInt(v, 10) || defaultValue
  } catch {}
  return defaultValue
}

function loadPersistedSet(key: string): Set<string> {
  try {
    const raw = localStorage.getItem(key)
    if (raw) return new Set(JSON.parse(raw))
  } catch {}
  return new Set()
}

function savePersistedSet(key: string, ids: Set<string>) {
  try { localStorage.setItem(key, JSON.stringify([...ids])) } catch {}
}

interface SidebarProps {
  sessions: Session[]
  selected: string | null
  onSelect: (name: string) => void
  onNewSession: () => void
  onDelete: (name: string) => void
  onQuickNewSession?: (session: Session) => void
  theme: 'dark' | 'light'
  onToggleTheme: () => void
  usage: Usage | null
  onReorder?: (order: string[]) => void
  onFocusTerminal?: () => void
  onSelectIdea?: (idea: Idea) => void
  selectedIdeaId?: string | null
  enrichingIds?: Set<string>
  onShowUsageDetail?: () => void
  showUsageDetail?: boolean
  epics?: EpicInfo[]
  selectedEpic?: string | null
  onSelectEpic?: (name: string | null) => void
  onCreateEpic?: (name: string) => void
  onDeleteEpic?: (name: string) => void
  activePlanSessions?: ActivePlanSession[]
  onShowRecovery?: () => void
}

export default function Sidebar({ sessions, selected, onSelect, onNewSession, onDelete, onQuickNewSession, theme, onToggleTheme, usage, onReorder, onFocusTerminal, onSelectIdea, selectedIdeaId, enrichingIds, onShowUsageDetail, showUsageDetail, epics, selectedEpic, onSelectEpic, onCreateEpic, onDeleteEpic, activePlanSessions, onShowRecovery }: SidebarProps) {
  const { t, i18n } = useTranslation()
  const [dragIndex, setDragIndex] = useState<number | null>(null)
  const [dropIndex, setDropIndex] = useState<number | null>(null)
  const dragNode = useRef<EventTarget | null>(null)
  const [epicInputValue, setEpicInputValue] = useState('')
  const [showSettings, setShowSettings] = useState(false)

  const [openSection, setOpenSection] = useState<string | null>(() => {
    try { return localStorage.getItem('piper-sidebar-open-section') || null } catch { return null }
  })

  const handleToggleSection = useCallback((key: string) => {
    setOpenSection((prev) => {
      const next = prev === key ? null : key
      try { localStorage.setItem('piper-sidebar-open-section', next ?? '') } catch {}
      return next
    })
  }, [])

  const [inboxMaxItems, setInboxMaxItems] = useState(() => loadPersistedNumber('piper-inbox-max-visible', 3))
  const [planMaxItems, setPlanMaxItems] = useState(() => loadPersistedNumber('piper-plan-max-visible', 3))
  const [ideaCount, setIdeaCount] = useState(0)

  const handleInboxMaxChange = useCallback((n: number) => {
    setInboxMaxItems(n)
    try { localStorage.setItem('piper-inbox-max-visible', String(n)) } catch {}
  }, [])

  const handlePlanMaxChange = useCallback((n: number) => {
    setPlanMaxItems(n)
    try { localStorage.setItem('piper-plan-max-visible', String(n)) } catch {}
  }, [])

  const [pinnedEpicIds, setPinnedEpicIds] = useState<Set<string>>(
    () => loadPersistedSet('piper-pinned-epics')
  )

  const toggleEpicPin = useCallback((id: string) => {
    setPinnedEpicIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        if (next.size >= planMaxItems) return prev
        next.add(id)
      }
      savePersistedSet('piper-pinned-epics', next)
      return next
    })
  }, [planMaxItems])

  const sortedEpics = epics ? [...epics].sort((a, b) => {
    const ap = pinnedEpicIds.has(a.id) ? 0 : 1
    const bp = pinnedEpicIds.has(b.id) ? 0 : 1
    return ap - bp
  }) : []

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 'i') {
        e.preventDefault()
        setOpenSection((prev) => {
          if (prev !== 'inbox') {
            try { localStorage.setItem('piper-sidebar-open-section', 'inbox') } catch {}
          }
          return 'inbox'
        })
        requestAnimationFrame(() => {
          ;(document.querySelector('.inbox-input') as HTMLElement | null)?.focus()
        })
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [])

  const handleEpicInputSubmit = useCallback(() => {
    const name = epicInputValue.trim()
    if (name && onCreateEpic) {
      onCreateEpic(name)
    }
    setEpicInputValue('')
  }, [epicInputValue, onCreateEpic])

  const handleDragStart = (e: React.DragEvent, index: number) => {
    setDragIndex(index)
    dragNode.current = e.target
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', '')
  }

  const handleDragOver = (e: React.DragEvent, index: number) => {
    e.preventDefault()
    e.dataTransfer.dropEffect = 'move'
    if (index !== dropIndex) {
      setDropIndex(index)
    }
  }

  const handleDragEnd = () => {
    if (dragIndex !== null && dropIndex !== null && dragIndex !== dropIndex) {
      const reordered = [...sessions]
      const [removed] = reordered.splice(dragIndex, 1)
      reordered.splice(dropIndex, 0, removed)
      const order = reordered.map((s) => s.name)
      if (onReorder) onReorder(order)
      updateSessionOrder(order).catch((err: Error) => {
        console.error(`Failed to update session order: ${err.message}`)
      })
    }
    setDragIndex(null)
    setDropIndex(null)
    dragNode.current = null
  }

  const toggleLanguage = () => {
    i18n.changeLanguage(i18n.language === 'zh-TW' ? 'en' : 'zh-TW')
  }

  const EPIC_ITEM_HEIGHT = 34

  return (
    <aside className="w-[300px] min-w-[300px] bg-secondary border-r border-border flex flex-col overflow-hidden">
      <div className="flex items-center justify-between px-4 py-4 border-b border-border">
        <div className="flex items-center gap-2">
          <h1 className="text-base font-semibold text-foreground">{'\uD83D\uDC2D\uD83C\uDFB5'} PIPER</h1>
        </div>
        <div className="flex items-center gap-1">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button variant="ghost" size="icon" className="h-7 w-7 text-sidebar-muted-foreground hover:text-sidebar-foreground" onClick={() => onShowRecovery?.()}>
                <RotateCcw className="size-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent side="bottom">{t('recovery.title')}</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button variant="ghost" size="icon" className="h-7 w-7 text-sidebar-muted-foreground hover:text-sidebar-foreground" onClick={() => setShowSettings(true)}>
                <Settings className="size-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent side="bottom">Settings</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button variant="ghost" size="icon" className="h-7 w-7 text-sidebar-muted-foreground hover:text-sidebar-foreground" onClick={toggleLanguage}>
                <span className="text-[13px] font-semibold leading-none">
                  {i18n.language === 'zh-TW' ? '\u4E2D' : 'EN'}
                </span>
              </Button>
            </TooltipTrigger>
            <TooltipContent side="bottom">Switch language</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button variant="ghost" size="icon" className="h-7 w-7 text-sidebar-muted-foreground hover:text-sidebar-foreground" onClick={onToggleTheme}>
                {theme === 'dark' ? <Sun className="size-4" /> : <Moon className="size-4" />}
              </Button>
            </TooltipTrigger>
            <TooltipContent side="bottom">Toggle theme</TooltipContent>
          </Tooltip>
          <Separator orientation="vertical" className="mx-1 h-4" />
          <Tooltip>
            <TooltipTrigger asChild>
              <Button size="icon" className="h-[22px] w-[22px] rounded-full" onClick={onNewSession}>
                <Plus className="size-3.5" />
              </Button>
            </TooltipTrigger>
            <TooltipContent side="bottom">{t('sidebar.newSession')}</TooltipContent>
          </Tooltip>
        </div>
      </div>

      {/* Usage */}
      <div
        className={cn(
          "cursor-pointer rounded-md transition-colors duration-150 mx-2 px-1 py-1.5",
          "hover:bg-accent",
          showUsageDetail && "bg-accent outline outline-1 outline-primary"
        )}
        onClick={onShowUsageDetail}
        role="button"
        tabIndex={0}
      >
        <UsageBar usage={usage} />
      </div>

      {/* Inbox */}
      <CollapsibleSection
        storageKey="inbox"
        title={t('inbox.title')}
        badge={ideaCount || undefined}
        isOpen={openSection === 'inbox'}
        onToggle={() => handleToggleSection('inbox')}
        footer={
          <ItemLimitStepper
            current={inboxMaxItems}
            total={ideaCount}
            onChange={handleInboxMaxChange}
          />
        }
      >
        <InboxCapture
          onSelectIdea={onSelectIdea}
          selectedIdeaId={selectedIdeaId}
          maxVisible={inboxMaxItems}
          onIdeaCountChange={setIdeaCount}
          enrichingIds={enrichingIds}
        />
      </CollapsibleSection>

      {/* Plan */}
      {(epics && epics.length > 0) || onCreateEpic ? (
        <CollapsibleSection
          storageKey="plan"
          title="Plan"
          badge={epics?.length}
          isOpen={openSection === 'plan'}
          onToggle={() => handleToggleSection('plan')}
          footer={
            <ItemLimitStepper
              current={planMaxItems}
              total={sortedEpics.length}
              onChange={handlePlanMaxChange}
            />
          }
        >
          <div className="sidebar-plan-content">
            {onCreateEpic && (
              <form onSubmit={(e) => { e.preventDefault(); handleEpicInputSubmit() }} className="epic-input-row">
                <input
                  className="inbox-input"
                  type="text"
                  value={epicInputValue}
                  onChange={(e) => setEpicInputValue(e.target.value)}
                  placeholder={t('plan.placeholder') as string}
                />
              </form>
            )}
            <div
              className="epic-list"
              style={{
                maxHeight: planMaxItems * EPIC_ITEM_HEIGHT,
                overflowY: sortedEpics.length > planMaxItems ? 'auto' : 'hidden',
              }}
            >
              {sortedEpics.map((epic) => {
                const isPinned = pinnedEpicIds.has(epic.id)
                const activePS = activePlanSessions?.find(s => s.epic === epic.id)
                return (
                  <div
                    key={epic.id}
                    className={`epic-item${selectedEpic === epic.id ? ' epic-item--selected' : ''}${isPinned ? ' epic-item--pinned' : ''}`}
                    onClick={() => onSelectEpic?.(epic.id)}
                  >
                    <button
                      type="button"
                      className={`pin-btn${isPinned ? ' pin-btn--active' : ''}`}
                      onClick={(e) => { e.stopPropagation(); toggleEpicPin(epic.id) }}
                      title={(isPinned ? t('sidebar.unpinItem') : pinnedEpicIds.size >= planMaxItems ? t('sidebar.maxPinsReached') : t('sidebar.pinItem')) as string}
                      disabled={!isPinned && pinnedEpicIds.size >= planMaxItems}
                    >
                      <Pin className="size-2" fill={isPinned ? 'currentColor' : 'none'} />
                    </button>
                    <span className="epic-item-name">{epic.name}</span>
                    {activePS && (
                      <span
                        className="epic-active-indicator"
                        title={activePS.type === 'plan' ? t('plan.plan') : activePS.type === 'dispatch' ? t('plan.dispatch') : t('plan.retro')}
                      >
                        {activePS.type === 'plan' ? '\uD83D\uDCCB' : activePS.type === 'dispatch' ? '\u2709\uFE0F' : '\uD83C\uDF3E'}
                      </span>
                    )}
                    {onDeleteEpic && (
                      <button
                        className="epic-item-delete"
                        onClick={(e) => { e.stopPropagation(); onDeleteEpic(epic.id) }}
                        title="Delete epic"
                      >
                        <X className="size-2.5" />
                      </button>
                    )}
                  </div>
                )
              })}
            </div>
          </div>
        </CollapsibleSection>
      ) : null}

      <div className="px-3 pt-2 pb-1 border-t border-border flex items-center gap-2">
        <span className="text-[11px] font-semibold uppercase tracking-wide text-muted-foreground shrink-0">
          {t('sidebar.sessions')}
        </span>
        <div className="flex-1 min-w-0 flex items-center overflow-hidden">
          {sessions.map((session) => (
            <MouseAvatar key={session.name} name={session.name} size={16} />
          ))}
        </div>
      </div>
      <div
        className="flex-1 overflow-y-auto px-2 py-1"
        onMouseDown={(e) => {
          const target = e.target as HTMLElement
          if (target.tagName !== 'INPUT' && target.tagName !== 'TEXTAREA') {
            e.preventDefault()
          }
        }}
      >
        {sessions.map((session, index) => (
          <SessionCard
            key={session.name}
            session={session}
            selected={selected === session.name}
            onClick={() => onSelect(session.name)}
            onDelete={() => onDelete(session.name)}
            onQuickNew={onQuickNewSession ? () => onQuickNewSession(session) : undefined}
            onEditDone={onFocusTerminal}
            draggable
            onDragStart={(e) => handleDragStart(e, index)}
            onDragOver={(e) => handleDragOver(e, index)}
            onDragEnd={handleDragEnd}
            isDragging={dragIndex === index}
          />
        ))}
        {sessions.length === 0 && (
          <div className="px-4 py-6 text-center text-text-tertiary text-[13px]">
            {t('sidebar.noSessions')}
          </div>
        )}
      </div>

      <SettingsModal
        isOpen={showSettings}
        onClose={() => setShowSettings(false)}
      />
    </aside>
  )
}
