import React, { useState } from 'react'
import { Cpu, RadioTower, Signal } from 'lucide-react'
import { Card, COLORS, Dot, TowerIcon } from '../components/common'

function TDD({ c }) {
  if (c.duplex !== 'TDD' || !c.tddPeriodMs) return <span className="muted">FDD</span>
  const slots = Math.round(c.tddPeriodMs * (c.scsKHz / 15))
  const special = Math.max(0, slots - c.dlSlots - c.ulSlots)
  const pattern = [...Array(c.dlSlots).fill('D'), ...Array(special).fill('S'), ...Array(c.ulSlots).fill('U')]
  return (
    <div>
      <div className="tdd">{pattern.map((s, i) => <span key={i} className={s}>{s}</span>)}</div>
      <div className="dim small" style={{ marginTop: 6 }}>{c.tddPeriodMs} ms period · {slots} slots · {c.dlSlots}D / {special}S / {c.ulSlots}U · DL ratio ≈ {Math.round(((c.dlSlots + special * 0.5) / slots) * 100)}%</div>
    </div>
  )
}

function Row({ k, v }) { return <><dt>{k}</dt><dd>{v ?? '—'}</dd></> }

function bler(v) {
  const p = v * 100
  return <span className={p > 10 ? 'red' : p > 3 ? 'orange' : 'green'}>{p.toFixed(1)}%</span>
}

export default function Ran({ snap }) {
  const [idx, setIdx] = useState(0)
  const g = snap.gnbs[idx] || snap.gnbs[0]
  const c = g?.config || {}
  const radio = snap.ues.filter((u) => u.rnti)
  const ranNFs = snap.core.filter((n) => n.kind === 'ran' || n.kind === 'ric')

  return (
    <>
      {snap.sources?.xapp && snap.sources.xapp !== 'ok' && snap.mode === 'live' && <div className="notice">RIC MAC statistics: {snap.sources.xapp}</div>}
      <div className="grid g-ov1">
        <Card title="gNB" icon={<RadioTower size={22} color={COLORS.dl} />}
          right={snap.gnbs.length > 1 && <select value={idx} onChange={(e) => setIdx(+e.target.value)}>{snap.gnbs.map((x, i) => <option key={x.id} value={i}>{x.name}</option>)}</select>}>
          {g ? (
            <div className="gnb" style={{ gridTemplateColumns: '170px 1fr 1fr', alignItems: 'start' }}>
              <div className="gnb-vis">
                <TowerIcon size={140} status={g.status} />
                <span className={`badge ${g.status}`}>{g.status}</span>
                <div className="gnb-name">{g.name}</div>
                <div className="dim small mono">ID {g.id} · PLMN {g.plmn || `${c.mcc},${c.mnc}`}</div>
              </div>
              <dl className="kv">
                <Row k="NR Cell ID" v={c.cellId} />
                <Row k="PCI" v={c.pci} />
                <Row k="TAC" v={c.tac} />
                <Row k="PLMN" v={`${c.mcc}-${c.mnc}`} />
                <Row k="S-NSSAI" v={`SST ${c.sst}${c.sd ? ` / SD ${c.sd}` : ''}`} />
                <Row k="Connected UEs" v={g.connectedUEs} />
                <Row k="AMF (N2)" v={<span className="mono">{c.amfAddr}</span>} />
                <Row k="gNB NG address" v={<span className="mono">{c.ngAddr}</span>} />
              </dl>
              <dl className="kv">
                <Row k="Band" v={`n${c.band}`} />
                <Row k="SSB frequency" v={c.freqMHz ? `${c.freqMHz} MHz` : '—'} />
                <Row k="SSB / Point A ARFCN" v={<span className="mono">{c.arfcnSsb} / {c.arfcnPointA}</span>} />
                <Row k="Bandwidth" v={`${c.bandwidthMHz} MHz (${c.prbs} PRB)`} />
                <Row k="Subcarrier spacing" v={`${c.scsKHz} kHz`} />
                <Row k="Antennas (Tx × Rx)" v={`${c.nbTx} × ${c.nbRx}`} />
                <Row k="SSB block power" v={`${c.ssbPower} dBm`} />
                <Row k="PRACH config idx" v={c.prachIndex} />
              </dl>
            </div>
          ) : <div className="empty">No gNB connected to the AMF</div>}
        </Card>

        <Card title="Frame Structure" icon={<Signal size={22} color={COLORS.dl} />}>
          {g ? <>
            <div className="muted small" style={{ marginBottom: 10 }}>Duplex: <b className="blue">{c.duplex}</b></div>
            <TDD c={c} />
            <div className="grid g-2 mt">
              <div className="stat"><div className="l">Max RX gain</div><div className="v">{c.maxRxGain}<small>dB</small></div></div>
              <div className="stat"><div className="l">Numerology µ</div><div className="v">{Math.log2((c.scsKHz || 15) / 15)}</div></div>
            </div>
          </> : <div className="empty">—</div>}
        </Card>
      </div>

      <Card className="mt" title="UE Radio Metrics (MAC, via Near-RT RIC)" icon={<Signal size={22} color={COLORS.green} />}>
        {radio.length ? (
          <div className="table-wrap">
            <table>
              <thead><tr><th>UE</th><th>IMSI</th><th>C-RNTI</th><th>MCS DL</th><th>MCS UL</th><th>BLER DL</th><th>BLER UL</th><th>PRB DL</th><th>PRB UL</th><th>DL MCS</th></tr></thead>
              <tbody>
                {radio.map((u) => (
                  <tr key={u.imsi}>
                    <td>{u.id} {u.radioStale && <span className="badge degraded">stale</span>}</td>
                    <td className="mono">{u.imsi}</td>
                    <td className="mono">0x{u.rnti.toString(16)}</td>
                    <td>{u.mcsDl}</td><td>{u.mcsUl}</td>
                    <td>{bler(u.blerDl)}</td><td>{bler(u.blerUl)}</td>
                    <td>{u.prbDl}</td><td>{u.prbUl}</td>
                    <td style={{ minWidth: 120 }}><div className="bar"><i style={{ width: `${(u.mcsDl / 28) * 100}%` }} /></div></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : <div className="empty">No MAC statistics available{snap.mode === 'live' ? ' (set XAPP_ZMQ to the monitor xApp publisher)' : ''}</div>}
      </Card>

      <Card className="mt" title="RAN & RIC Components" icon={<Cpu size={22} color={COLORS.dl} />}>
        <div className="nf-grid" style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(200px, 1fr))' }}>
          {ranNFs.map((n) => (
            <div className="nf" key={n.container}>
              <Dot s={n.status} /><div className="name">{n.name}</div>
              <div className="st muted small">{n.container}</div>
              <div className="det">CPU {n.cpuPct.toFixed(1)}% · {n.memMB >= 1024 ? `${(n.memMB / 1024).toFixed(1)} GB` : `${n.memMB.toFixed(0)} MB`}</div>
            </div>
          ))}
          {!ranNFs.length && <div className="empty">No RAN containers detected</div>}
        </div>
      </Card>
    </>
  )
}
