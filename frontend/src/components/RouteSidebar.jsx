import { ArrowUpRight, Search } from 'lucide-react'
import { useState } from 'react'

export function RouteSidebar({ groups, selected, onSelect, pending }) {
  const [search, setSearch] = useState('')
  const filtered = groups.filter((group) => `${group.number} ${group.name}`.toLowerCase().includes(search.trim().toLowerCase()))

  function selectGroup(number) {
    const group = groups.find((entry) => entry.number === number)
    if (group) onSelect(group.directions[0].id)
  }

  return (
    <aside className="route-sidebar" aria-label="Choose a transit route">
      <div className="route-list-header"><h2>Find a route</h2><span className="route-count">{groups.length}</span></div>
      <div className="desktop-route-picker">
        <label className="search-field" htmlFor="route-search"><Search size={17} aria-hidden="true" /><span className="visually-hidden">Search routes by number or name</span>
          <input id="route-search" type="search" placeholder="Route number or name" value={search} onChange={(event) => setSearch(event.target.value)} autoComplete="off" />
        </label>
        <div className="route-list" aria-busy={pending}>
          {filtered.map((group) => (
            <button className={`route-option${selected === group.number ? ' is-selected' : ''}`} key={group.number} type="button"
              onClick={() => selectGroup(group.number)} aria-pressed={selected === group.number}>
              <span className="visually-hidden">Route</span>{' '}<span className="route-number">{group.number}</span>{' '}
              <span className="route-option-copy"><span>{group.name}</span>{' '}<small>{group.directions.length} {group.directions.length === 1 ? 'direction' : 'directions'}</small></span>
              {selected === group.number && <ArrowUpRight size={17} aria-hidden="true" />}
            </button>
          ))}
          {!pending && filtered.length === 0 && <p className="route-no-results">{search ? `No routes match “${search}”. Try a route number or another name.` : 'No active routes to show.'}</p>}
        </div>
      </div>
      <div className="mobile-route-picker"><label htmlFor="route-picker">Route</label>
        <select id="route-picker" value={selected ?? ''} onChange={(event) => selectGroup(event.target.value)} disabled={!groups.length}>
          {!groups.length && <option value="">{pending ? 'Loading routes…' : 'No active routes'}</option>}
          {groups.map((group) => <option key={group.number} value={group.number}>{group.number} · {group.name}</option>)}
        </select>
      </div>
      <div className="sidebar-note"><span className="small-rule" /><p>Choose a route to explore its timetable and upcoming departures.</p></div>
    </aside>
  )
}
