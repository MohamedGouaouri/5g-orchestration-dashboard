import React from 'react'
import {
  Area, AreaChart, CartesianGrid, Cell, Line, LineChart, Pie, PieChart, ResponsiveContainer, Tooltip, XAxis, YAxis,
} from 'recharts'
import { ArrowRight, Download, Upload } from 'lucide-react'
import { fmtBytes, fmtClock, fmtTime, rateStr } from '../format'

export const COLORS = { dl: '#3b82f6', ul: '#a855f7', green: '#22c55e', orange: '#f59e0b', red: '#ef4444', cyan: '#22d3ee' }
export const SLICE_COLORS = { eMBB: '#3b82f6', URLLC: '#22c55e', mMTC: '#f59e0b' }

export function Card({ title, icon, right, children, className = '', style }) {
  return (
    <section className={`card ${className}`} style={style}>
      {(title || icon) && (
        <div className="card-h">
          {icon}
          <h3>{title}</h3>
          {right && <div className="right">{right}</div>}
        </div>
      )}
      {children}
    </section>
  )
}

export function Dot({ s }) { return <span className={`dot ${s}`} /> }

export function Sparkline({ data, k, color }) {
  if (!data || data.length < 2 || !data.some((d) => d[k] > 0)) return null
  const id = `sp-${k}`
  return (
    <div className="spark">
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={data} margin={{ top: 4, right: 0, bottom: 0, left: 0 }}>
          <defs>
            <linearGradient id={id} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={color} stopOpacity={0.45} />
              <stop offset="100%" stopColor={color} stopOpacity={0} />
            </linearGradient>
          </defs>
          <YAxis hide domain={['dataMin', 'dataMax']} />
          <Area type="monotone" dataKey={k} stroke={color} strokeWidth={1.6} fill={`url(#${id})`} isAnimationActive={false} dot={false} />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  )
}

export function KpiCard({ icon, value, unit, label, delta, spark }) {
  return (
    <div className="kpi">
      <div className="ic">{icon}</div>
      <div style={{ position: 'relative', zIndex: 1 }}>
        <div className="val">{value}{unit && <small>{unit}</small>}</div>
        <div className="lbl">{label}</div>
      </div>
      {spark}
      {delta && <div className={`delta ${delta.startsWith('-') ? 'neg' : ''}`}>{delta.startsWith('-') ? '▼' : '▲'} {delta.replace('-', '')}</div>}
    </div>
  )
}

function ChartTip({ active, payload, label, unit = 'Mbps', fmt }) {
  if (!active || !payload?.length) return null
  return (
    <div className="tt">
      <div className="muted" style={{ marginBottom: 4 }}>{fmtTime(label)}</div>
      {payload.map((p) => (
        <div className="r" key={p.dataKey}>
          <span className="dot" style={{ background: p.color }} />{p.name}: <b>{fmt ? fmt(p.value) : `${p.value} ${unit}`}</b>
        </div>
      ))}
    </div>
  )
}

// Downsample to ~n points for readable overview charts.
export function bucket(data, n) {
  if (!data || data.length <= n) return data || []
  const step = data.length / n
  const out = []
  for (let i = 0; i < n; i++) {
    const s = data.slice(Math.floor(i * step), Math.floor((i + 1) * step))
    if (!s.length) continue
    const avg = (k) => s.reduce((a, p) => a + (p[k] || 0), 0) / s.length
    out.push({ t: s[s.length - 1].t, dl: +avg('dl').toFixed(2), ul: +avg('ul').toFixed(2), dlPps: +avg('dlPps').toFixed(1), ulPps: +avg('ulPps').toFixed(1), loss: +avg('loss').toFixed(3), drops: s.reduce((a, p) => a + (p.drops || 0), 0) })
  }
  return out
}

export function ThroughputChart({ data, height = 230, dots = false, area = true, series }) {
  const s = series || [
    { k: 'dl', name: 'DL Throughput', color: COLORS.dl },
    { k: 'ul', name: 'UL Throughput', color: COLORS.ul },
  ]
  const unit = series?.[0]?.unit || 'Mbps'
  const Chart = area ? AreaChart : LineChart
  const span = data?.length > 1 ? data[data.length - 1].t - data[0].t : 0
  const tickFmt = span && span < 15 * 60 * 1000 ? fmtTime : fmtClock
  return (
    <ResponsiveContainer width="100%" height={height}>
      <Chart data={data} margin={{ top: 8, right: 14, bottom: 0, left: -6 }}>
        <defs>
          {s.map((x) => (
            <linearGradient key={x.k} id={`g-${x.k}`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={x.color} stopOpacity={0.28} />
              <stop offset="100%" stopColor={x.color} stopOpacity={0} />
            </linearGradient>
          ))}
        </defs>
        <CartesianGrid vertical />
        <XAxis dataKey="t" tickFormatter={tickFmt} stroke="#5f7090" tick={{ fill: '#c2cee2', fontSize: 12 }} minTickGap={40} />
        <YAxis stroke="#5f7090" tick={{ fill: '#c2cee2', fontSize: 12 }} width={52} tickFormatter={(v) => (v >= 1000 ? `${(v / 1000).toFixed(1)}k` : v)} />
        <Tooltip content={<ChartTip unit={unit} />} />
        {s.map((x) => area ? (
          <Area key={x.k} type="monotone" dataKey={x.k} name={x.name} stroke={x.color} strokeWidth={2.2} fill={`url(#g-${x.k})`}
            dot={dots ? { r: 4, fill: x.color, strokeWidth: 0 } : false} isAnimationActive={false} />
        ) : (
          <Line key={x.k} type="monotone" dataKey={x.k} name={x.name} stroke={x.color} strokeWidth={2} dot={false} isAnimationActive={false} />
        ))}
      </Chart>
    </ResponsiveContainer>
  )
}

export function TrafficSide({ k }) {
  return (
    <div className="traffic-side">
      <div className="row"><Download size={26} color={COLORS.dl} /><div><div className="big">{rateStr(k.dlMbps)}</div><div className="muted">DL</div></div></div>
      <div className="row"><Upload size={26} color={COLORS.ul} /><div><div className="big">{rateStr(k.ulMbps)}</div><div className="muted">UL</div></div></div>
      <hr />
      <div><div className="muted small">Total Traffic</div><div className="big">{fmtBytes(k.totalBytes)}</div></div>
    </div>
  )
}

export function NFCard({ nf }) {
  const label = { online: 'Online', degraded: 'Degraded', offline: 'Offline' }[nf.status]
  const cls = { online: 'green', degraded: 'orange', offline: 'red' }[nf.status]
  return (
    <div className="nf">
      <Dot s={nf.status} />
      <div className="name">{nf.name}</div>
      <div className={`st ${cls}`}>{label}</div>
      {nf.detail && <div className="det">{nf.detail}</div>}
    </div>
  )
}

export function TowerIcon({ size = 120, status = 'online' }) {
  const c = status === 'online' ? '#3b82f6' : status === 'degraded' ? '#f59e0b' : '#ef4444'
  return (
    <svg width={size} height={size} viewBox="0 0 120 120" aria-hidden>
      <defs>
        <radialGradient id="tg" cx="50%" cy="45%" r="50%">
          <stop offset="0%" stopColor={c} stopOpacity=".35" />
          <stop offset="100%" stopColor={c} stopOpacity="0" />
        </radialGradient>
      </defs>
      <circle cx="60" cy="56" r="56" fill="url(#tg)" />
      <circle cx="60" cy="56" r="52" fill="none" stroke={c} strokeOpacity=".25" />
      <circle cx="60" cy="56" r="40" fill="none" stroke={c} strokeOpacity=".18" />
      <g stroke={c} strokeWidth="3" fill="none" strokeLinecap="round" strokeLinejoin="round">
        <path d="M60 38 L46 96 M60 38 L74 96 M50 78 L70 78 M48 88 L72 88 M53 62 L67 62 M50 78 L67 62 M70 78 L53 62" />
        <path d="M49 30 a15 15 0 0 0 0 20 M71 30 a15 15 0 0 1 0 20" opacity=".9" />
        <path d="M42 24 a24 24 0 0 0 0 32 M78 24 a24 24 0 0 1 0 32" opacity=".55" />
      </g>
      <circle cx="60" cy="40" r="4.5" fill={c} />
    </svg>
  )
}

export function GnbConfigList({ g }) {
  const c = g.config || {}
  return (
    <dl className="kv">
      <dt>Cell ID</dt><dd>{c.cellId || '—'}</dd>
      <dt>PCI</dt><dd>{c.pci ?? '—'}</dd>
      <dt>TAC</dt><dd>{String(c.tac ?? '').padStart(4, '0') || '—'}</dd>
      <dt>Band</dt><dd>n{c.band} ({c.freqMHz ? (c.freqMHz / 1000).toFixed(2) : '?'} GHz)</dd>
      <dt>Bandwidth</dt><dd>{c.bandwidthMHz} MHz</dd>
      <dt>Connected UEs</dt><dd>{g.connectedUEs}</dd>
    </dl>
  )
}

export function UETable({ ues, limit, onSelect, selected, columns = 'basic' }) {
  const rows = limit ? ues.slice(0, limit) : ues
  if (!rows.length) return <div className="empty">No UEs registered</div>
  return (
    <div className="table-wrap">
      <table>
        <thead>
          <tr>
            <th>UE ID</th>
            {columns === 'full' && <th>IMSI</th>}
            <th>IP Address</th><th>DNN</th><th>Status</th><th>Serving Cell</th>
            {columns === 'full' && <><th>Slice</th><th>RNTI</th><th>DL</th><th>UL</th><th>BLER DL/UL</th></>}
          </tr>
        </thead>
        <tbody>
          {rows.map((u) => (
            <tr key={u.imsi} className={`${onSelect ? 'clickable' : ''} ${selected === u.imsi ? 'selected' : ''}`} onClick={() => onSelect?.(u.imsi)}>
              <td>{u.id}</td>
              {columns === 'full' && <td className="mono">{u.imsi}</td>}
              <td className="mono">{u.ip || '—'}</td>
              <td>{u.dnn || '—'}</td>
              <td><span className={`status ${u.status}`}><Dot s={u.status} />{u.status}</span></td>
              <td>{u.cellId}</td>
              {columns === 'full' && <>
                <td><span className={`badge ${u.slice === 'eMBB' ? 'blue' : u.slice === 'URLLC' ? '' : 'degraded'}`}>{u.slice}</span></td>
                <td className="mono">{u.rnti ? `0x${u.rnti.toString(16)}` : '—'}</td>
                <td>{rateStr(u.dlMbps)}</td>
                <td>{rateStr(u.ulMbps)}</td>
                <td className="mono">{(u.blerDl * 100).toFixed(1)}% / {(u.blerUl * 100).toFixed(1)}%</td>
              </>}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

export function SliceDonut({ slices, totalBytes, size = 160 }) {
  const has = slices.some((s) => s.bytes > 0)
  const data = has ? slices.filter((s) => s.bytes > 0) : [{ name: 'none', bytes: 1 }]
  return (
    <div className="donut">
      <div style={{ position: 'relative', width: size, height: size }}>
        <ResponsiveContainer width="100%" height="100%">
          <PieChart>
            <Pie data={data} dataKey="bytes" innerRadius={size * 0.36} outerRadius={size * 0.48} startAngle={90} endAngle={-270} stroke="none" isAnimationActive={false}>
              {data.map((s) => <Cell key={s.name} fill={SLICE_COLORS[s.name] || '#26375a'} />)}
            </Pie>
          </PieChart>
        </ResponsiveContainer>
        <div className="center"><b>{fmtBytes(totalBytes)}</b><span>Total</span></div>
      </div>
      <div className="donut-legend">
        {slices.map((s) => (
          <div key={s.name}><span className="dot" style={{ background: SLICE_COLORS[s.name] }} /><span>{s.name}</span><b>{Math.round(s.pct)}%</b></div>
        ))}
      </div>
    </div>
  )
}

export function Events({ events, limit = 4, onViewAll }) {
  return (
    <Card title="Recent Events" icon={<BellIcon />} className="flush-x"
      right={onViewAll && <button className="link-btn" onClick={onViewAll}>View All <ArrowRight size={14} /></button>}>
      <div className="events">
        {events.slice(0, limit).map((e, i) => (
          <div className="event" key={e.t + '-' + i}>
            <Dot s={e.level} /><span className="t">{fmtTime(e.t)}</span><span>{e.msg}</span>
          </div>
        ))}
        {!events.length && <div className="empty">No events yet</div>}
      </div>
    </Card>
  )
}

function BellIcon() {
  return <svg width="22" height="22" viewBox="0 0 24 24" fill="#3b82f6"><path d="M12 22a2.5 2.5 0 0 0 2.45-2h-4.9A2.5 2.5 0 0 0 12 22Zm7-6V11a7 7 0 0 0-5.5-6.84V3.5a1.5 1.5 0 0 0-3 0v.66A7 7 0 0 0 5 11v5l-2 2v1h18v-1l-2-2Z" /></svg>
}

// Decorative skyline with a transmitting tower.
export function CityScape() {
  const bld = []
  let x = 0
  let seed = 7
  const rnd = () => ((seed = (seed * 9301 + 49297) % 233280) / 233280)
  while (x < 600) {
    const w = 18 + rnd() * 26
    const h = 30 + rnd() * 90
    bld.push({ x, w, h })
    x += w + 3
  }
  return (
    <div className="card city">
      <svg viewBox="0 0 600 170" preserveAspectRatio="xMidYMax slice" aria-hidden>
        <defs>
          <linearGradient id="bg1" x1="0" x2="0" y1="0" y2="1"><stop offset="0" stopColor="#1e3a8a" /><stop offset="1" stopColor="#0b1a38" /></linearGradient>
          <radialGradient id="glow" cx="50%" cy="50%" r="50%"><stop offset="0" stopColor="#3b82f6" stopOpacity=".6" /><stop offset="1" stopColor="#3b82f6" stopOpacity="0" /></radialGradient>
        </defs>
        <circle cx="330" cy="58" r="70" fill="url(#glow)" />
        {[18, 32, 46, 60].map((r) => <circle key={r} cx="330" cy="50" r={r} fill="none" stroke="#60a5fa" strokeOpacity={0.5 - r / 150} />)}
        {bld.map((b, i) => (
          <g key={i}>
            <rect x={b.x} y={170 - b.h} width={b.w} height={b.h} fill="url(#bg1)" stroke="#2b4f9a" strokeWidth=".6" />
            {Array.from({ length: Math.floor(b.h / 10) }).map((_, j) =>
              Array.from({ length: Math.floor(b.w / 7) }).map((__, k) =>
                ((i + j + k) % 3 !== 0) && <rect key={`${j}-${k}`} x={b.x + 3 + k * 7} y={170 - b.h + 5 + j * 10} width="3" height="4" fill="#93c5fd" opacity={0.25 + ((i * j + k) % 5) / 8} />,
              ),
            )}
          </g>
        ))}
        <g stroke="#93c5fd" strokeWidth="2.4" fill="none" strokeLinejoin="round">
          <path d="M330 50 L312 168 M330 50 L348 168 M318 130 L342 130 M315 150 L345 150 M322 100 L338 100 M318 130 L338 100 M342 130 L322 100" />
          <path d="M318 40 a16 16 0 0 0 0 20 M342 40 a16 16 0 0 1 0 20" />
        </g>
        <circle cx="330" cy="50" r="5" fill="#bfdbfe" />
        <rect x="0" y="166" width="600" height="4" fill="#1d4ed8" opacity=".5" />
      </svg>
    </div>
  )
}
