import { cn } from '../../lib/utils'

interface TagFilterProps {
  tags: string[]
  activeTags: string[]
  onToggle: (tag: string) => void
}

export function TagFilter({ tags, activeTags, onToggle }: TagFilterProps) {
  return (
    <div className="flex gap-1.5 mb-4 flex-wrap">
      {tags.map((tag) => {
        const active = activeTags.includes(tag)
        return (
          <button
            key={tag}
            className={cn(
              "[font-family:inherit] text-xs px-2.5 py-[3px]",
              "border border-plan-border rounded-sm",
              "cursor-pointer transition-all duration-150",
              active
                ? "bg-plan-accent border-plan-accent text-white"
                : "bg-plan-surface text-plan-text-muted hover:border-plan-accent hover:text-plan-text"
            )}
            onClick={() => onToggle(tag)}
          >
            {tag}
          </button>
        )
      })}
    </div>
  )
}
