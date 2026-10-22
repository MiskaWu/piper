import React, { useEffect, useRef, useState, useCallback, forwardRef, useImperativeHandle } from 'react'
import { useTranslation } from 'react-i18next'
import { uploadClipboardImage, uploadClipboardFile } from '../api'
import SidePanel from './SidePanel'
import type { FileClipboard } from '../types'
import { Terminal as XTerm } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import { WebglAddon } from '@xterm/addon-webgl'
import type { ITheme } from '@xterm/xterm'
import '@xterm/xterm/css/xterm.css'

const DARK_THEME: ITheme = {
  background: '#0d1117',
  foreground: '#e6edf3',
  cursor: '#e6edf3',
  selectionBackground: '#264f78',
  black: '#484f58',
  red: '#ff7b72',
  green: '#3fb950',
  yellow: '#d29922',
  blue: '#58a6ff',
  magenta: '#bc8cff',
  cyan: '#39c5cf',
  white: '#b1bac4',
  brightBlack: '#6e7681',
  brightRed: '#ffa198',
  brightGreen: '#56d364',
  brightYellow: '#e3b341',
  brightBlue: '#79c0ff',
  brightMagenta: '#d2a8ff',
  brightCyan: '#56d4dd',
  brightWhite: '#f0f6fc',
}

const LIGHT_THEME: ITheme = {
  background: '#ffffff',
  foreground: '#1f2328',
  cursor: '#1f2328',
  selectionBackground: '#add6ff',
  black: '#24292f',
  red: '#cf222e',
  green: '#1a7f37',
  yellow: '#9a6700',
  blue: '#0969da',
  magenta: '#8250df',
  cyan: '#1b7c83',
  white: '#6e7781',
  brightBlack: '#57606a',
  brightRed: '#a40e26',
  brightGreen: '#2da44e',
  brightYellow: '#bf8700',
  brightBlue: '#218bff',
  brightMagenta: '#a475f9',
  brightCyan: '#3192aa',
  brightWhite: '#8c959f',
}

const RECONNECT_BASE_DELAY = 1000
const RECONNECT_MAX_DELAY = 10000
const RECONNECT_MAX_ATTEMPTS = 10
// Don't show reconnect messages until this many attempts — hides brief blips
const RECONNECT_QUIET_THRESHOLD = 2

export interface TerminalHandle {
  focus: () => void
}

interface TerminalProps {
  sessionName: string
  theme: 'dark' | 'light'
  projectPath?: string
  gitAvailable?: boolean
  clipboard?: FileClipboard | null
  onCopy?: (paths: string[]) => void
  onCut?: (paths: string[]) => void
  onClearClipboard?: () => void
}

interface ToastState {
  url: string
  path: string
  isFile?: boolean
}

export default forwardRef<TerminalHandle, TerminalProps>(function Terminal({ sessionName, theme, projectPath, gitAvailable, clipboard, onCopy, onCut, onClearClipboard }, ref) {
  const containerRef = useRef<HTMLDivElement>(null)
  const termRef = useRef<XTerm | null>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const fitAddonRef = useRef<FitAddon | null>(null)
  const { t } = useTranslation()
  const tRef = useRef(t)
  tRef.current = t
  // Generation counter: each effect run increments, stale callbacks check before acting
  const generationRef = useRef(0)
  const inputBufferRef = useRef<string[]>([])
  const sendOrBufferRef = useRef<((data: string) => void) | null>(null)
  const [toast, setToast] = useState<ToastState | null>(null)
  const [sidePanel, setSidePanel] = useState(true)
  const toastTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useImperativeHandle(ref, () => ({
    focus: () => termRef.current?.focus(),
  }))

  const showImageToast = useCallback((path: string, blob: Blob) => {
    if (toastTimerRef.current) clearTimeout(toastTimerRef.current)
    const url = URL.createObjectURL(blob)
    setToast({ url, path })
    toastTimerRef.current = setTimeout(() => {
      setToast(null)
      URL.revokeObjectURL(url)
      toastTimerRef.current = null
    }, 4000)
  }, [])

  const showFileToast = useCallback((path: string) => {
    if (toastTimerRef.current) clearTimeout(toastTimerRef.current)
    setToast({ url: '', path, isFile: true })
    toastTimerRef.current = setTimeout(() => {
      setToast(null)
      toastTimerRef.current = null
    }, 4000)
  }, [])

  useEffect(() => {
    if (!containerRef.current) return

    // --- Ownership: local cancelled + generation ID ---
    let cancelled = false
    const gen = ++generationRef.current
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null
    let reconnectAttempt = 0

    function isStale(): boolean {
      return cancelled || generationRef.current !== gen
    }

    const termTheme = theme === 'dark' ? DARK_THEME : LIGHT_THEME
    const term = new XTerm({
      fontFamily: "'JetBrains Mono', 'Cascadia Code', 'Fira Code', 'Consolas', monospace",
      fontSize: 14,
      theme: termTheme,
      cursorBlink: true,
      customGlyphs: true,
      allowProposedApi: true,
      rightClickSelectsWord: true,
    })

    // Helper: send data to WS or buffer if disconnected
    function sendOrBuffer(data: string) {
      const ws = wsRef.current
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(new TextEncoder().encode(data))
      } else {
        inputBufferRef.current.push(data)
      }
    }
    sendOrBufferRef.current = sendOrBuffer

    // Flush buffered input after reconnect
    function flushBuffer() {
      const ws = wsRef.current
      if (!ws || ws.readyState !== WebSocket.OPEN) return
      const encoder = new TextEncoder()
      const items = inputBufferRef.current.splice(0)
      for (const data of items) {
        ws.send(encoder.encode(data))
      }
    }

    // Allow Ctrl+C to copy when text is selected, Ctrl+V to paste
    term.attachCustomKeyEventHandler((event: KeyboardEvent) => {
      // Ctrl+Shift+C / Cmd+C -> always copy
      if ((event.ctrlKey && event.shiftKey && event.key === 'C') ||
          (event.metaKey && event.key === 'c')) {
        const sel = term.getSelection()
        if (sel) {
          navigator.clipboard.writeText(sel)
          term.clearSelection()
        }
        return false
      }
      // Ctrl+C with selection -> copy instead of SIGINT
      if (event.ctrlKey && !event.shiftKey && event.key === 'c' && term.hasSelection()) {
        navigator.clipboard.writeText(term.getSelection())
        term.clearSelection()
        return false
      }
      // Ctrl+V / Ctrl+Shift+V / Cmd+V / Shift+Insert -> paste from clipboard
      if ((event.ctrlKey && event.key === 'v') ||
          (event.ctrlKey && event.shiftKey && event.key === 'V') ||
          (event.metaKey && event.key === 'v') ||
          (event.shiftKey && event.key === 'Insert')) {
        if (event.type === 'keydown') {
          // Prevent browser from also firing a paste event (which would cause
          // xterm's internal handler -> onData -> double send)
          event.preventDefault()
          // Try clipboard.read() to detect images; text always uses readText()
          ;(async () => {
            if (navigator.clipboard.read) {
              try {
                const items = await navigator.clipboard.read()
                for (const item of items) {
                  const imageType = item.types.find((t: string) => t.startsWith('image/'))
                  if (imageType) {
                    const blob = await item.getType(imageType)
                    const { path } = await uploadClipboardImage(blob)
                    sendOrBuffer(path + ' ')
                    showImageToast(path, blob)
                    return
                  }
                }
              } catch {
                // clipboard.read() not supported or denied — fall through to text
              }
            }
            // Text paste via readText() — more reliable than clipboard.read()
            try {
              const text = await navigator.clipboard.readText()
              if (text) sendOrBuffer(text)
            } catch {
              // clipboard not available
            }
          })()
        }
        return false
      }
      // Shift+Enter -> send newline (without Ctrl)
      if (event.shiftKey && !event.ctrlKey && !event.metaKey && event.key === 'Enter') {
        if (event.type === 'keydown') {
          event.preventDefault()
          sendOrBuffer('\n')
        }
        return false
      }
      return true
    })
    termRef.current = term

    const fitAddon = new FitAddon()
    fitAddonRef.current = fitAddon
    term.loadAddon(fitAddon)
    term.loadAddon(new WebLinksAddon())

    term.open(containerRef.current)

    // Load WebGL renderer for pixel-perfect box-drawing characters
    try {
      const webglAddon = new WebglAddon()
      webglAddon.onContextLoss(() => {
        webglAddon.dispose()
      })
      term.loadAddon(webglAddon)
    } catch (e) {
      console.warn('WebGL addon failed to load, using default renderer', e)
    }

    // Small delay to ensure DOM is ready before fitting and focusing
    requestAnimationFrame(() => {
      fitAddon.fit()
      term.focus()
    })

    function connect() {
      if (isStale()) return

      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      const ws = new WebSocket(`${protocol}//${window.location.host}/ws/terminal/${encodeURIComponent(sessionName)}`)
      ws.binaryType = 'arraybuffer'
      wsRef.current = ws

      ws.onmessage = (event: MessageEvent) => {
        if (isStale()) return
        if (event.data instanceof ArrayBuffer) {
          term.write(new Uint8Array(event.data))
        } else {
          term.write(event.data as string)
        }
      }

      ws.onopen = () => {
        if (isStale()) return
        // Only show reconnected message if it took multiple attempts (not brief blips)
        if (reconnectAttempt >= RECONNECT_QUIET_THRESHOLD) {
          term.write(`\r\n\x1b[32m[${tRef.current('terminal.reconnected')}]\x1b[0m\r\n`)
        }
        reconnectAttempt = 0
        const dims = { cols: term.cols, rows: term.rows }
        ws.send(JSON.stringify(dims))
        flushBuffer()
      }

      ws.onerror = () => {
        if (isStale()) return
        if (reconnectAttempt === 0) {
          term.write(`\r\n\x1b[31m[${tRef.current('terminal.connectionError')}]\x1b[0m\r\n`)
        }
      }

      ws.onclose = () => {
        // Guard: if this WS belongs to a stale generation, do NOT touch shared refs
        if (isStale()) return
        wsRef.current = null
        scheduleReconnect()
      }
    }

    function scheduleReconnect() {
      if (isStale()) return
      if (reconnectTimer) return

      reconnectAttempt += 1

      if (reconnectAttempt > RECONNECT_MAX_ATTEMPTS) {
        term.write(`\r\n\x1b[31m[${tRef.current('terminal.reconnectFailed')}]\x1b[0m\r\n`)
        return
      }

      const delay = Math.min(
        RECONNECT_BASE_DELAY * Math.pow(2, reconnectAttempt - 1),
        RECONNECT_MAX_DELAY
      )

      // Only show reconnecting message after quiet threshold
      if (reconnectAttempt >= RECONNECT_QUIET_THRESHOLD) {
        term.write(`\r\n\x1b[33m[${tRef.current('terminal.reconnecting', { attempt: reconnectAttempt })}]\x1b[0m\r\n`)
      }

      reconnectTimer = setTimeout(() => {
        reconnectTimer = null
        connect()
      }, delay)
    }

    connect()

    // Send user input to WS as binary (or buffer if disconnected)
    const dataDisposable = term.onData((data: string) => {
      sendOrBuffer(data)
    })

    // Resize handling
    const resizeObserver = new ResizeObserver(() => {
      requestAnimationFrame(() => {
        if (fitAddonRef.current) {
          fitAddonRef.current.fit()
        }
      })
    })
    resizeObserver.observe(containerRef.current)

    const resizeDisposable = term.onResize(({ cols, rows }: { cols: number; rows: number }) => {
      if (wsRef.current?.readyState === WebSocket.OPEN) {
        wsRef.current.send(JSON.stringify({ cols, rows }))
      }
    })

    // Clipboard image paste handler (for right-click paste / non-keyboard paste)
    const handlePaste = async (e: ClipboardEvent) => {
      const items = e.clipboardData?.items
      if (!items) return

      for (const item of items) {
        if (item.type.startsWith('image/')) {
          e.preventDefault()
          e.stopPropagation()
          const blob = item.getAsFile()
          if (!blob) return
          try {
            const { path } = await uploadClipboardImage(blob)
            sendOrBuffer(path + ' ')
            showImageToast(path, blob)
          } catch (err) {
            console.error('Image upload failed:', err)
          }
          return
        }
      }
      // Text paste: let xterm handle it natively via onData
    }

    // Drag & drop file handler
    const handleDrop = async (e: DragEvent) => {
      e.preventDefault()
      const files = e.dataTransfer?.files
      if (!files) return
      for (const file of files) {
        try {
          if (file.type.startsWith('image/')) {
            const { path } = await uploadClipboardImage(file)
            sendOrBuffer(path + ' ')
            showImageToast(path, file)
          } else {
            const { path } = await uploadClipboardFile(file)
            sendOrBuffer(path + ' ')
            showFileToast(path)
          }
        } catch (err) {
          console.error('File upload failed:', err)
        }
      }
    }

    const handleDragOver = (e: DragEvent) => {
      e.preventDefault()
    }

    const el = containerRef.current
    el.addEventListener('paste', handlePaste)
    el.addEventListener('drop', handleDrop)
    el.addEventListener('dragover', handleDragOver)

    return () => {
      cancelled = true
      if (reconnectTimer) {
        clearTimeout(reconnectTimer)
        reconnectTimer = null
      }
      inputBufferRef.current = []
      sendOrBufferRef.current = null
      dataDisposable.dispose()
      resizeDisposable.dispose()
      resizeObserver.disconnect()
      el.removeEventListener('paste', handlePaste)
      el.removeEventListener('drop', handleDrop)
      el.removeEventListener('dragover', handleDragOver)
      const ws = wsRef.current
      if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
        ws.close()
      }
      term.dispose()
      termRef.current = null
      wsRef.current = null
      fitAddonRef.current = null
    }
  }, [sessionName, theme])

  const toastStyle: React.CSSProperties = {
    position: 'absolute',
    bottom: '12px',
    right: '12px',
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    padding: '8px 12px',
    borderRadius: '6px',
    backgroundColor: theme === 'dark' ? 'rgba(22, 27, 34, 0.9)' : 'rgba(255, 255, 255, 0.95)',
    color: theme === 'dark' ? '#e6edf3' : '#1f2328',
    border: `1px solid ${theme === 'dark' ? '#30363d' : '#d0d7de'}`,
    fontSize: '12px',
    zIndex: 10,
    boxShadow: '0 2px 8px rgba(0,0,0,0.2)',
    maxWidth: '300px',
    animation: 'fadeIn 0.2s ease-in',
  }

  return (
    <div className="relative w-full h-full flex">
      <div className="flex-1 min-w-0 relative h-full">
        <div className="terminal-container w-full h-full" ref={containerRef} />
        {toast && (
          <div style={toastStyle}>
            {toast.isFile ? (
              <span className="text-xl">📎</span>
            ) : (
              <img
                src={toast.url}
                alt="pasted"
                className="w-8 h-8 object-cover rounded-sm"
              />
            )}
            <span className="overflow-hidden text-ellipsis whitespace-nowrap">
              {toast.path}
            </span>
          </div>
        )}
      </div>
      {projectPath && (
        sidePanel ? (
          <div className="w-[340px] min-w-[280px] h-full flex flex-col">
            <SidePanel
              projectPath={projectPath}
              gitAvailable={!!gitAvailable}
              onClose={() => setSidePanel(false)}
              clipboard={clipboard}
              onCopy={onCopy}
              onCut={onCut}
              onClearClipboard={onClearClipboard}
            />
          </div>
        ) : (
          <button
            className="w-6 h-full bg-background border-none border-l border-border text-text-tertiary cursor-pointer text-sm flex items-center justify-center shrink-0 hover:bg-secondary hover:text-foreground"
            onClick={() => setSidePanel(true)}
            title="展開面板"
          >
            <svg width="14" height="14" viewBox="0 0 16 16" fill="currentColor">
              <path d="M8.884 8l-3.638-3.515a.508.508 0 0 1 0-.74.568.568 0 0 1 .764 0L10.13 7.63a.508.508 0 0 1 0 .74L6.01 12.255a.568.568 0 0 1-.764 0 .508.508 0 0 1 0-.74L8.884 8z"/>
            </svg>
          </button>
        )
      )}
    </div>
  )
})
