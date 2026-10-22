import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { getEpicClaudeVersion, startEpicClaudeUpdate } from '../../api'
import type { EpicClaudeVersionResult } from '../../api'

export const epicClaudeKeys = {
  version: ['epic', 'claude-version'] as const,
}

/** 查詢 epic image 的 Claude Code 版本狀態。更新中時加速輪詢，否則 60s 輪詢一次 */
export function useEpicClaudeVersion() {
  return useQuery<EpicClaudeVersionResult>({
    queryKey: epicClaudeKeys.version,
    queryFn: () => getEpicClaudeVersion(),
    refetchInterval: (query) => {
      const data = query.state.data as EpicClaudeVersionResult | undefined
      return data?.update?.inProgress ? 2_000 : 60_000
    },
  })
}

/** 觸發背景 rebuild。成功即 invalidate，交由 refetchInterval 輪詢狀態 */
export function useStartEpicClaudeUpdate() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => startEpicClaudeUpdate(),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: epicClaudeKeys.version })
    },
  })
}
