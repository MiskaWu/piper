import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import type { Session } from '../types'

// Mock react-i18next
vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { language: 'en', changeLanguage: vi.fn() },
  }),
}))

// Mock api
vi.mock('../api', () => ({
  updateSessionOrder: vi.fn(() => Promise.resolve()),
  updateSessionMeta: vi.fn(() => Promise.resolve()),
  listProjects: vi.fn(() => Promise.resolve({ projects: [], devcontainerCLI: false })),
}))

import Sidebar from '../components/Sidebar'
import { TooltipProvider } from '../components/ui/tooltip'
import { updateSessionOrder } from '../api'

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>{children}</TooltipProvider>
    </QueryClientProvider>
  )
}

const makeSessions = (count: number): Session[] =>
  Array.from({ length: count }, (_, i) => ({
    name: `claude-session-${i}`,
    title: `project-${i}`,
    status: 'idle' as const,
    order: i,
  }))

describe('Sidebar drag-and-drop reorder', () => {
  let onReorder: (order: string[]) => void

  beforeEach(() => {
    vi.clearAllMocks()
    onReorder = vi.fn()
  })

  it('calls onReorder and updateSessionOrder when drag completes', () => {
    const sessions = makeSessions(3)
    render(
      <Sidebar
        sessions={sessions}
        selected={null}
        onSelect={vi.fn()}
        onNewSession={vi.fn()}
        onDelete={vi.fn()}
        theme="dark"
        onToggleTheme={vi.fn()}
        usage={null}
        onReorder={onReorder}
      />,
      { wrapper: createWrapper() },
    )

    const cards = screen.getAllByText(/project-/)
    // Simulate drag: start on card 0, over card 2, end
    const card0 = cards[0].closest('.session-card') as HTMLElement
    const card2 = cards[2].closest('.session-card') as HTMLElement

    fireEvent.dragStart(card0, { dataTransfer: { effectAllowed: '', setData: vi.fn() } })
    fireEvent.dragOver(card2, { dataTransfer: { dropEffect: '' } })
    fireEvent.dragEnd(card0)

    // Should call API with reordered names: [1, 2, 0] (moved 0 to position 2)
    expect(updateSessionOrder).toHaveBeenCalledWith([
      'claude-session-1',
      'claude-session-2',
      'claude-session-0',
    ])

    // Should call onReorder for immediate UI update
    expect(onReorder).toHaveBeenCalledWith([
      'claude-session-1',
      'claude-session-2',
      'claude-session-0',
    ])
  })

  it('does not call onReorder when dropped on same position', () => {
    const sessions = makeSessions(3)
    render(
      <Sidebar
        sessions={sessions}
        selected={null}
        onSelect={vi.fn()}
        onNewSession={vi.fn()}
        onDelete={vi.fn()}
        theme="dark"
        onToggleTheme={vi.fn()}
        usage={null}
        onReorder={onReorder}
      />,
      { wrapper: createWrapper() },
    )

    const cards = screen.getAllByText(/project-/)
    const card0 = cards[0].closest('.session-card') as HTMLElement

    fireEvent.dragStart(card0, { dataTransfer: { effectAllowed: '', setData: vi.fn() } })
    fireEvent.dragOver(card0, { dataTransfer: { dropEffect: '' } })
    fireEvent.dragEnd(card0)

    expect(updateSessionOrder).not.toHaveBeenCalled()
    expect(onReorder).not.toHaveBeenCalled()
  })
})
