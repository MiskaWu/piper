import React, { useState, useCallback } from 'react'
import { ChevronRight } from 'lucide-react'
import { Button } from './ui/button'
import { cn } from '../lib/utils'

interface CollapsibleSectionProps {
  storageKey: string
  title: string
  badge?: number | string | null
  defaultOpen?: boolean
  // Controlled mode (for accordion)
  isOpen?: boolean
  onToggle?: () => void
  // Footer slot
  footer?: React.ReactNode
  children: React.ReactNode
}

export default function CollapsibleSection({
  storageKey,
  title,
  badge,
  defaultOpen = false,
  isOpen: controlledOpen,
  onToggle: controlledToggle,
  footer,
  children,
}: CollapsibleSectionProps) {
  const controlled = controlledOpen !== undefined

  const [internalOpen, setInternalOpen] = useState(() => {
    if (controlled) return false
    try {
      const saved = localStorage.getItem(`piper-section-${storageKey}`)
      if (saved !== null) return saved === 'true'
    } catch {}
    return defaultOpen
  })

  const open = controlled ? controlledOpen : internalOpen

  const toggle = useCallback(() => {
    if (controlled) {
      controlledToggle?.()
    } else {
      setInternalOpen((prev) => {
        const next = !prev
        try { localStorage.setItem(`piper-section-${storageKey}`, String(next)) } catch {}
        return next
      })
    }
  }, [controlled, controlledToggle, storageKey])

  return (
    <div className="border-b border-border">
      <Button
        variant="ghost"
        className={cn(
          "w-full justify-start gap-2 rounded-none px-3 py-2.5 h-auto",
          "text-muted-foreground text-xs font-semibold",
          "uppercase tracking-wide"
        )}
        onClick={toggle}
      >
        <ChevronRight
          className={cn(
            "size-3.5 shrink-0 text-sidebar-muted-foreground transition-transform duration-200",
            open && "rotate-90"
          )}
        />
        <span className="flex-1 text-left">{title}</span>
        {badge != null && badge !== 0 && (
          <span className={cn(
            "shrink-0 min-w-4 h-4 px-[5px]",
            "rounded-full bg-accent text-muted-foreground",
            "text-[10px] font-semibold leading-4 text-center"
          )}>
            {badge}
          </span>
        )}
      </Button>
      {open && (
        <div>
          {children}
          {footer}
        </div>
      )}
    </div>
  )
}
