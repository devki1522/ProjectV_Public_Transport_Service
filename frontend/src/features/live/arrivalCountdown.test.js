import { expect, it } from 'vitest'
import { arrivalCountdown } from './arrivalCountdown.js'
import { formatTime } from '../../lib/transit.js'

const now = Date.parse('2026-11-19T15:00:00Z')

it('rounds up upcoming arrivals so a bus more than two minutes away displays three minutes', () => {
  expect(arrivalCountdown('2026-11-19T15:02:01Z', now)).toEqual({ value: '3', unit: 'min' })
})

it('shows due at the current time without inferring that a bus has departed from an old estimate', () => {
  expect(arrivalCountdown('2026-11-19T15:00:00Z', now)).toEqual({ value: 'Due', unit: '' })
  const oldEstimate = '2026-11-19T14:55:00Z'
  expect(arrivalCountdown(oldEstimate, now)).toEqual({ value: formatTime(oldEstimate), unit: '' })
})

it('does not invent a countdown for invalid data', () => {
  expect(arrivalCountdown(null, now)).toEqual({ value: '—', unit: '' })
  expect(arrivalCountdown('invalid', now)).toEqual({ value: '—', unit: '' })
})
