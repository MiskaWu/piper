import React, { useState, useRef } from 'react'
import { PanelLeftClose } from 'lucide-react'
import GitGraph from './GitGraph'
import FilesTree from './FilesTree'
import { Button } from './ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from './ui/tooltip'
import type { FileClipboard } from '../types'

interface SidePanelProps {
  projectPath: string
  gitAvailable: boolean
  onClose: () => void
  clipboard?: FileClipboard | null
  onCopy?: (paths: string[]) => void
  onCut?: (paths: string[]) => void
  onClearClipboard?: () => void
}

type SectionId = 'git' | 'files'

interface SidePanelLayout {
  order: [SectionId, SectionId]
  expanded: SectionId
}

const STORAGE_KEY = 'piper-sidepanel-layout'

function loadLayout(): SidePanelLayout {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) {
      const parsed = JSON.parse(raw)
      if (Array.isArray(parsed.order) && parsed.order.length === 2) {
        return {
          order: parsed.order,
          expanded: parsed.expanded || parsed.order[0],
        }
      }
    }
  } catch { /* ignore */ }
  return { order: ['git', 'files'], expanded: 'git' }
}

function saveLayout(layout: SidePanelLayout) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(layout))
  } catch { /* ignore */ }
}

const GitIcon = () => (
  <svg width="14" height="14" viewBox="0 0 16 16" fill="none" style={{ flexShrink: 0 }}>
    <path d="M9 3a2 2 0 1 1 0-4 2 2 0 0 1 0 4zm0-1a1 1 0 1 0 0-2 1 1 0 0 0 0 2z" fill="#f0883e"/>
    <path d="M9 3v4H5" stroke="#f0883e" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"/>
    <path d="M5 11a2 2 0 1 1 0 4 2 2 0 0 1 0-4zm0 1a1 1 0 1 0 0 2 1 1 0 0 0 0-2z" fill="#f0883e"/>
    <path d="M5 7v4" stroke="#f0883e" strokeWidth="1.5" strokeLinecap="round"/>
    <circle cx="5" cy="7" r="2" fill="none" stroke="#f0883e" strokeWidth="1.5"/>
  </svg>
)

const FilesIcon = () => (
  <svg width="14" height="14" viewBox="0 0 16 16" fill="none" style={{ flexShrink: 0 }}>
    <path d="M1 2.5A1.5 1.5 0 0 1 2.5 1h3.379a1.5 1.5 0 0 1 1.06.44L8.062 2.56A1.5 1.5 0 0 0 9.122 3H13.5A1.5 1.5 0 0 1 15 4.5v8a1.5 1.5 0 0 1-1.5 1.5h-11A1.5 1.5 0 0 1 1 12.5v-10z"
      stroke="#58a6ff" strokeWidth="1.3" fill="none"/>
  </svg>
)

const ChevronIcon = ({ expanded }: { expanded: boolean }) => (
  <svg
    className={`sp-chevron${expanded ? ' sp-chevron-expanded' : ''}`}
    width="12" height="12" viewBox="0 0 16 16" fill="currentColor"
  >
    <path d="M6.427 4.427l3.396 3.396a.25.25 0 0 1 0 .354l-3.396 3.396A.25.25 0 0 1 6 11.396V4.604a.25.25 0 0 1 .427-.177z"/>
  </svg>
)

export default function SidePanel({ projectPath, gitAvailable, onClose, clipboard, onCopy, onCut, onClearClipboard }: SidePanelProps) {
  const [layout, setLayout] = useState<SidePanelLayout>(loadLayout)
  const dragSourceRef = useRef<SectionId | null>(null)

  const handleToggle = (sectionId: SectionId) => {
    if (layout.expanded === sectionId) return // already expanded
    setLayout(prev => {
      const next = { ...prev, expanded: sectionId }
      saveLayout(next)
      return next
    })
  }

  const handleDragStart = (section: SectionId) => (e: React.DragEvent) => {
    dragSourceRef.current = section
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', section)
  }

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault()
    e.dataTransfer.dropEffect = 'move'
  }

  const handleDrop = (targetSection: SectionId) => (e: React.DragEvent) => {
    e.preventDefault()
    const source = dragSourceRef.current
    if (source && source !== targetSection) {
      setLayout(prev => {
        const newOrder = [...prev.order].reverse() as [SectionId, SectionId]
        const newLayout = { ...prev, order: newOrder }
        saveLayout(newLayout)
        return newLayout
      })
    }
    dragSourceRef.current = null
  }

  // If no git, just show files
  if (!gitAvailable) {
    return (
      <div className="sp-container">
        <div className="sp-panel-header">
          <span className="sp-panel-header-title">Panel</span>
          <button className="sp-collapse-panel-btn" onClick={onClose} title="收合面板">
            <svg width="14" height="14" viewBox="0 0 16 16" fill="currentColor">
              <path d="M7.116 8l3.638-3.515a.508.508 0 0 0 0-.74.568.568 0 0 0-.764 0L5.87 7.63a.508.508 0 0 0 0 .74L9.99 12.255a.568.568 0 0 0 .764 0 .508.508 0 0 0 0-.74L7.116 8z"/>
            </svg>
          </button>
        </div>
        <div className="sp-section sp-expanded">
          <div className="sp-section-header">
            <span className="sp-drag-handle" style={{ visibility: 'hidden' }}>⠿</span>
            <FilesIcon />
            <span className="sp-section-title">Files</span>
          </div>
          <div className="sp-section-content">
            <FilesTree projectPath={projectPath} clipboard={clipboard} onCopy={onCopy} onCut={onCut} onClearClipboard={onClearClipboard} gitAvailable={false} />
          </div>
        </div>
      </div>
    )
  }

  const renderSection = (sectionId: SectionId) => {
    const isExpanded = layout.expanded === sectionId
    const isGit = sectionId === 'git'

    return (
      <div
        key={sectionId}
        className={`sp-section ${isExpanded ? 'sp-expanded' : 'sp-collapsed'}`}
        onDragOver={handleDragOver}
        onDrop={handleDrop(sectionId)}
      >
        <div
          className="sp-section-header"
          onClick={() => handleToggle(sectionId)}
        >
          <span
            className="sp-drag-handle"
            draggable
            onDragStart={handleDragStart(sectionId)}
            onClick={e => e.stopPropagation()}
          >⠿</span>
          <ChevronIcon expanded={isExpanded} />
          {isGit ? <GitIcon /> : <FilesIcon />}
          <span className="sp-section-title">{isGit ? 'Git Graph' : 'Files'}</span>

        </div>
        <div className="sp-section-content">
          {isGit ? (
            <GitGraph projectPath={projectPath} />
          ) : (
            <FilesTree projectPath={projectPath} clipboard={clipboard} onCopy={onCopy} onCut={onCut} onClearClipboard={onClearClipboard} gitAvailable={gitAvailable} />
          )}
        </div>
      </div>
    )
  }

  return (
    <div className="sp-container">
      <div className="sp-panel-header">
        <span className="sp-panel-header-title">Panel</span>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button variant="ghost" size="icon" className="sp-collapse-panel-btn" onClick={onClose}>
              <PanelLeftClose className="size-3.5" />
            </Button>
          </TooltipTrigger>
          <TooltipContent side="left">收合面板</TooltipContent>
        </Tooltip>
      </div>
      {renderSection(layout.order[0])}
      {renderSection(layout.order[1])}
    </div>
  )
}
