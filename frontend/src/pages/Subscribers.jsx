import React, { useMemo, useState } from 'react'
import { Plus, Search, Trash2, UserCheck, Users } from 'lucide-react'
import { api, useFetch } from '../api'
import { Card, COLORS, Dot, UETable } from '../components/common'
import { fmtBytes, rateStr } from '../format'

export default function Subscribers({ snap }) {
  const [q, setQ] = useState('')
  const [status, setStatus] = useState('all')
  const [sel, setSel] = useState(null)
  const [adding, setAdding] = useState(false)
  const subs = useFetch('/api/subscribers', 10000)

  const ues = useMemo(() => snap.ues.filter((u) =>
    (status === 'all' || u.status === status) &&
    (!q || [u.id, u.imsi, u.ip, u.dnn, u.cellId].some((f) => (f || '').toLowerCase().includes(q.toLowerCase())))), [snap.ues, q, status])
  const ue = snap.ues.find((u) => u.imsi === sel)
  const counts = snap.ues.reduce((a, u) => ({ ...a, [u.status]: (a[u.status] || 0) + 1 }), {})
  const dbBacked = subs.data?.some((s) => s.source === 'db')

  const remove = async (imsi) => {
    if (!confirm(`Remove subscriber ${imsi} from the UDR database? The UE will no longer be able to register.`)) return
    try { await api(`/api/subscribers/${imsi}`, { method: 'DELETE' }); subs.reload() } catch (e) { alert(e.message) }
  }

  return (
    <>
      <div className="grid g-4">
        <div className="stat"><div className="l">Provisioned subscribers</div><div className="v">{snap.kpis.subscribers}</div></div>
        <div className="stat"><div className="l">Registered UEs</div><div className="v">{snap.kpis.registeredUEs}</div></div>
        <div className="stat"><div className="l">Active / Idle</div><div className="v"><span className="green">{counts.Active || 0}</span> <small>/</small> <span className="orange">{counts.Idle || 0}</span></div></div>
        <div className="stat"><div className="l">PDU sessions</div><div className="v">{snap.kpis.pduSessions}</div></div>
      </div>

      <div className="grid g-ov1 mt">
        <Card title="Connected UEs" icon={<Users size={22} color={COLORS.dl} fill={COLORS.dl} />}>
          <div className="toolbar">
            <div style={{ position: 'relative' }}>
              <Search size={15} style={{ position: 'absolute', left: 10, top: 10, color: '#8a9bb8' }} />
              <input placeholder="Search IMSI, IP, DNN…" value={q} onChange={(e) => setQ(e.target.value)} style={{ paddingLeft: 32, width: 240 }} />
            </div>
            <div className="seg">
              {['all', 'Active', 'Idle', 'Deregistered'].map((s) => <button key={s} className={status === s ? 'active' : ''} onClick={() => setStatus(s)}>{s === 'all' ? 'All' : s}</button>)}
            </div>
            <span className="muted small" style={{ marginLeft: 'auto' }}>{ues.length} shown · click a row for details</span>
          </div>
          <UETable ues={ues} columns="full" onSelect={setSel} selected={sel} />
          {snap.ues.some((u) => u.estimated) && <p className="dim small">Per-UE throughput is apportioned from UPF totals by scheduled PRB share (OAI UPF does not expose per-UE counters).</p>}
        </Card>

        <Card title={ue ? ue.id : 'UE details'} icon={<UserCheck size={22} color={COLORS.dl} />}>
          {ue ? (
            <>
              <div className="toolbar"><span className={`status ${ue.status}`}><Dot s={ue.status} />{ue.status}</span><span className="badge blue">{ue.slice}</span>{ue.radioStale && <span className="badge degraded">radio stats stale</span>}</div>
              <dl className="kv" style={{ marginBottom: 16 }}>
                <dt>IMSI</dt><dd className="mono">{ue.imsi}</dd>
                <dt>5GMM state</dt><dd>{ue.state}</dd>
                <dt>IP address</dt><dd className="mono">{ue.ip || '—'}</dd>
                <dt>DNN</dt><dd>{ue.dnn || '—'}</dd>
                <dt>Serving gNB / cell</dt><dd>{ue.gnb || '—'} / {ue.cellId}</dd>
                <dt>RAN / AMF UE NGAP ID</dt><dd className="mono">{ue.ranUeId} / {ue.amfUeId}</dd>
                <dt>C-RNTI</dt><dd className="mono">{ue.rnti ? `0x${ue.rnti.toString(16)} (${ue.rnti})` : '—'}</dd>
              </dl>
              <div className="grid g-2">
                <div className="stat"><div className="l">DL throughput</div><div className="v blue">{rateStr(ue.dlMbps)}</div><div className="dim small">{fmtBytes(ue.dlBytes)} total</div></div>
                <div className="stat"><div className="l">UL throughput</div><div className="v purple">{rateStr(ue.ulMbps)}</div><div className="dim small">{fmtBytes(ue.ulBytes)} total</div></div>
                <div className="stat"><div className="l">MCS DL / UL</div><div className="v">{ue.mcsDl} <small>/</small> {ue.mcsUl}</div></div>
                <div className="stat"><div className="l">BLER DL / UL</div><div className="v">{(ue.blerDl * 100).toFixed(1)}<small>%</small> / {(ue.blerUl * 100).toFixed(1)}<small>%</small></div></div>
                <div className="stat"><div className="l">PRBs DL / UL (max)</div><div className="v">{ue.prbDl} <small>/</small> {ue.prbUl}</div></div>
              </div>
            </>
          ) : <div className="empty">Select a UE in the table to inspect its session and radio metrics.</div>}
        </Card>
      </div>

      <Card className="mt" title="Subscriber Database (UDR)" icon={<Users size={22} color={COLORS.dl} />}
        right={snap.control && dbBacked && <button className="btn" onClick={() => setAdding(true)}><Plus size={16} />Add subscriber</button>}>
        {!dbBacked && subs.data && <div className="notice info">{snap.mode === 'mock' ? 'Demo subscribers.' : 'MYSQL_DSN is not configured — listing IMSIs observed in AMF/SMF logs. Configure it to view and provision the full subscriber database.'}</div>}
        {subs.error && <div className="notice">{subs.error}</div>}
        <div className="table-wrap" style={{ maxHeight: 420, overflowY: 'auto' }}>
          <table>
            <thead><tr><th>IMSI</th><th>Auth</th><th>DNN</th><th>Slice</th><th>SQN</th><th>Registered</th>{snap.control && dbBacked && <th />}</tr></thead>
            <tbody>
              {(subs.data || []).map((s) => (
                <tr key={s.imsi}>
                  <td className="mono">{s.imsi}</td><td>{s.authMethod}</td><td>{s.dnn || '—'}</td><td>{s.slice || '—'}</td><td className="mono">{s.sqn || '—'}</td>
                  <td>{s.registered ? <span className="status Active"><Dot s="Active" />Yes</span> : <span className="muted">No</span>}</td>
                  {snap.control && dbBacked && <td><button className="btn danger" onClick={() => remove(s.imsi)} title="Remove subscriber"><Trash2 size={14} /></button></td>}
                </tr>
              ))}
            </tbody>
          </table>
          {subs.data && !subs.data.length && <div className="empty">No subscribers</div>}
        </div>
      </Card>
      {adding && <AddSubscriber onClose={() => setAdding(false)} onDone={() => { setAdding(false); subs.reload() }} />}
    </>
  )
}

function AddSubscriber({ onClose, onDone }) {
  const [f, setF] = useState({ imsi: '', key: '', opc: '', dnn: 'oai', sst: 1, sd: 'FFFFFF' })
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const set = (k) => (e) => setF({ ...f, [k]: k === 'sst' ? +e.target.value : e.target.value })
  const submit = async (e) => {
    e.preventDefault()
    setBusy(true); setErr('')
    try { await api('/api/subscribers', { method: 'POST', body: JSON.stringify(f) }); onDone() } catch (x) { setErr(x.message) } finally { setBusy(false) }
  }
  return (
    <div className="modal-bg" onClick={onClose}>
      <form className="modal form" onClick={(e) => e.stopPropagation()} onSubmit={submit}>
        <h3>Provision subscriber</h3>
        <label>IMSI (15 digits)<input value={f.imsi} onChange={set('imsi')} pattern="\d{15}" required placeholder="262990100016070" /></label>
        <label>K (32 hex)<input className="mono" value={f.key} onChange={set('key')} pattern="[0-9a-fA-F]{32}" required /></label>
        <label>OPc (32 hex)<input className="mono" value={f.opc} onChange={set('opc')} pattern="[0-9a-fA-F]{32}" required /></label>
        <div className="row2">
          <label>DNN<input value={f.dnn} onChange={set('dnn')} /></label>
          <label>Slice (SST)<select value={f.sst} onChange={set('sst')}><option value={1}>1 · eMBB</option><option value={2}>2 · URLLC</option><option value={3}>3 · mMTC</option></select></label>
        </div>
        <label>SD<input className="mono" value={f.sd} onChange={set('sd')} /></label>
        {err && <div className="err">{err}</div>}
        <div className="toolbar" style={{ justifyContent: 'flex-end', margin: 0 }}>
          <button type="button" className="btn ghost" onClick={onClose}>Cancel</button>
          <button className="btn" disabled={busy}>{busy ? 'Saving…' : 'Provision'}</button>
        </div>
      </form>
    </div>
  )
}
