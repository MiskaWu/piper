import React, { useState, useRef, useEffect, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { GitBranch, ChevronDown, Loader2, ArrowUpToLine, Filter, RefreshCw, Ellipsis, Tag, Copy, X, Plus, GitFork } from 'lucide-react'
import { initGitRepo, pushGit, fetchCommitBody, addGitRemote, fetchSubmodules, pushSubmodules, createGitTag, pushGitTag, fetchGitTags } from '../api'
import type { GitPushResult } from '../api'
import type { SubmoduleInfo } from '../types'
import { useGitGraph, useGitBranches, BATCH_SIZE } from '../hooks/useGitGraph'
import { useQueryClient, useQuery } from '@tanstack/react-query'
import { Terminal as XTerm } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { Button } from './ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from './ui/tooltip'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from './ui/dropdown-menu'
import { Popover, PopoverContent, PopoverTrigger } from './ui/popover'

const ROW_HEIGHT = 32
const COL_WIDTH = 16
const LEFT_PAD = 12
const CURVE_RADIUS = 10
const COLORS = ['#0085d9', '#d9008f', '#00d960', '#d98500', '#a300d9', '#009bd9', '#d90000', '#00d9a3']

function buildRailroadPath(
  x1: number, y1: number,
  x2: number, y2: number,
): string {
  if (x1 === x2) {
    return `M ${x1} ${y1} L ${x2} ${y2}`
  }
  const R = Math.min(CURVE_RADIUS, Math.abs(y2 - y1) / 2, Math.abs(x2 - x1))
  const signX = x2 > x1 ? 1 : -1
  const midY = y2 - R

  return `M ${x1} ${y1}`
    + ` L ${x1} ${midY}`
    + ` Q ${x1} ${y2}, ${x1 + signX * R} ${y2}`
    + (Math.abs(x2 - x1) > R ? ` L ${x2} ${y2}` : '')
}

const ANGLE_EMAIL_RE = /<([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})>/g

function highlightBody(text: string): React.ReactNode[] {
  const result: React.ReactNode[] = []
  let last = 0
  for (const m of text.matchAll(ANGLE_EMAIL_RE)) {
    if (m.index! > last) result.push(text.slice(last, m.index!))
    result.push(<span key={m.index} className="git-graph-email">{m[1]}</span>)
    last = m.index! + m[0].length
  }
  if (last < text.length) result.push(text.slice(last))
  return result
}

interface GitGraphProps {
  projectPath: string
}

export default function GitGraph({ projectPath }: GitGraphProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [limit, setLimit] = useState(BATCH_SIZE)
  const [selectedRef, setSelectedRef] = useState<string | undefined>(undefined)
  const [branchDropdownOpen, setBranchDropdownOpen] = useState(false)
  const [hiddenRemotes, setHiddenRemotes] = useState<Set<string>>(new Set())
  const [remoteFilterOpen, setRemoteFilterOpen] = useState(false)
  const hideRemotesArr = hiddenRemotes.size > 0 ? Array.from(hiddenRemotes) : undefined
  const { data, isLoading, isFetching, refetch } = useGitGraph(projectPath, limit, selectedRef, hideRemotesArr)
  const { data: branchData } = useGitBranches(projectPath, !!data?.isRepo)
  const scrollContainerRef = useRef<HTMLDivElement>(null)
  const [initUrl, setInitUrl] = useState('')
  const [expandedCommits, setExpandedCommits] = useState<Set<string>>(new Set())
  const [pushing, setPushing] = useState(false)
  const [pushError, setPushError] = useState<GitPushResult | null>(null)
  const [showHostShell, setShowHostShell] = useState(false)
  const [pushToast, setPushToast] = useState<{ type: 'success' | 'info'; message: string } | null>(null)
  const toastTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const [hoveredHash, setHoveredHash] = useState<string | null>(null)
  const [showRemoteForm, setShowRemoteForm] = useState(false)
  const [remoteUrl, setRemoteUrl] = useState('')
  const [addingRemote, setAddingRemote] = useState(false)
  const [bodyCache, setBodyCache] = useState<Record<string, string>>({})
  const hoverTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const [showPushPanel, setShowPushPanel] = useState(false)
  const [pushIncludeSubs, setPushIncludeSubs] = useState(false)
  const [pushIncludeTags, setPushIncludeTags] = useState(false)
  const [tagFormHash, setTagFormHash] = useState<string | null>(null)
  const [tagName, setTagName] = useState('')
  const [tagPushAfter, setTagPushAfter] = useState(true)
  const [tagging, setTagging] = useState(false)
  const tagInputRef = useRef<HTMLInputElement>(null)
  const [kebabHash, setKebabHash] = useState<string | null>(null)

  const { data: submodules } = useQuery<SubmoduleInfo[]>({
    queryKey: ['git-submodules', projectPath],
    queryFn: () => fetchSubmodules(projectPath),
    enabled: !!projectPath && !!data?.isRepo,
  })

  const { data: tags } = useQuery<{ name: string; hash: string; pushed: boolean }[]>({
    queryKey: ['git-tags', projectPath],
    queryFn: () => fetchGitTags(projectPath),
    enabled: !!projectPath && !!data?.isRepo,
  })

  const pushableSubs = (submodules || []).filter(s => s.ahead > 0)
  const unpushedTags = (tags || []).filter(t => !t.pushed)

  const handleCreateTag = async () => {
    if (!tagName.trim() || !tagFormHash) return
    setTagging(true)
    try {
      await createGitTag(projectPath, tagName.trim(), tagFormHash)
      if (tagPushAfter) {
        const result = await pushGitTag(projectPath, tagName.trim())
        if (result.status !== 'ok') {
          setPushError(result)
        } else {
          showToast('success', t('git.tagPushed', { name: tagName.trim() }))
        }
      } else {
        showToast('success', t('git.tagCreated', { name: tagName.trim() }))
      }
      setTagFormHash(null)
      setTagName('')
      setKebabHash(null)
      queryClient.invalidateQueries({ queryKey: ['git-tags', projectPath] })
      refetch()
    } catch (err) {
      setPushError({ status: 'error', code: 'PUSH_FAILED', message: String(err) })
    } finally {
      setTagging(false)
    }
  }

  const handleRowEnter = useCallback((hash: string) => {
    if (hoverTimerRef.current) clearTimeout(hoverTimerRef.current)
    hoverTimerRef.current = setTimeout(() => {
      setHoveredHash(hash)
      if (!(hash in bodyCache)) {
        fetchCommitBody(projectPath, hash).then(res => {
          setBodyCache(prev => ({ ...prev, [hash]: res.body }))
        }).catch(() => {
          setBodyCache(prev => ({ ...prev, [hash]: '' }))
        })
      }
    }, 300)
  }, [bodyCache, projectPath])

  const handleRowLeave = useCallback(() => {
    if (hoverTimerRef.current) clearTimeout(hoverTimerRef.current)
    setHoveredHash(null)
  }, [])

  // Infinite scroll: load more when user scrolls near bottom
  const hasMore = data?.hasMore ?? false
  const isFetchingRef = useRef(isFetching)
  isFetchingRef.current = isFetching
  const [isPaginating, setIsPaginating] = useState(false)
  // Clear isPaginating when fetch completes
  useEffect(() => {
    if (!isFetching) setIsPaginating(false)
  }, [isFetching])
  // Auto-fill: if container has remaining space, load more
  useEffect(() => {
    const root = scrollContainerRef.current
    if (!root || !hasMore || isFetching) return
    // scrollHeight <= clientHeight means content doesn't fill the viewport
    if (root.scrollHeight <= root.clientHeight) {
      setIsPaginating(true)
      setLimit(prev => prev + BATCH_SIZE)
    }
  }, [hasMore, isFetching, data])
  useEffect(() => {
    const root = scrollContainerRef.current
    if (!root || !hasMore) return
    const onScroll = () => {
      if (isFetchingRef.current) return
      const nearBottom = root.scrollHeight - root.scrollTop - root.clientHeight < 200
      if (nearBottom) {
        setIsPaginating(true)
        setLimit(prev => prev + BATCH_SIZE)
      }
    }
    root.addEventListener('scroll', onScroll, { passive: true })
    return () => root.removeEventListener('scroll', onScroll)
  }, [hasMore])


  const toggleHiddenRemote = (remote: string) => {
    setHiddenRemotes(prev => {
      const next = new Set(prev)
      if (next.has(remote)) next.delete(remote)
      else next.add(remote)
      return next
    })
  }

  const handleBranchSelect = (branchName: string | undefined) => {
    setSelectedRef(branchName)
    setLimit(BATCH_SIZE)
    setBranchDropdownOpen(false)
  }

  const toggleExpand = (hash: string) => {
    setExpandedCommits(prev => {
      const next = new Set(prev)
      if (next.has(hash)) next.delete(hash)
      else next.add(hash)
      return next
    })
  }

  const handleInit = async () => {
    try {
      await initGitRepo(projectPath, initUrl || undefined)
      queryClient.invalidateQueries({ queryKey: ['git-graph', projectPath] })
    } catch (err) {
      console.error('Init failed:', err)
    }
  }

  const showToast = (type: 'success' | 'info', message: string) => {
    if (toastTimerRef.current) clearTimeout(toastTimerRef.current)
    setPushToast({ type, message })
    toastTimerRef.current = setTimeout(() => setPushToast(null), 2500)
  }

  const handlePush = async () => {
    if (data && data.ahead === 0) {
      showToast('info', t('git.alreadyUpToDate'))
      return
    }
    setPushing(true)
    setPushError(null)
    try {
      const result = await pushGit(projectPath, selectedRef)
      if (result.status === 'ok') {
        setShowHostShell(false)
        setPushError(null)
        showToast('success', t('git.pushed', { count: data?.ahead ?? 0 }))
        queryClient.invalidateQueries({ queryKey: ['git-graph', projectPath] })
      } else {
        setPushError(result)
        if (result.code === 'SSH_AGENT_UNAVAILABLE' || result.code === 'SSH_NO_KEYS') {
          setShowHostShell(true)
        }
      }
    } catch (err) {
      setPushError({ status: 'error', code: 'PUSH_FAILED', message: String(err) })
    } finally {
      setPushing(false)
    }
  }

  const handlePushClick = () => {
    const hasPendingSubs = pushableSubs.length > 0
    const hasPendingTags = unpushedTags.length > 0
    if (hasPendingSubs || hasPendingTags) {
      setPushIncludeSubs(false)
      setPushIncludeTags(false)
      setShowPushPanel(true)
    } else {
      handlePush()
    }
  }

  const handlePushAll = async () => {
    setPushing(true)
    setPushError(null)
    try {
      // Push branch
      if (data && data.ahead > 0) {
        const result = await pushGit(projectPath, selectedRef)
        if (result.status !== 'ok') {
          setPushError(result)
          if (result.code === 'SSH_AGENT_UNAVAILABLE' || result.code === 'SSH_NO_KEYS') {
            setShowHostShell(true)
          }
          return
        }
      }
      // Push submodules
      if (pushIncludeSubs && pushableSubs.length > 0) {
        const subPaths = pushableSubs.map(s => s.path)
        const result = await pushSubmodules(projectPath, subPaths)
        if (result.status !== 'ok') {
          setPushError(result)
          return
        }
        queryClient.invalidateQueries({ queryKey: ['git-submodules', projectPath] })
      }
      // Push all tags
      if (pushIncludeTags && unpushedTags.length > 0) {
        for (const tag of unpushedTags) {
          const result = await pushGitTag(projectPath, tag.name)
          if (result.status !== 'ok') {
            setPushError(result)
            return
          }
        }
        queryClient.invalidateQueries({ queryKey: ['git-tags', projectPath] })
      }
      setShowPushPanel(false)
      showToast('success', t('git.pushed', { count: data?.ahead ?? 0 }))
      queryClient.invalidateQueries({ queryKey: ['git-graph', projectPath] })
    } catch (err) {
      setPushError({ status: 'error', code: 'PUSH_FAILED', message: String(err) })
    } finally {
      setPushing(false)
    }
  }

  const handleAddRemote = async () => {
    if (!remoteUrl.trim()) return
    setAddingRemote(true)
    try {
      await addGitRemote(projectPath, 'origin', remoteUrl.trim())
      setShowRemoteForm(false)
      setRemoteUrl('')
      showToast('success', t('git.remoteAdded'))
      queryClient.invalidateQueries({ queryKey: ['git-graph', projectPath] })
    } catch (err) {
      showToast('info', String(err))
    } finally {
      setAddingRemote(false)
    }
  }

  const handleShellClose = () => {
    setShowHostShell(false)
    setPushError(null)
  }

  if (isLoading && !data) {
    return <div className="git-graph-panel"><div className="git-graph-empty">{t('git.loading')}</div></div>
  }

  if (!data || !data.isRepo) {
    return (
      <div className="git-graph-panel">
        <div className="git-graph-header">
          <span>Git Graph</span>
        </div>
        <div className="git-graph-init">
          <p>{t('git.notARepo')}</p>
          <input
            placeholder={t('git.remoteUrlPlaceholder')}
            value={initUrl}
            onChange={e => setInitUrl(e.target.value)}
          />
          <Button onClick={handleInit}>{t('git.initialize')}</Button>
        </div>
      </div>
    )
  }

  const commits = data.commits || []
  const edges = data.edges || []
  const allRemotes = data.remotes || []
  const hasRemotes = allRemotes.length > 0

  // Helper: check if a ref belongs to any known remote
  const isRemoteRef = (ref: string) => allRemotes.some(r => ref.startsWith(r + '/'))
  // Helper: check if a ref is a tag (not local, not remote)
  const isTagRef = (ref: string) => ref.startsWith('tag: ')

  const svgWidth = (data.maxCol + 1) * COL_WIDTH + LEFT_PAD

  // Calculate actual Y positions accounting for expanded file lists
  const FILE_ROW_HEIGHT = 20
  const FILES_PADDING = 10 // top+bottom padding for file list
  const rowYPositions: number[] = []
  let cumulativeY = 0
  commits.forEach((commit) => {
    rowYPositions.push(cumulativeY)
    cumulativeY += ROW_HEIGHT
    if (expandedCommits.has(commit.hash)) {
      const fileCount = (commit.files || []).length
      cumulativeY += fileCount * FILE_ROW_HEIGHT + FILES_PADDING
    }
  })
  const svgHeight = cumulativeY

  // Build hash -> row index map
  const hashToRow: Record<string, number> = {}
  commits.forEach((c, i) => { hashToRow[c.hash] = i })

  // Find which commit has a remote tracking ref to mark outgoing boundary
  // Check all remotes, not just origin (e.g. upstream/main, fork/main)
  const trackingRefs = allRemotes.map(r => `${r}/${data.branch}`)
  const originIdx = commits.findIndex(c =>
    (c.refs || []).some(r => trackingRefs.includes(r))
  )

  return (
    <div className="git-graph-panel" ref={scrollContainerRef}>
      <div className="git-graph-header">
        <div style={{ display: 'flex', alignItems: 'center', gap: '6px', minWidth: 0 }}>
          <Tooltip>
            <Popover open={branchDropdownOpen} onOpenChange={setBranchDropdownOpen}>
              <TooltipTrigger asChild>
                <PopoverTrigger asChild>
                  <Button variant="ghost" size="sm" className="branch-selector-btn gap-1.5">
                    <GitBranch className="size-3 shrink-0" />
                    <span className="branch-selector-name">{selectedRef || data.branch || 'detached'}</span>
                    <ChevronDown className="size-2.5 shrink-0" />
                  </Button>
                </PopoverTrigger>
              </TooltipTrigger>
              <TooltipContent>{t('git.switchBranch')}</TooltipContent>
            {branchData && (
              <PopoverContent className="branch-dropdown w-56 p-1" align="start">
                <button
                  className={`branch-dropdown-item${!selectedRef ? ' branch-dropdown-active' : ''}`}
                  onClick={() => handleBranchSelect(undefined)}
                >
                  <span className="branch-dropdown-label">{t('git.allBranches')}</span>
                  {!selectedRef && <span className="branch-dropdown-check">✓</span>}
                </button>
                <div className="branch-dropdown-divider" />
                {branchData.branches.filter(b => !b.isRemote).map(b => (
                  <button
                    key={b.name}
                    className={`branch-dropdown-item${selectedRef === b.name ? ' branch-dropdown-active' : ''}`}
                    onClick={() => handleBranchSelect(b.name)}
                  >
                    <span className="branch-dropdown-label">
                      {b.name}
                      {b.isCurrent && <span className="branch-dropdown-current">HEAD</span>}
                    </span>
                    {selectedRef === b.name && <span className="branch-dropdown-check">✓</span>}
                  </button>
                ))}
                {branchData.branches.some(b => b.isRemote) && (() => {
                  const remoteBranches = branchData.branches.filter(b => b.isRemote)
                  const grouped: Record<string, typeof remoteBranches> = {}
                  for (const b of remoteBranches) {
                    const slashIdx = b.name.indexOf('/')
                    const remoteName = slashIdx > 0 ? b.name.slice(0, slashIdx) : 'other'
                    if (!grouped[remoteName]) grouped[remoteName] = []
                    grouped[remoteName].push(b)
                  }
                  return Object.entries(grouped).map(([remoteName, branches]) => (
                    <React.Fragment key={remoteName}>
                      <div className="branch-dropdown-divider" />
                      <div className="branch-dropdown-section">{remoteName}</div>
                      {branches.map(b => (
                        <button
                          key={b.name}
                          className={`branch-dropdown-item${selectedRef === b.name ? ' branch-dropdown-active' : ''}`}
                          onClick={() => handleBranchSelect(b.name)}
                        >
                          <span className="branch-dropdown-label">{b.name}</span>
                          {selectedRef === b.name && <span className="branch-dropdown-check">✓</span>}
                        </button>
                      ))}
                    </React.Fragment>
                  ))
                })()}
              </PopoverContent>
            )}
            </Popover>
          </Tooltip>
          {data.ahead > 0 && (
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="git-graph-ahead">↑{data.ahead}</span>
              </TooltipTrigger>
              <TooltipContent>{`${data.ahead} commit(s) ahead of origin`}</TooltipContent>
            </Tooltip>
          )}
          {data.behind > 0 && (
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="git-graph-behind">↓{data.behind}</span>
              </TooltipTrigger>
              <TooltipContent>{`${data.behind} commit(s) behind origin`}</TooltipContent>
            </Tooltip>
          )}
        </div>
        <div style={{ display: 'flex', gap: '4px', alignItems: 'center' }}>
          {hasRemotes ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  className={`git-push-icon-btn${data.ahead > 0 || pushableSubs.length > 0 || unpushedTags.length > 0 ? ' git-push-active' : ''}`}
                  onClick={handlePushClick}
                  disabled={pushing}
                >
                  {pushing ? (
                    <Loader2 className="size-4 animate-spin" />
                  ) : (
                    <ArrowUpToLine className="size-4" />
                  )}
                </Button>
              </TooltipTrigger>
              <TooltipContent>{data.ahead > 0 ? t('git.pushed', { count: data.ahead }) : t('git.nothingToPush')}</TooltipContent>
            </Tooltip>
          ) : (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  className="git-push-icon-btn git-remote-add-btn"
                  onClick={() => setShowRemoteForm(v => !v)}
                >
                  <Plus className="size-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>{t('git.addRemote')}</TooltipContent>
            </Tooltip>
          )}
          {allRemotes.length > 1 && (
            <Tooltip>
              <Popover open={remoteFilterOpen} onOpenChange={setRemoteFilterOpen}>
                <TooltipTrigger asChild>
                  <PopoverTrigger asChild>
                    <Button
                      variant="ghost"
                      size="icon"
                      className={`git-push-icon-btn${hiddenRemotes.size > 0 ? ' git-filter-active' : ''}`}
                    >
                      <Filter className="size-3.5" />
                    </Button>
                  </PopoverTrigger>
                </TooltipTrigger>
                <TooltipContent>{t('git.filterRemotes')}</TooltipContent>
                <PopoverContent className="remote-filter-dropdown w-48 p-2" align="end">
                  {allRemotes.map(r => (
                    <label key={r} className="remote-filter-item">
                      <input
                        type="checkbox"
                        checked={!hiddenRemotes.has(r)}
                        onChange={() => toggleHiddenRemote(r)}
                      />
                      <span>{r}</span>
                    </label>
                  ))}
                </PopoverContent>
              </Popover>
            </Tooltip>
          )}
          <Tooltip>
            <TooltipTrigger asChild>
              <Button variant="ghost" size="icon" onClick={() => refetch()}>
                <RefreshCw className="size-3.5" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t('git.refresh')}</TooltipContent>
          </Tooltip>
        </div>
      </div>

      {/* Push panel — shown when there are tags/submodules to optionally include */}
      {showPushPanel && (
        <div className="git-push-panel">
          {pushableSubs.length > 0 && (
            <label className="git-push-panel-option">
              <input
                type="checkbox"
                checked={pushIncludeSubs}
                onChange={() => setPushIncludeSubs(v => !v)}
              />
              <GitFork className="size-3" />
              <span>Submodules</span>
              <span className="git-graph-ahead">↑{pushableSubs.reduce((a, s) => a + s.ahead, 0)}</span>
            </label>
          )}
          {unpushedTags.length > 0 && (
            <label className="git-push-panel-option">
              <input
                type="checkbox"
                checked={pushIncludeTags}
                onChange={() => setPushIncludeTags(v => !v)}
              />
              <Tag className="size-3" />
              <span>Tags</span>
              <span className="git-push-panel-count">{unpushedTags.length}</span>
            </label>
          )}
          <Button
            variant="outline"
            size="sm"
            className="git-push-panel-btn"
            onClick={handlePushAll}
            disabled={pushing}
          >
            {pushing ? <Loader2 className="size-3 animate-spin" /> : 'Push'}
          </Button>
        </div>
      )}

      {/* Push toast */}
      {pushToast && (
        <div className={`git-push-toast git-push-toast-${pushToast.type}`}>
          {pushToast.type === 'success' ? '✓' : 'ℹ'} {pushToast.message}
        </div>
      )}

      {/* Add remote form */}
      {showRemoteForm && !hasRemotes && (
        <div className="git-remote-form">
          <input
            placeholder={t('git.remoteUrlPlaceholder')}
            value={remoteUrl}
            onChange={e => setRemoteUrl(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && handleAddRemote()}
            autoFocus
          />
          <Button variant="outline" size="sm" onClick={handleAddRemote} disabled={addingRemote || !remoteUrl.trim()}>
            {addingRemote ? '...' : 'Add'}
          </Button>
          <Button variant="ghost" size="icon" onClick={() => setShowRemoteForm(false)}>
            <X className="size-3.5" />
          </Button>
        </div>
      )}

      {/* Push error banner */}
      {pushError && (
        <div className="git-push-error">
          <div className="git-push-error-msg">{pushError.message}</div>
          {(pushError.code === 'SSH_AGENT_UNAVAILABLE' || pushError.code === 'SSH_NO_KEYS') && !showHostShell && (
            <Button variant="outline" size="sm" className="git-push-error-action" onClick={() => setShowHostShell(true)}>
              Open Host Terminal
            </Button>
          )}
          {pushError.code !== 'SSH_AGENT_UNAVAILABLE' && pushError.code !== 'SSH_NO_KEYS' && (
            <Button variant="ghost" size="icon" className="git-push-error-dismiss" onClick={() => setPushError(null)}>
              <X className="size-3.5" />
            </Button>
          )}
        </div>
      )}

      {/* Inline host shell for SSH troubleshooting */}
      {showHostShell && (
        <HostShellMini onClose={handleShellClose} onRetryPush={handlePush} />
      )}

      {data.ahead > 0 && originIdx > 0 && (
        <div className="git-graph-section-label">Outgoing ({data.ahead})</div>
      )}
      <div style={{ display: 'flex' }}>
        <svg width={svgWidth} height={svgHeight} style={{ flexShrink: 0 }}>
          {/* Pre-compute edge paths */}
          {(() => {
            const edgePaths = edges.map(edge => {
              const fromRow = hashToRow[edge.fromHash]
              const toRow = hashToRow[edge.toHash]
              if (fromRow === undefined || toRow === undefined) return null
              const x1 = edge.fromCol * COL_WIDTH + LEFT_PAD
              const y1 = rowYPositions[fromRow] + ROW_HEIGHT / 2
              const x2 = edge.toCol * COL_WIDTH + LEFT_PAD
              const y2 = rowYPositions[toRow] + ROW_HEIGHT / 2
              const color = COLORS[edge.color % COLORS.length]
              const d = buildRailroadPath(x1, y1, x2, y2)
              return { d, color }
            })
            return (
              <>
                {/* Shadow layer */}
                {edgePaths.map((ep, i) => ep && (
                  <path key={`es${i}`} d={ep.d} stroke="rgba(0,0,0,0.3)" strokeWidth={4} fill="none" strokeLinecap="round" />
                ))}
                {/* Color layer */}
                {edgePaths.map((ep, i) => ep && (
                  <path key={`e${i}`} d={ep.d} stroke={ep.color} strokeWidth={2} fill="none" strokeLinecap="round" />
                ))}
              </>
            )
          })()}
          {/* Mouse head nodes */}
          {commits.map((commit, i) => {
            const cx = commit.column * COL_WIDTH + LEFT_PAD
            const cy = rowYPositions[i] + ROW_HEIGHT / 2
            const color = COLORS[commit.color % COLORS.length]
            const s = commit.isHead ? 1.3 : 1
            return (
              <g key={commit.hash} transform={`translate(${cx},${cy}) scale(${s})`}>
                {/* HEAD outline — follows mouse silhouette */}
                {commit.isHead && (
                  <>
                    <circle cx={-3} cy={-3.5} r={3.4} fill="none" stroke="#fff" strokeWidth={1.2} />
                    <circle cx={3} cy={-3.5} r={3.4} fill="none" stroke="#fff" strokeWidth={1.2} />
                    <circle cx={0} cy={0} r={4.7} fill="none" stroke="#fff" strokeWidth={1.2} />
                  </>
                )}
                {/* Left ear */}
                <circle cx={-3} cy={-3.5} r={2.2} fill={color} />
                {/* Right ear */}
                <circle cx={3} cy={-3.5} r={2.2} fill={color} />
                {/* Face */}
                <circle cx={0} cy={0} r={3.5} fill={color} />
              </g>
            )
          })}
        </svg>
        <div style={{ flex: 1, minWidth: 0 }}>
          {commits.map((commit) => {
            const files = commit.files || []
            const expanded = expandedCommits.has(commit.hash)
            const hasFiles = files.length > 0
            const stats = commit.stats || { insertions: 0, deletions: 0 }
            const hasStats = stats.insertions > 0 || stats.deletions > 0
            const isHovered = hoveredHash === commit.hash
            const body = bodyCache[commit.hash]
            return (
              <div key={commit.hash}>
                <div
                  className="git-graph-row"
                  style={{ height: ROW_HEIGHT, cursor: hasFiles ? 'pointer' : 'default' }}
                  onClick={() => hasFiles && toggleExpand(commit.hash)}
                  onMouseEnter={() => handleRowEnter(commit.hash)}
                  onMouseLeave={handleRowLeave}
                >
                  <div className="git-graph-info">
                    <div style={{ display: 'flex', alignItems: 'center' }}>
                      <span className="git-graph-hash">{commit.hash.slice(0, 7)}</span>
                      <span className="git-graph-subject" style={{ flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{commit.subject}</span>
                      {(commit.refs || []).length > 0 && (
                        <span style={{ flexShrink: 0, whiteSpace: 'nowrap' }}>
                          {(commit.refs || []).map(ref => (
                            <span
                              key={ref}
                              className={`git-graph-ref ${isRemoteRef(ref) ? 'git-graph-ref-remote' : isTagRef(ref) ? 'git-graph-ref-tag' : 'git-graph-ref-head'}`}
                              data-remote={isRemoteRef(ref) ? ref.split('/')[0] : undefined}
                              onClick={e => e.stopPropagation()}
                            >
                              {isTagRef(ref) ? ref.replace('tag: ', '') : ref}
                            </span>
                          ))}
                        </span>
                      )}
                      <Tooltip>
                        <DropdownMenu open={kebabHash === commit.hash} onOpenChange={open => setKebabHash(open ? commit.hash : null)}>
                          <TooltipTrigger asChild>
                            <DropdownMenuTrigger asChild>
                              <Button
                                variant="ghost"
                                size="icon"
                                className="git-kebab-btn"
                                onClick={e => e.stopPropagation()}
                              >
                                <Ellipsis className="size-3.5" />
                              </Button>
                            </DropdownMenuTrigger>
                          </TooltipTrigger>
                          <TooltipContent>{t('git.actions')}</TooltipContent>
                          <DropdownMenuContent align="end" onClick={e => e.stopPropagation()}>
                          <DropdownMenuItem onClick={() => { setTagFormHash(commit.hash); setTagName(''); setKebabHash(null); setTimeout(() => tagInputRef.current?.focus(), 50) }}>
                            <Tag className="size-3.5" />
                            {t('git.createTag')}
                          </DropdownMenuItem>
                          <DropdownMenuItem onClick={() => { navigator.clipboard.writeText(commit.hash); showToast('success', t('git.hashCopied')); setKebabHash(null) }}>
                            <Copy className="size-3.5" />
                            {t('git.copyHash')}
                          </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      </Tooltip>
                    </div>
                    {isHovered && (
                      <div className="git-graph-meta">
                        <span className="git-graph-author">{commit.authorName}</span>
                        <span className="git-graph-sep">·</span>
                        <span className="git-graph-date">{commit.relDate}</span>
                        {hasFiles && (
                          <>
                            <span className="git-graph-sep">·</span>
                            <span className="git-graph-file-count">{files.length} file{files.length > 1 ? 's' : ''}</span>
                          </>
                        )}
                        {hasStats && (
                          <span className="git-graph-stats">
                            <span className="git-graph-stat-add">+{stats.insertions}</span>
                            <span className="git-graph-stat-del">-{stats.deletions}</span>
                          </span>
                        )}
                        {body && (
                          <div className="git-graph-body">{highlightBody(body)}</div>
                        )}
                      </div>
                    )}
                  </div>
                </div>
                {tagFormHash === commit.hash && (
                  <div className="git-tag-form" onClick={e => e.stopPropagation()}>
                    <input
                      ref={tagInputRef}
                      className="git-tag-input"
                      placeholder={t('git.tagPlaceholder')}
                      value={tagName}
                      onChange={e => setTagName(e.target.value)}
                      onKeyDown={e => { if (e.key === 'Enter') handleCreateTag(); if (e.key === 'Escape') { setTagFormHash(null); setTagName('') } }}
                      disabled={tagging}
                    />
                    {hasRemotes && (
                      <label className="git-tag-push-label">
                        <input type="checkbox" checked={tagPushAfter} onChange={e => setTagPushAfter(e.target.checked)} disabled={tagging} />
                        {t('git.push')}
                      </label>
                    )}
                    <Button variant="outline" size="sm" className="git-tag-submit" onClick={handleCreateTag} disabled={tagging || !tagName.trim()}>
                      {tagging ? '...' : 'Tag'}
                    </Button>
                    <Button variant="ghost" size="icon" className="git-tag-cancel" onClick={() => { setTagFormHash(null); setTagName('') }}>
                      <X className="size-3.5" />
                    </Button>
                  </div>
                )}
                {expanded && (
                  <div className="git-graph-files">
                    {files.map((f, fi) => (
                      <div key={fi} className={`git-graph-file git-graph-file-${f.status.toLowerCase()}`}>
                        <span className="git-graph-file-status">{f.status}</span>
                        <span className="git-graph-file-path">{f.path}</span>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )
          })}
          {isPaginating && isFetching && commits.length > 0 && (
            <div className="git-graph-loading">{t('git.loading')}</div>
          )}
        </div>
      </div>
    </div>
  )
}

// --- Inline mini terminal for host SSH troubleshooting ---

interface HostShellMiniProps {
  onClose: () => void
  onRetryPush: () => void
}

function HostShellMini({ onClose, onRetryPush }: HostShellMiniProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const termRef = useRef<XTerm | null>(null)
  const wsRef = useRef<WebSocket | null>(null)

  useEffect(() => {
    if (!containerRef.current) return

    const term = new XTerm({
      fontFamily: "'JetBrains Mono', 'Cascadia Code', monospace",
      fontSize: 12,
      theme: {
        background: '#161b22',
        foreground: '#e6edf3',
        cursor: '#e6edf3',
        selectionBackground: '#264f78',
      },
      cursorBlink: true,
      rows: 8,
    })
    termRef.current = term

    const fitAddon = new FitAddon()
    term.loadAddon(fitAddon)
    term.open(containerRef.current)

    requestAnimationFrame(() => {
      fitAddon.fit()
      term.focus()
    })

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const ws = new WebSocket(`${protocol}//${window.location.host}/ws/host-shell`)
    ws.binaryType = 'arraybuffer'
    wsRef.current = ws

    ws.onopen = () => {
      ws.send(JSON.stringify({ cols: term.cols, rows: term.rows }))
    }

    ws.onmessage = (event: MessageEvent) => {
      if (event.data instanceof ArrayBuffer) {
        term.write(new Uint8Array(event.data))
      } else {
        term.write(event.data as string)
      }
    }

    term.onData((data: string) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(new TextEncoder().encode(data))
      }
    })

    term.onResize(({ cols, rows }) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ cols, rows }))
      }
    })

    const resizeObserver = new ResizeObserver(() => {
      requestAnimationFrame(() => fitAddon.fit())
    })
    resizeObserver.observe(containerRef.current)

    return () => {
      resizeObserver.disconnect()
      if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) {
        ws.close()
      }
      term.dispose()
      termRef.current = null
      wsRef.current = null
    }
  }, [])

  return (
    <div className="git-host-shell">
      <div className="git-host-shell-header">
        <span className="git-host-shell-title">Host Terminal</span>
        <span className="git-host-shell-hint">ssh-add</span>
        <div style={{ display: 'flex', gap: '4px' }}>
          <Button variant="outline" size="sm" onClick={onRetryPush}>
            <ArrowUpToLine className="size-3.5" /> Retry
          </Button>
          <Button variant="ghost" size="icon" onClick={onClose}>
            <X className="size-3.5" />
          </Button>
        </div>
      </div>
      <div className="git-host-shell-term" ref={containerRef} />
    </div>
  )
}
