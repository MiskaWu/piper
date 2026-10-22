import { useState, useEffect, useCallback } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useStoryMap, planKeys } from './usePlanQueries'
import {
  moveStoryRelease,
  updateStoryStatus,
  reorderStoryInStep,
  addRelease,
  removeRelease,
  addStory,
  addStep,
  addActivity,
  deleteStory,
  addTask,
  updateTask,
  deleteTask,
  updateStoryAcceptance,
  saveStoryMap,
} from '../lib/story-map-actions'
import type { StoryMapData, Story, Task, Release, StoryStatus } from '../types/story-map'

export function useStoryMapEditor(epicName: string | null) {
  const queryClient = useQueryClient()
  const { data: queryData, error: queryError } = useStoryMap(epicName ?? '')
  const [localData, setLocalData] = useState<StoryMapData | null>(null)
  const [selectedStory, setSelectedStory] = useState<Story | null>(null)

  const [prevEpicName, setPrevEpicName] = useState(epicName)
  if (prevEpicName !== epicName) {
    setPrevEpicName(epicName)
    setSelectedStory(null)
    setLocalData(null)
  }

  // 同步 query 資料到 local state（供 optimistic update 使用）
  const data = localData ?? queryData ?? null
  const error = queryError ? (queryError as Error).message : null

  // 當 query 資料更新時，清除 local override
  useEffect(() => {
    if (queryData) {
      setLocalData(null)
    }
  }, [queryData])

  // 監聽 plan_update 事件，觸發 invalidate 而非手動 fetch
  useEffect(() => {
    if (!epicName) return

    const handler = (e: Event) => {
      const detail = (e as CustomEvent).detail
      if (!detail?.epic || detail.epic === epicName) {
        queryClient.invalidateQueries({ queryKey: planKeys.storyMap(epicName) })
      }
    }

    window.addEventListener('plan_update', handler)
    return () => window.removeEventListener('plan_update', handler)
  }, [epicName, queryClient])

  const save = useCallback(
    (updated: StoryMapData) => {
      if (!epicName) return
      // Optimistic update：立即更新 local state
      setLocalData(updated)
      saveStoryMap(epicName, updated)
        .then(() => {
          queryClient.invalidateQueries({ queryKey: planKeys.storyMap(epicName) })
        })
        .catch((err) => console.error('Failed to save:', err))
    },
    [epicName, queryClient],
  )

  const handleMoveRelease = useCallback(
    (storyId: string, newRelease: Release) => {
      if (!data) return
      const updated = moveStoryRelease(data, storyId, newRelease)
      save(updated)
      setSelectedStory((prev) =>
        prev?.id === storyId ? { ...prev, release: newRelease } : prev,
      )
    },
    [data, save],
  )

  const handleAddRelease = useCallback(
    (label: string) => {
      if (!data) return
      save(addRelease(data, label))
    },
    [data, save],
  )

  const handleRemoveRelease = useCallback(
    (releaseId: string) => {
      if (!data) return
      save(removeRelease(data, releaseId))
    },
    [data, save],
  )

  const handleReorderStory = useCallback(
    (stepId: string, storyId: string, insertBeforeStoryId: string | null) => {
      if (!data) return
      save(reorderStoryInStep(data, stepId, storyId, insertBeforeStoryId))
    },
    [data, save],
  )

  const handleStatusChange = useCallback(
    (storyId: string, newStatus: StoryStatus) => {
      if (!data) return
      const updated = updateStoryStatus(data, storyId, newStatus)
      save(updated)
      setSelectedStory((prev) =>
        prev?.id === storyId ? { ...prev, status: newStatus } : prev,
      )
    },
    [data, save],
  )

  const handleSelectStory = useCallback((story: Story) => {
    setSelectedStory((prev) => (prev?.id === story.id ? null : story))
  }, [])

  const handleAddStory = useCallback(
    (stepId: string, releaseId: string, name: string) => {
      if (!data) return
      const result = addStory(data, stepId, releaseId, name)
      save(result.data)
      if (result.story) setSelectedStory(result.story)
    },
    [data, save],
  )

  const handleAddStep = useCallback(
    (activityId: string, name: string) => {
      if (!data) return
      save(addStep(data, activityId, name))
    },
    [data, save],
  )

  const handleAddActivity = useCallback(
    (name: string) => {
      if (!data) return
      save(addActivity(data, name))
    },
    [data, save],
  )

  const handleDeleteStory = useCallback(
    (storyId: string) => {
      if (!data) return
      save(deleteStory(data, storyId))
      setSelectedStory((prev) => (prev?.id === storyId ? null : prev))
    },
    [data, save],
  )

  const handleAddTask = useCallback(
    (storyId: string, name: string, targetProject: string) => {
      if (!data) return
      const result = addTask(data, storyId, name, targetProject)
      save(result.data)
      // Update selected story to reflect new task
      if (result.task) {
        setSelectedStory((prev) =>
          prev?.id === storyId ? { ...prev, tasks: [...prev.tasks, result.task!] } : prev,
        )
      }
    },
    [data, save],
  )

  const handleUpdateTask = useCallback(
    (storyId: string, taskId: string, changes: Partial<Pick<Task, 'name' | 'targetProject' | 'status'>>) => {
      if (!data) return
      const updated = updateTask(data, storyId, taskId, changes)
      save(updated)
      setSelectedStory((prev) => {
        if (prev?.id !== storyId) return prev
        return {
          ...prev,
          tasks: prev.tasks.map((t) => (t.id === taskId ? { ...t, ...changes } : t)),
        }
      })
    },
    [data, save],
  )

  const handleDeleteTask = useCallback(
    (storyId: string, taskId: string) => {
      if (!data) return
      save(deleteTask(data, storyId, taskId))
      setSelectedStory((prev) => {
        if (prev?.id !== storyId) return prev
        return { ...prev, tasks: prev.tasks.filter((t) => t.id !== taskId) }
      })
    },
    [data, save],
  )

  const handleUpdateAcceptance = useCallback(
    (storyId: string, acceptance: string[]) => {
      if (!data) return
      save(updateStoryAcceptance(data, storyId, acceptance))
      setSelectedStory((prev) =>
        prev?.id === storyId ? { ...prev, acceptance } : prev,
      )
    },
    [data, save],
  )

  return {
    data,
    error,
    selectedStory,
    handlers: {
      handleMoveRelease,
      handleAddRelease,
      handleRemoveRelease,
      handleReorderStory,
      handleStatusChange,
      handleSelectStory,
      handleAddStory,
      handleAddStep,
      handleAddActivity,
      handleDeleteStory,
      handleAddTask,
      handleUpdateTask,
      handleDeleteTask,
      handleUpdateAcceptance,
    },
  }
}
