import { formatTime } from '../../lib/transit.js'

export function arrivalCountdown(value, now) {
  const timestamp = Date.parse(value)
  if (!Number.isFinite(timestamp)) return { value: '—', unit: '' }
  const minutes = Math.ceil((timestamp - now) / 60_000)
  if (minutes < -1) return { value: formatTime(value), unit: '' }
  return minutes <= 0 ? { value: 'Due', unit: '' } : { value: String(minutes), unit: 'min' }
}

