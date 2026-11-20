import React, { useState } from 'react'
import { Bell, RotateCcw, Server } from 'lucide-react'
import { api } from '../api'
import { Card, COLORS, Dot, NFCard } from '../components/common'
import { fmtTime, fmtUptime } from '../format'

const KIND_LABEL = { core: '5G Core NFs', db: 'Data', ran: 'RAN', ric: 'RIC / xApps', other: 'Other' }

export default function Core({ snap }) {
  const [busy, setBusy] = useState('')
  const [level, setLevel] = useState('all')
  const restart = async (nf) => {
    if (!confirm(`Restart ${nf.name} (${nf.container})? This interrupts service for connected UEs.`)) return
    setBusy(nf.container)
    try { await api(`/api/nf/${encodeURIComponent(nf.container)}/restart`, { method: 'POST' }) } catch (e) { alert(e.message) } finally { setBusy('') }
  }
  const counts = snap.core.reduce((a, n) => ({ ...a, [n.status]: (a[n.status] || 0) + 1 }), {})
  const events = snap.events.filter((e) => level === 'all' || e.level === level)

  return (
    <>
      <div className="grid g-4">
        <div className="stat"><div className="l">Network functions</div><div className="v">{snap.core.length}</div></div>
        <div className="stat"><div className="l">Online</div><div className="v green">{counts.online || 0}</div></div>
        <div className="stat"><div className="l">Degraded</div><div className="v orange">{counts.degraded || 0}</div></div>
        <div className="stat"><div className="l">Offline</div><div className="v red">{counts.offline || 0}</div></div>
      </div>

      <Card className="mt" title="Core Health" icon={<Server size={22} color={COLORS.dl} />}>
        <div className="nf-grid" style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(170px, 1fr))' }}>
          {snap.core.filter((n) => n.kind === 'core' || n.kind === 'db').map((nf) => <NFCard key={nf.container} nf={nf} />)}
        </div>
      </Card>

      <Card className="mt" title="Network Function Inventory" icon={<Server size={22} color={COLORS.dl} />}
        right={!snap.control && <span className="muted small">Management actions disabled (ENABLE_CONTROL)</span>}>
        <div className="table-wrap">
          <table>
            <thead><tr><th>NF</th><th>Type</th><th>Status</th><th>Health</th><th>Uptime</th><th>CPU</th><th>Memory</th><th>Container / Image</th><th>Networks</th>{snap.control && <th />}</tr></thead>
            <tbody>
              {snap.core.map((nf) => (
                <tr key={nf.container}>
                  <td><b>{nf.name}</b><div className="dim small">{nf.detail}</div></td>
                  <td>{KIND_LABEL[nf.kind]}</td>
                  <td><span className={`status ${nf.status === 'online' ? 'Active' : nf.status === 'degraded' ? 'Idle' : 'Deregistered'}`}><Dot s={nf.status} />{nf.status}</span></td>
                  <td>{nf.health}</td>
                  <td>{fmtUptime(nf.uptimeSec)}</td>
                  <td style={{ minWidth: 110 }}>
                    <div className="small">{nf.cpuPct.toFixed(1)}%</div>
                    <div className="bar"><i style={{ width: `${Math.min(100, nf.cpuPct)}%`, background: nf.cpuPct > 80 ? COLORS.red : COLORS.dl }} /></div>
                  </td>
                  <td>{nf.memMB >= 1024 ? `${(nf.memMB / 1024).toFixed(2)} GB` : `${nf.memMB.toFixed(0)} MB`}</td>
                  <td><div>{nf.container}</div><div className="dim small mono">{nf.image}</div></td>
                  <td className="dim small mono" style={{ whiteSpace: 'normal', maxWidth: 260 }}>{nf.ips || '—'}</td>
                  {snap.control && <td><button className="btn ghost" disabled={busy === nf.container} onClick={() => restart(nf)}><RotateCcw size={14} />{busy === nf.container ? 'Restarting…' : 'Restart'}</button></td>}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Card>

      <Card className="mt" title="Event Log" icon={<Bell size={22} color={COLORS.dl} />}
        right={<div className="seg">{['all', 'success', 'info', 'warning', 'error'].map((l) => <button key={l} className={level === l ? 'active' : ''} onClick={() => setLevel(l)}>{l}</button>)}</div>}>
        <div className="events" style={{ maxHeight: 360, overflowY: 'auto' }}>
          {events.map((e, i) => (
            <div className="event" key={e.t + '-' + i}><Dot s={e.level} /><span className="t">{fmtTime(e.t)}</span><span>{e.msg}</span></div>
          ))}
          {!events.length && <div className="empty">No events</div>}
        </div>
      </Card>
    </>
  )
}
