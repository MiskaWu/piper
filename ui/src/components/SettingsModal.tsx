import type { ReactNode, Dispatch, SetStateAction } from 'react'
import { useState, useEffect, useCallback } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import {
  setModelConfig, setEpicGitConfig,
  triggerEpicGitSync,
  setFirewallConfig,
  setDigestConfig, sendDigestSSE,
  setRecoveryConfig,
} from '../api'
import type {
  ModelConfig, EpicGitConfig, EpicGitSyncStatus,
  FirewallConfig, DigestConfig, RecoveryConfig,
} from '../api'
import {
  useModelConfig,
  useEpicGitConfig,
  useEpicGitSyncStatus,
  useFirewallConfig,
  useDigestConfig,
  useRecoveryConfig,
  settingsKeys,
} from '../hooks/useSettings'
import { Button } from './ui/button'

const MODEL_OPTIONS = [
  { value: '', label: '預設' },
  { value: 'haiku', label: 'Haiku' },
  { value: 'sonnet', label: 'Sonnet' },
  { value: 'opus', label: 'Opus' },
]

const LOCALE_OPTIONS = [
  { value: '', label: '自動偵測' },
  { value: 'zh-TW', label: '繁體中文' },
  { value: 'zh-CN', label: '简体中文' },
  { value: 'en', label: 'English' },
  { value: 'ja', label: '日本語' },
  { value: 'ko', label: '한국어' },
]

const RETENTION_OPTIONS = [
  { value: 3, label: '3 天' },
  { value: 7, label: '7 天' },
  { value: 14, label: '14 天' },
  { value: 30, label: '30 天' },
]

const FEATURES: { key: keyof ModelConfig; label: string; defaultModel: string }[] = [
  { key: 'devcontainerGen', label: 'DevContainer 產生', defaultModel: 'opus' },
  { key: 'summarizer', label: '自動摘要', defaultModel: 'haiku' },
  { key: 'ideaEnrich', label: 'Idea 充實', defaultModel: 'sonnet' },
]

type SettingsTab = 'general' | 'git' | 'firewall' | 'digest'

const TABS: { key: SettingsTab; label: string; icon?: ReactNode }[] = [
  { key: 'general', label: '一般', icon: (<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="3" /><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" /></svg>) },
  { key: 'git', label: 'Git', icon: (<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="18" cy="18" r="3" /><circle cx="6" cy="6" r="3" /><path d="M13 6h3a2 2 0 0 1 2 2v7" /><line x1="6" y1="9" x2="6" y2="21" /></svg>) },
  { key: 'firewall', label: '防火牆', icon: (<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" /></svg>) },
  { key: 'digest', label: '每日摘要', icon: (<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" /><polyline points="14 2 14 8 20 8" /><line x1="16" y1="13" x2="8" y2="13" /><line x1="16" y1="17" x2="8" y2="17" /><polyline points="10 9 9 9 8 9" /></svg>) },
]

/* ---- Tab content: General ---- */

function GeneralTab({
  modelConfig,
  onModelChange,
  recoveryRetentionDays,
  onRecoveryChange,
}: {
  modelConfig: ModelConfig
  onModelChange: (key: keyof ModelConfig, value: string) => void
  recoveryRetentionDays: number
  onRecoveryChange: (days: number) => void
}) {
  return (
    <div className="flex flex-col gap-3">
      {FEATURES.map(({ key, label, defaultModel }) => (
        <div key={key} className="settings-row">
          <label className="settings-label">{label}</label>
          <select
            className="settings-select"
            value={modelConfig[key] || ''}
            onChange={(e) => onModelChange(key, e.target.value)}
          >
            {MODEL_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.value === '' ? `${opt.label} (${defaultModel})` : opt.label}
              </option>
            ))}
          </select>
        </div>
      ))}
      <hr className="border-t border-[var(--border)] my-1" />
      <div className="settings-row">
        <label className="settings-label">AI 輸出語言</label>
        <select
          className="settings-select"
          value={modelConfig.locale || ''}
          onChange={(e) => onModelChange('locale', e.target.value)}
        >
          {LOCALE_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>{opt.label}</option>
          ))}
        </select>
      </div>
      <hr className="border-t border-[var(--border)] my-1" />
      <div className="settings-row">
        <div>
          <label className="settings-label">工作階段恢復保留</label>
          <div className="text-xs text-[var(--text-tertiary)] mt-0.5">關閉的工作階段可被恢復的天數</div>
        </div>
        <select
          className="settings-select"
          value={recoveryRetentionDays}
          onChange={(e) => onRecoveryChange(parseInt(e.target.value))}
        >
          {RETENTION_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>{opt.label}</option>
          ))}
        </select>
      </div>
    </div>
  )
}

/* ---- Tab content: Git ---- */

function GitTab({
  gitConfig,
  setGitCfg,
  syncStatus,
  syncing,
  onSync,
}: {
  gitConfig: EpicGitConfig
  setGitCfg: Dispatch<SetStateAction<EpicGitConfig>>
  syncStatus: EpicGitSyncStatus | null
  syncing: boolean
  onSync: () => void
}) {
  return (
    <div className="flex flex-col gap-3">
      {/* Remote URL */}
      <div>
        <label className="settings-label block mb-1">Remote URL</label>
        <input
          type="text"
          className="settings-input"
          value={gitConfig.remoteUrl || ''}
          onChange={(e) => setGitCfg((prev) => ({ ...prev, remoteUrl: e.target.value }))}
          placeholder="git@github.com:user/epics.git"
        />
        <div className="text-xs text-[var(--text-tertiary)] mt-0.5">
          SSH 或 HTTPS 皆可。儲存時會驗證連線與權限。
        </div>
      </div>

      {/* Sync toggle */}
      <div className="settings-row">
        <label className="settings-label">自動同步</label>
        <label className="flex items-center gap-1.5 cursor-pointer text-[13px] text-[var(--text-secondary)]">
          <input
            type="checkbox"
            checked={gitConfig.syncEnabled || false}
            onChange={(e) => setGitCfg((prev) => ({ ...prev, syncEnabled: e.target.checked }))}
          />
          啟用
        </label>
      </div>

      {/* Sync interval */}
      {gitConfig.syncEnabled && (
        <div className="settings-row">
          <label className="settings-label">同步間隔（分鐘）</label>
          <input
            type="number"
            className="settings-input settings-input--narrow"
            min={1}
            max={60}
            value={gitConfig.syncIntervalMin || 5}
            onChange={(e) => setGitCfg((prev) => ({ ...prev, syncIntervalMin: parseInt(e.target.value) || 5 }))}
          />
        </div>
      )}

      {/* Sync status */}
      {syncStatus && (
        <div className="text-xs p-2.5 rounded-md bg-[var(--bg-primary)] border border-[var(--border)]">
          <div className="settings-row mb-1">
            <span className="text-[var(--text-secondary)]">同步狀態</span>
            <Button
              variant="outline"
              size="sm"
              onClick={onSync}
              disabled={syncing || syncStatus.syncing}
            >
              {syncing || syncStatus.syncing ? <><span className="btn-spinner" /> 同步中...</> : '立即同步'}
            </Button>
          </div>
          {syncStatus.lastSync && (
            <div className="text-[var(--text-secondary)]">
              上次同步：{new Date(syncStatus.lastSync).toLocaleString()}
            </div>
          )}
          {syncStatus.pendingCommits > 0 && (
            <div className="text-[var(--warning)]">
              待推送 commit：{syncStatus.pendingCommits}
            </div>
          )}
          {syncStatus.lastError && (
            <div className="text-[var(--danger)] mt-1 break-all">
              {syncStatus.lastError}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

/* ---- Tab content: Firewall ---- */

function FirewallTab({
  config,
  onChange,
}: {
  config: FirewallConfig
  onChange: (cfg: FirewallConfig) => void
}) {
  const [newDomain, setNewDomain] = useState('')

  const addDomain = () => {
    const d = newDomain.trim()
    if (d && !config.domains.includes(d)) {
      onChange({ ...config, domains: [...config.domains, d] })
    }
    setNewDomain('')
  }

  const removeDomain = (idx: number) => {
    onChange({ ...config, domains: config.domains.filter((_, i) => i !== idx) })
  }

  return (
    <div className="flex flex-col gap-3">
      {/* Mode selector */}
      <div className="settings-row">
        <label className="settings-label">Mode</label>
        <select
          className="settings-select"
          value={config.mode}
          onChange={(e) => onChange({ ...config, mode: e.target.value })}
        >
          <option value="open">Open（全放行）</option>
          <option value="whitelist">Whitelist（白名單）</option>
        </select>
      </div>

      {/* Domain list — only shown in whitelist mode */}
      {config.mode === 'whitelist' && (
        <div>
          <div className="settings-label mb-1 font-medium">白名單域名</div>
          <div className="flex flex-col gap-1.5 max-h-[200px] overflow-y-auto">
            {config.domains.map((d, i) => (
              <div key={i} className="flex items-center gap-1.5">
                <span className="flex-1 px-2.5 py-1.5 text-xs rounded bg-[var(--bg-primary)] border border-[var(--border)] text-[var(--text-primary)] font-mono">{d}</span>
                <button
                  type="button"
                  className="bg-transparent border-none text-[var(--text-tertiary)] cursor-pointer px-2 py-1 text-sm leading-none rounded hover:text-[var(--danger)] transition-colors"
                  onClick={() => removeDomain(i)}
                  title="移除"
                >
                  ×
                </button>
              </div>
            ))}
            <div className="flex gap-2">
              <input
                type="text"
                className="settings-input font-mono"
                value={newDomain}
                onChange={(e) => setNewDomain(e.target.value)}
                onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); addDomain() } }}
                placeholder="example.com"
              />
              <Button variant="outline" size="sm" onClick={addDomain}>
                + 新增
              </Button>
            </div>
          </div>
        </div>
      )}

      {/* Info note */}
      <div className="text-xs text-[var(--text-tertiary)]">
        變更只影響之後新建的容器，不會改變已存在的容器設定。
      </div>
    </div>
  )
}

/* ---- Tab content: Digest ---- */

const BROWSER_TZ = Intl.DateTimeFormat().resolvedOptions().timeZone

// Build grouped timezone list for <select>
function getTimezoneGroups(): { region: string; zones: string[] }[] {
  const allZones = typeof Intl.supportedValuesOf === 'function'
    ? (Intl.supportedValuesOf('timeZone') as string[])
    : FALLBACK_TIMEZONES
  const groups = new Map<string, string[]>()
  for (const tz of allZones) {
    const slash = tz.indexOf('/')
    const region = slash > 0 ? tz.slice(0, slash) : 'Other'
    if (!groups.has(region)) groups.set(region, [])
    groups.get(region)!.push(tz)
  }
  return Array.from(groups.entries()).map(([region, zones]) => ({ region, zones }))
}

const FALLBACK_TIMEZONES = [
  'Asia/Taipei', 'Asia/Tokyo', 'Asia/Shanghai', 'Asia/Hong_Kong', 'Asia/Seoul', 'Asia/Singapore',
  'Asia/Kolkata', 'Asia/Dubai', 'Asia/Bangkok',
  'Europe/London', 'Europe/Paris', 'Europe/Berlin', 'Europe/Moscow',
  'America/New_York', 'America/Chicago', 'America/Denver', 'America/Los_Angeles',
  'America/Sao_Paulo', 'America/Toronto',
  'Australia/Sydney', 'Australia/Melbourne', 'Pacific/Auckland',
  'UTC',
]

const TZ_GROUPS = getTimezoneGroups()

function buildHourOptions(timezone: string) {
  const ref = new Date('2026-01-15T00:00:00Z')
  const localStr = ref.toLocaleString('en-US', { timeZone: timezone, hour12: false, hour: '2-digit' })
  const localHour = parseInt(localStr, 10)
  const offset = ((localHour - 0) + 24) % 24

  return Array.from({ length: 24 }, (_, i) => {
    const utc = `UTC ${String(i).padStart(2, '0')}:00`
    const local = `${String((i + offset) % 24).padStart(2, '0')}:00`
    return { value: i, label: `${utc} (${timezone} ${local})` }
  })
}

function DigestTab({
  config,
  onChange,
  onSendNow,
  sending,
  progressMessage,
}: {
  config: DigestConfig
  onChange: (cfg: DigestConfig) => void
  onSendNow: () => void
  sending: boolean
  progressMessage: string
}) {
  return (
    <div className="flex flex-col gap-3">
      {/* Webhook URL */}
      <div>
        <label className="settings-label block mb-1">
          Discord Webhook URL
        </label>
        <input
          type="url"
          className="settings-input"
          value={config.webhookUrl || ''}
          onChange={(e) => onChange({ ...config, webhookUrl: e.target.value })}
          placeholder="https://discord.com/api/webhooks/..."
        />
      </div>

      {/* Enable scheduler */}
      <div className="settings-row">
        <label className="settings-label">自動排程</label>
        <label className="flex items-center gap-1.5 cursor-pointer text-[13px] text-[var(--text-secondary)]">
          <input
            type="checkbox"
            checked={config.enabled}
            onChange={(e) => onChange({ ...config, enabled: e.target.checked })}
          />
          {config.enabled ? '已啟用' : '未啟用'}
        </label>
      </div>

      {/* Timezone + Send hour */}
      {config.enabled && (
        <>
          <div className="settings-row">
            <label className="settings-label">時區</label>
            <select
              className="settings-select settings-select--wide"
              value={config.timezone || BROWSER_TZ}
              onChange={(e) => onChange({ ...config, timezone: e.target.value })}
            >
              {TZ_GROUPS.map(({ region, zones }) => (
                <optgroup key={region} label={region}>
                  {zones.map((tz) => (
                    <option key={tz} value={tz}>{tz.replace(/^[^/]+\//, '').replace(/_/g, ' ')}</option>
                  ))}
                </optgroup>
              ))}
            </select>
          </div>
          <div className="settings-row">
            <label className="settings-label">發送時間</label>
            <select
              className="settings-select"
              value={config.sendHour}
              onChange={(e) => onChange({ ...config, sendHour: parseInt(e.target.value) })}
            >
              {buildHourOptions(config.timezone || BROWSER_TZ).map((opt) => (
                <option key={opt.value} value={opt.value}>{opt.label}</option>
              ))}
            </select>
          </div>
        </>
      )}

      {/* Manual send */}
      <div className="border-t border-[var(--border)] pt-3 mt-1">
        <div className="settings-row">
          <div>
            <div className="settings-label">手動發送</div>
            <div className="text-xs text-[var(--text-tertiary)] mt-0.5">
              立即發送昨日摘要到 Discord
            </div>
          </div>
          <Button variant="outline" size="sm" disabled={!config.webhookUrl || sending} onClick={onSendNow}>
            {sending ? <><span className="btn-spinner" /> 發送中</> : '立即發送'}
          </Button>
        </div>
        {sending && progressMessage && (
          <div className="text-xs text-[var(--text-tertiary)] mt-1.5">
            {progressMessage}
          </div>
        )}
      </div>
    </div>
  )
}

/* ---- Main modal ---- */

interface Props {
  isOpen: boolean
  onClose: () => void
}

export default function SettingsModal({ isOpen, onClose }: Props) {
  const queryClient = useQueryClient()
  const [activeTab, setActiveTab] = useState<SettingsTab>('general')

  // --- TanStack Query：server state ---
  const modelQuery = useModelConfig(isOpen)
  const epicGitQuery = useEpicGitConfig(isOpen)
  const syncStatusQuery = useEpicGitSyncStatus(isOpen)
  const firewallQuery = useFirewallConfig(isOpen)
  const digestQuery = useDigestConfig(isOpen)
  const recoveryQuery = useRecoveryConfig(isOpen)

  // --- Local state：使用者正在編輯的值 ---
  const [modelConfig, setModelCfg] = useState<ModelConfig>({})
  const [gitConfig, setGitCfg] = useState<EpicGitConfig>({})
  const [firewallConfig, setFirewallCfg] = useState<FirewallConfig>({ mode: 'open', domains: [] })
  const [digestConfig, setDigestCfg] = useState<DigestConfig>({ enabled: false, sendHour: 1, timezone: BROWSER_TZ })
  const [recoveryConfig, setRecoveryCfg] = useState<RecoveryConfig>({ retentionDays: 7 })
  const [sendingDigest, setSendingDigest] = useState(false)
  const [digestProgress, setDigestProgress] = useState('')

  const [saving, setSaving] = useState(false)
  const [syncing, setSyncing] = useState(false)
  const [error, setError] = useState('')

  // 當 query data 變化時同步到 local state（初次載入 + invalidate 後）
  useEffect(() => {
    if (modelQuery.data) setModelCfg(modelQuery.data)
  }, [modelQuery.data])

  useEffect(() => {
    if (epicGitQuery.data) setGitCfg(epicGitQuery.data)
  }, [epicGitQuery.data])

  useEffect(() => {
    if (firewallQuery.data) setFirewallCfg(firewallQuery.data)
  }, [firewallQuery.data])

  useEffect(() => {
    if (digestQuery.data) setDigestCfg({
      ...digestQuery.data,
      timezone: digestQuery.data.timezone || BROWSER_TZ,
    })
  }, [digestQuery.data])

  useEffect(() => {
    if (recoveryQuery.data) setRecoveryCfg(recoveryQuery.data)
  }, [recoveryQuery.data])

  // Dirty check：比較 local state 與 query data（server truth）
  const hasChanges = JSON.stringify(modelConfig) !== JSON.stringify(modelQuery.data ?? {})
    || JSON.stringify(gitConfig) !== JSON.stringify(epicGitQuery.data ?? {})
    || JSON.stringify(firewallConfig) !== JSON.stringify(firewallQuery.data ?? { mode: 'open', domains: [] })
    || JSON.stringify(digestConfig) !== JSON.stringify(digestQuery.data ?? { enabled: false, sendHour: 1, timezone: BROWSER_TZ })
    || JSON.stringify(recoveryConfig) !== JSON.stringify(recoveryQuery.data ?? { retentionDays: 7 })

  const handleModelChange = useCallback((key: keyof ModelConfig, value: string) => {
    setModelCfg((prev) => ({ ...prev, [key]: value }))
  }, [])

  const handleSave = useCallback(async () => {
    setSaving(true)
    setError('')
    try {
      const promises: Promise<any>[] = []
      if (JSON.stringify(modelConfig) !== JSON.stringify(modelQuery.data ?? {})) {
        promises.push(setModelConfig(modelConfig))
      }
      if (JSON.stringify(gitConfig) !== JSON.stringify(epicGitQuery.data ?? {})) {
        promises.push(setEpicGitConfig(gitConfig))
      }
      if (JSON.stringify(firewallConfig) !== JSON.stringify(firewallQuery.data ?? { mode: 'open', domains: [] })) {
        promises.push(setFirewallConfig(firewallConfig))
      }
      if (JSON.stringify(digestConfig) !== JSON.stringify(digestQuery.data ?? { enabled: false, sendHour: 1, timezone: BROWSER_TZ })) {
        promises.push(setDigestConfig(digestConfig))
      }
      if (JSON.stringify(recoveryConfig) !== JSON.stringify(recoveryQuery.data ?? { retentionDays: 7 })) {
        promises.push(setRecoveryConfig(recoveryConfig))
      }
      await Promise.all(promises)
      queryClient.invalidateQueries({ queryKey: settingsKeys.all })
      onClose()
    } catch (err) {
      setError((err as Error).message || '儲存失敗')
    } finally {
      setSaving(false)
    }
  }, [modelConfig, gitConfig, firewallConfig, digestConfig, recoveryConfig, onClose, queryClient, modelQuery.data, epicGitQuery.data, firewallQuery.data, digestQuery.data, recoveryQuery.data])

  const handleSync = useCallback(async () => {
    setSyncing(true)
    try {
      await triggerEpicGitSync()
      setTimeout(() => {
        queryClient.invalidateQueries({ queryKey: settingsKeys.epicGitStatus })
        setSyncing(false)
      }, 1000)
    } catch {
      setSyncing(false)
    }
  }, [queryClient])

  const handleSendDigest = useCallback(async () => {
    setSendingDigest(true)
    setDigestProgress('')
    try {
      const { promise } = sendDigestSSE((event) => {
        if (event.total && event.total > 1) {
          setDigestProgress(`${event.message} (${event.current}/${event.total})`)
        } else {
          setDigestProgress(event.message)
        }
      })
      await promise
    } catch (err) {
      setError((err as Error).message || '發送失敗')
    } finally {
      setSendingDigest(false)
      setDigestProgress('')
    }
  }, [])

  // Escape key disabled — modal must be closed via Cancel button to prevent accidental state loss

  if (!isOpen) return null

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-[200]">
      <div className="modal-card settings-dialog" onClick={(e) => e.stopPropagation()}>
        <div className="px-5 pt-4 pb-3">
          <h2 className="modal-title">設定</h2>
        </div>

        <div className="flex flex-col flex-1 min-h-0">
          {/* Tab bar */}
          <nav className="modal-tabs mb-0 px-5">
            {TABS.map(({ key, label, icon }) => (
              <button
                key={key}
                type="button"
                className={`modal-tab${activeTab === key ? ' modal-tab--active' : ''}`}
                onClick={() => setActiveTab(key)}
              >
                <span className="modal-tab-icon">{icon}</span>
                {label}
              </button>
            ))}
          </nav>

          {/* Content area */}
          <div className="flex-1 px-6 py-5 overflow-y-auto min-h-[280px]">
            {activeTab === 'general' && (
              <GeneralTab
                modelConfig={modelConfig}
                onModelChange={handleModelChange}
                recoveryRetentionDays={recoveryConfig.retentionDays}
                onRecoveryChange={(days) => setRecoveryCfg({ retentionDays: days })}
              />
            )}
            {activeTab === 'git' && (
              <GitTab
                gitConfig={gitConfig}
                setGitCfg={setGitCfg}
                syncStatus={syncStatusQuery.data ?? null}
                syncing={syncing}
                onSync={handleSync}
              />
            )}
            {activeTab === 'firewall' && (
              <FirewallTab config={firewallConfig} onChange={setFirewallCfg} />
            )}
            {activeTab === 'digest' && (
              <DigestTab
                config={digestConfig}
                onChange={setDigestCfg}
                onSendNow={handleSendDigest}
                sending={sendingDigest}
                progressMessage={digestProgress}
              />
            )}

            {/* Error message */}
            {error && (
              <div className="text-xs text-[var(--danger)] py-1">{error}</div>
            )}
          </div>
        </div>

        <div className="flex items-center justify-end gap-2 px-5 py-3 border-t border-[var(--border)]">
          <Button variant="outline" onClick={onClose}>
            取消
          </Button>
          <Button onClick={handleSave} disabled={saving || !hasChanges}>
            {saving ? <><span className="btn-spinner" /> 儲存中...</> : '儲存'}
          </Button>
        </div>
      </div>
    </div>
  )
}
