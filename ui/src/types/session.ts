// Session 相關型別

export type SessionStatus = 'working' | 'completed' | 'needs_input' | 'idle'

export interface GitStatus {
  sessionDirty: number
}

// 來自 Claude Code hooks/statusline 的 session metadata
export interface HookMeta {
  model?: string
  costUsd?: number
  contextUsedPct?: number
  durationMs?: number
  statusSource: 'hook' | 'capture-pane'
}

export interface Session {
  name: string
  status: SessionStatus
  order?: number
  title?: string
  description?: string
  descriptionAuto?: boolean
  lastLine?: string
  projectPath?: string
  projectName?: string
  windows?: number
  created?: string
  attached?: string
  git?: GitStatus
  sessionFileEdits?: number
  hookMeta?: HookMeta
  _optimistic?: boolean
}

// Active plan/dispatch/retro session indicator
export interface ActivePlanSession {
  epic: string   // epic slug
  type: 'plan' | 'dispatch' | 'retro'
  name: string   // tmux session name
}
