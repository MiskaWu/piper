import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import en from './locales/en.json'
import zhTW from './locales/zh-TW.json'

const stored = localStorage.getItem('i18nextLng')
const defaultLng = stored || (navigator.language.startsWith('zh') ? 'zh-TW' : 'en')

i18n.use(initReactI18next).init({
  resources: { en: { translation: en }, 'zh-TW': { translation: zhTW } },
  lng: defaultLng,
  fallbackLng: 'en',
  interpolation: { escapeValue: false },
})

i18n.on('languageChanged', (lng: string) => localStorage.setItem('i18nextLng', lng))

export default i18n
