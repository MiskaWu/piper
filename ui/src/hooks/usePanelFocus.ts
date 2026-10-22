import { useState, useCallback } from 'react'
import type { Idea } from '../types'

export type PanelFocus =
  | { type: 'session'; name: string }
  | { type: 'epic'; name: string }
  | { type: 'idea'; idea: Idea }
  | { type: 'usage' }
  | { type: 'recovery' }
  | { type: 'empty' }

export default function usePanelFocus() {
  const [focus, setFocus] = useState<PanelFocus>({ type: 'empty' })

  const selectSession = useCallback((name: string | null) => {
    setFocus(name ? { type: 'session', name } : { type: 'empty' })
  }, [])

  const selectEpic = useCallback((name: string | null) => {
    setFocus(name ? { type: 'epic', name } : { type: 'empty' })
  }, [])

  const selectIdea = useCallback((idea: Idea) => {
    setFocus({ type: 'idea', idea })
  }, [])

  const updateIdea = useCallback((idea: Idea) => {
    setFocus(prev =>
      prev.type === 'idea' && prev.idea.id === idea.id
        ? { type: 'idea', idea }
        : prev
    )
  }, [])

  const showUsage = useCallback(() => {
    setFocus({ type: 'usage' })
  }, [])

  const showRecovery = useCallback(() => {
    setFocus({ type: 'recovery' })
  }, [])

  const clear = useCallback(() => {
    setFocus({ type: 'empty' })
  }, [])

  return {
    focus,
    selectSession,
    selectEpic,
    selectIdea,
    updateIdea,
    showUsage,
    showRecovery,
    clear,
    // 便利 getter — 讓子元件 props 不需要改動
    selectedSession: focus.type === 'session' ? focus.name : null,
    selectedEpic: focus.type === 'epic' ? focus.name : null,
    selectedIdea: focus.type === 'idea' ? focus.idea : null,
    showUsageDetail: focus.type === 'usage',
    isRecoveryVisible: focus.type === 'recovery',
  }
}
