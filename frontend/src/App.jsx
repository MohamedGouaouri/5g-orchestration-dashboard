import React, { useEffect, useState } from 'react'
import { BarChart3, CheckCircle2, Database, Download, Home, RadioTower, RefreshCw, Server, Settings as Gear, Upload, Users } from 'lucide-react'
import { useSnapshot } from './api'
import { COLORS, KpiCard, Sparkline } from './components/common'
import { fmtBytes, fmtRate } from './format'
import Overview from './pages/Overview'
import Subscribers from './pages/Subscribers'
import Traffic from './pages/Traffic'
import Core from './pages/Core'
import Ran from './pages/Ran'
import Settings from './pages/Settings'

const PAGES = [
  { id: 'overview', label: 'Overview', icon: Home, C: Overview },
  { id: 'subscribers', label: 'Subscribers & UEs', icon: Users, C: Subscribers },
  { id: 'traffic', label: 'Traffic Analytics', icon: BarChart3, C: Traffic },
  { id: 'core', label: '5G Core', icon: Server, C: Core },
  { id: 'ran', label: 'RAN & gNB', icon: RadioTower, C: Ran },
]

function useHashRoute() {
  const get = () => window.location.hash.replace('#/', '') || 'overview'
  const [route, setRoute] = useState(get)
  useEffect(() => {
    const f = () => setRoute(get())
    window.addEventListener('hashchange', f)
    return () => window.removeEventListener('hashchange', f)
  }, [])
  return [route, (r) => { window.location.hash = `#/${r}` }]
}

function useClock() {
  const [now, setNow] = useState(new Date())
  useEffect(() => { const id = setInterval(() => setNow(new Date()), 1000); return () => clearInterval(id) }, [])
  return now
}

function Logo() {
  return (
    <div className="brand">
      <svg width="54" height="44" viewBox="0 0 54 44" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round">
        <path d="M27 16 L19 42 M27 16 L35 42 M22 32 L32 32" />
        <circle cx="27" cy="14" r="3.5" fill="currentColor" stroke="none" />
        <path d="M19 7 a11 11 0 0 0 0 14 M35 7 a11 11 0 0 1 0 14" />
        <path d="M12 2 a19 19 0 0 0 0 24 M42 2 a19 19 0 0 1 0 24" opacity=".7" />
      </svg>
      <div className="brand-5g">5G</div>
    </div>
  )
}

export default function App() {
  const { snap, connected } = useSnapshot()
  const [route, go] = useHashRoute()
  const now = useClock()

  if (!snap) {
    return (
      <div className="loading">
        <Logo />
        <div>{connected ? 'Loading network state…' : 'Connecting to backend…'}</div>
      </div>
    )
  }

  const k = snap.kpis
  const page = route === 'settings' ? { C: Settings } : PAGES.find((p) => p.id === route) || PAGES[0]
  const dl = fmtRate(k.dlMbps)
  const ul = fmtRate(k.ulMbps)
  const spark = snap.traffic.slice(-60)
  const status = !connected ? 'offline' : snap.network
  const statusLabel = !connected ? 'Backend Disconnected' : { online: 'Network Online', degraded: 'Network Degraded', offline: 'Network Offline' }[snap.network]

  return (
    <div className="app">
      <aside className="sidebar">
        <Logo />
        <nav className="nav">
          {PAGES.map((p) => (
            <button key={p.id} className={route === p.id || (p.id === 'overview' && !PAGES.some((x) => x.id === route) && route !== 'settings') ? 'active' : ''} onClick={() => go(p.id)} title={p.label}>
              <p.icon size={20} /> <span className="lbl">{p.label}</span>
            </button>
          ))}
        </nav>
        <div className="nav bottom">
          <button className={route === 'settings' ? 'active' : ''} onClick={() => go('settings')} title="Settings">
            <Gear size={20} /> <span className="lbl">Settings</span>
          </button>
        </div>
      </aside>

      <main className="main">
        <header className="header">
          <h1>5G Network Dashboard</h1>
          <span className="sub">Real-time network monitoring</span>
          <div className="right">
            {snap.mode === 'mock' && <span className="pill mock">Demo data</span>}
            <span className={`pill ${status}`}><span className={`dot ${status}`} />{statusLabel}</span>
            <span>{now.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' })} • {now.toLocaleTimeString([], { hour12: false })}</span>
            <button className="icon-btn" title="Reload" onClick={() => window.location.reload()}><RefreshCw size={20} /></button>
          </div>
        </header>

        <div className="kpis">
          <KpiCard icon={<Users size={34} color={COLORS.dl} fill={COLORS.dl} />} value={k.registeredUEs} label="Registered UEs"
            delta={k.subscribers ? `${Math.round((k.registeredUEs / k.subscribers) * 100)}% of subs` : undefined} />
          <KpiCard icon={<CheckCircle2 size={38} color="#0b1426" fill={COLORS.green} />} value={k.activeUEs} label="Active UEs"
            delta={k.registeredUEs ? `${Math.round((k.activeUEs / k.registeredUEs) * 100)}%` : undefined} />
          <KpiCard icon={<Download size={32} color={COLORS.dl} strokeWidth={2.6} />} value={dl.v} unit={dl.u} label="DL Throughput"
            spark={<Sparkline data={spark} k="dl" color={COLORS.dl} />} />
          <KpiCard icon={<Upload size={32} color={COLORS.ul} strokeWidth={2.6} />} value={ul.v} unit={ul.u} label="UL Throughput"
            spark={<Sparkline data={spark} k="ul" color={COLORS.ul} />} />
          <KpiCard icon={<Database size={34} color={COLORS.dl} fill="rgba(59,130,246,.25)" />} value={fmtBytes(k.totalBytes).split(' ')[0]} unit={fmtBytes(k.totalBytes).split(' ')[1]} label="Total Traffic" />
        </div>

        {route !== 'settings' && (
          <nav className="tabs">
            {PAGES.map((p) => <button key={p.id} className={page.id === p.id ? 'active' : ''} onClick={() => go(p.id)}>{p.label}</button>)}
          </nav>
        )}

        <page.C snap={snap} go={go} />
      </main>
    </div>
  )
}
