import { Fragment, useState } from 'react'
import type { DragEvent } from 'react'
import type { StoryMapData, Story, Release, ReleaseConfig } from '../types/story-map'
import type { ResolvedTheme } from '../types/theme'
import { resolveReleaseColor } from '../lib/release-palette'
import { canRemoveRelease } from '../lib/story-map-actions'
import { useStoryDragDrop } from '../hooks/use-story-drag-drop'
import { StoryCard } from './StoryCard'
import { AddStoryButton } from './AddStoryButton'
import { AddReleaseRow } from './AddReleaseRow'

interface StoryMapProps {
  data: StoryMapData
  activeTags: string[]
  theme: ResolvedTheme
  onMoveRelease?: (storyId: string, newRelease: Release) => void
  onReorderStory?: (stepId: string, storyId: string, insertBeforeStoryId: string | null) => void
  onSelectStory?: (story: Story) => void
  onAddRelease?: (label: string) => void
  onRemoveRelease?: (releaseId: string) => void
  onAddStory?: (stepId: string, releaseId: string, name: string) => void
  onAddStep?: (activityId: string, name: string) => void
  onAddActivity?: (name: string) => void
}

function isDimmed(story: Story, activeTags: string[]): boolean {
  if (activeTags.length === 0) return false
  return !story.tags.some((t) => activeTags.includes(t))
}

function buildColumns(data: StoryMapData) {
  const activities: { id: string; name: string; span: number }[] = []
  const steps: { id: string; name: string; stories: Story[]; activityStart: boolean }[] = []

  for (const act of data.activities) {
    activities.push({ id: act.id, name: act.name, span: act.steps.length })
    act.steps.forEach((step, i) => {
      steps.push({ id: step.id, name: step.name, stories: step.stories, activityStart: i === 0 })
    })
  }

  return { activities, steps }
}

function countStories(data: StoryMapData, releaseId: string): number {
  let count = 0
  for (const act of data.activities) {
    for (const step of act.steps) {
      for (const story of step.stories) {
        if (story.release === releaseId) count++
      }
    }
  }
  return count
}

function ReleaseLabel({
  rc,
  color,
  removable,
  storyCount,
  onRemove,
}: {
  rc: ReleaseConfig
  color: string
  removable: boolean
  storyCount: number
  onRemove?: () => void
}) {
  return (
    <div className="release-label" style={{ color }}>
      <span>{rc.label}</span>
      {onRemove && (
        <button
          className="release-remove-btn"
          disabled={!removable}
          title={removable ? `刪除 ${rc.label}` : `尚有 ${storyCount} 個 story`}
          onClick={onRemove}
          aria-label={`remove-${rc.id}`}
        >
          ✕
        </button>
      )}
    </div>
  )
}

export function StoryMap({
  data,
  activeTags,
  theme,
  onMoveRelease,
  onReorderStory,
  onSelectStory,
  onAddRelease,
  onRemoveRelease,
  onAddStory,
  onAddStep,
  onAddActivity,
}: StoryMapProps) {
  const { activities, steps } = buildColumns(data)
  const totalCols = steps.length
  const releases = data.meta.releases
  const hasAddColumn = !!onAddActivity

  const {
    getDragSource,
    setDropIndicator,
    handleDragStart,
    handleDragEnd,
    handleDrop,
    isIndicatorAt,
  } = useStoryDragDrop()
  const [addingStepFor, setAddingStepFor] = useState<string | null>(null)
  const [stepName, setStepName] = useState('')
  const [addingActivity, setAddingActivity] = useState(false)
  const [activityName, setActivityName] = useState('')

  function handleCellDragOver(e: DragEvent<HTMLDivElement>, stepId: string, releaseId: string) {
    e.preventDefault()
    e.dataTransfer.dropEffect = 'move'

    const source = getDragSource()
    if (source && source.stepId === stepId && source.releaseId === releaseId) {
      // Same cell → calculate insertion position for reorder
      e.currentTarget.classList.remove('drag-over')
      const cards = e.currentTarget.querySelectorAll<HTMLElement>('.story-card')
      let insertBeforeId: string | null = null

      for (const card of cards) {
        const cardStoryId = card.getAttribute('data-story-id')
        if (cardStoryId === source.storyId) continue
        const rect = card.getBoundingClientRect()
        const midY = rect.top + rect.height / 2
        if (e.clientY < midY) {
          insertBeforeId = cardStoryId
          break
        }
      }

      setDropIndicator({ stepId, releaseId, insertBeforeId })
    } else {
      // Different cell → show cell highlight
      e.currentTarget.classList.add('drag-over')
      setDropIndicator(null)
    }
  }

  function handleCellDragLeave(e: DragEvent<HTMLDivElement>) {
    e.currentTarget.classList.remove('drag-over')
    if (!e.currentTarget.contains(e.relatedTarget as Node)) {
      setDropIndicator(null)
    }
  }

  function handleCellDrop(e: DragEvent<HTMLDivElement>, stepId: string, releaseId: string) {
    e.preventDefault()
    e.currentTarget.classList.remove('drag-over')

    const storyId = e.dataTransfer.getData('text/plain')
    if (!storyId) return

    handleDrop(storyId, stepId, releaseId, onMoveRelease, onReorderStory)
  }

  function submitStep(activityId: string) {
    const trimmed = stepName.trim()
    if (trimmed && onAddStep) onAddStep(activityId, trimmed)
    setStepName('')
    setAddingStepFor(null)
  }

  function submitActivity() {
    const trimmed = activityName.trim()
    if (trimmed && onAddActivity) onAddActivity(trimmed)
    setActivityName('')
    setAddingActivity(false)
  }

  const gridCols = hasAddColumn
    ? `auto repeat(${totalCols}, minmax(200px, 1fr)) 48px`
    : `auto repeat(${totalCols}, minmax(200px, 1fr))`

  return (
    <div className="story-map-grid" style={{ gridTemplateColumns: gridCols }}>
      {/* Row 1: Activity headers */}
      <div className="grid-corner" />
      {activities.map((act) => (
        <div
          key={act.id}
          className="activity-header"
          style={{ gridColumn: `span ${act.span}` }}
        >
          <span>{act.name}</span>
          {onAddStep && (
            <button
              className="add-step-btn"
              title="新增步驟"
              onClick={() => {
                setAddingStepFor(act.id)
                setStepName('')
              }}
            >
              +
            </button>
          )}
        </div>
      ))}
      {hasAddColumn && (
        <div className="activity-header add-col-header">
          {!addingActivity ? (
            <button
              className="add-activity-btn"
              title="新增活動"
              onClick={() => setAddingActivity(true)}
            >
              +
            </button>
          ) : (
            <input
              className="add-inline-input"
              placeholder="活動名稱"
              value={activityName}
              onChange={(e) => setActivityName(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') submitActivity()
                else if (e.key === 'Escape') { setAddingActivity(false); setActivityName('') }
              }}
              onBlur={submitActivity}
              autoFocus
            />
          )}
        </div>
      )}

      {/* Row 2: Step headers */}
      <div className="grid-corner" />
      {steps.map((step) => (
        <div key={step.id} className={`step-header${step.activityStart ? ' activity-start' : ''}`}>
          {step.name}
        </div>
      ))}
      {hasAddColumn && <div className="step-header add-col-spacer" />}

      {/* Add step inline input (overlays on top of step header row) */}
      {addingStepFor && onAddStep && (
        <div className="add-step-overlay">
          <input
            className="add-inline-input"
            placeholder="步驟名稱"
            value={stepName}
            onChange={(e) => setStepName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') submitStep(addingStepFor)
              else if (e.key === 'Escape') { setAddingStepFor(null); setStepName('') }
            }}
            onBlur={() => submitStep(addingStepFor)}
            autoFocus
          />
        </div>
      )}

      {/* Release rows */}
      {releases.map((rc, idx) => {
        const color = resolveReleaseColor(rc.color, theme)
        const removable = canRemoveRelease(data, rc.id)
        const storyCount = countStories(data, rc.id)
        return (
          <div key={rc.id} className="release-row-contents" style={{ display: 'contents' }}>
            <ReleaseLabel
              rc={rc}
              color={color}
              removable={removable}
              storyCount={storyCount}
              onRemove={onRemoveRelease ? () => onRemoveRelease(rc.id) : undefined}
            />
            {steps.map((step) => {
              const stories = step.stories.filter((s) => s.release === rc.id)
              return (
                <div
                  key={`${step.id}-${rc.id}`}
                  className={`story-cell${step.activityStart ? ' activity-start' : ''}`}
                  style={idx === 0 ? { borderTopColor: color, borderTopWidth: 2, borderTopStyle: 'solid' } : undefined}
                  onDragOver={(e) => handleCellDragOver(e, step.id, rc.id)}
                  onDragLeave={handleCellDragLeave}
                  onDrop={(e) => handleCellDrop(e, step.id, rc.id)}
                >
                  {stories.map((story) => (
                    <Fragment key={story.id}>
                      {isIndicatorAt(step.id, rc.id, story.id) && (
                        <div className="drop-indicator" />
                      )}
                      <StoryCard
                        story={story}
                        dimmed={isDimmed(story, activeTags)}
                        onClick={onSelectStory}
                        onDragStartCb={() => handleDragStart(story.id, step.id, rc.id)}
                        onDragEndCb={handleDragEnd}
                      />
                    </Fragment>
                  ))}
                  {isIndicatorAt(step.id, rc.id, null) && (
                    <div className="drop-indicator" />
                  )}
                  {onAddStory && (
                    <AddStoryButton onAdd={(name) => onAddStory(step.id, rc.id, name)} />
                  )}
                </div>
              )
            })}
            {hasAddColumn && <div className="add-col-spacer" />}
          </div>
        )
      })}

      {/* Add release row */}
      {onAddRelease && (
        <AddReleaseRow totalCols={totalCols + (hasAddColumn ? 1 : 0)} onAdd={onAddRelease} />
      )}
    </div>
  )
}
