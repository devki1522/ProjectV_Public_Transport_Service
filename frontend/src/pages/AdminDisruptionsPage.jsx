import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Activity,
  AlertCircle,
  CheckCircle2,
  Edit3,
  Flame,
  MapPin,
  Play,
  Plus,
  RefreshCw,
  Send,
  ShieldAlert,
  Wrench,
} from 'lucide-react'
import {
  createDisruption,
  getAllDisruptions,
  getAllRoutes,
  getEnterpriseServiceStatus,
  submitEmergencyIncident,
  submitSecurityTrafficAlert,
  submitUtilitiesNotice,
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
import { useResource } from '../hooks/useResource.js'
import { directionLabel, formatTime } from '../lib/transit.js'

const FEED_TABS = [
  {
    id: 'utilities',
    label: 'Utilities (US 186)',
    endpoint: 'POST /api/integrations/utilities/notices',
    submitLabel: 'Submit utilities notice',
    subtypeLabel: 'Notice type',
    subtypes: [
      { value: 'construction', label: 'Construction' },
      { value: 'road_closure', label: 'Road closure' },
    ],
  },
  {
    id: 'emergency',
    label: 'Emergency Services (US 187)',
    endpoint: 'POST /api/integrations/emergency/incidents',
    submitLabel: 'Submit emergency incident',
    subtypeLabel: 'Incident type',
    subtypes: [
      { value: 'emergency', label: 'Emergency' },
      { value: 'road_closure', label: 'Road closure' },
    ],
  },
  {
    id: 'security',
    label: 'Security / Police (US 188)',
    endpoint: 'POST /api/integrations/security/traffic-alerts',
    submitLabel: 'Submit security traffic alert',
    subtypeLabel: 'Alert type',
    subtypes: [
      { value: 'traffic_restriction', label: 'Traffic restriction' },
      { value: 'road_closure', label: 'Road closure' },
    ],
  },
]

const FEED_TEMPLATES = {
  utilities: [
    {
      label: 'King St Watermain Repair',
      externalId: 'UTIL-WR-2026-101',
      subtype: 'construction',
      severity: 'medium',
      status: 'active',
      title: 'King St N watermain construction',
      description: 'Lane reductions and transit detour near University Ave due to watermain replacement.',
      affectedArea: 'King St N & University Ave, Waterloo',
      latitude: '43.4723',
      longitude: '-80.5449',
      radiusMeters: '450',
      routeIds: '7',
      stopIds: '23',
      tripIds: '',
      segments: 'King St N between University Ave and Columbia St',
    },
    {
      label: 'Weber St Hydro Closure',
      externalId: 'UTIL-WR-2026-102',
      subtype: 'road_closure',
      severity: 'high',
      status: 'active',
      title: 'Weber St E hydro vault closure',
      description: 'Full northbound road closure for emergency underground transformer work.',
      affectedArea: 'Weber St E & Victoria St, Kitchener',
      latitude: '43.4516',
      longitude: '-80.4981',
      radiusMeters: '400',
      routeIds: '1',
      stopIds: '21',
      tripIds: '',
      segments: 'Weber St E between Victoria St and Frederick St',
    },
  ],
  emergency: [
    {
      label: 'Central Station Fire Response',
      externalId: 'EMS-WR-2026-201',
      subtype: 'emergency',
      severity: 'high',
      status: 'active',
      title: 'Emergency response near Central Station',
      description: 'Fire and paramedic units staging on King St W; buses detouring around Central Station.',
      affectedArea: 'King St W & Victoria St, Kitchener',
      latitude: '43.4516',
      longitude: '-80.4981',
      radiusMeters: '400',
      routeIds: '1, 7',
      stopIds: '21',
      tripIds: '100',
      segments: 'King St W at Victoria St',
    },
    {
      label: 'Fairway Rd Collision Closure',
      externalId: 'EMS-WR-2026-202',
      subtype: 'road_closure',
      severity: 'critical',
      status: 'active',
      title: 'Multi-vehicle collision blocking Fairway Rd',
      description: 'Emergency road restriction affecting approaches to Fairway Station.',
      affectedArea: 'Fairway Rd S & Wilson Ave, Kitchener',
      latitude: '43.4209',
      longitude: '-80.4388',
      radiusMeters: '500',
      routeIds: '1',
      stopIds: '22',
      tripIds: '',
      segments: 'Fairway Rd S approaching Wilson Ave',
    },
  ],
  security: [
    {
      label: 'Uptown Waterloo Traffic Restriction',
      externalId: 'WRPS-2026-301',
      subtype: 'traffic_restriction',
      severity: 'medium',
      status: 'active',
      title: 'Police traffic control in Uptown Waterloo',
      description: 'Rolling traffic restrictions along King St S for public safety perimeter.',
      affectedArea: 'King St S & Erb St, Waterloo',
      latitude: '43.4643',
      longitude: '-80.5204',
      radiusMeters: '400',
      routeIds: '7',
      stopIds: '23',
      tripIds: '',
      segments: 'King St S between Erb St and William St',
    },
    {
      label: 'Queen St Security Perimeter',
      externalId: 'WRPS-2026-302',
      subtype: 'road_closure',
      severity: 'high',
      status: 'active',
      title: 'Police road closure on Queen St S',
      description: 'Security perimeter closing Queen St S transit corridor until further notice.',
      affectedArea: 'Queen St S & King St W, Kitchener',
      latitude: '43.4501',
      longitude: '-80.4890',
      radiusMeters: '350',
      routeIds: '1',
      stopIds: '21',
      tripIds: '',
      segments: 'Queen St S between King St and Charles St',
    },
  ],
}

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

const INITIAL_FEED_FORM = {
  externalId: '',
  subtype: 'construction',
  severity: 'high',
  status: 'active',
  title: '',
  description: '',
  affectedArea: '',
  latitude: '',
  longitude: '',
  radiusMeters: '400',
  routeIds: '',
  stopIds: '',
  tripIds: '',
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

function validateFeedForm(activeFeed, form) {
  const title = form.title.trim()
  if (!title || [...title].length > 100 || title.includes('\0')) {
    return { error: 'Notice title is required and must be 100 characters or fewer.' }
  }

  const routeParsed = parsePositiveIdList(form.routeIds, 'Route IDs')
  if (routeParsed.error) return { error: routeParsed.error }

  const stopParsed = parsePositiveIdList(form.stopIds, 'Stop IDs')
  if (stopParsed.error) return { error: stopParsed.error }

  const tripParsed = activeFeed === 'utilities'
    ? { ids: [] }
    : parsePositiveIdList(form.tripIds, 'Trip IDs')
  if (tripParsed.error) return { error: tripParsed.error }

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
    title,
    status: form.status || 'active',
  }
  if (form.externalId.trim()) payload.external_id = form.externalId.trim()
  if (form.description.trim()) payload.description = form.description.trim()
  if (form.affectedArea.trim()) payload.affected_area = form.affectedArea.trim()
  if (routeParsed.ids.length > 0) payload.route_ids = routeParsed.ids
  if (stopParsed.ids.length > 0) payload.stop_ids = stopParsed.ids
  if (tripParsed.ids.length > 0 && activeFeed !== 'utilities') payload.trip_ids = tripParsed.ids

  const segments = parseSegmentsList(form.segments)
  if (segments.length > 0) payload.affected_segments = segments

  if (latitude !== null && longitude !== null) {
    payload.latitude = latitude
    payload.longitude = longitude
    if (radiusMeters !== null) payload.radius_meters = radiusMeters
  }

  if (activeFeed === 'utilities') {
    payload.notice_type = form.subtype || 'construction'
  } else if (activeFeed === 'emergency') {
    payload.incident_type = form.subtype || 'emergency'
    payload.severity = form.severity || 'high'
  } else {
    payload.alert_type = form.subtype || 'traffic_restriction'
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

  const [activeFeed, setActiveFeed] = useState('utilities')
  const [feedForm, setFeedForm] = useState(INITIAL_FEED_FORM)
  const [feedPending, setFeedPending] = useState(false)
  const [feedError, setFeedError] = useState(null)
  const [feedResult, setFeedResult] = useState(null)

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

  const currentFeedConfig = FEED_TABS.find((tab) => tab.id === activeFeed) ?? FEED_TABS[0]
  const currentTemplates = FEED_TEMPLATES[activeFeed] ?? []
  const mutationPending = formPending || feedPending || rowPendingId !== null

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

  function handleSelectFeedTab(feedId) {
    const tabConfig = FEED_TABS.find((item) => item.id === feedId) ?? FEED_TABS[0]
    setActiveFeed(feedId)
    setFeedForm((current) => ({
      ...current,
      subtype: tabConfig.subtypes[0].value,
    }))
    setFeedError(null)
  }

  function applyFeedTemplate(template) {
    setFeedForm({
      externalId: template.externalId,
      subtype: template.subtype,
      severity: template.severity,
      status: template.status,
      title: template.title,
      description: template.description,
      affectedArea: template.affectedArea,
      latitude: template.latitude,
      longitude: template.longitude,
      radiusMeters: template.radiusMeters,
      routeIds: template.routeIds,
      stopIds: template.stopIds,
      tripIds: template.tripIds,
      segments: template.segments,
    })
    setFeedError(null)
  }

  async function handleFeedSubmit(event) {
    event.preventDefault()
    if (mutationPending) return

    const { payload, error: validationError } = validateFeedForm(activeFeed, feedForm)
    if (validationError) {
      setFeedError(validationError)
      return
    }

    setFeedPending(true)
    setFeedError(null)
    setActionNotice(null)
    setActionError(null)

    try {
      let result
      if (activeFeed === 'utilities') {
        result = await submitUtilitiesNotice(payload, csrfToken)
      } else if (activeFeed === 'emergency') {
        result = await submitEmergencyIncident(payload, csrfToken)
      } else {
        result = await submitSecurityTrafficAlert(payload, csrfToken)
      }
      setFeedResult(result)
      setActionNotice(
        `${result.created ? 'Ingested new' : 'Updated'} external notice "${result.disruption?.title ?? payload.title}" (${result.transit_impact || 'impact evaluated'}).`,
      )
      disruptionsRes.retry()
      enterpriseStatusRes.retry()
    } catch (error) {
      if (error.status === 401 || error.status === 403) {
        handleApiError(error)
        return
      }
      setFeedError(error.message)
    } finally {
      setFeedPending(false)
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
              Real-time operational status summary from <code>GET /api/integrations/service-status</code> for smart-city enterprise consumers.
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

        <section className="admin-panel" aria-labelledby="external-feed-heading">
          <div className="admin-panel-header">
            <h2 id="external-feed-heading">External incident &amp; notice feeds (US 186, 187, 188)</h2>
            <p>
              Ingest road closures, emergency restrictions, and police traffic alerts with spatial stop/route impact matching.
            </p>
          </div>

          <div className="feed-tabs" role="tablist" aria-label="External incident feeds">
            {FEED_TABS.map((tab) => (
              <button
                key={tab.id}
                type="button"
                role="tab"
                aria-selected={activeFeed === tab.id}
                className={`feed-tab-button ${activeFeed === tab.id ? 'is-active' : ''}`}
                onClick={() => handleSelectFeedTab(tab.id)}
                disabled={mutationPending}
              >
                {tab.id === 'utilities' ? (
                  <Wrench size={14} aria-hidden="true" />
                ) : tab.id === 'emergency' ? (
                  <Flame size={14} aria-hidden="true" />
                ) : (
                  <ShieldAlert size={14} aria-hidden="true" />
                )}
                <span>{tab.label}</span>
              </button>
            ))}
          </div>

          <div className="field feed-template-bar">
            <span className="preset-label">Waterloo Region 1-click sample templates</span>
            <div className="preset-buttons" role="group" aria-label="Waterloo Region sample templates">
              {currentTemplates.map((template) => (
                <button
                  key={template.label}
                  type="button"
                  className="button button-secondary button-compact preset-button"
                  onClick={() => applyFeedTemplate(template)}
                  disabled={mutationPending}
                >
                  <MapPin size={13} aria-hidden="true" />
                  <span>{template.label}</span>
                </button>
              ))}
            </div>
          </div>

          {feedError && (
            <div className="error-notice auth-error" role="alert">
              <AlertCircle size={20} aria-hidden="true" />
              <div>
                <strong>Could not submit external notice</strong>
                <p>{feedError}</p>
              </div>
            </div>
          )}

          <form className="admin-form" onSubmit={handleFeedSubmit} noValidate>
            <div className="admin-vehicle-form-row">
              <div className="field">
                <label htmlFor="feed-external-id">External reference ID</label>
                <input
                  id="feed-external-id"
                  type="text"
                  maxLength={120}
                  placeholder="e.g. UTIL-WR-2026-101"
                  value={feedForm.externalId}
                  onChange={(event) => setFeedForm((f) => ({ ...f, externalId: event.target.value }))}
                  disabled={mutationPending}
                />
              </div>

              <div className="field">
                <label htmlFor="feed-subtype">{currentFeedConfig.subtypeLabel}</label>
                <select
                  id="feed-subtype"
                  value={feedForm.subtype}
                  onChange={(event) => setFeedForm((f) => ({ ...f, subtype: event.target.value }))}
                  disabled={mutationPending}
                >
                  {currentFeedConfig.subtypes.map((subtype) => (
                    <option key={subtype.value} value={subtype.value}>
                      {subtype.label}
                    </option>
                  ))}
                </select>
              </div>
            </div>

            <div className="admin-vehicle-form-row">
              {activeFeed === 'emergency' && (
                <div className="field">
                  <label htmlFor="feed-severity">Severity</label>
                  <select
                    id="feed-severity"
                    value={feedForm.severity}
                    onChange={(event) => setFeedForm((f) => ({ ...f, severity: event.target.value }))}
                    disabled={mutationPending}
                  >
                    <option value="critical">Critical</option>
                    <option value="high">High</option>
                    <option value="medium">Medium</option>
                    <option value="low">Low</option>
                  </select>
                </div>
              )}

              <div className="field">
                <label htmlFor="feed-status">Feed status</label>
                <select
                  id="feed-status"
                  value={feedForm.status}
                  onChange={(event) => setFeedForm((f) => ({ ...f, status: event.target.value }))}
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
              <label htmlFor="feed-title">Notice title</label>
              <input
                id="feed-title"
                type="text"
                maxLength={100}
                placeholder="e.g. King St N watermain construction"
                value={feedForm.title}
                onChange={(event) => setFeedForm((f) => ({ ...f, title: event.target.value }))}
                disabled={mutationPending}
                required
              />
            </div>

            <div className="field">
              <label htmlFor="feed-description">Notice description</label>
              <textarea
                id="feed-description"
                rows={2}
                placeholder="Operational details from the external service…"
                value={feedForm.description}
                onChange={(event) => setFeedForm((f) => ({ ...f, description: event.target.value }))}
                disabled={mutationPending}
              />
            </div>

            <div className="field">
              <label htmlFor="feed-affected-area">Affected area / corridor</label>
              <input
                id="feed-affected-area"
                type="text"
                maxLength={200}
                placeholder="e.g. King St N & University Ave, Waterloo"
                value={feedForm.affectedArea}
                onChange={(event) => setFeedForm((f) => ({ ...f, affectedArea: event.target.value }))}
                disabled={mutationPending}
              />
            </div>

            <div className="admin-feed-coords-row">
              <div className="field">
                <label htmlFor="feed-latitude">Latitude (optional)</label>
                <input
                  id="feed-latitude"
                  type="number"
                  step="any"
                  placeholder="43.4723"
                  value={feedForm.latitude}
                  onChange={(event) => setFeedForm((f) => ({ ...f, latitude: event.target.value }))}
                  disabled={mutationPending}
                />
              </div>

              <div className="field">
                <label htmlFor="feed-longitude">Longitude (optional)</label>
                <input
                  id="feed-longitude"
                  type="number"
                  step="any"
                  placeholder="-80.5449"
                  value={feedForm.longitude}
                  onChange={(event) => setFeedForm((f) => ({ ...f, longitude: event.target.value }))}
                  disabled={mutationPending}
                />
              </div>

              <div className="field">
                <label htmlFor="feed-radius">Impact radius (m)</label>
                <input
                  id="feed-radius"
                  type="number"
                  min="50"
                  max="5000"
                  step="50"
                  placeholder="400"
                  value={feedForm.radiusMeters}
                  onChange={(event) => setFeedForm((f) => ({ ...f, radiusMeters: event.target.value }))}
                  disabled={mutationPending}
                />
              </div>
            </div>

            <div className="admin-vehicle-form-row">
              <div className="field">
                <label htmlFor="feed-route-ids">Route IDs (optional)</label>
                <input
                  id="feed-route-ids"
                  type="text"
                  placeholder="e.g. 1, 7"
                  value={feedForm.routeIds}
                  onChange={(event) => setFeedForm((f) => ({ ...f, routeIds: event.target.value }))}
                  disabled={mutationPending}
                />
              </div>

              <div className="field">
                <label htmlFor="feed-stop-ids">Stop IDs (optional)</label>
                <input
                  id="feed-stop-ids"
                  type="text"
                  placeholder="e.g. 21, 23"
                  value={feedForm.stopIds}
                  onChange={(event) => setFeedForm((f) => ({ ...f, stopIds: event.target.value }))}
                  disabled={mutationPending}
                />
              </div>
            </div>

            <div className="admin-vehicle-form-row">
              {activeFeed !== 'utilities' && (
                <div className="field">
                  <label htmlFor="feed-trip-ids">Trip IDs (optional)</label>
                  <input
                    id="feed-trip-ids"
                    type="text"
                    placeholder="e.g. 100"
                    value={feedForm.tripIds}
                    onChange={(event) => setFeedForm((f) => ({ ...f, tripIds: event.target.value }))}
                    disabled={mutationPending}
                  />
                </div>
              )}

              <div className="field">
                <label htmlFor="feed-segments">Affected segments (optional)</label>
                <input
                  id="feed-segments"
                  type="text"
                  placeholder="e.g. King St N between University & Columbia"
                  value={feedForm.segments}
                  onChange={(event) => setFeedForm((f) => ({ ...f, segments: event.target.value }))}
                  disabled={mutationPending}
                />
              </div>
            </div>

            <button className="button button-primary" type="submit" disabled={mutationPending}>
              <Send size={15} aria-hidden="true" />
              <span>{feedPending ? 'Submitting notice…' : currentFeedConfig.submitLabel}</span>
            </button>
          </form>

          {feedResult && (
            <div
              className="feed-result-card"
              role="status"
              aria-label="External integration result"
            >
              <div className="feed-result-header">
                <strong>
                  {feedResult.created ? 'Created new disruption' : 'Updated existing disruption'}
                  {feedResult.disruption?.id ? ` #${feedResult.disruption.id}` : ''}
                </strong>
                <span className="status-pill is-active">
                  {sourceServiceLabel(feedResult.disruption?.source_service)}
                </span>
              </div>
              {feedResult.transit_impact && (
                <p className="feed-result-impact">{feedResult.transit_impact}</p>
              )}
              <div className="alert-chips">
                <span className="alert-chip alert-chip-route">
                  Matched routes:{' '}
                  {(feedResult.matched_route_ids ?? []).length > 0
                    ? feedResult.matched_route_ids.join(', ')
                    : 'None'}
                </span>
                <span className="alert-chip alert-chip-stop">
                  Matched stops:{' '}
                  {(feedResult.matched_stop_ids ?? []).length > 0
                    ? feedResult.matched_stop_ids.join(', ')
                    : 'None'}
                </span>
                {feedResult.disruption?.received_at && (
                  <span className="alert-chip">
                    Received {formatAlertTimestamp(feedResult.disruption.received_at)}
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
