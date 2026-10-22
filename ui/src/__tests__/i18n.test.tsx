import { describe, it, expect, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import i18n from '../i18n'
import en from '../locales/en.json'
import zhTW from '../locales/zh-TW.json'
import { useTranslation } from 'react-i18next'

function TestComponent() {
  const { t, i18n: i18nInstance } = useTranslation()
  return (
    <div>
      <span data-testid="text">{t('sidebar.newSession')}</span>
      <span data-testid="lang">{i18nInstance.language}</span>
      <button onClick={() => i18nInstance.changeLanguage('zh-TW')}>toZH</button>
      <button onClick={() => i18nInstance.changeLanguage('en')}>toEN</button>
    </div>
  )
}

describe('i18n', () => {
  beforeEach(async () => {
    localStorage.clear()
    await i18n.changeLanguage('en')
  })

  it('defaults to English', () => {
    expect(i18n.language).toBe('en')
  })

  it('renders English translations', () => {
    render(<TestComponent />)
    expect(screen.getByTestId('text')).toHaveTextContent((en as Record<string, any>).sidebar.newSession)
  })

  it('switches to zh-TW and renders Chinese translations', async () => {
    render(<TestComponent />)
    fireEvent.click(screen.getByText('toZH'))
    expect(screen.getByTestId('text')).toHaveTextContent((zhTW as Record<string, any>).sidebar.newSession)
    expect(screen.getByTestId('lang')).toHaveTextContent('zh-TW')
  })

  it('persists language to localStorage', async () => {
    await i18n.changeLanguage('zh-TW')
    expect(localStorage.getItem('i18nextLng')).toBe('zh-TW')
  })

  it('switches back to English', async () => {
    render(<TestComponent />)
    fireEvent.click(screen.getByText('toZH'))
    fireEvent.click(screen.getByText('toEN'))
    expect(screen.getByTestId('text')).toHaveTextContent((en as Record<string, any>).sidebar.newSession)
  })

  it('en.json and zh-TW.json have the same keys', () => {
    const enKeys = Object.keys(flatten(en)).sort()
    const zhKeys = Object.keys(flatten(zhTW)).sort()
    expect(enKeys).toEqual(zhKeys)
  })
})

function flatten(obj: Record<string, any>, prefix = ''): Record<string, string> {
  return Object.entries(obj).reduce<Record<string, string>>((acc, [key, val]) => {
    const path = prefix ? `${prefix}.${key}` : key
    if (typeof val === 'object' && val !== null) {
      Object.assign(acc, flatten(val, path))
    } else {
      acc[path] = val
    }
    return acc
  }, {})
}
