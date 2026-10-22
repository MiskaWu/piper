import type { Dispatch, SetStateAction } from 'react'
import { useState, useEffect, useRef } from 'react'
import type { Session, Usage, ActivePlanSession } from '../types'

type UseStatusSocketReturn = [Session[], Dispatch<SetStateAction<Session[]>>, Usage | null, ActivePlanSession[]]

// 合併 WS 資料與 optimistic sessions：保留 server 尚未回傳的本地 session
function mergeSessions(prev: Session[], incoming: Session[]): Session[] {
  const serverNames = new Set(incoming.map(s => s.name))
  const optimistic = prev.filter(s => !serverNames.has(s.name) && s._optimistic)
  return [...incoming, ...optimistic]
}

export default function useStatusSocket(): UseStatusSocketReturn {
  const [sessions, setSessions] = useState<Session[]>([])
  const [usage, setUsage] = useState<Usage | null>(null)
  const [activePlanSessions, setActivePlanSessions] = useState<ActivePlanSession[]>([])
  const wsRef = useRef<WebSocket | null>(null)
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    function connect() {
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      const ws = new WebSocket(`${protocol}//${window.location.host}/ws/status`)
      wsRef.current = ws

      ws.onmessage = (event: MessageEvent) => {
        try {
          const data = JSON.parse(event.data)
          if (data.type === 'plan_update') {
            window.dispatchEvent(new CustomEvent('plan_update', { detail: data }))
            return
          }
          if (data.type === 'setup_progress') {
            window.dispatchEvent(new CustomEvent('setup_progress', { detail: data }))
            return
          }
          if (data.type === 'git_changed') {
            window.dispatchEvent(new CustomEvent('git_changed', { detail: data }))
            return
          }
          if (Array.isArray(data)) {
            setSessions(prev => mergeSessions(prev, data))
          } else if (data.sessions) {
            setSessions(prev => mergeSessions(prev, data.sessions))
            if (data.usage) {
              setUsage(data.usage)
            }
            setActivePlanSessions(data.activePlanSessions ?? [])
          }
        } catch {
          // ignore malformed messages
        }
      }

      ws.onclose = () => {
        wsRef.current = null
        reconnectTimer.current = setTimeout(connect, 3000)
      }

      ws.onerror = () => {
        ws.close()
      }
    }

    connect()

    return () => {
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current)
      if (wsRef.current) {
        wsRef.current.close()
        wsRef.current = null
      }
    }
  }, [])

  return [sessions, setSessions, usage, activePlanSessions]
}
