// Project 相關型別

export interface Project {
  name: string
  slug: string
  path: string
  containerState: 'running' | 'stopped' | 'none' | 'error'
  autoMode?: boolean
  updateAvailable?: boolean
  claudeUpdateAvailable?: boolean
  errorMessage?: string
}

export type SetupPhase = 'writing' | 'validating' | 'building' | 'verifying' | 'done' | 'failed'

export interface SetupProgressEvent {
  type: 'setup_progress'
  setupID: string
  phase: SetupPhase
  message: string
  error?: string
}
