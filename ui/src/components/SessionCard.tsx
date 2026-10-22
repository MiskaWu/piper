import React, { useState, useRef, useEffect, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Plus, X } from 'lucide-react'
import StatusBadge from './StatusBadge'
import MouseAvatar from './MouseAvatar'
import { Card } from './ui/card'
import { Button } from './ui/button'
import { Badge } from './ui/badge'
import { updateSessionMeta } from '../api'
import { useProjects } from '../hooks/useProjects'
import { cn } from '../lib/utils'
import type { Session, SessionStatus } from '../types'

interface InlineEditProps {
  value: string
  placeholder: string
  className?: string
  title?: string
  onSave: (val: string) => void
  enabled: boolean
  onEditDone?: () => void
}

function InlineEdit({ value, placeholder, className, title, onSave, enabled, onEditDone }: InlineEditProps) {
  const [editing, setEditing] = useState(false)
  const [text, setText] = useState(value || '')
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    setText(value || '')
  }, [value])

  useEffect(() => {
    if (editing && inputRef.current) {
      inputRef.current.focus()
      inputRef.current.select()
    }
  }, [editing])

  const save = () => {
    setEditing(false)
    if (text !== (value || '')) {
      onSave(text)
    }
    onEditDone?.()
  }

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      save()
    } else if (e.key === 'Escape') {
      setText(value || '')
      setEditing(false)
      onEditDone?.()
    }
  }

  if (editing) {
    return (
      <input
        ref={inputRef}
        className={cn("inline-edit", className)}
        value={text}
        placeholder={placeholder}
        onChange={(e) => setText(e.target.value)}
        onBlur={save}
        onKeyDown={handleKeyDown}
        onClick={(e) => e.stopPropagation()}
      />
    )
  }

  if (!enabled) {
    return (
      <span className={className} title={title}>
        {value || placeholder}
      </span>
    )
  }

  return (
    <span
      className={className}
      onClick={(e) => {
        e.stopPropagation()
        setEditing(true)
      }}
      title={title || "Click to edit"}
    >
      {value || placeholder}
    </span>
  )
}

const STATUS_BORDER_COLOR: Record<SessionStatus, string> = {
  working: 'border-l-status-working',
  completed: 'border-l-status-completed',
  needs_input: 'border-l-status-needs-input',
  idle: 'border-l-status-idle',
}

interface SessionCardProps {
  session: Session
  selected: boolean
  onClick: () => void
  onDelete: () => void
  onQuickNew?: () => void
  onEditDone?: () => void
  draggable?: boolean
  onDragStart?: (e: React.DragEvent) => void
  onDragOver?: (e: React.DragEvent) => void
  onDragEnd?: () => void
  isDragging?: boolean
  forceSSR?: boolean
}

export default function SessionCard({ session, selected, onClick, onDelete, onQuickNew, onEditDone, draggable, onDragStart, onDragOver, onDragEnd, isDragging, forceSSR }: SessionCardProps) {
  const { t } = useTranslation()
  const { data: projectsData } = useProjects()
  const borderColor = STATUS_BORDER_COLOR[session.status] || STATUS_BORDER_COLOR.idle
  const isDevContainer = useMemo(() => {
    if (!session.projectPath || !projectsData?.projects) return false
    const proj = projectsData.projects.find(p => p.path === session.projectPath)
    return proj?.containerState === 'running' || proj?.containerState === 'stopped'
  }, [session.projectPath, projectsData?.projects])

  const handleMetaSave = (field: string, value: string) => {
    updateSessionMeta(session.name, { [field]: value }).catch((err: Error) => {
      console.error(`Failed to update session meta: ${err.message}`)
    })
  }

  return (
    <Card
      className={cn(
        "session-card group relative px-3 py-3 mb-2 cursor-pointer",
        "border-l-[3px]",
        "transition-[background,border-color,box-shadow] duration-150",
        "shadow-[0_1px_2px_rgba(0,0,0,0.1)]",
        "hover:bg-accent hover:border-text-tertiary",
        borderColor,
        selected && "bg-accent border-primary shadow-[0_1px_4px_rgba(0,0,0,0.15)]",
        isDragging && "opacity-50"
      )}
      data-session-name={session.name}
      onClick={onClick}
      draggable={draggable}
      onDragStart={onDragStart}
      onDragOver={onDragOver}
      onDragEnd={onDragEnd}
    >
      <div className="flex items-start gap-2.5">
        <div className="cursor-grab select-none shrink-0 flex items-center pr-1" onMouseDown={(e) => e.stopPropagation()}>
          <MouseAvatar name={session.name} size={32} forceSSR={forceSSR} />
        </div>
        <div className="flex-1 min-w-0 overflow-hidden">
          <div className="flex items-center gap-1 min-w-0">
            <span className="text-xs shrink-0" title={isDevContainer ? t('sessionCard.devcontainer') : t('sessionCard.local')}>
              {isDevContainer ? '\uD83D\uDC33' : '\uD83D\uDCBB'}
            </span>
            <InlineEdit
              value={session.title || ''}
              placeholder={t('sessionCard.addTitle')}
              className="session-card-title"
              onSave={(val) => handleMetaSave('title', val)}
              enabled={selected}
              onEditDone={onEditDone}
            />
          </div>
          <div className="session-card-description-row">
            {session.description ? (
              <InlineEdit
                value={session.description}
                placeholder={t('sessionCard.addDescription')}
                className="session-card-description"
                title={session.description}
                onSave={(val) => handleMetaSave('description', val)}
                enabled={selected}
                onEditDone={onEditDone}
              />
            ) : (
              <InlineEdit
                value=""
                placeholder={t('sessionCard.addDescription')}
                className="session-card-description session-card-description--placeholder"
                onSave={(val) => handleMetaSave('description', val)}
                enabled={selected}
                onEditDone={onEditDone}
              />
            )}
            {session.descriptionAuto && session.description && (
              <Badge variant="secondary" className="session-card-auto-badge h-4 px-1 text-[9px]" title={t('sessionCard.autoSummary')}>AI</Badge>
            )}
          </div>
        </div>
        <div className="flex items-center gap-1.5 shrink-0">
          <span className="group-hover:hidden">
            <StatusBadge status={session.status} />
          </span>
          <div className="hidden group-hover:flex items-center gap-1">
            {onQuickNew && (
              <Button
                variant="ghost"
                size="icon"
                className="h-6 w-6 text-muted-foreground hover:text-foreground"
                onClick={(e) => {
                  e.stopPropagation()
                  onQuickNew()
                }}
                title={t('sessionCard.quickNew')}
              >
                <Plus className="size-3.5" />
              </Button>
            )}
            <Button
              variant="ghost"
              size="icon"
              className="h-6 w-6 text-muted-foreground hover:text-destructive"
              onClick={(e) => {
                e.stopPropagation()
                onDelete()
              }}
              title={t('sessionCard.delete')}
            >
              <X className="size-3.5" />
            </Button>
          </div>
        </div>
      </div>
      {/* Git available: show per-session uncommitted; No git: show edited count */}
      {session.git ? (
        session.git.sessionDirty > 0 && (
          <div className="session-card-git-row">
            <span className="session-card-git-tag session-card-git-tag--uncommitted">
              <svg className="session-card-git-icon" width="12" height="12" viewBox="0 0 16 16" fill="currentColor">
                <path d="M9.5 3.25a2.25 2.25 0 1 1 3 2.122V6.5A2.5 2.5 0 0 1 10 9H6a1 1 0 0 0-1 1v1.128a2.251 2.251 0 1 1-1.5 0V5.372a2.25 2.25 0 1 1 1.5 0v1.836A2.493 2.493 0 0 1 6 7h4a1 1 0 0 0 1-1v-1.128A2.251 2.251 0 0 1 9.5 3.25z" />
              </svg>
              {session.git.sessionDirty} uncommitted
            </span>
          </div>
        )
      ) : (
        session.sessionFileEdits != null && session.sessionFileEdits > 0 && (
          <div className="session-card-git-row">
            <span className="session-card-git-tag session-card-git-tag--edited">
              <svg className="session-card-git-icon" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M17 3a2.85 2.83 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5Z" />
                <path d="m15 5 4 4" />
              </svg>
              {session.sessionFileEdits} edited
            </span>
          </div>
        )
      )}
      {session.lastLine && (
        <div className="session-card-lastline">{session.lastLine}</div>
      )}
    </Card>
  )
}
