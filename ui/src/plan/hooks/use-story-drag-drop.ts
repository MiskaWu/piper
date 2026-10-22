import { useRef, useState, useCallback } from 'react'

export interface DragSource {
  storyId: string
  stepId: string
  releaseId: string
}

export interface DropIndicator {
  stepId: string
  releaseId: string
  insertBeforeId: string | null
}

export function useStoryDragDrop() {
  const dragSourceRef = useRef<DragSource | null>(null)
  const [dropIndicator, setDropIndicator] = useState<DropIndicator | null>(null)

  const handleDragStart = useCallback(
    (storyId: string, stepId: string, releaseId: string) => {
      dragSourceRef.current = { storyId, stepId, releaseId }
    },
    [],
  )

  const handleDragEnd = useCallback(() => {
    dragSourceRef.current = null
    setDropIndicator(null)
  }, [])

  const isIndicatorAt = useCallback(
    (stepId: string, releaseId: string, beforeId: string | null): boolean => {
      return (
        dropIndicator !== null &&
        dropIndicator.stepId === stepId &&
        dropIndicator.releaseId === releaseId &&
        dropIndicator.insertBeforeId === beforeId
      )
    },
    [dropIndicator],
  )

  const handleDrop = useCallback(
    (
      storyId: string,
      stepId: string,
      releaseId: string,
      onMoveRelease?: (storyId: string, newRelease: string) => void,
      onReorderStory?: (stepId: string, storyId: string, insertBeforeId: string | null) => void,
    ) => {
      const source = dragSourceRef.current
      if (!source) return

      if (source.stepId === stepId && source.releaseId === releaseId) {
        // Same cell → reorder
        onReorderStory?.(stepId, storyId, dropIndicator?.insertBeforeId ?? null)
      } else {
        // Different cell → move release
        onMoveRelease?.(storyId, releaseId)
      }

      setDropIndicator(null)
    },
    [dropIndicator],
  )

  const getDragSource = useCallback(() => dragSourceRef.current, [])

  return {
    getDragSource,
    dropIndicator,
    setDropIndicator,
    handleDragStart,
    handleDragEnd,
    handleDrop,
    isIndicatorAt,
  }
}
