export function formatTimeUntil(isoString: string | undefined): string | null {
  if (!isoString) return null
  const now = new Date()
  const reset = new Date(isoString)
  const diffMs = reset.getTime() - now.getTime()
  if (diffMs <= 0) return null

  const hours = Math.floor(diffMs / (1000 * 60 * 60))
  const minutes = Math.floor((diffMs % (1000 * 60 * 60)) / (1000 * 60))

  if (hours > 0) return `${hours}h${minutes > 0 ? `${minutes}m` : ''}`
  return `${minutes}m`
}
