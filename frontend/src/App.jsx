import { useState, useEffect } from 'react'

import './App.css'

function App() {
  const [status, setStatus] = useState(null)

  useEffect(() => {
    fetch('/api/health')
      .then((response) => {
        if (!response.ok) throw new Error('Backend unavailable')
        return response.json()
      })
      .then((data) => setStatus(data.status))
      .catch(() => setStatus('Connection failed'))
  }, [])

  return (
    <main>
      <h1>Public Transport</h1>
      <p>Backend status {status}</p>
    </main>
  )
}

export default App
