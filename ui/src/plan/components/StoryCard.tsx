import type { DragEvent } from 'react'
import { cn } from '../../lib/utils'
import type { Story } from '../types/story-map'

interface StoryCardProps {
  story: Story
  dimmed: boolean
  onClick?: (story: Story) => void
  onDragStartCb?: () => void
  onDragEndCb?: () => void
}

const STATUS_BORDER: Record<string, string> = {
  'todo': 'border-l-[3px] border-l-[var(--plan-status-todo)]',
  'in-progress': 'border-l-[3px] border-l-[var(--plan-status-in-progress)]',
  'in-review': 'border-l-[3px] border-l-[var(--plan-status-in-review)]',
  'done': 'border-l-[3px] border-l-[var(--plan-status-done)]',
}

export function StoryCard({ story, dimmed, onClick, onDragStartCb, onDragEndCb }: StoryCardProps) {
  function handleDragStart(e: DragEvent<HTMLDivElement>) {
    e.dataTransfer.setData('text/plain', story.id)
    e.dataTransfer.effectAllowed = 'move'
    e.currentTarget.classList.add('dragging')
    onDragStartCb?.()
  }

  function handleDragEnd(e: DragEvent<HTMLDivElement>) {
    e.currentTarget.classList.remove('dragging')
    onDragEndCb?.()
  }

  return (
    <div
      className={cn(
        "story-card bg-plan-card border border-plan-border rounded-sm",
        "px-2 py-1.5 w-full min-w-0",
        "transition-all duration-150 cursor-grab",
        "active:cursor-grabbing hover:bg-plan-card-hover",
        STATUS_BORDER[story.status] || '',
        dimmed && 'opacity-25',
      )}
      data-status={story.status}
      data-story-id={story.id}
      draggable
      onDragStart={handleDragStart}
      onDragEnd={handleDragEnd}
      onClick={() => onClick?.(story)}
    >
      <div className="text-xs font-medium mb-1 break-words">
        {story.name}
      </div>
      <div className="flex gap-1 flex-wrap">
        {story.tags.map((tag) => (
          <span
            key={tag}
            className={cn(
              "text-[10px] px-1.5 py-px",
              "border border-plan-border rounded-sm",
              "text-plan-text-muted bg-plan-surface"
            )}
          >
            {tag}
          </span>
        ))}
        {story.tasks.length > 0 && (
          <span className={cn(
            "text-[10px] px-1.5 py-px",
            "border border-plan-border rounded-sm",
            "text-plan-text-muted bg-plan-surface"
          )}>
            {story.tasks.length} tasks
          </span>
        )}
        {story.acceptance.length > 0 && (
          <span className={cn(
            "text-[10px] px-1.5 py-px",
            "border border-plan-border rounded-sm",
            "text-plan-text-muted bg-plan-surface"
          )}>
            {story.acceptance.length} AC
          </span>
        )}
      </div>
      {story.spike && <span className="inline-block text-[10px] text-plan-text-dim mt-[3px] mr-1.5 break-all">spike: {story.spike}</span>}
      {story.adr && <span className="inline-block text-[10px] text-plan-text-dim mt-[3px] mr-1.5 break-all">ADR-{story.adr}</span>}
    </div>
  )
}
