import { useState, useCallback } from 'react'
import type { FileClipboard } from '../types'

export interface FileClipboardActions {
  clipboard: FileClipboard | null
  setCopy: (paths: string[]) => void
  setCut: (paths: string[]) => void
  clear: () => void
}

export function useFileClipboard(): FileClipboardActions {
  const [clipboard, setClipboard] = useState<FileClipboard | null>(null)

  const setCopy = useCallback((paths: string[]) => {
    setClipboard({ sources: paths, operation: 'copy' })
  }, [])

  const setCut = useCallback((paths: string[]) => {
    setClipboard({ sources: paths, operation: 'cut' })
  }, [])

  const clear = useCallback(() => {
    setClipboard(null)
  }, [])

  return { clipboard, setCopy, setCut, clear }
}
