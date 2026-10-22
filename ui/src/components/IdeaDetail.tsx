import { useState, useRef, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { deleteIdea, updateIdea } from '../api'
import { Button } from './ui/button'
import { cn } from '../lib/utils'
import type { Idea } from '../types'
import type { TFunction } from 'i18next'

function formatRelativeTime(ts: number, t: TFunction): string {
  const diff = Math.floor((Date.now() - ts * 1000) / 1000)
  if (diff < 60) return t('import.justNow') as string
  const min = Math.floor(diff / 60)
  if (min < 60) return t('import.minutesAgo', { count: min }) as string
  const hr = Math.floor(min / 60)
  if (hr < 24) return t('import.hoursAgo', { count: hr }) as string
  const day = Math.floor(hr / 24)
  return t('import.daysAgo', { count: day }) as string
}

interface IdeaDetailProps {
  idea: Idea
  enriching?: boolean
  onStartEnrich: (id: string) => void
  onConvert: (text: string, enrichedDesc: string | undefined, id: string) => void
  onDeleted: () => void
  onUpdated?: (idea: Idea) => void
}

export default function IdeaDetail({ idea, enriching, onStartEnrich, onConvert, onDeleted, onUpdated }: IdeaDetailProps) {
  const { t } = useTranslation()
  const [editing, setEditing] = useState(false)
  const [editText, setEditText] = useState(idea.text)
  const [saving, setSaving] = useState(false)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    setEditText(idea.text)
    setEditing(false)
  }, [idea.id])

  useEffect(() => {
    if (editing && textareaRef.current) {
      textareaRef.current.focus()
      textareaRef.current.setSelectionRange(textareaRef.current.value.length, textareaRef.current.value.length)
    }
  }, [editing])

  const handleSave = async () => {
    const trimmed = editText.trim()
    if (!trimmed || trimmed === idea.text) {
      setEditing(false)
      setEditText(idea.text)
      return
    }
    setSaving(true)
    try {
      const updated = await updateIdea(idea.id, trimmed)
      setEditing(false)
      onUpdated?.(updated)
    } catch {
      // keep editing on failure
    } finally {
      setSaving(false)
    }
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
      e.preventDefault()
      handleSave()
    } else if (e.key === 'Escape') {
      setEditing(false)
      setEditText(idea.text)
    }
  }

  const handleDelete = async () => {
    if (!window.confirm(t('inbox.deleteConfirm') as string)) return
    try {
      await deleteIdea(idea.id)
      onDeleted()
    } catch {
      // ignore
    }
  }

  return (
    <div className="w-full h-full bg-secondary px-8 py-6 overflow-y-auto flex flex-col gap-4">
      {editing ? (
        <div className="flex flex-col gap-2">
          <textarea
            ref={textareaRef}
            className={cn(
              "[font-family:inherit] text-lg font-semibold leading-relaxed",
              "text-foreground bg-accent",
              "border border-primary rounded",
              "px-2.5 py-2 resize-y min-h-[60px] outline-none",
              "break-words"
            )}
            value={editText}
            onChange={e => setEditText(e.target.value)}
            onKeyDown={handleKeyDown}
            disabled={saving}
          />
          <div className="flex items-center gap-2">
            <Button
              onClick={handleSave}
              disabled={saving || !editText.trim()}
            >
              {saving && <span className="btn-spinner" />}
              {t('inbox.save')}
            </Button>
            <Button
              variant="outline"
              onClick={() => { setEditing(false); setEditText(idea.text) }}
              disabled={saving}
            >
              {t('inbox.cancel')}
            </Button>
            <span className="text-[11px] text-text-tertiary ml-1">{t('inbox.editHint')}</span>
          </div>
        </div>
      ) : (
        <h2
          className={cn(
            "text-lg font-semibold m-0 text-foreground leading-relaxed break-words",
            "cursor-pointer rounded p-1 -m-1 transition-colors duration-150",
            "hover:bg-accent"
          )}
          onClick={() => setEditing(true)}
          title={t('inbox.clickToEdit') as string}
        >
          {idea.text}
        </h2>
      )}

      <div className="flex items-center gap-4 flex-wrap">
        <span className="text-xs text-text-tertiary font-mono" data-testid="idea-detail-time">
          {formatRelativeTime(idea.createdAt, t)}
        </span>
        <div className="flex flex-row gap-2">
          <Button variant="outline" size="sm" onClick={() => setEditing(true)}>
            {t('inbox.edit')}
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => onStartEnrich(idea.id)}
            disabled={enriching}
          >
            {enriching ? '...' : '✨'} {t('inbox.enrich')}
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => onConvert(idea.text, idea.enrichedDesc, idea.id)}
          >
            → {t('inbox.convert')}
          </Button>
          <Button
            variant="destructive"
            size="sm"
            onClick={handleDelete}
          >
            ✕ {t('inbox.delete')}
          </Button>
        </div>
      </div>

      {idea.enrichedDesc && (
        <div className="p-2 border-l-2 border-l-primary bg-accent rounded-r">
          <span className="block text-[11px] text-text-tertiary uppercase tracking-wide mb-1.5">
            {t('inbox.enrichedDesc')}
          </span>
          <div className="text-[13px] text-muted-foreground leading-relaxed whitespace-pre-wrap break-words">
            {idea.enrichedDesc}
          </div>
        </div>
      )}

      {idea.context && idea.context.length > 0 && (
        <div className="p-2 bg-accent rounded" data-testid="idea-detail-context">
          <span className="block text-[11px] text-text-tertiary uppercase tracking-wide mb-1.5">
            {t('inbox.context')}
          </span>
          {idea.context.map((c, i) => (
            <div key={i} className="flex gap-1.5 py-[3px] text-xs font-mono text-muted-foreground">
              <span className="text-text-tertiary shrink-0">[{c.kind}]</span>
              <span className="text-primary shrink-0">{c.sessionName}</span>
              <span className="overflow-hidden text-ellipsis whitespace-nowrap">{c.detail}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
