import { act, renderHook, waitFor } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import { useResource } from './useResource.js'

it('aborts outdated requests and never replaces the selected route with a late response', async () => {
  let resolveFirst
  const first = new Promise((resolve) => { resolveFirst = resolve })
  const loadFirst = vi.fn(() => first)
  const loadSecond = vi.fn(async () => ({ route: 2 }))
  const { result, rerender } = renderHook(({ id, load }) => useResource(id, load), { initialProps: { id: 'first', load: loadFirst } })
  expect(result.current.pending).toBe(true)
  rerender({ id: 'second', load: loadSecond })
  await waitFor(() => expect(result.current.data).toEqual({ route: 2 }))
  expect(loadFirst.mock.calls[0][0].aborted).toBe(true)
  await act(async () => { resolveFirst({ route: 1 }); await first })
  expect(result.current.data).toEqual({ route: 2 })
})

it('preserves the last result on refresh failure and supports retry', async () => {
  const load = vi.fn().mockResolvedValueOnce(['scheduled']).mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(['updated'])
  const { result } = renderHook(() => useResource('departures', load))
  await waitFor(() => expect(result.current.data).toEqual(['scheduled']))
  act(() => result.current.retry())
  await waitFor(() => expect(result.current.error?.message).toBe('offline'))
  expect(result.current.data).toEqual(['scheduled'])
  act(() => result.current.retry())
  await waitFor(() => expect(result.current.data).toEqual(['updated']))
  expect(result.current.error).toBe(null)
})
