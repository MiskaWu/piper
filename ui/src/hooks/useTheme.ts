import { useState, useEffect } from 'react'

type Theme = 'dark' | 'light'

interface UseThemeReturn {
  theme: Theme
  toggle: () => void
}

export default function useTheme(): UseThemeReturn {
  const [theme, setTheme] = useState<Theme>(() => (localStorage.getItem('theme') as Theme) || 'dark')

  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme)
    localStorage.setItem('theme', theme)
  }, [theme])

  const toggle = () => setTheme(t => t === 'dark' ? 'light' : 'dark')
  return { theme, toggle }
}
