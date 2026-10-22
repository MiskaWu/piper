import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'

// Mock react-i18next
vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { language: 'en', changeLanguage: vi.fn() },
  }),
}))

// Mock api
const mockDeleteIdea = vi.fn(() => Promise.resolve())
const mockFetchIdeas = vi.fn(() =>
  Promise.resolve([
    { id: 'idea-1', text: 'Test idea', createdAt: Math.floor(Date.now() / 1000) },
  ])
)
vi.mock('../api', () => ({
  fetchIdeas: (...args: unknown[]) => mockFetchIdeas(...(args as Parameters<typeof mockFetchIdeas>)),
  createIdea: vi.fn(() => Promise.resolve()),
  deleteIdea: (...args: unknown[]) => mockDeleteIdea(...(args as Parameters<typeof mockDeleteIdea>)),
}))

import InboxCapture from '../components/InboxCapture'

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
}

describe('InboxCapture delete confirmation', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockFetchIdeas.mockResolvedValue([
      { id: 'idea-1', text: 'Test idea', createdAt: Math.floor(Date.now() / 1000) },
    ])
  })

  it('calls deleteIdea when confirm is accepted', async () => {
    vi.spyOn(window, 'confirm').mockReturnValueOnce(true)
    render(<InboxCapture />, { wrapper: createWrapper() })

    await waitFor(() => {
      expect(screen.getByText('Test idea')).toBeInTheDocument()
    })

    const deleteBtn = screen.getByTitle('inbox.delete')
    fireEvent.click(deleteBtn)

    expect(window.confirm).toHaveBeenCalledWith('inbox.deleteConfirm')
    await waitFor(() => {
      expect(mockDeleteIdea).toHaveBeenCalledWith('idea-1')
    })
  })

  it('does not call deleteIdea when confirm is cancelled', async () => {
    vi.spyOn(window, 'confirm').mockReturnValueOnce(false)
    render(<InboxCapture />, { wrapper: createWrapper() })

    await waitFor(() => {
      expect(screen.getByText('Test idea')).toBeInTheDocument()
    })

    const deleteBtn = screen.getByTitle('inbox.delete')
    fireEvent.click(deleteBtn)

    expect(window.confirm).toHaveBeenCalledWith('inbox.deleteConfirm')
    expect(mockDeleteIdea).not.toHaveBeenCalled()
  })
})
