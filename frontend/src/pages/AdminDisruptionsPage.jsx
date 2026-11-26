import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Activity,
  AlertCircle,
  CheckCircle2,
  Edit3,
  MapPin,
  Play,
  Plus,
  RefreshCw,
  Send,
} from 'lucide-react'
import {
  createDisruption,
  getAllDisruptions,
  getAllRoutes,
  getEnterpriseServiceStatus,
  submitRoadClosure,
  updateDisruption,
} from '../api/admin.js'
import { useAuth } from '../auth/authState.js'
import { EmptyState, LoadingRows } from '../components/DataState.jsx'
import { AdminLayout } from '../features/admin/AdminLayout.jsx'
import {
  DISRUPTION_STATUS_OPTIONS,
  DISRUPTION_TYPE_OPTIONS,
  disruptionStatusLabel,
  disruptionStatusPillClass,
  disruptionTypeClass,
  disruptionTypeLabel,
  formatAlertTimestamp,
  formatDisruptionWindow,
  operationalStatusLabel,
  sourceServiceLabel,
} from '../features/alerts/disruptionFormat.js'
import { API_BASE } from '../api/base.js'
import { useResource } from '../hooks/useResource.js'
import { directionLabel, formatTime } from '../lib/transit.js'

const ROAD_CLOSURE_TEMPLATES = [
  {
    label: 'King St N Watermain Closure',
    externalId: 'UTIL-WR-2026-101',
    sourceService: 'utilities',
    status: 'active',
    streetNumber: '200',
    streetName: 'University Ave W',
    city: 'Waterloo',
    postalCode: 'N2L 3G1',
    validUntilSeconds: '7200',
    title: 'King St N watermain road closure',
    description: 'Full northbound road closure near University Ave due to watermain replacement.',
    latitude: '43.4723',
    longitude: '-80.5449',
    radiusMeters: '450',
    routeIds: '7',
    stopIds: '23',
    segments: 'King St N between University Ave and Columbia St',
  },
  {
    label: 'Weber St E Hydro Closure',
    externalId: 'UTIL-WR-2026-102',
    sourceService: 'utilities',
    status: 'active',
    streetNumber: '123',
    streetName: 'Weber St E',
    city: 'Kitchener',
    postalCode: 'N2H 1C4',
    validUntilSeconds: '14400',
    title: 'Weber St E hydro vault closure',
    description: 'Full northbound road closure for emergency underground transformer work.',
    latitude: '43.4516',
    longitude: '-80.4981',
    radiusMeters: '400',
    routeIds: '1',
    stopIds: '21',
    segments: 'Weber St E between Victoria St and Frederick St',
  },
  {
    label: 'Fairway Rd Collision Closure',
    externalId: 'EMS-WR-2026-202',
    sourceService: 'emergency_services',
    status: 'active',
    streetNumber: '595',
    streetName: 'Fairway Rd S',
    city: 'Kitchener',
    postalCode: 'N2C 1X4',
    validUntilSeconds: '3600',
    title: 'Multi-vehicle collision blocking Fairway Rd',
    description: 'Emergency road closure affecting approaches to Fairway Station.',
    latitude: '43.4209',
    longitude: '-80.4388',
    radiusMeters: '500',
    routeIds: '1',
    stopIds: '22',
    segments: 'Fairway Rd S approaching Wilson Ave',
  },
]

const INITIAL_DISRUPTION_FORM = {
  type: 'detour',
  status: 'active',
  title: '',
  description: '',
  affectedArea: '',
  startTime: '',
  endTime: '',
  routeIds: [],
  stopIdsText: '',
  tripIdsText: '',
  vehicleIdsText: '',
  segmentsText: '',
}

const INITIAL_ROAD_CLOSURE_FORM = {
  externalId: '',
  sourceService: '',
  status: 'active',
  streetNumber: '',
  streetName: '',
  city: 'Waterloo',
  postalCode: '',
  validUntilSeconds: '7200',
  title: '',
  description: '',
  latitude: '',
  longitude: '',
  radiusMeters: '400',
  routeIds: '',
  stopIds: '',
  segments: '',
}

function parsePositiveIdList(raw, fieldLabel) {
  if (typeof raw !== 'string' || !raw.trim()) return { ids: [] }
  const tokens = raw
    .split(',')
    .map((part) => part.trim())
    .filter(Boolean)
  const seen = new Set()
  const ids = []
  for (const token of tokens) {
    const parsed = Number(token)
    if (!Number.isInteger(parsed) || parsed <= 0) {
      return { error: `${fieldLabel} must be comma-separated positive whole numbers.` }
    }
    if (!seen.has(parsed)) {
      seen.add(parsed)
      ids.push(parsed)
    }
  }
  return { ids }
}

function parseSegmentsList(raw) {
  if (typeof raw !== 'string' || !raw.trim()) return []
  return raw
    .split(/[\n;]+/)
    .map((item) => item.trim())
    .filter(Boolean)
}

function toLocalDateTimeInput(isoString) {
  if (!isoString) return ''
  const parsed = new Date(isoString)
  if (Number.isNaN(parsed.getTime())) return ''
  const pad = (value) => String(value).padStart(2, '0')
  return `${parsed.getFullYear()}-${pad(parsed.getMonth() + 1)}-${pad(parsed.getDate())}T${pad(parsed.getHours())}:${pad(parsed.getMinutes())}`
}

function validateDisruptionForm(form, isEditing = false) {
  const validTypes = new Set(DISRUPTION_TYPE_OPTIONS.map((item) => item.value))
  if (!validTypes.has(form.type)) {
    return { error: 'Select a valid disruption type.' }
  }
  const validStatuses = new Set(DISRUPTION_STATUS_OPTIONS.map((item) => item.value))
  if (!validStatuses.has(form.status)) {
    return { error: 'Select a valid operational status.' }
  }

  const title = form.title.trim()
  if (!title || [...title].length > 100 || title.includes('\0')) {
    return { error: 'Disruption title is required and must be 100 characters or fewer.' }
  }

  const affectedArea = form.affectedArea.trim()
  if ([...affectedArea].length > 200 || affectedArea.includes('\0')) {
    return { error: 'Affected area must be 200 characters or fewer.' }
  }

  let startIso = null
  if (form.startTime.trim()) {
    const parsedStart = new Date(form.startTime)
    if (Number.isNaN(parsedStart.getTime())) {
      return { error: 'Start time must be a valid date and time.' }
    }
    startIso = parsedStart.toISOString()
  }

  let endIso = null
  if (form.endTime.trim()) {
    const parsedEnd = new Date(form.endTime)
    if (Number.isNaN(parsedEnd.getTime())) {
      return { error: 'End time must be a valid date and time.' }
    }
    endIso = parsedEnd.toISOString()
  }

  if (startIso && endIso && new Date(endIso) < new Date(startIso)) {
    return { error: 'End time cannot be earlier than start time.' }
  }

  const stopParsed = parsePositiveIdList(form.stopIdsText, 'Affected stop IDs')
  if (stopParsed.error) return { error: stopParsed.error }

  const tripParsed = parsePositiveIdList(form.tripIdsText, 'Affected trip IDs')
  if (tripParsed.error) return { error: tripParsed.error }

  const vehicleParsed = parsePositiveIdList(form.vehicleIdsText, 'Affected vehicle IDs')
  if (vehicleParsed.error) return { error: vehicleParsed.error }

  const segments = parseSegmentsList(form.segmentsText)

  const payload = {
    type: form.type,
    status: form.status,
    title,
    route_ids: form.routeIds.map(Number).filter((id) => Number.isInteger(id) && id > 0),
    stop_ids: stopParsed.ids,
    trip_ids: tripParsed.ids,
    vehicle_ids: vehicleParsed.ids,
    affected_segments: segments,
  }
  if (form.description.trim()) payload.description = form.description.trim()
  else payload.description = ''
  if (affectedArea) payload.affected_area = affectedArea
  else payload.affected_area = ''
  if (startIso) payload.start_time = startIso
  if (endIso) payload.end_time = endIso
  else if (isEditing) payload.end_time = null

  return { payload }
}

function validateRoadClosureForm(form) {
  const streetNumber = form.streetNumber.trim()
  const streetName = form.streetName.trim()
  const city = form.city.trim()
  const postalCode = form.postalCode.trim()
  const title = form.title.trim()

  const hasAnyAddressField = Boolean(streetNumber || streetName || postalCode)
  if (hasAnyAddressField && (!streetName || !city)) {
    return { error: 'Street name and city are required when providing an address.' }
  }
  if ((!streetName || !city) && !title) {
    return { error: 'Provide either a street address (street name and city) or a road closure title.' }
  }
  if ([...title].length > 100 || title.includes('\0')) {
    return { error: 'Title must be 100 characters or fewer.' }
  }

  let validUntil = null
  if (form.validUntilSeconds.trim() !== '') {
    const parsed = Number(form.validUntilSeconds)
    if (!Number.isInteger(parsed) || parsed <= 0) {
      return { error: 'Valid-until must be a positive whole number of seconds.' }
    }
    validUntil = parsed
  }

  const routeParsed = parsePositiveIdList(form.routeIds, 'Route IDs')
  if (routeParsed.error) return { error: routeParsed.error }

  const stopParsed = parsePositiveIdList(form.stopIds, 'Stop IDs')
  if (stopParsed.error) return { error: stopParsed.error }

  const hasLat = form.latitude.trim() !== ''
  const hasLng = form.longitude.trim() !== ''
  if (hasLat !== hasLng) {
    return { error: 'Provide both latitude and longitude when using spatial impact matching.' }
  }

  let latitude = null
  let longitude = null
  let radiusMeters = null
  if (hasLat && hasLng) {
    latitude = Number(form.latitude)
    longitude = Number(form.longitude)
    if (!Number.isFinite(latitude) || latitude < -90 || latitude > 90) {
      return { error: 'Latitude must be between -90 and 90.' }
    }
    if (!Number.isFinite(longitude) || longitude < -180 || longitude > 180) {
      return { error: 'Longitude must be between -180 and 180.' }
    }
    if (form.radiusMeters.trim() !== '') {
      radiusMeters = Number(form.radiusMeters)
      if (!Number.isFinite(radiusMeters) || radiusMeters < 50 || radiusMeters > 5000) {
        return { error: 'Impact radius must be between 50 and 5000 meters.' }
      }
    }
  }

  const payload = {
    status: form.status || 'active',
  }
  if (streetName && city) {
    payload.address = {
      streetNumber,
      streetName,
      city,
      postalCode,
    }
  }
  if (validUntil !== null) {
    payload['valid-until'] = validUntil
  }
  if (title) payload.title = title
  if (form.externalId.trim()) payload.external_id = form.externalId.trim()
  if (form.sourceService.trim()) payload.source_service = form.sourceService.trim()
  if (form.description.trim()) payload.description = form.description.trim()
  if (routeParsed.ids.length > 0) payload.route_ids = routeParsed.ids
  if (stopParsed.ids.length > 0) payload.stop_ids = stopParsed.ids

  const segments = parseSegmentsList(form.segments)
  if (segments.length > 0) payload.affected_segments = segments

  if (latitude !== null && longitude !== null) {
    payload.latitude = latitude
    payload.longitude = longitude
    if (radiusMeters !== null) payload.radius_meters = radiusMeters
  }

  return { payload }
}

export function AdminDisruptionsPage() {
  const { csrfToken, handleApiError } = useAuth()
  const routesRes = useResource('admin-disruption-routes', getAllRoutes)
  const loadDisruptions = useCallback((signal) => getAllDisruptions({}, signal), [])
  const disruptionsRes = useResource('admin-disruptions-catalog', loadDisruptions)
  const enterpriseStatusRes = useResource('admin-enterprise-service-status', getEnterpriseServiceStatus)

  const [form, setForm] = useState(INITIAL_DISRUPTION_FORM)
  const [editingId, setEditingId] = useState(null)
  const [formPending, setFormPending] = useState(false)
  const [formError, setFormError] = useState(null)

  const [roadClosureForm, setRoadClosureForm] = useState(INITIAL_ROAD_CLOSURE_FORM)
  const [roadClosurePending, setRoadClosurePending] = useState(false)
  const [roadClosureError, setRoadClosureError] = useState(null)
  const [roadClosureResult, setRoadClosureResult] = useState(null)

  const [statusFilter, setStatusFilter] = useState('all')
  const [typeFilter, setTypeFilter] = useState('all')
  const [rowPendingId, setRowPendingId] = useState(null)

  const [actionNotice, setActionNotice] = useState(null)
  const [actionError, setActionError] = useState(null)

  useEffect(() => {
    for (const err of [routesRes.error, disruptionsRes.error, enterpriseStatusRes.error]) {
      if (err?.status === 401 || err?.status === 403) {
        handleApiError(err)
        break
      }
    }
  }, [routesRes.error, disruptionsRes.error, enterpriseStatusRes.error, handleApiError])

  const routes = useMemo(
    () =>
      [...(routesRes.data ?? [])].sort((a, b) => {
        if (a.is_active !== b.is_active) return a.is_active ? -1 : 1
        const numOrder = a.number.localeCompare(b.number, 'en', { numeric: true })
        return numOrder !== 0 ? numOrder : a.direction.localeCompare(b.direction)
      }),
    [routesRes.data],
  )

  const disruptions = useMemo(() => disruptionsRes.data ?? [], [disruptionsRes.data])

  const filteredDisruptions = useMemo(() => {
    return disruptions.filter((item) => {
      if (statusFilter !== 'all' && item.status !== statusFilter) return false
      if (typeFilter !== 'all' && item.type !== typeFilter) return false
      return true
    })
  }, [disruptions, statusFilter, typeFilter])

  const mutationPending = formPending || roadClosurePending || rowPendingId !== null

  function refreshAll() {
    routesRes.retry()
    disruptionsRes.retry()
    enterpriseStatusRes.retry()
  }

  function handleToggleRouteId(routeId) {
    setForm((current) => {
      const exists = current.routeIds.includes(routeId)
      const nextIds = exists
        ? current.routeIds.filter((id) => id !== routeId)
        : [...current.routeIds, routeId]
      return { ...current, routeIds: nextIds }
    })
  }

  function startEditingDisruption(disruption) {
    if (mutationPending) return
    setEditingId(disruption.id)
    setForm({
      type: disruption.type || 'detour',
      status: disruption.status || 'active',
      title: disruption.title || '',
      description: disruption.description || '',
      affectedArea: disruption.affected_area || '',
      startTime: toLocalDateTimeInput(disruption.start_time),
      endTime: toLocalDateTimeInput(disruption.end_time),
      routeIds: (disruption.affected_routes ?? []).map((r) => r.route_id),
      stopIdsText: (disruption.affected_stops ?? []).map((s) => s.stop_id).join(', '),
      tripIdsText: (disruption.affected_trips ?? []).join(', '),
      vehicleIdsText: (disruption.affected_vehicles ?? []).join(', '),
      segmentsText: (disruption.affected_segments ?? []).join('\n'),
    })
    setFormError(null)
    setActionNotice(null)
    setActionError(null)
  }

  function cancelEditingDisruption() {
    setEditingId(null)
    setForm(INITIAL_DISRUPTION_FORM)
    setFormError(null)
  }

  async function handleDisruptionSubmit(event) {
    event.preventDefault()
    if (mutationPending) return

    const { payload, error: validationError } = validateDisruptionForm(form, editingId !== null)
    if (validationError) {
      setFormError(validationError)
      return
    }

    setFormPending(true)
    setFormError(null)
    setActionNotice(null)
    setActionError(null)

    try {
      if (editingId !== null) {
        const updated = await updateDisruption(editingId, payload, csrfToken)
        setActionNotice(
          `Updated disruption #${updated.id} (${updated.title}) — status: ${disruptionStatusLabel(updated.status)}.`,
        )
        setEditingId(null)
      } else {
        const created = await createDisruption(payload, csrfToken)
        setActionNotice(
          `Created ${disruptionTypeLabel(created.type).toLowerCase()} disruption "${created.title}" (${disruptionStatusLabel(created.status)}).`,
        )
      }
      setForm(INITIAL_DISRUPTION_FORM)
      disruptionsRes.retry()
      enterpriseStatusRes.retry()
    } catch (error) {
      if (error.status === 401 || error.status === 403) {
        handleApiError(error)
        return
      }
      setFormError(error.message)
    } finally {
      setFormPending(false)
    }
  }

  function applyRoadClosureTemplate(template) {
    setRoadClosureForm({
      externalId: template.externalId,
      sourceService: template.sourceService,
      status: template.status,
      streetNumber: template.streetNumber,
      streetName: template.streetName,
      city: template.city,
      postalCode: template.postalCode,
      validUntilSeconds: template.validUntilSeconds,
      title: template.title,
      description: template.description,
      latitude: template.latitude,
      longitude: template.longitude,
      radiusMeters: template.radiusMeters,
      routeIds: template.routeIds,
      stopIds: template.stopIds,
      segments: template.segments,
    })
    setRoadClosureError(null)
  }

  async function handleRoadClosureSubmit(event) {
    event.preventDefault()
    if (mutationPending) return

    const { payload, error: validationError } = validateRoadClosureForm(roadClosureForm)
    if (validationError) {
      setRoadClosureError(validationError)
      return
    }

    setRoadClosurePending(true)
    setRoadClosureError(null)
    setActionNotice(null)
    setActionError(null)

    try {
      const result = await submitRoadClosure(payload, csrfToken)
      setRoadClosureResult(result)
      setActionNotice(
        `Recorded road closure #${result.id} "${result.title}" (${disruptionStatusLabel(result.status)}).`,
      )
      disruptionsRes.retry()
      enterpriseStatusRes.retry()
    } catch (error) {
      if (error.status === 401 || error.status === 403) {
        handleApiError(error)
        return
      }
      setRoadClosureError(error.message)
    } finally {
      setRoadClosurePending(false)
    }
  }

  async function handleQuickStatusChange(disruption, nextStatus) {
    if (mutationPending) return
    setRowPendingId(disruption.id)
    setActionNotice(null)
    setActionError(null)

    try {
      const updated = await updateDisruption(disruption.id, { status: nextStatus }, csrfToken)
      setActionNotice(
        `Marked disruption #${updated.id} (${updated.title}) as ${disruptionStatusLabel(updated.status)}.`,
      )
      if (editingId === disruption.id) {
        setForm((current) => ({ ...current, status: updated.status }))
      }
      disruptionsRes.retry()
      enterpriseStatusRes.retry()
    } catch (error) {
      if (error.status === 401 || error.status === 403) {
        handleApiError(error)
        return
      }
      setActionError(error.message)
    } finally {
      setRowPendingId(null)
    }
  }

  const enterpriseData = enterpriseStatusRes.data
  const enterpriseDisrupted =
    enterpriseData?.overall_status === 'disrupted' ||
    (enterpriseData?.active_disruption_count ?? 0) > 0
  const visibleEnterpriseError =
    enterpriseStatusRes.error &&
    enterpriseStatusRes.error.status !== 401 &&
    enterpriseStatusRes.error.status !== 403
      ? enterpriseStatusRes.error
      : null
  const visibleDisruptionsError =
    disruptionsRes.error &&
    disruptionsRes.error.status !== 401 &&
    disruptionsRes.error.status !== 403
      ? disruptionsRes.error
      : null

  return (
    <AdminLayout title="Disruptions">
      <div className="page-heading admin-page-heading">
        <div>
          <h1>Transit disruptions &amp; incident feeds</h1>
          <p>
            Manage service disruptions, operational statuses, and external incident feeds from Utilities, Emergency Services, and Security/Police.
          </p>
        </div>
        <button
          className="button button-secondary"
          type="button"
          onClick={refreshAll}
          disabled={disruptionsRes.pending || enterpriseStatusRes.pending}
        >
          <RefreshCw size={16} aria-hidden="true" />
          <span>Refresh</span>
        </button>
      </div>

      {actionNotice && (
        <div className="success-notice" role="status">
          <CheckCircle2 size={18} aria-hidden="true" />
          <span>{actionNotice}</span>
        </div>
      )}

      {actionError && (
        <div className="error-notice" role="alert">
          <AlertCircle size={20} aria-hidden="true" />
          <div>
            <strong>Disruption action failed</strong>
            <p>{actionError}</p>
          </div>
        </div>
      )}

      <section
        className="admin-panel enterprise-status-card"
        aria-labelledby="enterprise-status-heading"
      >
        <div className="simulator-card-header">
          <div>
            <div className="simulator-title-row">
              <h2 id="enterprise-status-heading">Enterprise service status snapshot (US 196)</h2>
              {enterpriseData && (
                <span className={`status-pill ${enterpriseDisrupted ? 'is-disrupted' : 'is-active'}`}>
                  {enterpriseDisrupted ? 'Disrupted' : 'Normal'}
                </span>
              )}
            </div>
            <p>
              Real-time operational status summary from <code>{`GET ${API_BASE}/integrations/service-status`}</code> for smart-city enterprise consumers.
            </p>
          </div>

          <button
            type="button"
            className="button button-secondary"
            onClick={enterpriseStatusRes.retry}
            disabled={enterpriseStatusRes.pending}
          >
            <Activity size={15} aria-hidden="true" />
            <span>{enterpriseStatusRes.pending ? 'Refreshing snapshot…' : 'Refresh snapshot'}</span>
          </button>
        </div>

        {visibleEnterpriseError ? (
          <div className="error-notice" role="alert">
            <AlertCircle size={20} aria-hidden="true" />
            <div>
              <strong>Enterprise service status couldn’t be loaded</strong>
              <p>{visibleEnterpriseError.message}</p>
            </div>
            <button className="button button-secondary" type="button" onClick={enterpriseStatusRes.retry}>
              <RefreshCw size={16} aria-hidden="true" />
              <span>Try again</span>
            </button>
          </div>
        ) : enterpriseStatusRes.pending && !enterpriseData ? (
          <LoadingRows label="Loading enterprise service status" />
        ) : enterpriseData ? (
          <>
            <div className="enterprise-metrics-grid">
              <div className="metric-card">
                <span className="metric-label">Active disruptions</span>
                <strong className="metric-value metric-value-accent">
                  {enterpriseData.active_disruption_count ?? enterpriseData.active_disruptions?.length ?? 0}
                </strong>
                <span className="metric-hint">
                  Overall status: {enterpriseData.overall_status ?? 'normal'}
                </span>
              </div>
              <div className="metric-card">
                <span className="metric-label">Active detours</span>
                <strong className="metric-value">
                  {enterpriseData.detours?.length ?? 0}
                </strong>
                <span className="metric-hint">Corridor detours in effect</span>
              </div>
              <div className="metric-card">
                <span className="metric-label">Affected routes</span>
                <strong className="metric-value">
                  {enterpriseData.affected_routes?.length ?? 0}
                </strong>
                <span className="metric-hint">Directional routes impacted</span>
              </div>
              <div className="metric-card">
                <span className="metric-label">Affected stops</span>
                <strong className="metric-value">
                  {enterpriseData.affected_stops?.length ?? 0}
                </strong>
                <span className="metric-hint">
                  {enterpriseData.generated_at
                    ? `Snapshot at ${formatTime(enterpriseData.generated_at)}`
                    : 'Live snapshot'}
                </span>
              </div>
            </div>

            {((enterpriseData.affected_routes?.length ?? 0) > 0 ||
              (enterpriseData.affected_stops?.length ?? 0) > 0) && (
              <div className="enterprise-affected-strip">
                {(enterpriseData.affected_routes ?? []).map((route) => (
                  <span key={`ent-route-${route.route_id}`} className="alert-chip alert-chip-route">
                    Route {route.route_number} ({route.route_name}) · {operationalStatusLabel(route.operational_status)}
                  </span>
                ))}
                {(enterpriseData.affected_stops ?? []).map((stop) => (
                  <span key={`ent-stop-${stop.stop_id}`} className="alert-chip alert-chip-stop">
                    Stop #{stop.stop_id} {stop.stop_name} · {operationalStatusLabel(stop.operational_status)}
                  </span>
                ))}
              </div>
            )}
          </>
        ) : null}
      </section>

      <div className="admin-disruptions-grid">
        <section className="admin-panel" aria-labelledby="disruption-form-heading">
          <div className="admin-panel-header">
            <div className="admin-edit-header">
              <h2 id="disruption-form-heading">
                {editingId !== null ? `Edit disruption #${editingId}` : 'Create service disruption (US 190)'}
              </h2>
              {editingId !== null && (
                <button
                  type="button"
                  className="text-button"
                  onClick={cancelEditingDisruption}
                  disabled={mutationPending}
                >
                  Cancel editing
                </button>
              )}
            </div>
            <p>
              Publish or update transit delays, detours, cancellations, and stop closures with affected routes, stops, trips, vehicles, and segments (US 191, US 192).
            </p>
          </div>

          {formError && (
            <div className="error-notice auth-error" role="alert">
              <AlertCircle size={20} aria-hidden="true" />
              <div>
                <strong>Could not save disruption</strong>
                <p>{formError}</p>
              </div>
            </div>
          )}

          <form className="admin-form" onSubmit={handleDisruptionSubmit} noValidate>
            <div className="admin-vehicle-form-row">
              <div className="field">
                <label htmlFor="disruption-type">Disruption type</label>
                <select
                  id="disruption-type"
                  value={form.type}
                  onChange={(event) => setForm((f) => ({ ...f, type: event.target.value }))}
                  disabled={mutationPending}
                >
                  {DISRUPTION_TYPE_OPTIONS.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </select>
              </div>

              <div className="field">
                <label htmlFor="disruption-status">Operational status</label>
                <select
                  id="disruption-status"
                  value={form.status}
                  onChange={(event) => setForm((f) => ({ ...f, status: event.target.value }))}
                  disabled={mutationPending}
                >
                  {DISRUPTION_STATUS_OPTIONS.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </select>
              </div>
            </div>

            <div className="field">
              <label htmlFor="disruption-title">Title</label>
              <input
                id="disruption-title"
                type="text"
                maxLength={100}
                placeholder="e.g. Route 7 detour via Weber St"
                value={form.title}
                onChange={(event) => setForm((f) => ({ ...f, title: event.target.value }))}
                disabled={mutationPending}
                required
              />
            </div>

            <div className="field">
              <label htmlFor="disruption-description">Description</label>
              <textarea
                id="disruption-description"
                rows={2}
                placeholder="Describe rider impact, alternate stops, or detour path…"
                value={form.description}
                onChange={(event) => setForm((f) => ({ ...f, description: event.target.value }))}
                disabled={mutationPending}
              />
            </div>

            <div className="field">
              <label htmlFor="disruption-affected-area">Affected area</label>
              <input
                id="disruption-affected-area"
                type="text"
                maxLength={200}
                placeholder="e.g. King St W between Victoria St and Francis St"
                value={form.affectedArea}
                onChange={(event) => setForm((f) => ({ ...f, affectedArea: event.target.value }))}
                disabled={mutationPending}
              />
            </div>

            <div className="admin-vehicle-form-row">
              <div className="field">
                <label htmlFor="disruption-start-time">Start time (optional)</label>
                <input
                  id="disruption-start-time"
                  type="datetime-local"
                  value={form.startTime}
                  onChange={(event) => setForm((f) => ({ ...f, startTime: event.target.value }))}
                  disabled={mutationPending}
                />
              </div>

              <div className="field">
                <label htmlFor="disruption-end-time">End time (optional)</label>
                <input
                  id="disruption-end-time"
                  type="datetime-local"
                  value={form.endTime}
                  onChange={(event) => setForm((f) => ({ ...f, endTime: event.target.value }))}
                  disabled={mutationPending}
                />
              </div>
            </div>

            <fieldset className="route-checkbox-fieldset" disabled={mutationPending}>
              <legend>Affected routes</legend>
              {routes.length === 0 ? (
                <p className="affected-empty-note">No routes loaded yet.</p>
              ) : (
                <div className="route-checkbox-list">
                  {routes.map((route) => {
                    const checked = form.routeIds.includes(route.id)
                    return (
                      <label key={route.id} className="checkbox-field route-checkbox-item">
                        <input
                          type="checkbox"
                          checked={checked}
                          onChange={() => handleToggleRouteId(route.id)}
                        />
                        <span>
                          Route {route.number} · {route.name} ({directionLabel(route.direction)})
                        </span>
                      </label>
                    )
                  })}
                </div>
              )}
            </fieldset>

            <div className="admin-vehicle-form-row">
              <div className="field">
                <label htmlFor="disruption-stop-ids">Affected stop IDs (optional)</label>
                <input
                  id="disruption-stop-ids"
                  type="text"
                  placeholder="e.g. 21, 22"
                  value={form.stopIdsText}
                  onChange={(event) => setForm((f) => ({ ...f, stopIdsText: event.target.value }))}
                  disabled={mutationPending}
                />
              </div>

              <div className="field">
                <label htmlFor="disruption-trip-ids">Affected trip IDs (optional)</label>
                <input
                  id="disruption-trip-ids"
                  type="text"
                  placeholder="e.g. 100, 101"
                  value={form.tripIdsText}
                  onChange={(event) => setForm((f) => ({ ...f, tripIdsText: event.target.value }))}
                  disabled={mutationPending}
                />
              </div>
            </div>

            <div className="admin-vehicle-form-row">
              <div className="field">
                <label htmlFor="disruption-vehicle-ids">Affected vehicle IDs (optional)</label>
                <input
                  id="disruption-vehicle-ids"
                  type="text"
                  placeholder="e.g. 501"
                  value={form.vehicleIdsText}
                  onChange={(event) => setForm((f) => ({ ...f, vehicleIdsText: event.target.value }))}
                  disabled={mutationPending}
                />
              </div>

              <div className="field">
                <label htmlFor="disruption-segments">Affected route segments (optional)</label>
                <textarea
                  id="disruption-segments"
                  rows={2}
                  placeholder="e.g. King St N between University & Columbia"
                  value={form.segmentsText}
                  onChange={(event) => setForm((f) => ({ ...f, segmentsText: event.target.value }))}
                  disabled={mutationPending}
                />
              </div>
            </div>

            <button className="button button-primary" type="submit" disabled={mutationPending}>
              <Plus size={15} aria-hidden="true" />
              <span>
                {formPending
                  ? 'Saving disruption…'
                  : editingId !== null
                    ? 'Save disruption'
                    : 'Create disruption'}
              </span>
            </button>
          </form>
        </section>

        <section className="admin-panel" aria-labelledby="road-closure-intake-heading">
          <div className="admin-panel-header">
            <h2 id="road-closure-intake-heading">Road closure intake</h2>
            <p>
              Submit municipal road closures via <code>{`POST ${API_BASE}/road-closures`}</code> with SH-I06 addresses and <code>valid-until</code> duration.
            </p>
          </div>

          <div className="field feed-template-bar">
            <span className="preset-label">Waterloo Region 1-click sample templates</span>
            <div className="preset-buttons" role="group" aria-label="Waterloo Region sample templates">
              {ROAD_CLOSURE_TEMPLATES.map((template) => (
                <button
                  key={template.label}
                  type="button"
                  className="button button-secondary button-compact preset-button"
                  onClick={() => applyRoadClosureTemplate(template)}
                  disabled={mutationPending}
                >
                  <MapPin size={13} aria-hidden="true" />
                  <span>{template.label}</span>
                </button>
              ))}
            </div>
          </div>

          {roadClosureError && (
            <div className="error-notice auth-error" role="alert">
              <AlertCircle size={20} aria-hidden="true" />
              <div>
                <strong>Could not submit road closure</strong>
                <p>{roadClosureError}</p>
              </div>
            </div>
          )}

          <form className="admin-form" onSubmit={handleRoadClosureSubmit} noValidate>
            <div className="admin-vehicle-form-row">
              <div className="field">
                <label htmlFor="rc-street-number">Street number</label>
                <input
                  id="rc-street-number"
                  type="text"
                  maxLength={20}
                  placeholder="e.g. 123"
                  value={roadClosureForm.streetNumber}
                  onChange={(event) => setRoadClosureForm((f) => ({ ...f, streetNumber: event.target.value }))}
                  disabled={mutationPending}
                />
              </div>

              <div className="field">
                <label htmlFor="rc-street-name">Street name</label>
                <input
                  id="rc-street-name"
                  type="text"
                  maxLength={120}
                  placeholder="e.g. King St N"
                  value={roadClosureForm.streetName}
                  onChange={(event) => setRoadClosureForm((f) => ({ ...f, streetName: event.target.value }))}
                  disabled={mutationPending}
                />
              </div>
            </div>

            <div className="admin-vehicle-form-row">
              <div className="field">
                <label htmlFor="rc-city">City</label>
                <input
                  id="rc-city"
                  type="text"
                  maxLength={80}
                  placeholder="e.g. Waterloo"
                  value={roadClosureForm.city}
                  onChange={(event) => setRoadClosureForm((f) => ({ ...f, city: event.target.value }))}
                  disabled={mutationPending}
                />
              </div>

              <div className="field">
                <label htmlFor="rc-postal-code">Postal code</label>
                <input
                  id="rc-postal-code"
                  type="text"
                  maxLength={16}
                  placeholder="e.g. N2J 2X7"
                  value={roadClosureForm.postalCode}
                  onChange={(event) => setRoadClosureForm((f) => ({ ...f, postalCode: event.target.value }))}
                  disabled={mutationPending}
                />
              </div>
            </div>

            <div className="admin-vehicle-form-row">
              <div className="field">
                <label htmlFor="rc-valid-until">Valid until (seconds)</label>
                <input
                  id="rc-valid-until"
                  type="number"
                  min="1"
                  step="1"
                  placeholder="7200"
                  value={roadClosureForm.validUntilSeconds}
                  onChange={(event) => setRoadClosureForm((f) => ({ ...f, validUntilSeconds: event.target.value }))}
                  disabled={mutationPending}
                />
              </div>

              <div className="field">
                <label htmlFor="rc-status">Closure status</label>
                <select
                  id="rc-status"
                  value={roadClosureForm.status}
                  onChange={(event) => setRoadClosureForm((f) => ({ ...f, status: event.target.value }))}
                  disabled={mutationPending}
                >
                  {DISRUPTION_STATUS_OPTIONS.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </select>
              </div>
            </div>

            <div className="admin-vehicle-form-row">
              <div className="field">
                <label htmlFor="rc-external-id">External reference ID (optional)</label>
                <input
                  id="rc-external-id"
                  type="text"
                  maxLength={120}
                  placeholder="e.g. UTIL-WR-2026-101"
                  value={roadClosureForm.externalId}
                  onChange={(event) => setRoadClosureForm((f) => ({ ...f, externalId: event.target.value }))}
                  disabled={mutationPending}
                />
              </div>

              <div className="field">
                <label htmlFor="rc-source-service">Source service (optional)</label>
                <input
                  id="rc-source-service"
                  type="text"
                  maxLength={50}
                  placeholder="e.g. utilities, emergency_services"
                  value={roadClosureForm.sourceService}
                  onChange={(event) => setRoadClosureForm((f) => ({ ...f, sourceService: event.target.value }))}
                  disabled={mutationPending}
                />
              </div>
            </div>

            <div className="field">
              <label htmlFor="rc-title">Title (optional if address is provided)</label>
              <input
                id="rc-title"
                type="text"
                maxLength={100}
                placeholder="e.g. King St N watermain road closure"
                value={roadClosureForm.title}
                onChange={(event) => setRoadClosureForm((f) => ({ ...f, title: event.target.value }))}
                disabled={mutationPending}
              />
            </div>

            <div className="field">
              <label htmlFor="rc-description">Description (optional)</label>
              <textarea
                id="rc-description"
                rows={2}
                placeholder="Operational details about the road closure…"
                value={roadClosureForm.description}
                onChange={(event) => setRoadClosureForm((f) => ({ ...f, description: event.target.value }))}
                disabled={mutationPending}
              />
            </div>

            <div className="admin-feed-coords-row">
              <div className="field">
                <label htmlFor="rc-latitude">Latitude (optional)</label>
                <input
                  id="rc-latitude"
                  type="number"
                  step="any"
                  placeholder="43.4723"
                  value={roadClosureForm.latitude}
                  onChange={(event) => setRoadClosureForm((f) => ({ ...f, latitude: event.target.value }))}
                  disabled={mutationPending}
                />
              </div>

              <div className="field">
                <label htmlFor="rc-longitude">Longitude (optional)</label>
                <input
                  id="rc-longitude"
                  type="number"
                  step="any"
                  placeholder="-80.5449"
                  value={roadClosureForm.longitude}
                  onChange={(event) => setRoadClosureForm((f) => ({ ...f, longitude: event.target.value }))}
                  disabled={mutationPending}
                />
              </div>

              <div className="field">
                <label htmlFor="rc-radius">Impact radius (m)</label>
                <input
                  id="rc-radius"
                  type="number"
                  min="50"
                  max="5000"
                  step="50"
                  placeholder="400"
                  value={roadClosureForm.radiusMeters}
                  onChange={(event) => setRoadClosureForm((f) => ({ ...f, radiusMeters: event.target.value }))}
                  disabled={mutationPending}
                />
              </div>
            </div>

            <div className="admin-vehicle-form-row">
              <div className="field">
                <label htmlFor="rc-route-ids">Route IDs (optional)</label>
                <input
                  id="rc-route-ids"
                  type="text"
                  placeholder="e.g. 1, 7"
                  value={roadClosureForm.routeIds}
                  onChange={(event) => setRoadClosureForm((f) => ({ ...f, routeIds: event.target.value }))}
                  disabled={mutationPending}
                />
              </div>

              <div className="field">
                <label htmlFor="rc-stop-ids">Stop IDs (optional)</label>
                <input
                  id="rc-stop-ids"
                  type="text"
                  placeholder="e.g. 21, 23"
                  value={roadClosureForm.stopIds}
                  onChange={(event) => setRoadClosureForm((f) => ({ ...f, stopIds: event.target.value }))}
                  disabled={mutationPending}
                />
              </div>
            </div>

            <div className="field">
              <label htmlFor="rc-segments">Affected segments (optional)</label>
              <input
                id="rc-segments"
                type="text"
                placeholder="e.g. King St N between University & Columbia"
                value={roadClosureForm.segments}
                onChange={(event) => setRoadClosureForm((f) => ({ ...f, segments: event.target.value }))}
                disabled={mutationPending}
              />
            </div>

            <button className="button button-primary" type="submit" disabled={mutationPending}>
              <Send size={15} aria-hidden="true" />
              <span>{roadClosurePending ? 'Submitting road closure…' : 'Submit road closure'}</span>
            </button>
          </form>

          {roadClosureResult && (
            <div
              className="feed-result-card"
              role="status"
              aria-label="Road closure result"
            >
              <div className="feed-result-header">
                <strong>
                  Recorded road closure{roadClosureResult.id ? ` #${roadClosureResult.id}` : ''}
                </strong>
                <span className="status-pill is-active">
                  {sourceServiceLabel(roadClosureResult.source_service)}
                </span>
              </div>
              <p className="feed-result-impact">
                {roadClosureResult.title}
                {roadClosureResult.affected_area ? ` · ${roadClosureResult.affected_area}` : ''}
                {' · '}
                {formatDisruptionWindow(roadClosureResult.start_time, roadClosureResult.end_time)}
              </p>
              <div className="alert-chips">
                <span className="alert-chip alert-chip-route">
                  Affected routes:{' '}
                  {(roadClosureResult.affected_routes ?? []).length > 0
                    ? roadClosureResult.affected_routes.map((r) => r.route_number || r.route_id).join(', ')
                    : 'None'}
                </span>
                <span className="alert-chip alert-chip-stop">
                  Affected stops:{' '}
                  {(roadClosureResult.affected_stops ?? []).length > 0
                    ? roadClosureResult.affected_stops.map((s) => s.stop_name || s.stop_id).join(', ')
                    : 'None'}
                </span>
                {roadClosureResult.received_at && (
                  <span className="alert-chip">
                    Received {formatAlertTimestamp(roadClosureResult.received_at)}
                  </span>
                )}
              </div>
            </div>
          )}
        </section>
      </div>

      <section className="admin-panel disruptions-catalog-panel" aria-labelledby="disruptions-table-heading">
        <div className="admin-catalog-toolbar">
          <div>
            <h2 id="disruptions-table-heading">Disruptions management</h2>
            <p>
              {disruptions.length} total {disruptions.length === 1 ? 'disruption' : 'disruptions'} (
              {disruptions.filter((d) => d.status === 'active').length} active)
            </p>
          </div>

          <div className="admin-filter-controls">
            <select
              aria-label="Filter disruptions by status"
              value={statusFilter}
              onChange={(event) => setStatusFilter(event.target.value)}
            >
              <option value="all">All statuses</option>
              {DISRUPTION_STATUS_OPTIONS.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>

            <select
              aria-label="Filter disruptions by type"
              value={typeFilter}
              onChange={(event) => setTypeFilter(event.target.value)}
            >
              <option value="all">All types</option>
              {DISRUPTION_TYPE_OPTIONS.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
          </div>
        </div>

        {visibleDisruptionsError ? (
          <div className="error-notice" role="alert">
            <AlertCircle size={20} aria-hidden="true" />
            <div>
              <strong>Disruptions couldn’t be loaded</strong>
              <p>{visibleDisruptionsError.message}</p>
            </div>
            <button className="button button-secondary" type="button" onClick={disruptionsRes.retry}>
              <RefreshCw size={16} aria-hidden="true" />
              <span>Try again</span>
            </button>
          </div>
        ) : disruptionsRes.pending && disruptions.length === 0 ? (
          <LoadingRows label="Loading transit disruptions" />
        ) : filteredDisruptions.length === 0 ? (
          <EmptyState
            title="No disruptions match this filter"
            description="Create a service disruption or submit an external notice to populate the disruptions table."
          />
        ) : (
          <div className="admin-table-wrap">
            <table className="admin-table">
              <thead>
                <tr>
                  <th>Disruption</th>
                  <th>Status</th>
                  <th>Source &amp; received</th>
                  <th>Affected services</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {filteredDisruptions.map((disruption) => (
                  <tr key={disruption.id}>
                    <td>
                      <div className="admin-disruption-cell">
                        <div className="alert-card-pills">
                          <span className={`status-pill ${disruptionTypeClass(disruption.type)}`}>
                            {disruptionTypeLabel(disruption.type)}
                          </span>
                          <small className="muted-code">ID {disruption.id}</small>
                        </div>
                        <strong>{disruption.title}</strong>
                        {disruption.affected_area && (
                          <small className="muted-block">{disruption.affected_area}</small>
                        )}
                        <small className="muted-block">
                          {formatDisruptionWindow(disruption.start_time, disruption.end_time)}
                        </small>
                      </div>
                    </td>
                    <td>
                      <span className={`status-pill ${disruptionStatusPillClass(disruption.status)}`}>
                        {disruptionStatusLabel(disruption.status)}
                      </span>
                    </td>
                    <td>
                      <div>
                        <strong>{sourceServiceLabel(disruption.source_service)}</strong>
                        {disruption.external_reference && (
                          <small className="muted-block">Ref: {disruption.external_reference}</small>
                        )}
                        {disruption.received_at && (
                          <small className="muted-block">
                            Received {formatAlertTimestamp(disruption.received_at)}
                          </small>
                        )}
                      </div>
                    </td>
                    <td>
                      <div className="alert-chips">
                        {(disruption.affected_routes ?? []).map((route) => (
                          <span key={`tbl-route-${route.route_id}`} className="alert-chip alert-chip-route">
                            Route {route.route_number}
                          </span>
                        ))}
                        {(disruption.affected_stops ?? []).map((stop) => (
                          <span key={`tbl-stop-${stop.stop_id}-${stop.route_id ?? 'any'}`} className="alert-chip alert-chip-stop">
                            {stop.stop_name}
                          </span>
                        ))}
                        {(disruption.affected_trips ?? []).map((tripId) => (
                          <span key={`tbl-trip-${tripId}`} className="alert-chip">
                            Trip {tripId}
                          </span>
                        ))}
                        {(disruption.affected_vehicles ?? []).map((vehicleId) => (
                          <span key={`tbl-veh-${vehicleId}`} className="alert-chip">
                            Vehicle #{vehicleId}
                          </span>
                        ))}
                        {(disruption.affected_segments ?? []).map((seg, idx) => (
                          <span key={`tbl-seg-${idx}`} className="alert-chip alert-chip-segment">
                            {seg}
                          </span>
                        ))}
                      </div>
                    </td>
                    <td>
                      <div className="table-actions">
                        {disruption.status !== 'active' && (
                          <button
                            type="button"
                            className="button button-secondary button-compact"
                            onClick={() => handleQuickStatusChange(disruption, 'active')}
                            disabled={mutationPending}
                          >
                            <Play size={13} aria-hidden="true" />
                            <span>Activate</span>
                          </button>
                        )}
                        {disruption.status !== 'resolved' && (
                          <button
                            type="button"
                            className="button button-secondary button-compact"
                            onClick={() => handleQuickStatusChange(disruption, 'resolved')}
                            disabled={mutationPending}
                          >
                            <CheckCircle2 size={13} aria-hidden="true" />
                            <span>Resolve</span>
                          </button>
                        )}
                        <button
                          type="button"
                          className="button button-secondary button-compact"
                          onClick={() => startEditingDisruption(disruption)}
                          disabled={mutationPending}
                        >
                          <Edit3 size={13} aria-hidden="true" />
                          <span>Edit</span>
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </AdminLayout>
  )
}
