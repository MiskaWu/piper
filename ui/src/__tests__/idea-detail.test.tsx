import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import type { Idea } from '../types'

// Mock react-i18next
vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { language: 'en', changeLanguage: vi.fn() },
  }),
}))

// Mock api
vi.mock('../api', () => ({
  enrichIdea: vi.fn(() => Promise.resolve()),
  deleteIdea: vi.fn(() => Promise.resolve()),
  updateIdea: vi.fn(() => Promise.resolve({ id: 'idea-1', text: 'Updated text', createdAt: Math.floor(Date.now() / 1000) })),
  fetchIdeas: vi.fn(() => Promise.resolve([])),
}))

import IdeaDetail from '../components/IdeaDetail'

const baseIdea: Idea = {
  id: 'idea-1',
  text: 'Implement caching layer',
  createdAt: Math.floor(Date.now() / 1000) - 3600, // 1 hour ago
}

describe('IdeaDetail', () => {
  let onStartEnrich: (id: string) => void
  let onConvert: (text: string, enrichedDesc: string | undefined, id: string) => void
  let onDeleted: () => void

  beforeEach(() => {
    vi.clearAllMocks()
    onStartEnrich = vi.fn()
    onConvert = vi.fn()
    onDeleted = vi.fn()
  })

  it('renders idea text and time', () => {
    render(
      <IdeaDetail
        idea={baseIdea}
        onStartEnrich={onStartEnrich}
        onConvert={onConvert}
        onDeleted={onDeleted}
      />
    )
    expect(screen.getByText('Implement caching layer')).toBeInTheDocument()
    expect(screen.getByTestId('idea-detail-time')).toBeInTheDocument()
  })

  it('renders context section when idea has context', () => {
    const ideaWithContext: Idea = {
      ...baseIdea,
      context: [
        { kind: 'terminal', sessionName: 'session-1', detail: 'some output' },
      ],
    }
    render(
      <IdeaDetail
        idea={ideaWithContext}
        onStartEnrich={onStartEnrich}
        onConvert={onConvert}
        onDeleted={onDeleted}
      />
    )
    expect(screen.getByText('[terminal]')).toBeInTheDocument()
    expect(screen.getByText('session-1')).toBeInTheDocument()
    expect(screen.getByText('some output')).toBeInTheDocument()
  })

  it('does not render context section when idea has no context', () => {
    render(
      <IdeaDetail
        idea={baseIdea}
        onStartEnrich={onStartEnrich}
        onConvert={onConvert}
        onDeleted={onDeleted}
      />
    )
    expect(screen.queryByTestId('idea-detail-context')).not.toBeInTheDocument()
  })

  it('renders enriched description when present', () => {
    const ideaWithEnriched: Idea = {
      ...baseIdea,
      enrichedDesc: 'AI generated description about caching',
    }
    render(
      <IdeaDetail
        idea={ideaWithEnriched}
        onStartEnrich={onStartEnrich}
        onConvert={onConvert}
        onDeleted={onDeleted}
      />
    )
    expect(screen.getByText('AI generated description about caching')).toBeInTheDocument()
  })

  it('calls onStartEnrich with idea id when enrich button clicked', () => {
    render(
      <IdeaDetail
        idea={baseIdea}
        onStartEnrich={onStartEnrich}
        onConvert={onConvert}
        onDeleted={onDeleted}
      />
    )
    const enrichBtn = screen.getByRole('button', { name: /inbox\.enrich$/ })
    fireEvent.click(enrichBtn)
    expect(onStartEnrich).toHaveBeenCalledWith('idea-1')
  })

  it('disables enrich button when enriching prop is true', () => {
    render(
      <IdeaDetail
        idea={baseIdea}
        enriching={true}
        onStartEnrich={onStartEnrich}
        onConvert={onConvert}
        onDeleted={onDeleted}
      />
    )
    const enrichBtn = screen.getByRole('button', { name: /inbox\.enrich$/ })
    expect(enrichBtn).toBeDisabled()
    expect(enrichBtn).toHaveTextContent('...')
  })

  it('calls onConvert with idea data when convert button clicked', () => {
    const ideaWithEnriched: Idea = {
      ...baseIdea,
      enrichedDesc: 'enriched text',
    }
    render(
      <IdeaDetail
        idea={ideaWithEnriched}
        onStartEnrich={onStartEnrich}
        onConvert={onConvert}
        onDeleted={onDeleted}
      />
    )
    const convertBtn = screen.getByRole('button', { name: /inbox\.convert$/ })
    fireEvent.click(convertBtn)
    expect(onConvert).toHaveBeenCalledWith('Implement caching layer', 'enriched text', 'idea-1')
  })

  it('calls deleteIdea and onDeleted when delete confirmed', async () => {
    vi.spyOn(window, 'confirm').mockReturnValueOnce(true)
    render(
      <IdeaDetail
        idea={baseIdea}
        onStartEnrich={onStartEnrich}
        onConvert={onConvert}
        onDeleted={onDeleted}
      />
    )
    const deleteBtn = screen.getByRole('button', { name: /inbox\.delete$/ })
    fireEvent.click(deleteBtn)
    await waitFor(() => {
      expect(onDeleted).toHaveBeenCalled()
    })
  })

  it('does not delete when confirm is cancelled', () => {
    vi.spyOn(window, 'confirm').mockReturnValueOnce(false)
    render(
      <IdeaDetail
        idea={baseIdea}
        onStartEnrich={onStartEnrich}
        onConvert={onConvert}
        onDeleted={onDeleted}
      />
    )
    const deleteBtn = screen.getByRole('button', { name: /inbox\.delete$/ })
    fireEvent.click(deleteBtn)
    expect(onDeleted).not.toHaveBeenCalled()
  })
})
