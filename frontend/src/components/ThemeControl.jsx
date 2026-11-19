import { useEffect } from 'react'
import { SunMoon } from 'lucide-react'
import { useStoredPreference } from '../hooks/usePreferences.js'

export function ThemeControl() {
  const [stored, setTheme] = useStoredPreference('transit-theme', 'system')
  const theme = ['system', 'light', 'dark'].includes(stored) ? stored : 'system'
  useEffect(() => {
    const media = window.matchMedia?.('(prefers-color-scheme: dark)')
    let frame
    const apply = () => {
      document.documentElement.classList.add('theme-changing')
      document.documentElement.dataset.theme = theme === 'system' ? (media?.matches ? 'dark' : 'light') : theme
      void window.getComputedStyle(document.documentElement).color
      frame = requestAnimationFrame(() => document.documentElement.classList.remove('theme-changing'))
    }
    apply()
    media?.addEventListener('change', apply)
    return () => {
      media?.removeEventListener('change', apply)
      cancelAnimationFrame(frame)
      document.documentElement.classList.remove('theme-changing')
    }
  }, [theme])
  return <label className="theme-control"><SunMoon size={17} aria-hidden="true" />
    <span className="visually-hidden">Appearance</span>
    <select aria-label="Appearance" value={theme} onChange={(event) => setTheme(event.target.value)}>
      <option value="system">System</option><option value="light">Light</option><option value="dark">Dark</option>
    </select>
  </label>
}
