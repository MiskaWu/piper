import { useQuery, useQueryClient } from '@tanstack/react-query'
import { fetchRecoverableSessions, restoreSession, dismissSession } from '../api'
import type { RecoverableSession } from '../api'

export const recoverableKeys = { all: ['recoverable-sessions'] as const }

export function useRecoverableSessions() {
  const queryClient = useQueryClient()
  const query = useQuery<RecoverableSession[]>({
    queryKey: recoverableKeys.all,
    queryFn: fetchRecoverableSessions,
    refetchInterval: 30_000,
  })

  const restore = async (name: string) => {
    const result = await restoreSession(name)
    queryClient.invalidateQueries({ queryKey: recoverableKeys.all })
    return result
  }

  const dismiss = async (name: string) => {
    await dismissSession(name)
    queryClient.invalidateQueries({ queryKey: recoverableKeys.all })
  }

  const restoreAll = async (names: string[]) => {
    const results = await Promise.allSettled(names.map(n => restoreSession(n)))
    queryClient.invalidateQueries({ queryKey: recoverableKeys.all })
    return results
      .filter((r): r is PromiseFulfilledResult<{ status: string; name: string }> => r.status === 'fulfilled')
      .map(r => r.value.name)
  }

  const refetchNow = () => queryClient.invalidateQueries({ queryKey: recoverableKeys.all })

  return { ...query, restore, restoreAll, dismiss, refetchNow }
}
