export const fmtBytes = (b = 0) => {
  const u = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let i = 0
  let v = b
  while (v >= 1000 && i < u.length - 1) { v /= 1000; i++ }
  return `${v >= 100 || i === 0 ? v.toFixed(0) : v.toFixed(1)} ${u[i]}`
}

export const fmtRate = (mbps = 0) => {
  if (mbps >= 1000) return { v: (mbps / 1000).toFixed(2), u: 'Gbps' }
  if (mbps >= 1) return { v: mbps.toFixed(mbps >= 100 ? 0 : 1), u: 'Mbps' }
  return { v: (mbps * 1000).toFixed(0), u: 'kbps' }
}
export const rateStr = (m) => { const r = fmtRate(m); return `${r.v} ${r.u}` }

export const fmtNum = (n = 0) => {
  if (n >= 1e6) return (n / 1e6).toFixed(1) + 'M'
  if (n >= 1e3) return (n / 1e3).toFixed(1) + 'k'
  return Math.round(n).toString()
}

export const fmtTime = (ms) => new Date(ms).toLocaleTimeString([], { hour12: false })
export const fmtClock = (ms) => new Date(ms).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false })

export const fmtUptime = (s = 0) => {
  if (!s) return '—'
  const d = Math.floor(s / 86400)
  const h = Math.floor((s % 86400) / 3600)
  const m = Math.floor((s % 3600) / 60)
  if (d) return `${d}d ${h}h`
  if (h) return `${h}h ${m}m`
  return `${m}m ${s % 60}s`
}

export const pct = (v, d = 1) => `${(v * 100).toFixed(d)}%`
