import React, { useState, useEffect, useCallback, useRef } from 'react'
import { browseDirectory, setupProject, cancelSetup } from '../api'
import { Button } from './ui/button'
import type { BrowseResult, SetupPhase } from '../types'

interface NewProjectModalProps {
  isOpen: boolean
  onClose: () => void
  onProjectCreated: () => void
  onMinimize?: () => void
}

const PHASES: { key: SetupPhase; label: string; desc: string }[] = [
  { key: 'writing', label: '寫入檔案', desc: '正在寫入 .devcontainer/ 目錄下的配置檔案...' },
  { key: 'validating', label: '驗證配置', desc: '正在預先驗證 DevContainer 配置是否正確...' },
  { key: 'building', label: '建置容器', desc: '正在拉取映像並建置容器，首次可能需要數分鐘...' },
  { key: 'verifying', label: '驗證環境', desc: '正在驗證容器內的執行環境和掛載目錄...' },
  { key: 'done', label: '完成', desc: '專案已就緒' },
]

const phaseIndex = (phase: SetupPhase): number => {
  const idx = PHASES.findIndex((p) => p.key === phase)
  return idx >= 0 ? idx : -1
}

function formatElapsed(seconds: number): string {
  const m = Math.floor(seconds / 60)
  const s = seconds % 60
  if (m > 0) return `${m}:${s.toString().padStart(2, '0')}`
  return `${s}s`
}

function DirPicker({ value, onChange }: { value: string; onChange: (path: string) => void }) {
  const [browseResult, setBrowseResult] = useState<BrowseResult | null>(null)
  const [loading, setLoading] = useState(false)
  const [manualPath, setManualPath] = useState(false)

  const loadDir = useCallback(async (path: string) => {
    setLoading(true)
    try {
      const result = await browseDirectory(path || '')
      setBrowseResult(result)
      if (!value) {
        onChange(result.current)
      }
    } catch {
      // keep current state on error
    } finally {
      setLoading(false)
    }
  }, [value, onChange])

  useEffect(() => {
    loadDir('')
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const navigateTo = async (path: string) => {
    setLoading(true)
    try {
      const result = await browseDirectory(path)
      setBrowseResult(result)
      onChange(result.current)
    } catch {
      // ignore
    } finally {
      setLoading(false)
    }
  }

  const breadcrumbs = browseResult
    ? browseResult.current.split('/').filter(Boolean)
    : []

  return (
    <div className="dir-picker">
      <div className="dir-picker-header">
        <label className="dir-picker-label">專案路徑</label>
        <button
          type="button"
          className="dir-picker-toggle"
          onClick={() => setManualPath(!manualPath)}
        >
          {manualPath ? '瀏覽' : '手動輸入'}
        </button>
      </div>

      {manualPath ? (
        <input
          type="text"
          className="modal-input"
          placeholder="/path/to/project"
          value={value}
          onChange={(e) => onChange(e.target.value)}
        />
      ) : (
        <div className="dir-browser">
          <div className="dir-breadcrumb">
            <button
              type="button"
              className="dir-crumb"
              onClick={() => navigateTo('/')}
            >/</button>
            {breadcrumbs.map((part, i) => {
              const path = '/' + breadcrumbs.slice(0, i + 1).join('/')
              const isLast = i === breadcrumbs.length - 1
              return (
                <span key={path}>
                  <button
                    type="button"
                    className={`dir-crumb${isLast ? ' dir-crumb--active' : ''}`}
                    onClick={() => !isLast && navigateTo(path)}
                  >
                    {part}
                  </button>
                  {!isLast && <span className="dir-crumb-sep">/</span>}
                </span>
              )
            })}
          </div>

          <div className="dir-list">
            {loading && <div className="dir-list-loading">載入中...</div>}
            {!loading && browseResult && (
              <>
                {browseResult.current !== browseResult.parent && (
                  <button
                    type="button"
                    className="dir-item dir-item--parent"
                    onClick={() => navigateTo(browseResult.parent)}
                  >
                    ..
                  </button>
                )}
                {browseResult.entries.length === 0 && (
                  <div className="dir-list-empty">沒有子目錄</div>
                )}
                {browseResult.entries.map((entry) => (
                  <button
                    key={entry.path}
                    type="button"
                    className={`dir-item${entry.path === value ? ' dir-item--selected' : ''}`}
                    onClick={() => onChange(entry.path)}
                    onDoubleClick={() => navigateTo(entry.path)}
                    title="雙擊開啟目錄"
                  >
                    <span className="dir-item-icon">{'\uD83D\uDCC1'}</span>
                    <span className="dir-item-name">{entry.name}</span>
                  </button>
                ))}
              </>
            )}
          </div>
        </div>
      )}

      <div className="dir-selected">
        <span className="dir-selected-label">已選擇:</span>
        <code className="dir-selected-path">{value || '~'}</code>
      </div>
    </div>
  )
}

// -- Progress Steps --

const stepStyles: Record<string, React.CSSProperties> = {
  container: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    margin: '24px 0 16px',
    position: 'relative',
  },
  step: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    flex: 1,
    position: 'relative',
    zIndex: 1,
  },
  circle: {
    width: 28,
    height: 28,
    borderRadius: '50%',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    fontSize: 12,
    fontWeight: 600,
    marginBottom: 6,
    transition: 'background 0.3s, color 0.3s',
  },
  label: {
    fontSize: 11,
    textAlign: 'center' as const,
    whiteSpace: 'nowrap' as const,
  },
  connector: {
    position: 'absolute' as const,
    top: 14,
    left: 0,
    right: 0,
    height: 2,
    zIndex: 0,
  },
}

function ProgressSteps({ currentPhase }: { currentPhase: SetupPhase }) {
  const current = phaseIndex(currentPhase)
  const isFailed = currentPhase === 'failed'

  return (
    <div style={{ position: 'relative' }}>
      {/* Connector line behind steps */}
      <div style={{
        ...stepStyles.connector,
        background: 'var(--border-color, #333)',
        marginLeft: '12.5%',
        marginRight: '12.5%',
      }} />
      <div style={stepStyles.container}>
        {PHASES.map((phase, i) => {
          const isCompleted = !isFailed && current > i
          const isActive = !isFailed && current === i
          const isFail = isFailed && current === i

          let bg = 'var(--bg-secondary, #2a2a2a)'
          let color = 'var(--text-tertiary, #666)'
          if (isCompleted) {
            bg = 'var(--color-success, #22c55e)'
            color = '#fff'
          } else if (isActive) {
            bg = 'var(--color-primary, #3b82f6)'
            color = '#fff'
          } else if (isFail) {
            bg = 'var(--color-error, #ef4444)'
            color = '#fff'
          }

          return (
            <div key={phase.key} style={stepStyles.step}>
              <div style={{ ...stepStyles.circle, background: bg, color }}>
                {isCompleted ? '\u2713' : i + 1}
              </div>
              <span style={{
                ...stepStyles.label,
                color: isActive || isCompleted ? 'var(--text-primary, #eee)' : 'var(--text-tertiary, #666)',
                fontWeight: isActive ? 600 : 400,
              }}>
                {phase.label}
              </span>
            </div>
          )
        })}
      </div>
    </div>
  )
}

// -- Elapsed Timer Hook --

function useElapsedTimer(running: boolean): number {
  const [elapsed, setElapsed] = useState(0)
  const startRef = useRef<number>(0)

  useEffect(() => {
    if (!running) {
      setElapsed(0)
      return
    }
    startRef.current = Date.now()
    setElapsed(0)
    const timer = setInterval(() => {
      setElapsed(Math.floor((Date.now() - startRef.current) / 1000))
    }, 1000)
    return () => clearInterval(timer)
  }, [running])

  return elapsed
}

// -- Main Component --

type Mode = 'form' | 'progress' | 'done' | 'error'

export default function NewProjectModal({ isOpen, onClose, onProjectCreated, onMinimize }: NewProjectModalProps) {
  // Form state
  const [name, setName] = useState('')
  const [path, setPath] = useState('')
  const [autoMode, setAutoMode] = useState(true)
  const [submitting, setSubmitting] = useState(false)

  // Progress state
  const [mode, setMode] = useState<Mode>('form')
  const [setupID, setSetupID] = useState<string | null>(null)
  const [phase, setPhase] = useState<SetupPhase>('writing')
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const [minimized, _setMinimized] = useState(false)
  const minimizedRef = useRef(false)
  const setMinimized = useCallback((v: boolean) => {
    minimizedRef.current = v
    _setMinimized(v)
  }, [])
  const [animating, setAnimating] = useState<'minimizing' | 'restoring' | null>(null)

  const elapsed = useElapsedTimer(mode === 'progress')

  // Reset state when modal opens
  useEffect(() => {
    if (isOpen) {
      setName('')
      setPath('')
      setAutoMode(true)
      setSubmitting(false)
      setMode('form')
      setSetupID(null)
      setPhase('writing')
      setMessage('')
      setError('')
      setMinimized(false)
      setAnimating(null)
    }
  }, [isOpen])

  const handleCancel = useCallback(async () => {
    if (setupID) {
      try {
        await cancelSetup(setupID)
      } catch {
        // ignore cancel errors
      }
    }
    setMinimized(false)
    setAnimating(null)
    onClose()
  }, [setupID, onClose])

  // Prevent body scroll when open and not minimized
  useEffect(() => {
    if (isOpen && !minimized) {
      document.body.style.overflow = 'hidden'
    } else {
      document.body.style.overflow = ''
    }
    return () => {
      document.body.style.overflow = ''
    }
  }, [isOpen, minimized])

  // Listen for setup_progress WebSocket events
  useEffect(() => {
    if (!setupID) return
    const handler = (e: Event) => {
      const detail = (e as CustomEvent).detail
      if (detail.setupID !== setupID) return
      setPhase(detail.phase)
      setMessage(detail.message || '')
      if (detail.phase === 'done') {
        setMode('done')
        onProjectCreated()
        if (!minimizedRef.current) {
          setTimeout(() => {
            onClose()
          }, 1500)
        }
      }
      if (detail.phase === 'failed') {
        setMode('error')
        setMinimized(false)
        setError(detail.error || detail.message || '設定失敗')
      }
    }
    window.addEventListener('setup_progress', handler)
    return () => window.removeEventListener('setup_progress', handler)
  }, [setupID, onProjectCreated, onClose])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim() || !path.trim()) return

    setSubmitting(true)
    try {
      const result = await setupProject(name.trim(), path.trim(), { autoMode })
      setSetupID(result.setupID)
      setMode('progress')
      setPhase('writing')
      setMessage('正在啟動...')
    } catch (err) {
      setMode('error')
      setError((err as Error).message || '建立專案失敗')
    } finally {
      setSubmitting(false)
    }
  }

  const handleMinimize = useCallback(() => {
    setAnimating('minimizing')
    setTimeout(() => {
      setMinimized(true)
      setAnimating(null)
      onMinimize?.()
    }, 300)
  }, [onMinimize])

  // Escape key: always minimize (never close)
  useEffect(() => {
    if (!isOpen) return
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        handleMinimize()
      }
    }
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [isOpen, handleMinimize])

  const handleRestore = useCallback(() => {
    setMinimized(false)
    setAnimating('restoring')
    setTimeout(() => {
      setAnimating(null)
    }, 300)
    // If restoring after completion, show done modal then auto-close
    if (mode === 'done') {
      setTimeout(() => {
        onClose()
      }, 1500)
    }
  }, [mode, onClose, setMinimized])

  const handleOverlayClick = () => {
    handleMinimize()
  }

  const handleRetry = () => {
    setMode('form')
    setSetupID(null)
    setPhase('writing')
    setMessage('')
    setError('')
  }

  if (!isOpen) return null

  // Get phase description for detailed info
  const currentPhaseInfo = PHASES.find((p) => p.key === phase)
  const phaseDesc = currentPhaseInfo?.desc || message

  // -- Minimized: floating status bar at bottom-left --
  if (minimized) {
    return (
      <div
        className="setup-mini-bar setup-mini-bar--enter"
        onClick={handleRestore}
        title="點擊展開"
      >
        {mode === 'done'
          ? <span className="setup-mini-bar__check">{'\u2713'}</span>
          : <span className="setup-mini-bar__spinner" />
        }
        <span className="setup-mini-bar__text">
          {name} — {mode === 'done' ? '建置完成' : (currentPhaseInfo?.label || '處理中')}
        </span>
        {mode !== 'done' && (
          <span className="setup-mini-bar__elapsed">{formatElapsed(elapsed)}</span>
        )}
      </div>
    )
  }

  const modalCardClass = [
    'modal-card modal-card--wide',
    animating === 'minimizing' ? 'modal-card--minimizing' : '',
    animating === 'restoring' ? 'modal-card--restoring' : '',
  ].filter(Boolean).join(' ')

  return (
    <div
      className={`fixed inset-0 bg-black/50 flex items-center justify-center z-[200]${animating === 'minimizing' ? ' modal-overlay--fading' : ''}`}
      onClick={handleOverlayClick}
    >
      <div className={modalCardClass} onClick={(e) => e.stopPropagation()}>
        <h2 className="modal-title">建立新專案</h2>

        {mode === 'form' && (
          <form onSubmit={handleSubmit}>
            <div style={{ marginBottom: 12 }}>
              <label style={{ display: 'block', fontSize: 13, fontWeight: 600, marginBottom: 4, color: 'var(--text-secondary, #aaa)' }}>
                專案名稱
              </label>
              <input
                className="modal-input"
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="my-project"
                autoFocus
              />
            </div>

            <div style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              marginBottom: 12,
              padding: '8px 12px',
              borderRadius: 6,
              background: autoMode
                ? 'var(--bg-success, rgba(34,197,94,0.08))'
                : 'var(--bg-secondary, #2a2a2a)',
              border: `1px solid ${autoMode ? 'var(--color-success, #22c55e)' : 'var(--border-color, #333)'}`,
              transition: 'background 0.2s, border-color 0.2s',
            }}>
              <div>
                <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-primary, #eee)' }}>
                  {autoMode ? '全自動模式' : '標準模式'}
                </div>
                <div style={{ fontSize: 11, color: 'var(--text-tertiary, #666)', marginTop: 2 }}>
                  {autoMode
                    ? 'claude 指令帶 --dangerously-skip-permissions'
                    : 'claude 指令需手動確認權限'}
                </div>
              </div>
              <button
                type="button"
                onClick={() => setAutoMode(!autoMode)}
                style={{
                  position: 'relative',
                  width: 40,
                  height: 22,
                  borderRadius: 11,
                  border: 'none',
                  cursor: 'pointer',
                  background: autoMode ? 'var(--color-success, #22c55e)' : 'var(--text-tertiary, #555)',
                  transition: 'background 0.2s',
                  flexShrink: 0,
                  marginLeft: 12,
                }}
                title={autoMode ? '全自動模式：claude 指令帶 --dangerously-skip-permissions' : '標準模式：claude 指令需手動確認權限'}
              >
                <span style={{
                  position: 'absolute',
                  top: 2,
                  left: autoMode ? 20 : 2,
                  width: 18,
                  height: 18,
                  borderRadius: '50%',
                  background: '#fff',
                  transition: 'left 0.2s',
                }} />
              </button>
            </div>

            <div style={{ marginBottom: 12 }}>
              <DirPicker value={path} onChange={setPath} />
            </div>

            <div className="modal-actions">
              <Button variant="outline" onClick={onClose}>
                取消
              </Button>
              <Button
                type="submit"
                disabled={!name.trim() || !path.trim() || submitting}
              >
                {submitting ? '建立中...' : '建立'}
              </Button>
            </div>
          </form>
        )}

        {mode === 'progress' && (
          <div className="setup-progress">
            <ProgressSteps currentPhase={phase} />

            <div style={{
              textAlign: 'center',
              margin: '16px 0 8px',
              fontSize: 13,
              color: 'var(--text-secondary, #aaa)',
              minHeight: 20,
            }}>
              {message}
            </div>

            <div style={{
              textAlign: 'center',
              fontSize: 12,
              color: 'var(--text-tertiary, #666)',
              lineHeight: 1.5,
              minHeight: 36,
              marginBottom: 8,
            }}>
              {phaseDesc}
              <br />
              <span style={{ fontVariantNumeric: 'tabular-nums' }}>
                已經過 {formatElapsed(elapsed)}
              </span>
            </div>

            <div style={{
              textAlign: 'center',
              fontSize: 11,
              color: 'var(--text-tertiary, #555)',
              marginBottom: 12,
            }}>
              點擊視窗外可縮小到背景繼續
            </div>

            <div className="modal-actions">
              <Button variant="outline" onClick={handleCancel}>
                取消建置
              </Button>
            </div>
          </div>
        )}

        {mode === 'done' && (
          <div style={{ textAlign: 'center', padding: '32px 0' }}>
            <div style={{ fontSize: 40, marginBottom: 12 }}>{'\u2705'}</div>
            <div style={{ fontSize: 15, fontWeight: 600, color: 'var(--color-success, #22c55e)' }}>
              專案建立完成
            </div>
          </div>
        )}

        {mode === 'error' && (
          <div style={{ padding: '24px 0' }}>
            <div style={{
              background: 'var(--bg-error, rgba(239,68,68,0.1))',
              border: '1px solid var(--color-error, #ef4444)',
              borderRadius: 6,
              padding: '12px 16px',
              fontSize: 13,
              color: 'var(--color-error, #ef4444)',
              marginBottom: 16,
              wordBreak: 'break-word',
            }}>
              {error}
            </div>
            <div className="modal-actions">
              <Button variant="outline" onClick={onClose}>
                關閉
              </Button>
              <Button onClick={handleRetry}>
                重試
              </Button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
