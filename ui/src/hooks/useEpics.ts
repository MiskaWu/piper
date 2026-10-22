import { useQuery } from '@tanstack/react-query'
import { fetchEpics, fetchGitAvailable } from '../api'
import type { EpicInfo } from '../types'

export const epicsKeys = {
  all: ['epics'] as const,
  list: ['epics', 'list'] as const,
  gitAvailable: ['epics', 'gitAvailable'] as const,
}

export function useEpics() {
  return useQuery<EpicInfo[]>({
    queryKey: epicsKeys.list,
    queryFn: () => fetchEpics(),
  })
}

export function useGitAvailable() {
  return useQuery<{ available: boolean }>({
    queryKey: epicsKeys.gitAvailable,
    queryFn: () => fetchGitAvailable(),
  })
}
