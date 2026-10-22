import type { Session, BrowseResult, Project, Idea, DashboardStats, EpicInfo, MousedexData, GitGraphResult, GitBranchListResult, SubmoduleInfo, FilesResult, GitFileStatus, FileOpResult } from './types'

const BASE = '/api'

async function request<T = any>(url: string, options: RequestInit = {}): Promise<T> {
  return requestRaw(url, options, (res) => res.json() as Promise<T>)
}

async function requestText(url: string, options: RequestInit = {}): Promise<string> {
  return requestRaw(url, options, (res) => res.text())
}

async function requestRaw<T>(
  url: string,
  options: RequestInit,
  parse: (res: Response) => Promise<T>,
): Promise<T> {
  const controller = new AbortController()
  const timeout = setTimeout(() => controller.abort(), 30000)
  try {
    const res = await fetch(`${BASE}${url}`, {
      headers: { 'Content-Type': 'application/json' },
      ...options,
      signal: options.signal ?? controller.signal,
    })
    if (!res.ok) {
      const text = await res.text().catch(() => res.statusText)
      throw new Error(`${res.status}: ${text}`)
    }
    if (res.status === 204) return null as T
    return parse(res)
  } finally {
    clearTimeout(timeout)
  }
}

export function fetchSessions(): Promise<Session[]> {
  return request<Session[]>('/sessions')
}

export function createSession(workDir?: string): Promise<Session> {
  return request<Session>('/sessions', {
    method: 'POST',
    body: JSON.stringify({ workDir }),
  })
}

export function browseDirectory(path?: string): Promise<BrowseResult> {
  const params = path ? `?path=${encodeURIComponent(path)}` : ''
  return request<BrowseResult>(`/directories/browse${params}`)
}

export function deleteSession(name: string): Promise<null> {
  return request<null>(`/sessions/${encodeURIComponent(name)}`, {
    method: 'DELETE',
  })
}

export function sendKeys(name: string, keys: string): Promise<null> {
  return request<null>(`/sessions/${encodeURIComponent(name)}/send-keys`, {
    method: 'POST',
    body: JSON.stringify({ keys }),
  })
}

export function updateSessionMeta(name: string, { title, description, projectPath }: { title?: string; description?: string; projectPath?: string }): Promise<Session> {
  return request<Session>(`/sessions/${encodeURIComponent(name)}/meta`, {
    method: 'PUT',
    body: JSON.stringify({ title, description, projectPath }),
  })
}

export function updateSessionOrder(order: string[]): Promise<null> {
  return request<null>('/sessions/order', {
    method: 'PUT',
    body: JSON.stringify({ order }),
  })
}

export function updateProjectOrder(order: string[]): Promise<null> {
  return request<null>('/projects/order', {
    method: 'PUT',
    body: JSON.stringify({ order }),
  })
}

export function listProjects(): Promise<Project[]> {
  return request<Project[]>('/projects')
}

export function addProject(path: string): Promise<Project> {
  return request<Project>('/projects', {
    method: 'POST',
    body: JSON.stringify({ path }),
  })
}

export interface RemoveProjectResult {
  status: string
  containerRemoved?: boolean
  containerError?: string
  imageRemoved?: boolean
  imageError?: string
}

export function removeProject(name: string, cleanup?: { containers?: boolean; images?: boolean }): Promise<RemoveProjectResult> {
  const params = new URLSearchParams()
  if (cleanup?.containers) params.append('cleanup', 'containers')
  if (cleanup?.images) params.append('cleanup', 'images')
  const qs = params.toString()
  return request<RemoveProjectResult>(`/projects/${encodeURIComponent(name)}${qs ? '?' + qs : ''}`, {
    method: 'DELETE',
  })
}

export function rebuildProject(name: string): Promise<null> {
  return request<null>(`/projects/${encodeURIComponent(name)}/rebuild`, {
    method: 'POST',
  })
}

export interface UpgradeResult {
  upgraded: boolean
  languageExtracted: boolean
  extractionSuccess?: boolean
  extractionError?: string
}

export function upgradeProject(name: string): Promise<UpgradeResult> {
  return request<UpgradeResult>(`/projects/${encodeURIComponent(name)}/upgrade`, {
    method: 'POST',
  })
}

export interface UpdateClaudeResult {
  status: 'updated' | 'up_to_date'
  previous?: string
  current: string
  latest: string
}

export function updateProjectClaude(name: string): Promise<UpdateClaudeResult> {
  return request<UpdateClaudeResult>(`/projects/${encodeURIComponent(name)}/update-claude`, {
    method: 'POST',
  })
}

export interface EpicClaudeUpdateState {
  inProgress: boolean
  startedAt?: string
  finishedAt?: string
  targetVersion?: string
  previousVersion?: string
  currentVersion?: string
  error?: string
}

export interface EpicClaudeVersionResult {
  dockerAvailable: boolean
  imageExists?: boolean
  current?: string
  latest?: string
  needsUpdate?: boolean
  error?: string
  latestError?: string
  update?: EpicClaudeUpdateState
}

export function getEpicClaudeVersion(): Promise<EpicClaudeVersionResult> {
  return request<EpicClaudeVersionResult>('/epics/claude-version')
}

export function startEpicClaudeUpdate(): Promise<{ status: string; targetVersion: string }> {
  return request<{ status: string; targetVersion: string }>('/epics/claude-update', {
    method: 'POST',
  })
}

export function createDevcontainerSession(projectPath: string): Promise<Session> {
  return request<Session>('/sessions', {
    method: 'POST',
    body: JSON.stringify({ projectPath }),
  })
}

export function setProjectAutoMode(name: string, autoMode: boolean): Promise<null> {
  return request<null>(`/projects/${encodeURIComponent(name)}/auto-mode`, {
    method: 'PUT',
    body: JSON.stringify({ autoMode }),
  })
}


export function fetchDashboardStats(year?: string | number, month?: string | number): Promise<DashboardStats> {
  const params = new URLSearchParams()
  if (year) params.set('year', String(year))
  if (month) params.set('month', String(month))
  const qs = params.toString()
  return request<DashboardStats>(`/dashboard-stats${qs ? `?${qs}` : ''}`)
}

export async function uploadClipboardImage(blob: Blob): Promise<{ id: string; path: string }> {
  const form = new FormData()
  form.append('image', blob)
  const res = await fetch(`${BASE}/clipboard/image`, { method: 'POST', body: form })
  if (!res.ok) throw new Error(`Upload failed: ${res.status}`)
  return res.json() // { id, path }
}

export async function uploadClipboardFile(file: File): Promise<{ id: string; path: string }> {
  const form = new FormData()
  form.append('file', file)
  const res = await fetch(`${BASE}/clipboard/file`, { method: 'POST', body: form })
  if (!res.ok) throw new Error(`Upload failed: ${res.status}`)
  return res.json() // { id, path }
}

export function fetchIdeas(): Promise<Idea[]> {
  return request<Idea[]>('/ideas')
}

export function createIdea(text: string): Promise<Idea> {
  return request<Idea>('/ideas', {
    method: 'POST',
    body: JSON.stringify({ text }),
  })
}

export function deleteIdea(id: string): Promise<null> {
  return request<null>(`/ideas/${id}`, { method: 'DELETE' })
}

export function updateIdea(id: string, text: string): Promise<Idea> {
  return request<Idea>(`/ideas/${id}`, {
    method: 'PUT',
    body: JSON.stringify({ text }),
  })
}

export function enrichIdea(id: string): Promise<Idea> {
  return request<Idea>(`/ideas/${id}/enrich`, { method: 'POST' })
}

// Model config API
export interface ModelConfig {
  devcontainerGen?: string
  summarizer?: string
  ideaEnrich?: string
  locale?: string
}

export function fetchModelConfig(): Promise<ModelConfig> {
  return request<ModelConfig>('/settings/models')
}

export function setModelConfig(config: ModelConfig): Promise<ModelConfig> {
  return request<ModelConfig>('/settings/models', {
    method: 'PUT',
    body: JSON.stringify(config),
  })
}

// Epic Git config API
export interface EpicGitConfig {
  remoteUrl?: string
  syncEnabled?: boolean
  syncIntervalMin?: number
}

export interface EpicGitSyncStatus {
  lastSync?: string
  lastError?: string
  pendingCommits: number
  syncing: boolean
}

export function fetchEpicGitConfig(): Promise<EpicGitConfig> {
  return request<EpicGitConfig>('/settings/epic-git')
}

export function setEpicGitConfig(config: EpicGitConfig): Promise<EpicGitConfig> {
  return request<EpicGitConfig>('/settings/epic-git', {
    method: 'PUT',
    body: JSON.stringify(config),
  })
}

export function fetchEpicGitSyncStatus(): Promise<EpicGitSyncStatus> {
  return request<EpicGitSyncStatus>('/settings/epic-git/status')
}

export function triggerEpicGitSync(): Promise<{ status: string }> {
  return request<{ status: string }>('/settings/epic-git/sync', { method: 'POST' })
}

// Firewall config API
export interface FirewallConfig {
  mode: string
  domains: string[]
}

export function fetchFirewallConfig(): Promise<FirewallConfig> {
  return request<FirewallConfig>('/settings/firewall')
}

export function setFirewallConfig(config: FirewallConfig): Promise<FirewallConfig> {
  return request<FirewallConfig>('/settings/firewall', {
    method: 'PUT',
    body: JSON.stringify(config),
  })
}

// Plan session API
export function createPlanSession(epicId: string): Promise<{ session: string }> {
  return request<{ session: string }>(`/epics/${encodeURIComponent(epicId)}/plan-session`, {
    method: 'POST',
  })
}

export function getPlanSessionStatus(epicId: string): Promise<{ session: string; exists: boolean }> {
  return request<{ session: string; exists: boolean }>(`/epics/${encodeURIComponent(epicId)}/plan-session`)
}

export function deletePlanSession(epicId: string): Promise<null> {
  return request<null>(`/epics/${encodeURIComponent(epicId)}/plan-session`, {
    method: 'DELETE',
  })
}

// Dispatch session API
export function createDispatchSession(epicId: string): Promise<{ session: string }> {
  return request<{ session: string }>(`/epics/${encodeURIComponent(epicId)}/dispatch-session`, {
    method: 'POST',
  })
}

export function getDispatchSessionStatus(epicId: string): Promise<{ session: string; exists: boolean }> {
  return request<{ session: string; exists: boolean }>(`/epics/${encodeURIComponent(epicId)}/dispatch-session`)
}

export function deleteDispatchSession(epicId: string): Promise<null> {
  return request<null>(`/epics/${encodeURIComponent(epicId)}/dispatch-session`, {
    method: 'DELETE',
  })
}

// Dispatch infra API
export interface InfraProjectStatus {
  name: string
  path: string
  version: number
  updateAvailable: boolean
}

export interface InfraStatus {
  currentVersion: number
  projects: InfraProjectStatus[]
}

export function getInfraStatus(epicId: string): Promise<InfraStatus> {
  return request<InfraStatus>(`/epics/${encodeURIComponent(epicId)}/dispatch/infra-status`)
}

export interface SyncInfraResult {
  updated: { name: string; fromVersion: number; toVersion: number }[]
}

export function syncInfra(epicId: string, projects?: string[]): Promise<SyncInfraResult> {
  return request<SyncInfraResult>(`/epics/${encodeURIComponent(epicId)}/dispatch/sync-infra`, {
    method: 'POST',
    body: JSON.stringify({ projects: projects ?? [] }),
  })
}

// Retro session API
export function createRetroSession(epicId: string): Promise<{ session: string }> {
  return request<{ session: string }>(`/epics/${encodeURIComponent(epicId)}/retro-session`, {
    method: 'POST',
  })
}

export function getRetroSessionStatus(epicId: string): Promise<{ session: string; exists: boolean }> {
  return request<{ session: string; exists: boolean }>(`/epics/${encodeURIComponent(epicId)}/retro-session`)
}

export function deleteRetroSession(epicId: string): Promise<null> {
  return request<null>(`/epics/${encodeURIComponent(epicId)}/retro-session`, {
    method: 'DELETE',
  })
}

// --- Retro data (trends / reports / actions) ---

export interface TrendEntry {
  releaseLabel: string
  reportName: string
  hasSnapshot: boolean
  firstPassRate: number
  firstPassCount: number
  totalTasks: number
  gapTotal: number
  gapDistribution?: Record<string, number>
  avgReworkCount: number
  aanPassRate: number
  nonAanPassRate: number
  aanTaskCount: number
  nonAanTaskCount: number
  verifyExhausted: number
  diffGateBlocked: number
  doneSamplingFailed: number
  avgLoopCount: number
}

export interface RetroTrends {
  epicId: string
  releases: TrendEntry[]
}

export function getRetroTrends(epicId: string): Promise<RetroTrends> {
  return request<RetroTrends>(`/epics/${encodeURIComponent(epicId)}/retro/trends`)
}

export function listRetroReports(epicId: string): Promise<string[]> {
  return request<string[]>(`/epics/${encodeURIComponent(epicId)}/retro/reports`)
}

export function getRetroReportMarkdown(epicId: string, release: string): Promise<string> {
  return requestText(`/epics/${encodeURIComponent(epicId)}/retro/reports/${encodeURIComponent(release)}`)
}

export const RETRO_ACTION_STATUS = {
  TODO: 'todo',
  DONE: 'done',
} as const
export type RetroActionStatus = typeof RETRO_ACTION_STATUS[keyof typeof RETRO_ACTION_STATUS]

export interface RetroAction {
  id: string
  from_retro: string
  description: string
  status: RetroActionStatus
  applied_at?: string
}

export function listRetroActions(epicId: string): Promise<{ actions: RetroAction[] }> {
  return request<{ actions: RetroAction[] }>(`/epics/${encodeURIComponent(epicId)}/retro/actions?format=json`)
}

export function addRetroAction(epicId: string, description: string, fromRetro = ''): Promise<RetroAction> {
  return request<RetroAction>(`/epics/${encodeURIComponent(epicId)}/retro/actions`, {
    method: 'POST',
    body: JSON.stringify({ description, fromRetro }),
  })
}

export function updateRetroActionStatus(epicId: string, id: string, status: RetroActionStatus): Promise<null> {
  return request<null>(`/epics/${encodeURIComponent(epicId)}/retro/actions/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    body: JSON.stringify({ status }),
  })
}

export function deleteRetroAction(epicId: string, id: string): Promise<null> {
  return request<null>(`/epics/${encodeURIComponent(epicId)}/retro/actions/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

export function forceRetroRerun(epicId: string, releaseId: string): Promise<unknown> {
  return request<unknown>(`/epics/${encodeURIComponent(epicId)}/retro/execute`, {
    method: 'POST',
    body: JSON.stringify({ epicId, forceReleases: [releaseId] }),
  })
}

export function setupProject(name: string, path: string, opts?: { autoMode?: boolean }): Promise<{ setupID: string }> {
  return request<{ setupID: string }>('/projects/setup', {
    method: 'POST',
    body: JSON.stringify({ name, path, autoMode: opts?.autoMode }),
  })
}

export function cancelSetup(setupID: string): Promise<null> {
  return request<null>(`/projects/setup/${encodeURIComponent(setupID)}`, {
    method: 'DELETE',
  })
}

export function fetchEpics(): Promise<EpicInfo[]> {
  return request<EpicInfo[]>('/epics')
}

export function createEpic(name: string): Promise<{ id: string; name: string }> {
  return request<{ id: string; name: string }>('/epics', {
    method: 'POST',
    body: JSON.stringify({ name }),
  })
}

export function deleteEpic(id: string): Promise<void> {
  return request<void>(`/epics/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

export function renameEpic(id: string, name: string): Promise<{ id: string; name: string }> {
  return request<{ id: string; name: string }>(`/epics/${encodeURIComponent(id)}/rename`, {
    method: 'PUT',
    body: JSON.stringify({ name }),
  })
}

// Git API
export function fetchGitAvailable(): Promise<{ available: boolean }> {
  return request<{ available: boolean }>('/git/available')
}

export function fetchGitGraph(path: string, limit?: number, ref?: string, hideRemotes?: string[]): Promise<GitGraphResult> {
  const params = new URLSearchParams({ path })
  if (limit) params.set('limit', String(limit))
  if (ref) params.set('ref', ref)
  if (hideRemotes && hideRemotes.length > 0) params.set('hideRemotes', hideRemotes.join(','))
  return request<GitGraphResult>(`/git/graph?${params}`)
}

export function fetchGitBranches(path: string): Promise<GitBranchListResult> {
  return request<GitBranchListResult>(`/git/branches?${new URLSearchParams({ path })}`)
}

export function fetchCommitBody(path: string, hash: string): Promise<{ hash: string; body: string }> {
  const params = new URLSearchParams({ path, hash })
  return request<{ hash: string; body: string }>(`/git/commit-body?${params}`)
}

export function initGitRepo(path: string, remoteUrl?: string): Promise<{ status: string; isRepo: boolean }> {
  return request<{ status: string; isRepo: boolean }>('/git/init', {
    method: 'POST',
    body: JSON.stringify({ path, remoteUrl }),
  })
}

export function addGitRemote(path: string, name: string, url: string): Promise<{ status: string }> {
  return request<{ status: string }>('/git/remote', {
    method: 'POST',
    body: JSON.stringify({ path, name, url }),
  })
}

export interface GitPushResult {
  status: 'ok' | 'ssh_error' | 'error'
  code?: 'SSH_AGENT_UNAVAILABLE' | 'SSH_NO_KEYS' | 'PUSH_FAILED'
  message?: string
}

export async function pushGit(path: string, branch?: string): Promise<GitPushResult> {
  const res = await fetch(`${BASE}/git/push`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path, branch }),
  })
  return res.json()
}

// Tag API
export function fetchGitTags(path: string): Promise<{ name: string; hash: string; pushed: boolean }[]> {
  return request<{ name: string; hash: string; pushed: boolean }[]>(`/git/tags?${new URLSearchParams({ path })}`)
}

export function createGitTag(path: string, name: string, hash: string): Promise<{ status: string; name: string; hash: string }> {
  return request<{ status: string; name: string; hash: string }>('/git/tag', {
    method: 'POST',
    body: JSON.stringify({ path, name, hash }),
  })
}

export function deleteGitTag(path: string, name: string): Promise<{ status: string }> {
  return request<{ status: string }>('/git/tag', {
    method: 'DELETE',
    body: JSON.stringify({ path, name }),
  })
}

export async function pushGitTag(path: string, name: string, remote?: string): Promise<GitPushResult> {
  const res = await fetch(`${BASE}/git/tag/push`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path, name, remote }),
  })
  return res.json()
}

// Submodule API
export function fetchSubmodules(path: string): Promise<SubmoduleInfo[]> {
  return request<SubmoduleInfo[]>(`/git/submodules?${new URLSearchParams({ path })}`)
}

export async function pushSubmodules(path: string, paths: string[]): Promise<GitPushResult> {
  const res = await fetch(`${BASE}/git/submodules/push`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path, paths }),
  })
  return res.json()
}

// Digest config API
export interface DigestConfig {
  webhookUrl?: string
  enabled: boolean
  sendHour: number
  timezone?: string // IANA timezone (e.g. "Asia/Taipei"); empty defaults to UTC
}

export function fetchDigestConfig(): Promise<DigestConfig> {
  return request<DigestConfig>('/settings/digest')
}

export function setDigestConfig(config: DigestConfig): Promise<DigestConfig> {
  return request<DigestConfig>('/settings/digest', {
    method: 'PUT',
    body: JSON.stringify(config),
  })
}

export interface DigestProgressEvent {
  step: string
  message: string
  current?: number
  total?: number
}

export function sendDigestSSE(
  onProgress: (event: DigestProgressEvent) => void,
  date?: string,
): { promise: Promise<{ status: string; date: string }>; abort: () => void } {
  const params = date ? `?date=${date}` : ''
  const controller = new AbortController()
  const timeout = setTimeout(() => controller.abort(), 180000) // 3 min

  const promise = (async () => {
    try {
      const res = await fetch(`${BASE}/digest/send${params}`, {
        method: 'POST',
        signal: controller.signal,
      })
      if (!res.ok) {
        const text = await res.text().catch(() => res.statusText)
        throw new Error(`${res.status}: ${text}`)
      }
      const reader = res.body?.getReader()
      if (!reader) throw new Error('No response body')

      const decoder = new TextDecoder()
      let buffer = ''
      let result: { status: string; date: string } | null = null

      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        buffer += decoder.decode(value, { stream: true })

        // Parse SSE events from buffer
        const parts = buffer.split('\n\n')
        buffer = parts.pop() || ''
        for (const part of parts) {
          let eventType = ''
          let data = ''
          for (const line of part.split('\n')) {
            if (line.startsWith('event: ')) eventType = line.slice(7)
            else if (line.startsWith('data: ')) data = line.slice(6)
          }
          if (!eventType || !data) continue
          const parsed = JSON.parse(data)
          if (eventType === 'progress') {
            onProgress(parsed as DigestProgressEvent)
          } else if (eventType === 'done') {
            result = parsed
          } else if (eventType === 'error') {
            throw new Error(parsed.message || '發送失敗')
          }
        }
      }

      if (!result) throw new Error('未收到完成事件')
      return result
    } finally {
      clearTimeout(timeout)
    }
  })()

  return { promise, abort: () => controller.abort() }
}

// Files Tree API
export function browseFiles(path: string): Promise<FilesResult> {
  return request<FilesResult>(`/directories/files?${new URLSearchParams({ path })}`)
}

export function fetchGitFileStatus(path: string): Promise<GitFileStatus[]> {
  return request<GitFileStatus[]>(`/git/file-status?${new URLSearchParams({ path })}`)
}

export function copyFiles(sources: string[], destination: string): Promise<FileOpResult> {
  return request<FileOpResult>('/files/copy', {
    method: 'POST',
    body: JSON.stringify({ sources, destination }),
  })
}

export function moveFiles(sources: string[], destination: string): Promise<FileOpResult> {
  return request<FileOpResult>('/files/move', {
    method: 'POST',
    body: JSON.stringify({ sources, destination }),
  })
}

// Mousedex API
export function fetchMousedex(): Promise<MousedexData> {
  return request<MousedexData>('/mousedex')
}

// Recovery API
export interface RecoverableSession {
  name: string
  title: string
  description: string
  projectPath: string
  claudeSessionId: string
  isDevContainer: boolean
  active: boolean
  lastSeenAt: string
}

export function fetchRecoverableSessions(): Promise<RecoverableSession[]> {
  return request<RecoverableSession[]>('/sessions/recoverable')
}

export function restoreSession(name: string): Promise<{ status: string; name: string }> {
  return request<{ status: string; name: string }>(`/sessions/${encodeURIComponent(name)}/restore`, {
    method: 'POST',
  })
}

export function dismissSession(name: string): Promise<{ status: string }> {
  return request<{ status: string }>(`/sessions/${encodeURIComponent(name)}/dismiss`, {
    method: 'POST',
  })
}

// Recovery config API
export interface RecoveryConfig {
  retentionDays: number
}

export function fetchRecoveryConfig(): Promise<RecoveryConfig> {
  return request<RecoveryConfig>('/settings/recovery')
}

export function setRecoveryConfig(config: RecoveryConfig): Promise<RecoveryConfig> {
  return request<RecoveryConfig>('/settings/recovery', {
    method: 'PUT',
    body: JSON.stringify(config),
  })
}

