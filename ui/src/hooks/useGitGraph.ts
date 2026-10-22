import { useEffect } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { fetchGitGraph, fetchGitBranches } from '../api'
import type { GitGraphResult, GitBranchListResult } from '../types'

const BATCH_SIZE = 15

export const gitGraphKeys = {
  graph: (path: string, limit: number, ref?: string, hideRemotes?: string[]) =>
    ['git-graph', path, limit, ref ?? '', (hideRemotes || []).join(',')] as const,
  branches: (path: string) => ['git-branches', path] as const,
}

export function useGitGraph(path: string, limit: number = BATCH_SIZE, ref?: string, hideRemotes?: string[]) {
  const queryClient = useQueryClient()

  useEffect(() => {
    const handler = (e: Event) => {
      const detail = (e as CustomEvent).detail
      if (detail?.path === path) {
        queryClient.invalidateQueries({ queryKey: ['git-graph', path] })
        queryClient.invalidateQueries({ queryKey: ['git-branches', path] })
      }
    }
    window.addEventListener('git_changed', handler)
    return () => window.removeEventListener('git_changed', handler)
  }, [path, queryClient])

  return useQuery<GitGraphResult>({
    queryKey: gitGraphKeys.graph(path, limit, ref, hideRemotes),
    queryFn: () => fetchGitGraph(path, limit, ref, hideRemotes),
    enabled: !!path,
    placeholderData: (prev) => prev,
    staleTime: 10_000,
  })
}

export function useGitBranches(path: string, enabled: boolean = true) {
  return useQuery<GitBranchListResult>({
    queryKey: gitGraphKeys.branches(path),
    queryFn: () => fetchGitBranches(path),
    enabled: !!path && enabled,
    staleTime: 10_000,
  })
}

export { BATCH_SIZE }
