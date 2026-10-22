import { useState, type FormEvent } from 'react'
import { cn } from '../../lib/utils'

interface AddReleaseRowProps {
  totalCols: number
  onAdd: (label: string) => void
}

export function AddReleaseRow({ totalCols, onAdd }: AddReleaseRowProps) {
  const [editing, setEditing] = useState(false)
  const [label, setLabel] = useState('')

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    const trimLabel = label.trim()
    if (!trimLabel) return
    onAdd(trimLabel)
    setLabel('')
    setEditing(false)
  }

  if (!editing) {
    return (
      <div style={{ display: 'contents' }}>
        <div className="px-1.5 py-1 bg-plan-bg border-r border-plan-border">
          <button
            className={cn(
              "[font-family:inherit] text-[11px] bg-transparent",
              "border border-dashed border-plan-border rounded-sm",
              "text-plan-text-dim cursor-pointer px-2 py-[3px] w-full",
              "transition-all duration-150",
              "hover:border-plan-accent hover:text-plan-accent"
            )}
            onClick={() => setEditing(true)}
          >
            + Release
          </button>
        </div>
        <div style={{ gridColumn: `span ${totalCols}` }} />
      </div>
    )
  }

  return (
    <div style={{ display: 'contents' }}>
      <div
        className="px-2 py-1.5 bg-plan-bg border-t border-plan-border"
        style={{ gridColumn: `span ${totalCols + 1}` }}
      >
        <form className="flex gap-1.5 items-center" onSubmit={handleSubmit}>
          <input
            className={cn(
              "[font-family:inherit] text-[11px] px-1.5 py-[3px]",
              "border border-plan-border rounded-sm",
              "bg-plan-surface text-plan-text w-[120px]",
              "focus:outline-none focus:border-plan-accent"
            )}
            placeholder="Label"
            value={label}
            onChange={(e) => setLabel(e.target.value)}
            autoFocus
          />
          <button
            className={cn(
              "[font-family:inherit] text-[11px] px-2 py-[3px]",
              "border border-plan-accent rounded-sm",
              "bg-plan-accent text-white cursor-pointer",
              "transition-all duration-150"
            )}
            type="submit"
          >
            新增
          </button>
          <button
            className={cn(
              "[font-family:inherit] text-[11px] px-2 py-[3px]",
              "border border-plan-border rounded-sm",
              "bg-plan-surface text-plan-text-muted cursor-pointer",
              "transition-all duration-150"
            )}
            type="button"
            onClick={() => setEditing(false)}
          >
            取消
          </button>
        </form>
      </div>
    </div>
  )
}
