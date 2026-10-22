import { useQuery } from '@tanstack/react-query'
import {
  fetchModelConfig,
  fetchEpicGitConfig,
  fetchEpicGitSyncStatus,
  fetchFirewallConfig,
  fetchDigestConfig,
  fetchRecoveryConfig,
} from '../api'
import type {
  ModelConfig,
  EpicGitConfig,
  EpicGitSyncStatus,
  FirewallConfig,
  DigestConfig,
  RecoveryConfig,
} from '../api'

export const settingsKeys = {
  all: ['settings'] as const,
  models: ['settings', 'models'] as const,
  epicGit: ['settings', 'epic-git'] as const,
  epicGitStatus: ['settings', 'epic-git-status'] as const,
  firewall: ['settings', 'firewall'] as const,
  digest: ['settings', 'digest'] as const,
  recovery: ['settings', 'recovery'] as const,
}

export function useModelConfig(enabled: boolean) {
  return useQuery<ModelConfig>({
    queryKey: settingsKeys.models,
    queryFn: () => fetchModelConfig(),
    enabled,
  })
}

export function useEpicGitConfig(enabled: boolean) {
  return useQuery<EpicGitConfig>({
    queryKey: settingsKeys.epicGit,
    queryFn: () => fetchEpicGitConfig(),
    enabled,
  })
}

export function useEpicGitSyncStatus(enabled: boolean) {
  return useQuery<EpicGitSyncStatus>({
    queryKey: settingsKeys.epicGitStatus,
    queryFn: () => fetchEpicGitSyncStatus(),
    enabled,
    refetchInterval: 3000,
  })
}

export function useFirewallConfig(enabled: boolean) {
  return useQuery<FirewallConfig>({
    queryKey: settingsKeys.firewall,
    queryFn: () => fetchFirewallConfig(),
    enabled,
  })
}

export function useDigestConfig(enabled: boolean) {
  return useQuery<DigestConfig>({
    queryKey: settingsKeys.digest,
    queryFn: () => fetchDigestConfig(),
    enabled,
  })
}

export function useRecoveryConfig(enabled: boolean) {
  return useQuery<RecoveryConfig>({
    queryKey: settingsKeys.recovery,
    queryFn: () => fetchRecoveryConfig(),
    enabled,
  })
}
