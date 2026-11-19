import { routeColor } from '../lib/routeColors.js'

export function RouteBadge({ number, large = false, color }) {
  return <span className={`route-number${large ? ' route-number-large' : ''}`} style={{ '--route-color': color ?? routeColor(number) }}>{number}</span>
}
