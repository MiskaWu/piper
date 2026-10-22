import { useState, useCallback, useMemo } from 'react'
import { useFilesTree } from '../hooks/useFilesTree'
import { copyFiles, moveFiles } from '../api'
import FileContextMenu from './FileContextMenu'
import type { FileEntry, FileClipboard } from '../types'

interface FilesTreeProps {
  projectPath: string
  gitAvailable: boolean
  clipboard?: FileClipboard | null
  onCopy?: (paths: string[]) => void
  onCut?: (paths: string[]) => void
  onClearClipboard?: () => void
}

const STATUS_COLORS: Record<string, string> = {
  M: '#d29922',
  A: '#3fb950',
  D: '#f85149',
  '?': '#8b949e',
  R: '#58a6ff',
}

interface ContextMenuState {
  x: number
  y: number
  entry: FileEntry | null
  targetDir: string | null
}

export default function FilesTree({
  projectPath, gitAvailable, clipboard, onCopy, onCut, onClearClipboard,
}: FilesTreeProps) {
  const { entries, expandedDirs, statusMap, loading, toggleDir, refresh } = useFilesTree(projectPath, gitAvailable)
  const [selectedPath, setSelectedPath] = useState<string | null>(null)
  const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(null)
  const [pasting, setPasting] = useState(false)

  const cutSourceSet = useMemo(() =>
    clipboard?.operation === 'cut' ? new Set(clipboard.sources) : null,
  [clipboard])

  const openContextMenu = useCallback((e: React.MouseEvent, entry: FileEntry | null, targetDir: string | null) => {
    e.preventDefault()
    e.stopPropagation()
    if (entry) setSelectedPath(entry.path)
    setContextMenu({ x: e.clientX, y: e.clientY, entry, targetDir })
  }, [])

  const handlePaste = useCallback(async (targetDir: string) => {
    if (!clipboard) return
    setPasting(true)
    try {
      if (clipboard.operation === 'copy') {
        await copyFiles(clipboard.sources, targetDir)
      } else {
        await moveFiles(clipboard.sources, targetDir)
      }
      onClearClipboard?.()
      refresh()
      window.dispatchEvent(new CustomEvent('git_changed', { detail: { path: projectPath } }))
    } catch (err) {
      console.error('Paste failed:', err)
    } finally {
      setPasting(false)
    }
  }, [clipboard, onClearClipboard, refresh, projectPath])

  const lookupEntry = useCallback((targetPath: string): FileEntry | null => {
    const parentDir = targetPath.substring(0, targetPath.lastIndexOf('/')) || projectPath
    const items = entries.get(parentDir)
    return items?.find(e => e.path === targetPath) ?? null
  }, [entries, projectPath])

  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (!selectedPath) return

    if ((e.ctrlKey || e.metaKey) && e.key === 'c') {
      e.preventDefault()
      onCopy?.([selectedPath])
    } else if ((e.ctrlKey || e.metaKey) && e.key === 'x') {
      e.preventDefault()
      onCut?.([selectedPath])
    } else if ((e.ctrlKey || e.metaKey) && e.key === 'v') {
      e.preventDefault()
      if (!clipboard) return
      const selected = lookupEntry(selectedPath)
      const targetDir = selected?.isDir ? selectedPath : projectPath
      handlePaste(targetDir)
    }
  }, [selectedPath, clipboard, onCopy, onCut, handlePaste, lookupEntry, projectPath])

  const renderEntries = (dirPath: string, depth: number) => {
    const items = entries.get(dirPath)
    if (!items) return null

    return items.map((entry: FileEntry) => {
      const isExpanded = expandedDirs.has(entry.path)
      const isLoading = loading.has(entry.path)
      const relPath = entry.path.startsWith(projectPath + '/')
        ? entry.path.slice(projectPath.length + 1)
        : entry.name
      const status = statusMap.get(relPath)
      const isSelected = selectedPath === entry.path
      const isCut = cutSourceSet?.has(entry.path) ?? false

      let dirHasChanges = false
      if (entry.isDir && statusMap.size > 0) {
        const prefix = relPath + '/'
        for (const key of statusMap.keys()) {
          if (key.startsWith(prefix)) {
            dirHasChanges = true
            break
          }
        }
      }

      return (
        <div key={entry.path}>
          <div
            className={`ft-row${entry.isDir ? ' ft-dir' : ' ft-file'}${isSelected ? ' ft-selected' : ''}${isCut ? ' ft-cut' : ''}`}
            style={{ paddingLeft: 8 + depth * 16 }}
            onClick={(e) => {
              e.stopPropagation()
              setSelectedPath(entry.path)
              if (entry.isDir) toggleDir(entry.path)
            }}
            onContextMenu={(e) => openContextMenu(e, entry, entry.isDir ? entry.path : null)}
          >
            <span className="ft-icon">
              {entry.isDir ? (isLoading ? '\u23F3' : isExpanded ? '\u25BE' : '\u25B8') : ''}
            </span>
            <span className="ft-name">{entry.name}</span>
            {status && (
              <span
                className="ft-status"
                style={{ color: STATUS_COLORS[status] || '#8b949e' }}
              >
                {status}
              </span>
            )}
            {!status && dirHasChanges && (
              <span className="ft-status ft-status-dot" style={{ color: '#d29922' }}>{'\u25CF'}</span>
            )}
          </div>
          {entry.isDir && isExpanded && (
            <div onContextMenu={(e) => openContextMenu(e, null, entry.path)}>
              {renderEntries(entry.path, depth + 1)}
            </div>
          )}
        </div>
      )
    })
  }

  const rootEntries = entries.get(projectPath)
  if (!rootEntries) {
    return <div className="ft-empty">Loading...</div>
  }

  if (rootEntries.length === 0) {
    return <div className="ft-empty">Empty directory</div>
  }

  return (
    <div
      className="ft-tree"
      tabIndex={0}
      onKeyDown={handleKeyDown}
      onContextMenu={(e) => openContextMenu(e, null, projectPath)}
    >
      {renderEntries(projectPath, 0)}
      {clipboard && (
        <div className="ft-clipboard-hint">
          {clipboard.operation === 'cut' ? 'Cut' : 'Copied'}: {clipboard.sources.length} item{clipboard.sources.length > 1 ? 's' : ''}
        </div>
      )}
      {pasting && (
        <div className="ft-clipboard-hint">Pasting...</div>
      )}
      {contextMenu && (
        <FileContextMenu
          x={contextMenu.x}
          y={contextMenu.y}
          entry={contextMenu.entry}
          targetDir={contextMenu.targetDir}
          clipboard={clipboard ?? null}
          onCopy={() => {
            if (contextMenu.entry) onCopy?.([contextMenu.entry.path])
            setContextMenu(null)
          }}
          onCut={() => {
            if (contextMenu.entry) onCut?.([contextMenu.entry.path])
            setContextMenu(null)
          }}
          onPaste={() => {
            if (contextMenu.targetDir) handlePaste(contextMenu.targetDir)
            setContextMenu(null)
          }}
          onClose={() => setContextMenu(null)}
        />
      )}
    </div>
  )
}
