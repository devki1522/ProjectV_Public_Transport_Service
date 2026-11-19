import { useEffect, useRef } from 'react'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import { directionLabel, formatTime } from '../../lib/transit.js'
import { vehicleStatusClass, vehicleStatusLabel } from './vehicleFormat.js'

const WATERLOO_CENTER = [43.4516, -80.4925]
const DEFAULT_ZOOM = 12
const OSM_TILE_URL = 'https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png'
const OSM_ATTRIBUTION = '© OpenStreetMap contributors'

function createBusIcon(vehicle, isSelected) {
  const shell = document.createElement('div')
  shell.className = 'bus-marker-shell'

  if (typeof vehicle.heading === 'number' && Number.isFinite(vehicle.heading)) {
    const arrow = document.createElement('span')
    arrow.className = 'bus-marker-arrow'
    arrow.style.transform = `rotate(${Math.round(vehicle.heading)}deg)`
    arrow.setAttribute('aria-hidden', 'true')
    const tip = document.createElement('span')
    tip.className = 'bus-marker-arrow-tip'
    arrow.appendChild(tip)
    shell.appendChild(arrow)
  }

  const badge = document.createElement('div')
  badge.className = `bus-marker ${vehicleStatusClass(vehicle.status)}${isSelected ? ' is-selected' : ''}`
  const label = document.createElement('span')
  label.className = 'bus-marker-label'
  label.textContent = String(vehicle.route_number ?? '?')
  badge.appendChild(label)
  shell.appendChild(badge)

  return L.divIcon({
    className: 'bus-marker-icon',
    html: shell,
    iconSize: [38, 38],
    iconAnchor: [19, 19],
    popupAnchor: [0, -20],
  })
}

function appendPopupRow(list, label, value) {
  const row = document.createElement('div')
  row.className = 'bus-popup-row'
  const term = document.createElement('dt')
  term.textContent = label
  const detail = document.createElement('dd')
  detail.textContent = value
  row.append(term, detail)
  list.appendChild(row)
}

function createPopupContent(vehicle) {
  const container = document.createElement('div')
  container.className = 'bus-popup'

  const title = document.createElement('strong')
  title.className = 'bus-popup-title'
  title.textContent = `Route ${vehicle.route_number} · ${vehicle.route_name}`
  container.appendChild(title)

  const subtitle = document.createElement('p')
  subtitle.className = 'bus-popup-subtitle'
  subtitle.textContent = `Bus #${vehicle.vehicle_number} · ${directionLabel(vehicle.direction)}`
  container.appendChild(subtitle)

  const details = document.createElement('dl')
  details.className = 'bus-popup-details'
  appendPopupRow(details, 'Status', vehicleStatusLabel(vehicle.status))

  if (typeof vehicle.speed_kmh === 'number' && Number.isFinite(vehicle.speed_kmh)) {
    appendPopupRow(details, 'Speed', `${Math.round(vehicle.speed_kmh)} km/h`)
  }
  if (typeof vehicle.heading === 'number' && Number.isFinite(vehicle.heading)) {
    appendPopupRow(details, 'Heading', `${Math.round(vehicle.heading)}°`)
  }
  if (vehicle.trip_id) {
    appendPopupRow(details, 'Trip', `Trip ${vehicle.trip_id}`)
  }
  if (vehicle.is_accessible) {
    appendPopupRow(details, 'Accessibility', 'Wheelchair accessible')
  }

  const recorded = formatTime(vehicle.recorded_at)
  appendPopupRow(details, 'Reported', vehicle.is_simulated ? `${recorded} · Simulated` : recorded)
  container.appendChild(details)

  return container
}

export function LeafletMap({ vehicles, routeShape, selectedVehicleId, onSelectVehicle }) {
  const containerRef = useRef(null)
  const mapRef = useRef(null)
  const markersRef = useRef(new Map())
  const polylineRef = useRef(null)
  const fittedRouteIdRef = useRef(null)
  const prevSelectedIdRef = useRef(null)
  const onSelectVehicleRef = useRef(onSelectVehicle)

  useEffect(() => {
    onSelectVehicleRef.current = onSelectVehicle
  }, [onSelectVehicle])

  useEffect(() => {
    const container = containerRef.current
    if (!container) return
    const map = L.map(container, {
      center: WATERLOO_CENTER,
      zoom: DEFAULT_ZOOM,
      scrollWheelZoom: true,
    })
    L.tileLayer(OSM_TILE_URL, {
      attribution: OSM_ATTRIBUTION,
      maxZoom: 19,
    }).addTo(map)
    mapRef.current = map
    const markers = markersRef.current

    return () => {
      for (const marker of markers.values()) {
        marker.remove()
      }
      markers.clear()
      polylineRef.current?.remove()
      polylineRef.current = null
      fittedRouteIdRef.current = null
      prevSelectedIdRef.current = null
      map.remove()
      mapRef.current = null
    }
  }, [])

  useEffect(() => {
    const map = mapRef.current
    if (!map) return

    polylineRef.current?.remove()
    polylineRef.current = null

    const coords = routeShape?.coordinates
    if (Array.isArray(coords) && coords.length >= 2) {
      const latLngs = coords
        .filter((pt) => Number.isFinite(pt?.lat) && Number.isFinite(pt?.lng))
        .map((pt) => [pt.lat, pt.lng])

      if (latLngs.length >= 2) {
        const polyline = L.polyline(latLngs, {
          color: '#246447',
          weight: 4,
          opacity: 0.78,
        }).addTo(map)
        polylineRef.current = polyline

        const routeKey = routeShape.route_id ?? 'shape'
        if (fittedRouteIdRef.current !== routeKey) {
          fittedRouteIdRef.current = routeKey
          const bounds = polyline.getBounds?.()
          if (bounds && (!bounds.isValid || bounds.isValid())) {
            map.fitBounds(bounds, { padding: [36, 36], maxZoom: 15 })
          }
        }
      }
    } else if (fittedRouteIdRef.current !== null) {
      fittedRouteIdRef.current = null
      map.setView(WATERLOO_CENTER, DEFAULT_ZOOM)
    }
  }, [routeShape])

  useEffect(() => {
    const map = mapRef.current
    if (!map) return

    const markers = markersRef.current
    const activeIds = new Set()

    for (const vehicle of vehicles) {
      if (!Number.isFinite(vehicle.latitude) || !Number.isFinite(vehicle.longitude)) continue
      activeIds.add(vehicle.vehicle_id)
      const isSelected = vehicle.vehicle_id === selectedVehicleId
      const icon = createBusIcon(vehicle, isSelected)
      const popupContent = createPopupContent(vehicle)
      const existing = markers.get(vehicle.vehicle_id)

      if (existing) {
        existing.setLatLng([vehicle.latitude, vehicle.longitude])
        existing.setIcon(icon)
        existing.setZIndexOffset?.(isSelected ? 500 : 0)
        if (existing.getPopup?.()) {
          existing.setPopupContent(popupContent)
        } else {
          existing.bindPopup(popupContent)
        }
      } else {
        const marker = L.marker([vehicle.latitude, vehicle.longitude], {
          icon,
          title: `Route ${vehicle.route_number} · Bus #${vehicle.vehicle_number}`,
          zIndexOffset: isSelected ? 500 : 0,
        })
        marker.bindPopup(popupContent)
        marker.on('click', () => {
          onSelectVehicleRef.current?.(vehicle.vehicle_id)
        })
        marker.addTo(map)
        markers.set(vehicle.vehicle_id, marker)
      }
    }

    for (const [vehicleId, marker] of markers.entries()) {
      if (!activeIds.has(vehicleId)) {
        marker.remove()
        markers.delete(vehicleId)
      }
    }
  }, [vehicles, selectedVehicleId])

  useEffect(() => {
    const map = mapRef.current
    if (!map) return
    if (selectedVehicleId === prevSelectedIdRef.current) return
    if (selectedVehicleId == null) {
      prevSelectedIdRef.current = null
      return
    }

    const marker = markersRef.current.get(selectedVehicleId)
    if (!marker) return
    prevSelectedIdRef.current = selectedVehicleId
    const latLng = marker.getLatLng?.()
    if (latLng) {
      map.panTo?.(latLng)
    }
    marker.openPopup?.()
  }, [selectedVehicleId, vehicles])

  return (
    <div className="live-map-canvas-wrap">
      <div
        ref={containerRef}
        className="live-map-canvas"
        role="region"
        aria-label="Waterloo Region live bus map"
      />
    </div>
  )
}
