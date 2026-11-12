import { AlertCircle, CalendarDays, RefreshCw } from 'lucide-react'

export function ErrorNotice({ error, onRetry }) {
  return (
    <div className="error-notice" role="alert">
      <AlertCircle size={20} aria-hidden="true" />
      <div><strong>Transit information couldn’t be loaded</strong><p>{error.message}</p></div>
      <button className="button button-secondary" type="button" onClick={onRetry}><RefreshCw size={16} aria-hidden="true" />Try again</button>
    </div>
  )
}

export function EmptyState({ title, description, onRetry }) {
  return (
    <div className="empty-state" role="status">
      <CalendarDays size={30} aria-hidden="true" />
      <h3>{title}</h3><p>{description}</p>
      {onRetry && <button className="button button-secondary" type="button" onClick={onRetry}>Try again</button>}
    </div>
  )
}

export function LoadingRows({ label = 'Loading transit information' }) {
  return (
    <div className="loading-rows" role="status" aria-label={label}>
      <span className="visually-hidden">{label}</span>
      {[0, 1, 2, 3].map((row) => <div className="skeleton-row" key={row} aria-hidden="true"><span /><span /><span /></div>)}
    </div>
  )
}
