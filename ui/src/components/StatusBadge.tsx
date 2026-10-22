import React from 'react'
import { useTranslation } from 'react-i18next'
import type { SessionStatus } from '../types'

interface MouseProps {
  color: string
}

const STATUS_CLASS: Record<SessionStatus, string> = {
  working: 'status--working',
  completed: 'status--completed',
  needs_input: 'status--needs-input',
  idle: 'status--idle',
}

/* Running mouse - legs stretched out, leaning forward */
function RunningMouse({ color }: MouseProps) {
  return (
    <svg viewBox="0 0 20 16" fill="none" xmlns="http://www.w3.org/2000/svg">
      {/* Body */}
      <ellipse cx="10" cy="9" rx="5.5" ry="4" fill={color} />
      {/* Head */}
      <ellipse cx="16" cy="7" rx="3" ry="2.8" fill={color} />
      {/* Ears */}
      <circle cx="15" cy="4" r="1.8" fill={color} />
      <circle cx="15" cy="4" r="1" fill="var(--bg-secondary)" />
      <circle cx="18" cy="5" r="1.5" fill={color} />
      <circle cx="18" cy="5" r="0.8" fill="var(--bg-secondary)" />
      {/* Eye */}
      <circle cx="17.5" cy="6.5" r="0.6" fill="var(--bg-primary)" />
      {/* Nose */}
      <circle cx="19" cy="7.2" r="0.5" fill="var(--bg-primary)" />
      {/* Tail */}
      <path d="M4.5 8 Q1 5 2 2" stroke={color} strokeWidth="1" strokeLinecap="round" fill="none" />
      {/* Back leg */}
      <g className="mouse-leg-back" style={{ transformOrigin: '7px 12px' }}>
        <line x1="7" y1="12" x2="5" y2="15" stroke={color} strokeWidth="1.2" strokeLinecap="round" />
      </g>
      {/* Front leg */}
      <g className="mouse-leg-front" style={{ transformOrigin: '13px 12px' }}>
        <line x1="13" y1="12" x2="15" y2="15" stroke={color} strokeWidth="1.2" strokeLinecap="round" />
      </g>
    </svg>
  )
}

/* Waving mouse - standing upright, one arm raised */
function WavingMouse({ color }: MouseProps) {
  return (
    <svg viewBox="0 0 20 16" fill="none" xmlns="http://www.w3.org/2000/svg">
      {/* Body - more upright */}
      <ellipse cx="10" cy="10" rx="4" ry="4.5" fill={color} />
      {/* Head */}
      <ellipse cx="10" cy="5" rx="3.2" ry="2.8" fill={color} />
      {/* Ears */}
      <circle cx="7.5" cy="2.5" r="2" fill={color} />
      <circle cx="7.5" cy="2.5" r="1.1" fill="var(--bg-secondary)" />
      <circle cx="12.5" cy="2.5" r="2" fill={color} />
      <circle cx="12.5" cy="2.5" r="1.1" fill="var(--bg-secondary)" />
      {/* Eyes - open wide */}
      <circle cx="8.8" cy="4.8" r="0.8" fill="var(--bg-primary)" />
      <circle cx="11.2" cy="4.8" r="0.8" fill="var(--bg-primary)" />
      {/* Nose */}
      <circle cx="10" cy="6.2" r="0.5" fill="var(--bg-primary)" />
      {/* Waving hand */}
      <g className="mouse-hand">
        <line x1="14" y1="9" x2="17" y2="5" stroke={color} strokeWidth="1.3" strokeLinecap="round" />
        <circle cx="17" cy="4.5" r="0.8" fill={color} />
      </g>
      {/* Other arm */}
      <line x1="6.5" y1="9" x2="5" y2="12" stroke={color} strokeWidth="1.2" strokeLinecap="round" />
      {/* Feet */}
      <ellipse cx="8" cy="14.5" rx="1.5" ry="0.7" fill={color} />
      <ellipse cx="12" cy="14.5" rx="1.5" ry="0.7" fill={color} />
    </svg>
  )
}

/* Celebrating mouse - standing with a checkmark */
function CelebratingMouse({ color }: MouseProps) {
  return (
    <svg viewBox="0 0 20 16" fill="none" xmlns="http://www.w3.org/2000/svg">
      {/* Body - upright */}
      <ellipse cx="10" cy="10" rx="4" ry="4.5" fill={color} />
      {/* Head */}
      <ellipse cx="10" cy="5" rx="3.2" ry="2.8" fill={color} />
      {/* Ears */}
      <circle cx="7.5" cy="2.5" r="2" fill={color} />
      <circle cx="7.5" cy="2.5" r="1.1" fill="var(--bg-secondary)" />
      <circle cx="12.5" cy="2.5" r="2" fill={color} />
      <circle cx="12.5" cy="2.5" r="1.1" fill="var(--bg-secondary)" />
      {/* Eyes - happy closed */}
      <path d="M8.2 4.5 Q8.8 5.2 9.4 4.5" stroke="var(--bg-primary)" strokeWidth="0.7" strokeLinecap="round" fill="none" />
      <path d="M10.6 4.5 Q11.2 5.2 11.8 4.5" stroke="var(--bg-primary)" strokeWidth="0.7" strokeLinecap="round" fill="none" />
      {/* Nose */}
      <circle cx="10" cy="6.2" r="0.5" fill="var(--bg-primary)" />
      {/* Mouth - happy smile */}
      <path d="M9.2 6.8 Q10 7.6 10.8 6.8" stroke="var(--bg-primary)" strokeWidth="0.5" strokeLinecap="round" fill="none" />
      {/* Blush */}
      <circle cx="8" cy="5.8" r="0.8" fill="#FF9999" opacity="0.5" />
      <circle cx="12" cy="5.8" r="0.8" fill="#FF9999" opacity="0.5" />
      {/* Arms raised */}
      <line x1="6.5" y1="9" x2="4" y2="6" stroke={color} strokeWidth="1.2" strokeLinecap="round" />
      <line x1="13.5" y1="9" x2="16" y2="6" stroke={color} strokeWidth="1.2" strokeLinecap="round" />
      {/* Feet */}
      <ellipse cx="8" cy="14.5" rx="1.5" ry="0.7" fill={color} />
      <ellipse cx="12" cy="14.5" rx="1.5" ry="0.7" fill={color} />
      {/* Stars */}
      <path className="mouse-star" d="M3 3 L3.4 1.8 L3.8 3 L5 3.4 L3.8 3.8 L3.4 5 L3 3.8 L1.8 3.4 Z" fill={color} />
      <path className="mouse-star mouse-star--delay" d="M16 0.5 L16.3 -0.3 L16.6 0.5 L17.4 0.8 L16.6 1.1 L16.3 1.9 L16 1.1 L15.2 0.8 Z" fill={color} />
      {/* Checkmark */}
      <g className="mouse-check">
        <path d="M15 2 L16.5 3.5 L19 1" stroke={color} strokeWidth="1.2" strokeLinecap="round" strokeLinejoin="round" fill="none" />
      </g>
    </svg>
  )
}

/* Sleeping mouse - curled up with Zzz */
function SleepingMouse({ color }: MouseProps) {
  return (
    <svg viewBox="0 0 22 16" fill="none" xmlns="http://www.w3.org/2000/svg">
      {/* Curled body */}
      <ellipse cx="10" cy="11" rx="6" ry="3.5" fill={color} />
      {/* Head resting */}
      <ellipse cx="15" cy="9.5" rx="3" ry="2.5" fill={color} />
      {/* Ears - folded */}
      <circle cx="14" cy="7" r="1.6" fill={color} />
      <circle cx="14" cy="7" r="0.9" fill="var(--bg-secondary)" />
      <circle cx="17" cy="7.5" r="1.3" fill={color} />
      <circle cx="17" cy="7.5" r="0.7" fill="var(--bg-secondary)" />
      {/* Closed eyes - just a line */}
      <path d="M16 9.5 Q16.8 9 17.5 9.5" stroke="var(--bg-primary)" strokeWidth="0.6" strokeLinecap="round" fill="none" />
      {/* Nose */}
      <circle cx="18" cy="10" r="0.4" fill="var(--bg-primary)" />
      {/* Tail curled around body */}
      <path d="M4 10 Q2 8 3 6 Q4 4.5 5.5 5.5" stroke={color} strokeWidth="1" strokeLinecap="round" fill="none" />
      {/* Zzz */}
      <g className="mouse-zzz">
        <text x="17" y="5" fill={color} fontSize="4" fontWeight="bold" fontFamily="sans-serif">z</text>
        <text x="19" y="3" fill={color} fontSize="3" fontWeight="bold" fontFamily="sans-serif" opacity="0.6">z</text>
      </g>
    </svg>
  )
}

const MOUSE_COMPONENT: Record<SessionStatus, React.FC<MouseProps>> = {
  working: RunningMouse,
  completed: CelebratingMouse,
  needs_input: WavingMouse,
  idle: SleepingMouse,
}

const MOUSE_COLOR: Record<SessionStatus, string> = {
  working: 'var(--status-working)',
  completed: 'var(--status-completed)',
  needs_input: 'var(--status-needs-input)',
  idle: 'var(--status-idle)',
}

interface StatusBadgeProps {
  status: SessionStatus
}

export default function StatusBadge({ status }: StatusBadgeProps) {
  const { t } = useTranslation()
  const className = STATUS_CLASS[status] || STATUS_CLASS.idle
  const label = t(`status.${status}`, t('status.idle'))
  const Mouse = MOUSE_COMPONENT[status] || MOUSE_COMPONENT.idle
  const color = MOUSE_COLOR[status] || MOUSE_COLOR.idle

  return (
    <span className={`status-badge ${className}`}>
      <span className="status-label">{label}</span>
      <span className="status-mouse">
        <Mouse color={color} />
      </span>
    </span>
  )
}
