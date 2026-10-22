import type { FormEvent } from 'react'
import { useState, useEffect, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { useQueryClient } from '@tanstack/react-query'
import { createIdea, deleteIdea } from '../api'
import { useIdeas, ideasKeys } from '../hooks/useIdeas'
import { cn } from '../lib/utils'
import type { Idea } from '../types'

function loadPinnedIds(): Set<string> {
  try {
    const raw = localStorage.getItem('piper-pinned-ideas')
    if (raw) return new Set(JSON.parse(raw))
  } catch {}
  return new Set()
}

function savePinnedIds(ids: Set<string>) {
  try { localStorage.setItem('piper-pinned-ideas', JSON.stringify([...ids])) } catch {}
}

interface InboxCaptureProps {
  onSelectIdea?: (idea: Idea) => void
  selectedIdeaId?: string | null
  maxVisible?: number
  onIdeaCountChange?: (count: number) => void
  enrichingIds?: Set<string>
}

export default function InboxCapture({ onSelectIdea, selectedIdeaId, maxVisible = 3, onIdeaCountChange, enrichingIds }: InboxCaptureProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { data: ideasData } = useIdeas()
  const ideas = ideasData ?? []
  const [inputValue, setInputValue] = useState('')
  const [pinnedIds, setPinnedIds] = useState<Set<string>>(loadPinnedIds)

  useEffect(() => {
    onIdeaCountChange?.(ideas.length)
  }, [ideas.length, onIdeaCountChange])

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    const text = inputValue.trim()
    if (!text) return
    try {
      await createIdea(text)
      setInputValue('')
      queryClient.invalidateQueries({ queryKey: ideasKeys.all })
    } catch {
      // ignore
    }
  }

  const togglePin = useCallback((id: string) => {
    setPinnedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        if (next.size >= maxVisible) return prev
        next.add(id)
      }
      savePinnedIds(next)
      return next
    })
  }, [maxVisible])

  const handleDeleteIdea = useCallback(async (id: string) => {
    if (!window.confirm(t('inbox.deleteConfirm') as string)) return
    try {
      await deleteIdea(id)
      queryClient.invalidateQueries({ queryKey: ideasKeys.all })
    } catch {
      // ignore
    }
  }, [queryClient, t])

  // Sort: pinned first, then unpinned (preserve original order within each group)
  const sortedIdeas = [...ideas].sort((a, b) => {
    const ap = pinnedIds.has(a.id) ? 0 : 1
    const bp = pinnedIds.has(b.id) ? 0 : 1
    return ap - bp
  })

  const ITEM_HEIGHT = 44
  const listMaxHeight = maxVisible * ITEM_HEIGHT

  return (
    <div className="px-3 pt-2 pb-1">
      <form onSubmit={handleSubmit}>
        <input
          type="text"
          className={cn(
            "inbox-input block w-full px-2.5 py-1.5",
            "bg-background border border-border rounded-md",
            "text-foreground font-ui text-[13px]",
            "outline-none transition-[border-color] duration-150",
            "focus:border-primary"
          )}
          placeholder={t('inbox.placeholder') as string}
          value={inputValue}
          onChange={(e) => setInputValue(e.target.value)}
        />
      </form>

      {sortedIdeas.length > 0 && (
        <div
          className="mt-1"
          style={{ maxHeight: listMaxHeight, overflowY: sortedIdeas.length > maxVisible ? 'auto' : 'hidden' }}
        >
          {sortedIdeas.map((idea) => {
            const isPinned = pinnedIds.has(idea.id)
            const isSelected = selectedIdeaId === idea.id
            const isEnriching = enrichingIds?.has(idea.id)
            return (
              <div
                key={idea.id}
                className={cn(
                  "inbox-item flex items-center gap-2 px-2 py-2 border-b border-border text-xs",
                  "last:border-b-0",
                  isPinned && "border-l-2 border-l-primary pl-1.5",
                  isSelected && "bg-accent",
                  isEnriching && "animate-pulse"
                )}
                onClick={() => onSelectIdea?.(idea)}
                style={{ cursor: onSelectIdea ? 'pointer' : undefined }}
              >
                <button
                  type="button"
                  className={cn(
                    "pin-btn items-center justify-center shrink-0 w-4 h-4 p-0",
                    "border-none rounded-sm bg-transparent",
                    "text-text-tertiary cursor-pointer transition-colors duration-100",
                    "hover:not-disabled:text-primary",
                    "disabled:opacity-30 disabled:cursor-default",
                    isPinned ? "flex text-primary" : "hidden"
                  )}
                  onClick={(e) => { e.stopPropagation(); togglePin(idea.id) }}
                  title={(isPinned ? t('sidebar.unpinItem') : pinnedIds.size >= maxVisible ? t('sidebar.maxPinsReached') : t('sidebar.pinItem')) as string}
                  disabled={!isPinned && pinnedIds.size >= maxVisible}
                >
                  <svg width="8" height="8" viewBox="0 0 24 24" fill={isPinned ? 'currentColor' : 'none'} stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M12 17v5M9 3h6l-1 7h3l-5 7h-1l1-7H9l1-7z" />
                  </svg>
                </button>
                {isEnriching && <span className="animate-pulse text-[10px] shrink-0 mr-1" title={t('inbox.enriching') as string}>...</span>}
                {!isEnriching && idea.enrichedDesc && <span className="shrink-0 text-[10px] mr-1 leading-none" title={t('inbox.enrichedDesc') as string}>✨</span>}
                <span className="flex-1 min-w-0 overflow-hidden text-ellipsis whitespace-nowrap text-foreground" title={idea.text}>{idea.text}</span>
                <button
                  className={cn(
                    "inbox-item-delete hidden bg-transparent border-none p-0.5",
                    "text-text-tertiary cursor-pointer rounded-sm shrink-0",
                    "hover:text-destructive"
                  )}
                  onClick={(e) => { e.stopPropagation(); handleDeleteIdea(idea.id) }}
                  title={t('inbox.delete') as string}
                >
                  <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                    <line x1="18" y1="6" x2="6" y2="18" />
                    <line x1="6" y1="6" x2="18" y2="18" />
                  </svg>
                </button>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
