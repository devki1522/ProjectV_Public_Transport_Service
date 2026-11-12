import { useCallback, useEffect, useMemo, useState } from 'react'
import { RouterContext, useRouter } from './routerState.js'

function readWindowLocation() {
  return `${window.location.pathname}${window.location.search}`
}

export function RouterProvider({ children, initialPath }) {
  const [location, setLocation] = useState(() => initialPath ?? readWindowLocation())

  useEffect(() => {
    if (initialPath !== undefined) {
      window.history.replaceState(null, '', initialPath)
    }
  }, [initialPath])

  useEffect(() => {
    function handlePopState() {
      setLocation(readWindowLocation())
    }
    window.addEventListener('popstate', handlePopState)
    return () => window.removeEventListener('popstate', handlePopState)
  }, [])

  const navigate = useCallback((to, { replace = false } = {}) => {
    const target = typeof to === 'string' && to.startsWith('/') && !to.startsWith('//') ? to : '/'
    if (replace) {
      window.history.replaceState(null, '', target)
    } else {
      window.history.pushState(null, '', target)
    }
    setLocation(target)
  }, [])

  const value = useMemo(() => {
    const parsed = new URL(location, 'https://transit.local')
    return {
      location,
      pathname: parsed.pathname,
      search: parsed.search,
      searchParams: parsed.searchParams,
      navigate,
    }
  }, [location, navigate])

  return <RouterContext.Provider value={value}>{children}</RouterContext.Provider>
}

export function Link({ to, replace = false, onClick, children, ...props }) {
  const { navigate } = useRouter()

  function handleClick(event) {
    if (onClick) onClick(event)
    if (
      event.defaultPrevented ||
      event.button !== 0 ||
      event.metaKey ||
      event.ctrlKey ||
      event.shiftKey ||
      event.altKey
    ) {
      return
    }
    event.preventDefault()
    navigate(to, { replace })
  }

  return (
    <a href={to} onClick={handleClick} {...props}>
      {children}
    </a>
  )
}
