import { useQuery } from '@tanstack/react-query'
import { fetchDashboardStats, fetchMousedex } from '../api'
import type { DashboardStats, MousedexData } from '../types'

export const dashboardKeys = {
  all: ['dashboard'] as const,
  stats: (year: number, month: number) => ['dashboard', 'stats', year, month] as const,
  mousedex: ['dashboard', 'mousedex'] as const,
}

export function useDashboardStats(year: number, month: number) {
  return useQuery<DashboardStats>({
    queryKey: dashboardKeys.stats(year, month),
    queryFn: () => fetchDashboardStats(year, month),
  })
}

export function useMousedex() {
  return useQuery<MousedexData>({
    queryKey: dashboardKeys.mousedex,
    queryFn: () => fetchMousedex(),
  })
}
