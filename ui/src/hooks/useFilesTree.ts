import { useState, useEffect, useCallback, useMemo } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { browseFiles, fetchGitFileStatus } from '../api'
import type { FileEntry, GitFileStatus } from '../types'

export interface FilesTreeState {
  entries: Map<string, FileEntry[]>  // dir path -> entries
  expandedDirs: Set<string>
  statusMap: Map<string, string>     // relative path -> status
  loading: Set<string>
  toggleDir: (path: string) => void
  refresh: () => void
}

export function useFilesTree(projectPath: string, gitAvailable: boolean): FilesTreeState {
  const queryClient = useQueryClient()
  const [expandedDirs, setExpandedDirs] = useState<Set<string>>(new Set())
  const [entries, setEntries] = useState<Map<string, FileEntry[]>>(new Map())
  const [loading, setLoading] = useState<Set<string>>(new Set())

  // Fetch root directory
  const { data: rootData } = useQuery({
    queryKey: ['files-tree', projectPath],
    queryFn: () => browseFiles(projectPath),
    enabled: !!projectPath,
  })

  // Fetch git file statuses
  const { data: gitStatuses } = useQuery<GitFileStatus[]>({
    queryKey: ['git-file-status', projectPath],
    queryFn: () => fetchGitFileStatus(projectPath),
    enabled: !!projectPath && gitAvailable,
  })

  // Listen for git_changed events to refresh statuses
  useEffect(() => {
    const handler = (e: Event) => {
      const detail = (e as CustomEvent).detail
      if (detail?.path === projectPath) {
        queryClient.invalidateQueries({ queryKey: ['git-file-status', projectPath] })
        queryClient.invalidateQueries({ queryKey: ['files-tree', projectPath] })
      }
    }
    window.addEventListener('git_changed', handler)
    return () => window.removeEventListener('git_changed', handler)
  }, [projectPath, queryClient])

  // Store root entries when they arrive
  useEffect(() => {
    if (rootData?.entries) {
      setEntries(prev => {
        const next = new Map(prev)
        next.set(projectPath, rootData.entries)
        return next
      })
    }
  }, [rootData, projectPath])

  // Build status map from git statuses
  const statusMap = useMemo(() => {
    const map = new Map<string, string>()
    if (gitStatuses) {
      for (const s of gitStatuses) {
        map.set(s.path, s.status)
      }
    }
    return map
  }, [gitStatuses])

  const toggleDir = useCallback(async (path: string) => {
    setExpandedDirs(prev => {
      const next = new Set(prev)
      if (next.has(path)) {
        next.delete(path)
      } else {
        next.add(path)
      }
      return next
    })

    // Lazy fetch if not already loaded
    if (!entries.has(path)) {
      setLoading(prev => new Set(prev).add(path))
      try {
        const result = await browseFiles(path)
        setEntries(prev => {
          const next = new Map(prev)
          next.set(path, result.entries)
          return next
        })
      } catch (err) {
        console.error('Failed to load directory:', err)
      } finally {
        setLoading(prev => {
          const next = new Set(prev)
          next.delete(path)
          return next
        })
      }
    }
  }, [entries])

  const refresh = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ['files-tree', projectPath] })
    queryClient.invalidateQueries({ queryKey: ['git-file-status', projectPath] })
    // Clear cached subdirectories to force re-fetch
    setEntries(new Map())
    setExpandedDirs(new Set())
  }, [projectPath, queryClient])

  return { entries, expandedDirs, statusMap, loading, toggleDir, refresh }
}
