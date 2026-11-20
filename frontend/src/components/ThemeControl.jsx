import { useEffect, useState } from 'react'
import { Moon, Sun } from 'lucide-react'
import { useStoredPreference } from '../hooks/usePreferences.js'

export function ThemeControl() {
  const [stored, setTheme] = useStoredPreference('transit-theme', 'system')
  const [effectiveTheme, setEffectiveTheme] = useState('light')

  useEffect(() => {
    const media = window.matchMedia?.('(prefers-color-scheme: dark)')
    let frame
    const apply = () => {
      const current = stored === 'system' ? (media?.matches ? 'dark' : 'light') : stored
      setEffectiveTheme(current)
      document.documentElement.classList.add('theme-changing')
      document.documentElement.dataset.theme = current
      void window.getComputedStyle(document.documentElement).color
      frame = requestAnimationFrame(() => document.documentElement.classList.remove('theme-changing'))
    }
    apply()
    media?.addEventListener?.('change', apply)
    return () => {
      media?.removeEventListener?.('change', apply)
      cancelAnimationFrame(frame)
      document.documentElement.classList.remove('theme-changing')
    }
  }, [stored])

  const toggleTheme = () => {
    const next = effectiveTheme === 'dark' ? 'light' : 'dark'
    setTheme(next)
  }

  const isDark = effectiveTheme === 'dark'

  return (
    <button
      type="button"
      className="theme-toggle-btn"
      onClick={toggleTheme}
      aria-label={isDark ? 'Switch to light mode' : 'Switch to dark mode'}
      title={isDark ? 'Switch to light mode' : 'Switch to dark mode'}
    >
      {isDark ? <Sun size={17} aria-hidden="true" /> : <Moon size={17} aria-hidden="true" />}
      <span className="theme-toggle-label">{isDark ? 'Light' : 'Dark'}</span>
    </button>
  )
}
