export const VEHICLE_STATUS_OPTIONS = [
  { value: 'in_service', label: 'In service' },
  { value: 'delayed', label: 'Delayed' },
  { value: 'layover', label: 'Layover' },
  { value: 'out_of_service', label: 'Out of service' },
]

const STATUS_META = {
  in_service: { label: 'In service', className: 'is-in-service', pillClass: 'is-active' },
  delayed: { label: 'Delayed', className: 'is-delayed', pillClass: 'is-delayed' },
  layover: { label: 'Layover', className: 'is-layover', pillClass: 'is-layover' },
  out_of_service: { label: 'Out of service', className: 'is-out-of-service', pillClass: 'is-inactive' },
}

export function vehicleStatusLabel(status) {
  return STATUS_META[status]?.label ?? 'Active'
}

export function vehicleStatusClass(status) {
  return STATUS_META[status]?.className ?? 'is-in-service'
}

export function vehicleStatusPillClass(status) {
  return STATUS_META[status]?.pillClass ?? 'is-active'
}

export function formatDistance(meters) {
  if (typeof meters !== 'number' || !Number.isFinite(meters) || meters < 0) return null
  if (meters >= 1000) return `${(meters / 1000).toFixed(1)} km`
  return `${Math.round(meters)} m`
}

export function formatDelayMinutes(delayMinutes) {
  if (typeof delayMinutes !== 'number' || !Number.isFinite(delayMinutes)) return null
  if (delayMinutes > 0) return `${delayMinutes} min later`
  if (delayMinutes < 0) return `${Math.abs(delayMinutes)} min earlier`
  return 'On time'
}
