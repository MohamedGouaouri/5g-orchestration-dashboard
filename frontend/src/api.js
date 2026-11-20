import { useEffect, useRef, useState } from 'react'

// Live snapshot via Server-Sent Events with automatic polling fallback.
export function useSnapshot() {
  const [snap, setSnap] = useState(null)
  const [connected, setConnected] = useState(false)
  const pollRef = useRef(null)

  useEffect(() => {
    let es
    let closed = false
    const startPolling = () => {
      if (pollRef.current) return
      pollRef.current = setInterval(async () => {
        try {
          const r = await fetch('/api/snapshot')
          if (r.ok) { setSnap(await r.json()); setConnected(true) }
        } catch { setConnected(false) }
      }, 2000)
    }
    const stopPolling = () => { clearInterval(pollRef.current); pollRef.current = null }
    const open = () => {
      if (closed) return
      es = new EventSource('/api/stream')
      es.onmessage = (e) => {
        try { setSnap(JSON.parse(e.data)); setConnected(true); stopPolling() } catch { /* ignore */ }
      }
      es.onerror = () => {
        setConnected(false)
        es.close()
        startPolling()
        setTimeout(open, 5000)
      }
    }
    open()
    return () => { closed = true; es && es.close(); stopPolling() }
  }, [])
  return { snap, connected }
}

export async function api(path, opts = {}) {
  const r = await fetch(path, { headers: { 'Content-Type': 'application/json' }, ...opts })
  const body = await r.json().catch(() => ({}))
  if (!r.ok) throw new Error(body.error || `HTTP ${r.status}`)
  return body
}

export function useFetch(path, intervalMs = 0, deps = []) {
  const [data, setData] = useState(null)
  const [error, setError] = useState(null)
  const [tick, setTick] = useState(0)
  useEffect(() => {
    let alive = true
    const load = () => api(path).then((d) => { if (alive) { setData(d); setError(null) } }).catch((e) => alive && setError(e.message))
    load()
    const id = intervalMs ? setInterval(load, intervalMs) : null
    return () => { alive = false; id && clearInterval(id) }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [path, intervalMs, tick, ...deps])
  return { data, error, reload: () => setTick((t) => t + 1) }
}
