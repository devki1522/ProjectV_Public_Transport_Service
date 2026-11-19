import { useEffect, useState } from 'react'
import { AlertTriangle } from 'lucide-react'
import { formatTime } from '../lib/transit.js'

export function FeedFreshness({ updatedAt, maxAge = 90_000, paused = false }) {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!updatedAt || paused) return
    const timer = setInterval(() => setNow(Date.now()), 15_000)
    return () => clearInterval(timer)
  }, [updatedAt, paused])
  const stale = !paused && updatedAt && now - updatedAt.getTime() > maxAge
  return <div className="feed-freshness" role="status">{stale && <p className="feed-notice"><AlertTriangle size={17} aria-hidden="true" />Live data may be out of date. Last update {formatTime(updatedAt)}. Refresh to check again.</p>}</div>
}
