import type { ResolvedTheme } from '../types/theme'

interface ColorPair {
  dark: string
  light: string
}

const PALETTE: Record<string, ColorPair> = {
  blue: { dark: '#53a8e2', light: '#2563eb' },
  purple: { dark: '#a78bfa', light: '#7c3aed' },
  gray: { dark: '#6b7280', light: '#6b7280' },
  green: { dark: '#22c55e', light: '#16a34a' },
  amber: { dark: '#f59e0b', light: '#d97706' },
  rose: { dark: '#f43f5e', light: '#e11d48' },
  cyan: { dark: '#22d3ee', light: '#0891b2' },
  orange: { dark: '#f97316', light: '#ea580c' },
}

export const PALETTE_NAMES = Object.keys(PALETTE)

const FALLBACK: ColorPair = PALETTE.gray

export function resolveReleaseColor(colorName: string, theme: ResolvedTheme): string {
  const pair = PALETTE[colorName] ?? FALLBACK
  return pair[theme]
}

export function nextAvailableColor(usedColors: string[]): string {
  const available = PALETTE_NAMES.find((name) => !usedColors.includes(name))
  return available ?? PALETTE_NAMES[0]
}
