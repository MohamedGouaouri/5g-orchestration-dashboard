import React, { useMemo, useState } from 'react'
import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { Activity, AlertTriangle, BarChart3, Gauge, Users } from 'lucide-react'
import { useFetch } from '../api'
import { bucket, Card, COLORS, SliceDonut, ThroughputChart } from '../components/common'
import { fmtBytes, fmtNum, rateStr } from '../format'

const WINDOWS = [{ s: 60, l: '1m' }, { s: 300, l: '5m' }, { s: 900, l: '15m' }, { s: 3600, l: '1h' }]

export default function Traffic({ snap }) {
  const [win, setWin] = useState(300)
  const hist = useFetch(`/api/traffic/history?seconds=${win}`, 2000, [win])
  const data = useMemo(() => bucket(hist.data || snap.traffic, 120), [hist.data, snap.traffic])
  const k = snap.kpis

  const stats = useMemo(() => {
    const d = hist.data || snap.traffic
    if (!d.length) return { pkDl: 0, pkUl: 0, avgDl: 0, avgUl: 0, drops: 0 }
    return {
      pkDl: Math.max(...d.map((p) => p.dl)), pkUl: Math.max(...d.map((p) => p.ul)),
      avgDl: d.reduce((a, p) => a + p.dl, 0) / d.length, avgUl: d.reduce((a, p) => a + p.ul, 0) / d.length,
      drops: d.reduce((a, p) => a + (p.drops || 0), 0),
    }
  }, [hist.data, snap.traffic])

  const topUEs = [...snap.ues].filter((u) => u.status !== 'Deregistered').sort((a, b) => (b.dlMbps + b.ulMbps) - (a.dlMbps + a.ulMbps)).slice(0, 12)
    .map((u) => ({ name: u.id, dl: u.dlMbps, ul: u.ulMbps }))

  return (
    <>
      <div className="grid g-4">
        <div className="stat"><div className="l">DL now · avg · peak</div><div className="v blue">{rateStr(k.dlMbps)}</div><div className="dim small">avg {rateStr(stats.avgDl)} · peak {rateStr(stats.pkDl)}</div></div>
        <div className="stat"><div className="l">UL now · avg · peak</div><div className="v purple">{rateStr(k.ulMbps)}</div><div className="dim small">avg {rateStr(stats.avgUl)} · peak {rateStr(stats.pkUl)}</div></div>
        <div className="stat"><div className="l">Packet rate DL / UL</div><div className="v">{fmtNum(k.dlPps)} <small>/</small> {fmtNum(k.ulPps)}<small>pps</small></div></div>
        <div className="stat"><div className="l">Packet loss (UPF) · drops in window</div><div className={`v ${k.lossPct > 1 ? 'red' : k.lossPct > 0.1 ? 'orange' : 'green'}`}>{k.lossPct.toFixed(2)}<small>%</small></div><div className="dim small">{fmtNum(stats.drops)} packets dropped</div></div>
      </div>

      <Card className="mt" title="Throughput" icon={<Activity size={22} color={COLORS.dl} />}
        right={<div className="seg">{WINDOWS.map((w) => <button key={w.s} className={win === w.s ? 'active' : ''} onClick={() => setWin(w.s)}>{w.l}</button>)}</div>}>
        <div className="legend">
          <span><span className="dot" style={{ background: COLORS.dl }} />Downlink</span>
          <span><span className="dot" style={{ background: COLORS.ul }} />Uplink</span>
        </div>
        <ThroughputChart data={data} height={260} />
      </Card>

      <div className="grid g-2 mt">
        <Card title="Packet Rate" icon={<Gauge size={22} color={COLORS.cyan} />}>
          <ThroughputChart data={data} height={200} area={false} series={[
            { k: 'dlPps', name: 'DL packets/s', color: COLORS.dl, unit: 'pps' },
            { k: 'ulPps', name: 'UL packets/s', color: COLORS.ul, unit: 'pps' },
          ]} />
        </Card>
        <Card title="Packet Loss" icon={<AlertTriangle size={22} color={COLORS.orange} />}
          right={<span className="muted small">avg BLER DL {(k.avgBlerDl * 100).toFixed(1)}% · UL {(k.avgBlerUl * 100).toFixed(1)}%</span>}>
          <ThroughputChart data={data} height={200} series={[{ k: 'loss', name: 'UPF loss', color: COLORS.orange, unit: '%' }]} />
        </Card>
      </div>

      <div className="grid g-ov1 mt">
        <Card title="Per-UE Throughput" icon={<Users size={22} color={COLORS.dl} />} right={<span className="muted small">top {topUEs.length} by current rate</span>}>
          {topUEs.length ? (
            <ResponsiveContainer width="100%" height={Math.max(160, topUEs.length * 28 + 40)}>
              <BarChart data={topUEs} layout="vertical" margin={{ left: 10, right: 20 }}>
                <CartesianGrid horizontal={false} />
                <XAxis type="number" stroke="#5f7090" tick={{ fill: '#c2cee2', fontSize: 12 }} unit=" M" />
                <YAxis type="category" dataKey="name" stroke="#5f7090" tick={{ fill: '#c2cee2', fontSize: 12 }} width={64} />
                <Tooltip cursor={{ fill: 'rgba(59,130,246,.06)' }} contentStyle={{ background: '#0d1830', border: '1px solid #263757', borderRadius: 8 }} formatter={(v) => `${v} Mbps`} />
                <Bar dataKey="dl" name="DL" stackId="a" fill={COLORS.dl} isAnimationActive={false} />
                <Bar dataKey="ul" name="UL" stackId="a" fill={COLORS.ul} radius={[0, 4, 4, 0]} isAnimationActive={false} />
              </BarChart>
            </ResponsiveContainer>
          ) : <div className="empty">No registered UEs</div>}
        </Card>
        <Card title="Traffic by Slice" icon={<BarChart3 size={22} color={COLORS.dl} />}>
          <SliceDonut slices={snap.slices} totalBytes={k.totalBytes} />
          <div className="grid g-2 mt">
            <div className="stat"><div className="l">Downlink volume</div><div className="v blue">{fmtBytes(k.dlBytes)}</div></div>
            <div className="stat"><div className="l">Uplink volume</div><div className="v purple">{fmtBytes(k.ulBytes)}</div></div>
          </div>
          {snap.mode === 'live' && <p className="dim small">Volumes count from when the dashboard backend started. Source: UPF N6 tunnel interface counters.</p>}
        </Card>
      </div>
    </>
  )
}
