import { useState, useCallback, useRef, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { useQueryClient } from '@tanstack/react-query'
import useTheme from './hooks/useTheme'
import useStatusSocket from './hooks/useStatusSocket'
import usePanelFocus from './hooks/usePanelFocus'
import Sidebar from './components/Sidebar'
import Terminal from './components/Terminal'
import EmptyState from './components/EmptyState'
import UsageDetail from './components/UsageDetail'
import Modal from './components/Modal'
import NewProjectModal from './components/NewProjectModal'
import { PlanView } from './plan/components/PlanView'
import IdeaDetail from './components/IdeaDetail'
import RecoveryPage from './components/RecoveryPage'
import RecoveryBanner from './components/RecoveryBanner'
import RocketFarewell from './components/RocketFarewell'
import type { FarewellInfo } from './components/RocketFarewell'
import { TooltipProvider } from './components/ui/tooltip'
import { createSession, createDevcontainerSession, deleteSession, deleteIdea, createEpic, deleteEpic, fetchIdeas, enrichIdea } from './api'
import { useRecoverableSessions } from './hooks/useRecoverableSessions'
import { dashboardKeys } from './hooks/useDashboardStats'
import { useEpics, useGitAvailable, epicsKeys } from './hooks/useEpics'
import { useFileClipboard } from './hooks/useFileClipboard'
import { ideasKeys } from './hooks/useIdeas'
import type { Session, Idea } from './types'

export default function App() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { theme, toggle: toggleTheme } = useTheme()
  const [rawSessions, setRawSessions, usage, activePlanSessions] = useStatusSocket()
  const sessions = [...rawSessions].sort((a: Session, b: Session) => (a.order ?? 0) - (b.order ?? 0))
  const panel = usePanelFocus()
  const { selectedSession, selectedEpic, selectedIdea, showUsageDetail, isRecoveryVisible } = panel
  const { data: recoverableSessions, restore: handleRestoreSession, restoreAll: handleRestoreAll, dismiss: handleDismissSession, refetchNow: refetchRecoverable } = useRecoverableSessions()
  const [bannerDismissed, setBannerDismissed] = useState(false)

  const { data: epicsData } = useEpics()
  const epics = epicsData ?? []
  const { data: gitAvailableData } = useGitAvailable()
  const gitAvailable = gitAvailableData?.available ?? false
  const fileClipboard = useFileClipboard()
  const [showModal, setShowModal] = useState(false)
  const [showNewProjectModal, setShowNewProjectModal] = useState(false)
  const [enrichingIds, setEnrichingIds] = useState<Set<string>>(new Set())
  const terminalRef = useRef<{ focus: () => void } | null>(null)
  const [farewellInfo, setFarewellInfo] = useState<FarewellInfo | null>(null)
  // Pending action to execute after farewell animation completes
  const pendingFarewellAction = useRef<(() => void) | null>(null)

  // Helper: get a session card's bounding rect by name
  const getCardRect = useCallback((name: string): DOMRect | null => {
    const el = document.querySelector(`[data-session-name="${CSS.escape(name)}"]`)
    return el ? el.getBoundingClientRect() : null
  }, [])

  // When selected session disappears (e.g. user typed `exit` in terminal),
  // auto-select the nearest remaining session — with farewell animation
  const prevSessionsRef = useRef<Session[]>([])
  // Track card rects for EXIT detection (since card is already gone when we detect)
  const cardRectsRef = useRef<Map<string, DOMRect>>(new Map())

  // Update card rects whenever sessions change
  useEffect(() => {
    requestAnimationFrame(() => {
      const map = new Map<string, DOMRect>()
      sessions.forEach((s: Session) => {
        const rect = getCardRect(s.name)
        if (rect) map.set(s.name, rect)
      })
      cardRectsRef.current = map
    })
  }, [sessions, getCardRect])

  useEffect(() => {
    if (selectedSession && !sessions.some((s: Session) => s.name === selectedSession)) {
      // Session disappeared (EXIT) — play farewell animation if we have its last known rect
      const lastRect = cardRectsRef.current.get(selectedSession)
      const prev = prevSessionsRef.current
      const idx = prev.findIndex((s: Session) => s.name === selectedSession)

      let nextName: string | null = null
      if (sessions.length > 0) {
        const nextIdx = Math.min(Math.max(idx, 0), sessions.length - 1)
        nextName = sessions[nextIdx].name
      }

      if (lastRect && !farewellInfo) {
        setFarewellInfo({ name: selectedSession, rect: lastRect })
        pendingFarewellAction.current = () => panel.selectSession(nextName)
      } else {
        panel.selectSession(nextName)
      }
    }
    prevSessionsRef.current = sessions
  }, [sessions, selectedSession, farewellInfo, getCardRect])

  const refreshEpics = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: epicsKeys.all })
  }, [queryClient])

  const handleSelectEpic = useCallback((name: string | null) => {
    panel.selectEpic(name)
  }, [panel.selectEpic])

  const handleCreateEpic = useCallback(async (name: string) => {
    try {
      await createEpic(name)
      refreshEpics()
    } catch (err) {
      alert(`Failed to create epic: ${(err as Error).message}`)
    }
  }, [refreshEpics])

  const handleDeleteEpic = useCallback(async (id: string) => {
    const epic = epics.find(e => e.id === id)
    if (!confirm(`Delete epic "${epic?.name ?? id}"?`)) return
    try {
      await deleteEpic(id)
      if (selectedEpic === id) {
        panel.clear()
      }
      refreshEpics()
    } catch (err) {
      alert(`Failed to delete epic: ${(err as Error).message}`)
    }
  }, [epics, selectedEpic, refreshEpics])

  const handleSelectIdea = useCallback((idea: Idea) => {
    panel.selectIdea(idea)
  }, [panel.selectIdea])

  const handleShowUsageDetail = useCallback(() => {
    panel.showUsage()
  }, [panel.showUsage])

  const handleStartEnrich = useCallback(async (id: string) => {
    setEnrichingIds(prev => new Set(prev).add(id))
    try {
      await enrichIdea(id)
      // Reload ideas to get updated enrichedDesc
      const ideas = await fetchIdeas()
      const updated = ideas?.find((i: Idea) => i.id === id)
      // Only update right panel if the user is still viewing this idea
      if (updated) {
        panel.updateIdea(updated)
      }
    } catch {
      // ignore
    } finally {
      setEnrichingIds(prev => {
        const next = new Set(prev)
        next.delete(id)
        return next
      })
      // Invalidate ideas query so InboxCapture reloads its list
      queryClient.invalidateQueries({ queryKey: ideasKeys.all })
    }
  }, [panel.updateIdea])

  const handleIdeaDeleted = useCallback(() => {
    panel.clear()
  }, [panel.clear])

  const focusTerminal = useCallback(() => {
    requestAnimationFrame(() => terminalRef.current?.focus())
  }, [])

  const handleNewSession = useCallback(async (workDir?: string) => {
    try {
      const result = await createSession(workDir)
      // Optimistic update: add to list immediately so useEffect won't
      // treat the selection as "session disappeared" before WebSocket delivers it
      setRawSessions((prev: Session[]) => {
        if (prev.some((s: Session) => s.name === result.name)) return prev
        return [...prev, { ...result, _optimistic: true }]
      })
      setShowModal(false)
      panel.selectSession(result.name)
    } catch (err) {
      alert(t('app.createFailed', { message: (err as Error).message }))
    }
  }, [t, panel.selectSession])

  const handleFarewellComplete = useCallback(() => {
    const action = pendingFarewellAction.current
    pendingFarewellAction.current = null
    setFarewellInfo(null)
    if (action) action()
  }, [])

  const handleDelete = useCallback(async (name: string) => {
    if (!confirm(t('app.deleteConfirm', { name }))) return

    // Capture card position BEFORE deletion
    const rect = getCardRect(name)

    try {
      await deleteSession(name)

      // Invalidate dashboard cache (completion recorded on backend)
      queryClient.invalidateQueries({ queryKey: dashboardKeys.all })

      // Calculate next session selection
      const idx = sessions.findIndex((s: Session) => s.name === name)
      const remaining = sessions.filter((s: Session) => s.name !== name)
      const nextName = selectedSession === name && remaining.length > 0
        ? remaining[Math.min(idx, remaining.length - 1)].name
        : selectedSession === name ? null : undefined  // undefined = don't change

      // Start farewell animation
      if (rect) {
        setFarewellInfo({ name, rect })
        pendingFarewellAction.current = () => {
          setRawSessions((prev: Session[]) => prev.filter((s: Session) => s.name !== name))
          if (nextName !== undefined) panel.selectSession(nextName)
        }
      } else {
        // No rect (shouldn't happen) — just remove immediately
        setRawSessions((prev: Session[]) => prev.filter((s: Session) => s.name !== name))
        if (nextName !== undefined) panel.selectSession(nextName)
      }
    } catch (err) {
      alert(t('app.deleteFailed', { message: (err as Error).message }))
    }
  }, [selectedSession, sessions, t, getCardRect])

  const handleReorder = useCallback((orderedNames: string[]) => {
    const orderMap = new Map(orderedNames.map((name, i) => [name, i]))
    setRawSessions((prev: Session[]) =>
      prev.map((s: Session) => ({ ...s, order: orderMap.get(s.name) ?? s.order }))
    )
  }, [])

  const handleDevcontainerSession = useCallback((session: Session) => {
    setRawSessions((prev: Session[]) => {
      if (prev.some((s: Session) => s.name === session.name)) return prev
      return [...prev, session]
    })
    setShowModal(false)
    panel.selectSession(session.name)
  }, [panel.selectSession])

  const handleQuickNewSession = useCallback(async (session: Session) => {
    try {
      const result = session.projectPath
        ? await createDevcontainerSession(session.projectPath)
        : await createSession()
      setRawSessions((prev: Session[]) => {
        if (prev.some((s: Session) => s.name === result.name)) return prev
        return [...prev, { ...result, _optimistic: true }]
      })
      panel.selectSession(result.name)
    } catch (err) {
      alert(t('app.createFailed', { message: (err as Error).message }))
    }
  }, [t, panel.selectSession])

  // Inbox: prefill state for converting idea → session
  const [prefillDescription, setPrefillDescription] = useState('')
  const [deleteIdeaIdOnCreate, setDeleteIdeaIdOnCreate] = useState<string | null>(null)

  const handleConvertIdea = useCallback((text: string, enrichedDesc: string | undefined, id: string) => {
    setPrefillDescription(enrichedDesc || text)
    setDeleteIdeaIdOnCreate(id)
    panel.clear()
    setShowModal(true)
  }, [panel.clear])

  const handleNewSessionWrapped = useCallback(async (workDir?: string) => {
    await handleNewSession(workDir)
    if (deleteIdeaIdOnCreate) {
      deleteIdea(deleteIdeaIdOnCreate).catch(() => {})
      setDeleteIdeaIdOnCreate(null)
    }
    setPrefillDescription('')
  }, [handleNewSession, deleteIdeaIdOnCreate])

  return (
    <TooltipProvider delayDuration={300}>
    <div className="flex h-screen">
      <Sidebar
        sessions={sessions}
        selected={selectedSession}
        onSelect={(name: string) => panel.selectSession(name)}
        onNewSession={() => setShowModal(true)}
        onShowUsageDetail={handleShowUsageDetail}
        showUsageDetail={showUsageDetail}
        epics={epics}
        selectedEpic={selectedEpic}
        onSelectEpic={handleSelectEpic}
        onCreateEpic={handleCreateEpic}
        onDeleteEpic={handleDeleteEpic}
        onDelete={handleDelete}
        onQuickNewSession={handleQuickNewSession}
        theme={theme}
        onToggleTheme={toggleTheme}
        usage={usage}
        onReorder={handleReorder}
        onFocusTerminal={focusTerminal}
        onSelectIdea={handleSelectIdea}
        selectedIdeaId={selectedIdea?.id ?? null}
        enrichingIds={enrichingIds}
        activePlanSessions={activePlanSessions}
        onShowRecovery={() => { refetchRecoverable(); panel.showRecovery() }}
      />
      <main className="flex-1 flex overflow-hidden bg-background flex-col">
        {!bannerDismissed && !isRecoveryVisible && (recoverableSessions ?? []).length > 0 && sessions.length === 0 && (
          <RecoveryBanner
            sessions={recoverableSessions ?? []}
            onRestoreAll={async () => {
              const names = (recoverableSessions ?? []).map(s => s.name)
              const restored = await handleRestoreAll(names)
              setBannerDismissed(true)
              if (restored.length > 0) panel.selectSession(restored[0])
            }}
            onDismiss={() => setBannerDismissed(true)}
            onShowDetails={() => { refetchRecoverable(); panel.showRecovery() }}
          />
        )}
        <div style={{ flex: 1, overflow: 'hidden', display: 'flex' }}>
        {isRecoveryVisible ? (
          <RecoveryPage
            sessions={recoverableSessions ?? []}
            onSelect={(name) => panel.selectSession(name)}
            onReopen={async (name) => {
              const result = await handleRestoreSession(name)
              panel.selectSession(result.name)
            }}
            onRemove={handleDismissSession}
          />
        ) : selectedEpic ? (
          <PlanView epicName={selectedEpic} theme={theme} />
        ) : selectedIdea ? (
          <IdeaDetail
            idea={selectedIdea}
            enriching={enrichingIds.has(selectedIdea.id)}
            onStartEnrich={handleStartEnrich}
            onConvert={handleConvertIdea}
            onDeleted={handleIdeaDeleted}
            onUpdated={(updated) => {
              panel.updateIdea(updated)
              queryClient.invalidateQueries({ queryKey: ['ideas'] })
            }}
          />
        ) : selectedSession ? (
          <Terminal ref={terminalRef} sessionName={selectedSession} theme={theme} projectPath={sessions.find((s: Session) => s.name === selectedSession)?.projectPath} gitAvailable={gitAvailable} clipboard={fileClipboard.clipboard} onCopy={fileClipboard.setCopy} onCut={fileClipboard.setCut} onClearClipboard={fileClipboard.clear} />
        ) : showUsageDetail ? (
          <UsageDetail usage={usage} />
        ) : (
          <EmptyState />
        )}
        </div>
      </main>
      <Modal
        isOpen={showModal}
        onClose={() => { setShowModal(false); focusTerminal(); setPrefillDescription(''); setDeleteIdeaIdOnCreate(null) }}
        onSubmit={handleNewSessionWrapped}
        onDevcontainerSession={handleDevcontainerSession}
        onNewProject={() => { setShowModal(false); setShowNewProjectModal(true) }}
        initialDescription={prefillDescription}
      />
      {farewellInfo && (
        <RocketFarewell farewell={farewellInfo} onComplete={handleFarewellComplete} />
      )}
      <NewProjectModal
        isOpen={showNewProjectModal}
        onClose={() => setShowNewProjectModal(false)}
        onProjectCreated={() => {
          setShowNewProjectModal(false)
        }}
        onMinimize={focusTerminal}
      />
    </div>
    </TooltipProvider>
  )
}
