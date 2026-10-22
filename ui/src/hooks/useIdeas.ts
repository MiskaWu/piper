import { useQuery } from '@tanstack/react-query'
import { fetchIdeas } from '../api'

export const ideasKeys = {
  all: ['ideas'] as const,
}

export function useIdeas() {
  return useQuery({
    queryKey: ideasKeys.all,
    queryFn: fetchIdeas,
  })
}
