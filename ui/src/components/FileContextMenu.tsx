import { useEffect, useRef, useState } from 'react'
import type { FileEntry, FileClipboard } from '../types'

interface FileContextMenuProps {
  x: number
  y: number
  entry: FileEntry | null
  targetDir: string | null
  clipboard: FileClipboard | null
  onCopy: () => void
  onCut: () => void
  onPaste: () => void
  onClose: () => void
}

export default function FileContextMenu({
  x, y, entry, targetDir, clipboard, onCopy, onCut, onPaste, onClose,
}: FileContextMenuProps) {
  const menuRef = useRef<HTMLDivElement>(null)
  const [pos, setPos] = useState({ left: x, top: y })

  useEffect(() => {
    const handleClick = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        onClose()
      }
    }
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('mousedown', handleClick)
    document.addEventListener('keydown', handleKey)
    return () => {
      document.removeEventListener('mousedown', handleClick)
      document.removeEventListener('keydown', handleKey)
    }
  }, [onClose])

  useEffect(() => {
    if (!menuRef.current) return
    const rect = menuRef.current.getBoundingClientRect()
    setPos({
      left: rect.right > window.innerWidth ? x - rect.width : x,
      top: rect.bottom > window.innerHeight ? y - rect.height : y,
    })
  }, [x, y])

  const showCopyCut = !!entry
  const showPaste = !!clipboard && !!targetDir

  if (!showCopyCut && !showPaste) return null

  return (
    <div
      ref={menuRef}
      className="ft-context-menu"
      style={pos}
    >
      {showCopyCut && (
        <>
          <div className="ft-context-item" onClick={onCopy}>
            Copy
          </div>
          <div className="ft-context-item" onClick={onCut}>
            Cut
          </div>
        </>
      )}
      {showCopyCut && showPaste && <div className="ft-context-separator" />}
      {showPaste && (
        <div className="ft-context-item" onClick={onPaste}>
          Paste
        </div>
      )}
    </div>
  )
}
