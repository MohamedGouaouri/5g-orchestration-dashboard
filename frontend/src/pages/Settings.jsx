import React from 'react'
import { Database, Plug, ShieldCheck } from 'lucide-react'
import { Card, COLORS, Dot } from '../components/common'

const DESCR = {
  docker: 'Docker Engine API — NF discovery, health, CPU/memory',
  amf: 'AMF logs — registered UEs and connected gNBs',
  smf: 'SMF logs — PDU sessions, UE IP and DNN',
  upf: 'UPF tunnel interface — throughput, packet rate, drops',
  'gnb-config': 'gNB configuration file — cell parameters',
  xapp: 'FlexRIC monitor xApp (ZeroMQ) — MCS, BLER, PRBs',
  mysql: 'UDR MySQL — subscriber database',
  mock: 'Synthetic demo data',
}

export default function Settings({ snap }) {
  const entries = Object.entries(snap.sources || {}).sort()
  return (
    <>
      <Card title="Data Sources" icon={<Plug size={22} color={COLORS.dl} />} right={<span className="badge blue">mode: {snap.mode}</span>}>
        <div className="table-wrap">
          <table>
            <thead><tr><th>Source</th><th>Description</th><th>Status</th></tr></thead>
            <tbody>
              {entries.map(([k, v]) => {
                const s = v === 'ok' || k === 'mock' ? 'online' : v.startsWith('disabled') || v.startsWith('stale') ? 'degraded' : 'offline'
                return (
                  <tr key={k}>
                    <td><b>{k}</b></td>
                    <td className="muted">{DESCR[k] || ''}</td>
                    <td style={{ whiteSpace: 'normal' }}><span className="status" style={{ alignItems: 'flex-start' }}><Dot s={s} /><span>{v}</span></span></td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      </Card>
      <div className="grid g-2 mt">
        <Card title="Management Actions" icon={<ShieldCheck size={22} color={COLORS.green} />}>
          <p>Status: <b className={snap.control ? 'green' : 'orange'}>{snap.control ? 'enabled' : 'disabled'}</b></p>
          <p className="muted">When <span className="mono">ENABLE_CONTROL=true</span> (live mode only), operators can restart network-function containers and provision or remove subscribers in the UDR database. Keep it disabled for read-only monitoring screens.</p>
        </Card>
        <Card title="Backend Configuration" icon={<Database size={22} color={COLORS.dl} />}>
          <p className="muted">Configured through environment variables on the backend container. See <span className="mono">docker-compose.yml</span> and <span className="mono">README.md</span>:</p>
          <p className="mono small" style={{ lineHeight: 1.8 }}>MODE · AMF_CONTAINER · SMF_CONTAINER · UPF_CONTAINER · GNB_CONTAINER · GNB_CONF_PATH · UPF_TUN_IFACE · XAPP_ZMQ · MYSQL_DSN · ENABLE_CONTROL</p>
        </Card>
      </div>
    </>
  )
}
