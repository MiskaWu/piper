import { useState, type KeyboardEvent } from 'react'
import { cn } from '../../lib/utils'

interface AddStoryButtonProps {
  onAdd: (name: string) => void
}

export function AddStoryButton({ onAdd }: AddStoryButtonProps) {
  const [editing, setEditing] = useState(false)
  const [name, setName] = useState('')

  function handleSubmit() {
    const trimmed = name.trim()
    if (!trimmed) return
    onAdd(trimmed)
    setName('')
    setEditing(false)
  }

  function handleKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'Enter') {
      e.preventDefault()
      handleSubmit()
    } else if (e.key === 'Escape') {
      setName('')
      setEditing(false)
    }
  }

  if (!editing) {
    return (
      <button
        className={cn(
          "add-story-btn [font-family:inherit] text-[10px] bg-transparent",
          "border border-dashed border-transparent rounded-sm",
          "text-plan-text-dim cursor-pointer px-1.5 py-0.5 w-full",
          "opacity-0 transition-all duration-150",
          "hover:border-plan-accent hover:text-plan-accent"
        )}
        onClick={() => setEditing(true)}
      >
        + Story
      </button>
    )
  }

  return (
    <input
      className={cn(
        "[font-family:inherit] text-[11px] px-1.5 py-[3px]",
        "border border-plan-accent rounded-sm",
        "bg-plan-surface text-plan-text",
        "w-full box-border focus:outline-none"
      )}
      placeholder="Story 名稱"
      value={name}
      onChange={(e) => setName(e.target.value)}
      onKeyDown={handleKeyDown}
      onBlur={handleSubmit}
      autoFocus
    />
  )
}
