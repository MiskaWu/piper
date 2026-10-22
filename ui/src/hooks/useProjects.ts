import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { listProjects, browseDirectory, addProject, removeProject, rebuildProject, upgradeProject, updateProjectClaude } from '../api'
import type { RemoveProjectResult, UpgradeResult, UpdateClaudeResult } from '../api'
import type { BrowseResult, Project } from '../types'

export const projectsKeys = {
  all: ['projects'] as const,
  list: ['projects', 'list'] as const,
  browse: (path: string) => ['projects', 'browse', path] as const,
}

// listProjects 回傳的實際結構（API 回傳包含 devcontainerCLI 欄位）
interface ListProjectsResponse {
  projects: Project[]
  devcontainerCLI: boolean
}

export function useProjects() {
  return useQuery<ListProjectsResponse>({
    queryKey: projectsKeys.list,
    queryFn: () => listProjects() as Promise<unknown> as Promise<ListProjectsResponse>,
  })
}

export function useBrowseDirectory(path: string) {
  return useQuery<BrowseResult>({
    queryKey: projectsKeys.browse(path),
    queryFn: () => browseDirectory(path || ''),
  })
}

export function useAddProject() {
  const queryClient = useQueryClient()
  return useMutation<Project, Error, string>({
    mutationFn: (path: string) => addProject(path),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: projectsKeys.list })
    },
  })
}

export function useRemoveProject() {
  const queryClient = useQueryClient()
  return useMutation<RemoveProjectResult, Error, { name: string; cleanup?: { containers: boolean; images: boolean } }>({
    mutationFn: ({ name, cleanup }) => removeProject(name, cleanup),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: projectsKeys.list })
    },
  })
}

export function useRebuildProject() {
  const queryClient = useQueryClient()
  return useMutation<void, Error, string>({
    mutationFn: (name: string) => rebuildProject(name) as Promise<unknown> as Promise<void>,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: projectsKeys.list })
    },
  })
}

export function useUpgradeProject() {
  const queryClient = useQueryClient()
  return useMutation<UpgradeResult, Error, string>({
    mutationFn: (name: string) => upgradeProject(name),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: projectsKeys.list })
    },
  })
}

export function useUpdateProjectClaude() {
  const queryClient = useQueryClient()
  return useMutation<UpdateClaudeResult, Error, string>({
    mutationFn: (name: string) => updateProjectClaude(name),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: projectsKeys.list })
    },
  })
}
