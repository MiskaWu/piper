import { useQuery } from '@tanstack/react-query'
import {
  getPlanSessionStatus,
  getDispatchSessionStatus,
  getRetroSessionStatus,
  getInfraStatus,
  getRetroTrends,
  listRetroActions,
  getRetroReportMarkdown,
} from '../../api'
import { loadStoryMap } from '../lib/yaml-loader'

export const planKeys = {
  all: ['plan'] as const,
  session: (epicName: string) => ['plan', 'session', epicName] as const,
  storyMap: (epicName: string) => ['plan', 'story-map', epicName] as const,
  infraStatus: (epicName: string) => ['plan', 'infra-status', epicName] as const,
  retroTrends: (epicName: string) => ['plan', 'retro-trends', epicName] as const,
  retroActions: (epicName: string) => ['plan', 'retro-actions', epicName] as const,
  retroReport: (epicName: string, release: string) =>
    ['plan', 'retro-report', epicName, release] as const,
}

/** 查詢 plan / dispatch / retro session 狀態 */
export function usePlanSession(epicName: string) {
  return useQuery({
    queryKey: planKeys.session(epicName),
    queryFn: async () => {
      const [plan, dispatch, retro] = await Promise.all([
        getPlanSessionStatus(epicName).catch(() => ({ exists: false, session: '' })),
        getDispatchSessionStatus(epicName).catch(() => ({ exists: false, session: '' })),
        getRetroSessionStatus(epicName).catch(() => ({ exists: false, session: '' })),
      ])
      return {
        planSession: plan.exists ? plan.session : null,
        dispatchSession: dispatch.exists ? dispatch.session : null,
        retroSession: retro.exists ? retro.session : null,
      }
    },
    enabled: !!epicName,
  })
}

/** 查詢各目標專案的基礎設施版本狀態 */
export function useInfraStatus(epicName: string) {
  return useQuery({
    queryKey: planKeys.infraStatus(epicName),
    queryFn: () => getInfraStatus(epicName),
    enabled: !!epicName,
    refetchInterval: 30_000,
  })
}

/** 查詢 story map 資料 */
export function useStoryMap(epicName: string) {
  return useQuery({
    queryKey: planKeys.storyMap(epicName),
    queryFn: () => loadStoryMap(epicName),
    enabled: !!epicName,
  })
}

/** 跨 release retro 趨勢 */
export function useRetroTrends(epicName: string, enabled = true) {
  return useQuery({
    queryKey: planKeys.retroTrends(epicName),
    queryFn: () => getRetroTrends(epicName),
    enabled: enabled && !!epicName,
    staleTime: 30_000,
  })
}

/** Retro actions（可修改） */
export function useRetroActions(epicName: string, enabled = true) {
  return useQuery({
    queryKey: planKeys.retroActions(epicName),
    queryFn: () => listRetroActions(epicName),
    enabled: enabled && !!epicName,
  })
}

/** 單份 retro markdown 報告（immutable snapshot：一旦產出就不會變，
 *  除非使用者 force rerun；需要更新時呼叫方 invalidateQueries）。 */
export function useRetroReport(epicName: string, release: string, enabled = true) {
  return useQuery({
    queryKey: planKeys.retroReport(epicName, release),
    queryFn: () => getRetroReportMarkdown(epicName, release),
    enabled: enabled && !!epicName && !!release,
    staleTime: Infinity,
  })
}
