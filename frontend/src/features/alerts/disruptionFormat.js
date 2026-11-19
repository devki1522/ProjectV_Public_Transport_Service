import { TRANSIT_TIMEZONE } from '../../lib/transit.js'

export const DISRUPTION_TYPE_OPTIONS = [
  { value: 'delay', label: 'Delay' },
  { value: 'detour', label: 'Detour' },
  { value: 'cancellation', label: 'Cancellation' },
  { value: 'stop_closure', label: 'Stop closure' },
  { value: 'road_closure', label: 'Road closure' },
  { value: 'construction', label: 'Construction' },
  { value: 'emergency', label: 'Emergency' },
  { value: 'traffic_restriction', label: 'Traffic restriction' },
  { value: 'accessibility', label: 'Accessibility' },
]

export const DISRUPTION_STATUS_OPTIONS = [
  { value: 'active', label: 'Active' },
  { value: 'scheduled', label: 'Scheduled' },
  { value: 'resolved', label: 'Resolved' },
]

const TYPE_META = {
  delay: { label: 'Delay', className: 'is-severity-medium' },
  detour: { label: 'Detour', className: 'is-severity-medium' },
  cancellation: { label: 'Cancellation', className: 'is-severity-high' },
  stop_closure: { label: 'Stop closure', className: 'is-severity-medium' },
  road_closure: { label: 'Road closure', className: 'is-severity-high' },
  construction: { label: 'Construction', className: 'is-severity-medium' },
  emergency: { label: 'Emergency', className: 'is-severity-high' },
  traffic_restriction: { label: 'Traffic restriction', className: 'is-severity-medium' },
  accessibility: { label: 'Accessibility', className: 'is-severity-info' },
}

const STATUS_META = {
  active: { label: 'Active', pillClass: 'is-disrupted' },
  scheduled: { label: 'Scheduled', pillClass: 'is-scheduled' },
  resolved: { label: 'Resolved', pillClass: 'is-resolved' },
}

const OPERATIONAL_STATUS_META = {
  normal: { label: 'Normal service', pillClass: 'is-active' },
  delayed: { label: 'Delayed', pillClass: 'is-delayed' },
  detour: { label: 'Detoured', pillClass: 'is-delayed' },
  detoured: { label: 'Detoured', pillClass: 'is-delayed' },
  cancellation: { label: 'Cancelled', pillClass: 'is-severity-high' },
  cancelled: { label: 'Cancelled', pillClass: 'is-severity-high' },
  suspended: { label: 'Suspended', pillClass: 'is-severity-high' },
  closed: { label: 'Closed', pillClass: 'is-severity-high' },
  stop_closure: { label: 'Stop closed', pillClass: 'is-severity-high' },
  road_closure: { label: 'Road closed', pillClass: 'is-severity-high' },
  emergency: { label: 'Emergency', pillClass: 'is-severity-high' },
  restricted: { label: 'Restricted', pillClass: 'is-delayed' },
  traffic_restriction: { label: 'Traffic restriction', pillClass: 'is-delayed' },
  construction: { label: 'Construction', pillClass: 'is-delayed' },
  accessibility: { label: 'Accessibility alert', pillClass: 'is-severity-info' },
  disrupted: { label: 'Disrupted', pillClass: 'is-delayed' },
}

const SOURCE_LABELS = {
  utilities: 'Utilities',
  emergency_services: 'Emergency Services',
  security_police: 'Security / Police',
  transit_operations: 'Transit Operations',
  transit: 'Transit Operations',
  admin: 'Transit Administration',
}

const dateTimeFormatter = new Intl.DateTimeFormat('en-CA', {
  timeZone: TRANSIT_TIMEZONE,
  month: 'short',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
})

function humanizeToken(value, fallback = 'Unknown') {
  if (typeof value !== 'string' || !value.trim()) return fallback
  return value
    .trim()
    .replace(/_/g, ' ')
    .replace(/\b\w/g, (char) => char.toUpperCase())
}

export function disruptionTypeLabel(type) {
  return TYPE_META[type]?.label ?? humanizeToken(type, 'Disruption')
}

export function disruptionTypeClass(type) {
  return TYPE_META[type]?.className ?? 'is-severity-medium'
}

export function disruptionStatusLabel(status) {
  return STATUS_META[status]?.label ?? humanizeToken(status, 'Active')
}

export function disruptionStatusPillClass(status) {
  return STATUS_META[status]?.pillClass ?? 'is-disrupted'
}

export function operationalStatusLabel(status) {
  return OPERATIONAL_STATUS_META[status]?.label ?? humanizeToken(status, 'Normal service')
}

export function operationalStatusPillClass(status) {
  return OPERATIONAL_STATUS_META[status]?.pillClass ?? 'is-delayed'
}

export function sourceServiceLabel(source) {
  if (typeof source !== 'string' || !source.trim()) return 'Transit Operations'
  return SOURCE_LABELS[source] ?? humanizeToken(source, 'Transit Operations')
}

export function formatAlertTimestamp(value) {
  if (!value) return '—'
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return '—'
  return dateTimeFormatter.format(parsed)
}

export function formatDisruptionWindow(startTime, endTime) {
  const start = startTime ? formatAlertTimestamp(startTime) : null
  const end = endTime ? formatAlertTimestamp(endTime) : null
  if (start && start !== '—' && end && end !== '—') {
    return `${start} → ${end}`
  }
  if (start && start !== '—') {
    return `Since ${start}`
  }
  if (end && end !== '—') {
    return `Until ${end}`
  }
  return 'Ongoing'
}
