import { useEffect, useRef, useState } from 'react'
import type { Story, Task, StoryStatus, TaskStatus } from '../types/story-map'

const STATUSES: StoryStatus[] = ['todo', 'in-progress', 'in-review', 'done']
const TASK_STATUSES: TaskStatus[] = ['todo', 'in-progress', 'in-review', 'done']

interface ProjectOption {
  slug: string
  name: string
}

interface StoryDetailProps {
  story: Story
  releaseColor?: string
  projects?: ProjectOption[]
  onClose: () => void
  onStatusChange?: (storyId: string, status: StoryStatus) => void
  onDelete?: (storyId: string) => void
  onAddTask?: (storyId: string, name: string, targetProject: string) => void
  onUpdateTask?: (storyId: string, taskId: string, changes: Partial<Pick<Task, 'name' | 'targetProject' | 'status'>>) => void
  onDeleteTask?: (storyId: string, taskId: string) => void
  onUpdateAcceptance?: (storyId: string, acceptance: string[]) => void
}

export function StoryDetail({
  story, releaseColor, projects, onClose, onStatusChange, onDelete,
  onAddTask, onUpdateTask, onDeleteTask, onUpdateAcceptance,
}: StoryDetailProps) {
  const panelRef = useRef<HTMLDivElement>(null)
  const [newTaskName, setNewTaskName] = useState('')
  const [newTaskProject, setNewTaskProject] = useState('')
  const [newAC, setNewAC] = useState('')

  useEffect(() => {
    function handleMouseDown(e: MouseEvent) {
      if (panelRef.current && !panelRef.current.contains(e.target as Node)) {
        onClose()
      }
    }
    document.addEventListener('mousedown', handleMouseDown)
    return () => document.removeEventListener('mousedown', handleMouseDown)
  }, [onClose])

  function handleAddTask() {
    if (!newTaskName.trim() || !newTaskProject.trim()) return
    onAddTask?.(story.id, newTaskName.trim(), newTaskProject.trim())
    setNewTaskName('')
    setNewTaskProject('')
  }

  function handleAddAC() {
    if (!newAC.trim()) return
    onUpdateAcceptance?.(story.id, [...story.acceptance, newAC.trim()])
    setNewAC('')
  }

  function handleRemoveAC(index: number) {
    onUpdateAcceptance?.(story.id, story.acceptance.filter((_, i) => i !== index))
  }

  return (
    <div ref={panelRef} className="story-detail-panel">
      <div className="story-detail-header">
        <span className="story-detail-id">{story.id}</span>
        <button className="story-detail-close" onClick={onClose} aria-label="close">✕</button>
      </div>

      <h2 className="story-detail-name">{story.name}</h2>
      <p className="story-detail-desc">{story.description}</p>

      <div className="story-detail-meta">
        <div className="story-detail-field">
          <span className="story-detail-label">Release</span>
          <span className="story-detail-value" style={releaseColor ? { color: releaseColor } : undefined}>
            {story.release}
          </span>
        </div>
        <div className="story-detail-field">
          <span className="story-detail-label">Status</span>
          <div className="status-selector">
            {STATUSES.map((s) => (
              <button
                key={s}
                className={`status-btn${s === story.status ? ' active' : ''}`}
                data-status={s}
                onClick={() => onStatusChange?.(story.id, s)}
                aria-pressed={s === story.status}
              >
                {s}
              </button>
            ))}
          </div>
        </div>
      </div>

      <div className="story-detail-tags">
        {story.tags.map((tag) => (
          <span key={tag} className="tag">{tag}</span>
        ))}
      </div>

      {story.spike && <div className="story-detail-link">Spike: {story.spike}</div>}
      {story.adr && <div className="story-detail-link">ADR-{story.adr}</div>}

      {/* Tasks Section */}
      <div className="story-detail-section">
        <h3 className="story-detail-section-title">Tasks ({story.tasks.length})</h3>
        {story.tasks.length > 0 && (
          <div className="task-list">
            {story.tasks.map((task) => (
              <div key={task.id} className="task-item" data-status={task.status} title={task.id}>
                <div className="task-item-row">
                  <span className="task-item-name">{task.name}</span>
                  {onDeleteTask && (
                    <button
                      className="task-item-delete"
                      onClick={() => onDeleteTask(story.id, task.id)}
                      aria-label={`刪除 ${task.id}`}
                    >
                      ×
                    </button>
                  )}
                </div>
                <div className="task-item-row">
                  <span className="task-item-project">{task.targetProject}</span>
                  <button
                    className="task-item-status-pill"
                    data-status={task.status}
                    onClick={() => {
                      const next = TASK_STATUSES[(TASK_STATUSES.indexOf(task.status) + 1) % TASK_STATUSES.length]
                      onUpdateTask?.(story.id, task.id, { status: next })
                    }}
                  >
                    <span className="task-status-dot" />
                    {task.status}
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
        {onAddTask && (
          <div className="task-add-form">
            <input
              type="text"
              placeholder="Task 名稱"
              value={newTaskName}
              onChange={(e) => setNewTaskName(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleAddTask()}
            />
            {projects && projects.length > 0 ? (
              <select
                value={newTaskProject}
                onChange={(e) => setNewTaskProject(e.target.value)}
              >
                <option value="">目標專案</option>
                {projects.map((p) => (
                  <option key={p.slug} value={p.slug}>{p.name}</option>
                ))}
              </select>
            ) : (
              <input
                type="text"
                placeholder="目標專案"
                value={newTaskProject}
                onChange={(e) => setNewTaskProject(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && handleAddTask()}
              />
            )}
            <button onClick={handleAddTask} disabled={!newTaskName.trim() || !newTaskProject.trim()}>
              + Task
            </button>
          </div>
        )}
      </div>

      {/* Acceptance Criteria Section */}
      <div className="story-detail-section">
        <h3 className="story-detail-section-title">驗收條件 ({story.acceptance.length})</h3>
        {story.acceptance.length > 0 && (
          <div className="acceptance-list">
            {story.acceptance.map((ac, i) => (
              <div key={i} className="acceptance-card">
                <span className="acceptance-card-text">{ac}</span>
                {onUpdateAcceptance && (
                  <button className="acceptance-remove" onClick={() => handleRemoveAC(i)} aria-label="移除">×</button>
                )}
              </div>
            ))}
          </div>
        )}
        {onUpdateAcceptance && (
          <div className="acceptance-add-form">
            <input
              type="text"
              placeholder="新增驗收條件"
              value={newAC}
              onChange={(e) => setNewAC(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleAddAC()}
            />
            <button onClick={handleAddAC} disabled={!newAC.trim()}>+</button>
          </div>
        )}
      </div>

      {onDelete && (
        <button
          className="story-detail-delete-btn"
          onClick={() => {
            if (window.confirm(`確定要刪除「${story.name}」嗎？`)) {
              onDelete(story.id)
            }
          }}
          aria-label="刪除"
        >
          刪除 Story
        </button>
      )}
    </div>
  )
}
