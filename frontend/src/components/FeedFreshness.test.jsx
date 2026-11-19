import { act, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { FeedFreshness } from './FeedFreshness.jsx'

afterEach(() => vi.useRealTimers())

it('warns when a successful update becomes too old and clears after recovery or pause', () => {
  vi.useFakeTimers()
  vi.setSystemTime(new Date('2026-11-19T14:00:00Z'))
  const firstUpdate = new Date()
  const view = render(<FeedFreshness updatedAt={firstUpdate} maxAge={30_000} />)
  expect(screen.queryByText(/Live data may be out of date/)).not.toBeInTheDocument()
  act(() => vi.advanceTimersByTime(45_000))
  expect(screen.getByText(/Live data may be out of date/)).toBeInTheDocument()
  view.rerender(<FeedFreshness updatedAt={new Date()} maxAge={30_000} />)
  expect(screen.queryByText(/Live data may be out of date/)).not.toBeInTheDocument()
  view.rerender(<FeedFreshness updatedAt={firstUpdate} maxAge={30_000} paused />)
  expect(screen.queryByText(/Live data may be out of date/)).not.toBeInTheDocument()
})
