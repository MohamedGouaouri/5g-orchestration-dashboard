import React from 'react'
import { CheckCircle2, LineChart as LineIcon, PieChart, RadioTower, Users } from 'lucide-react'
import {
  bucket, Card, CityScape, COLORS, Events, GnbConfigList, NFCard, SliceDonut, ThroughputChart, TowerIcon, TrafficSide, UETable,
} from '../components/common'

export default function Overview({ snap, go }) {
  const k = snap.kpis
  // 8 buckets over the last ~8 min like the design, falling back to what we have
  const traffic = bucket(snap.traffic, 8)
  const coreNFs = snap.core.filter((n) => n.kind === 'core' || n.kind === 'db').slice(0, 6)
  const coreOK = coreNFs.every((n) => n.status === 'online')
  const g = snap.gnbs[0]
  const sorted = [...snap.ues].sort((a, b) => (a.status === 'Active' ? -1 : 1) - (b.status === 'Active' ? -1 : 1) || a.id.localeCompare(b.id))

  return (
    <>
      <div className="grid g-ov1">
        <Card title="Network Traffic" icon={<LineIcon size={22} color={COLORS.dl} />}>
          <div className="traffic-wrap">
            <div>
              <div className="legend">
                <span><span className="dot" style={{ background: COLORS.dl }} />DL Throughput</span>
                <span><span className="dot" style={{ background: COLORS.ul }} />UL Throughput</span>
              </div>
              <ThroughputChart data={traffic} height={220} dots />
            </div>
            <TrafficSide k={k} />
          </div>
        </Card>

        <Card title="5G Core Health" icon={<CheckCircle2 size={24} color="#0b1426" fill={coreOK ? COLORS.green : COLORS.orange} />}
          right={<button className="link-btn" onClick={() => go('core')}>Details</button>}>
          <div className="nf-grid">
            {coreNFs.map((nf) => <NFCard key={nf.container} nf={nf} />)}
            {!coreNFs.length && <div className="empty">No core network functions found</div>}
          </div>
        </Card>
      </div>

      <div className="grid g-ov2 mt">
        <Card title="gNB Cells" icon={<RadioTower size={22} color={COLORS.dl} />} right={snap.gnbs.length > 1 && `${snap.gnbs.length} gNBs`}>
          {g ? (
            <div className="gnb">
              <div className="gnb-vis">
                <TowerIcon size={120} status={g.status} />
                <span className={`badge ${g.status}`}>{g.status === 'online' ? 'Online' : g.status === 'degraded' ? 'Degraded' : 'Offline'}</span>
                <div className="gnb-name">{g.name}</div>
              </div>
              <GnbConfigList g={g} />
            </div>
          ) : <div className="empty">No gNB connected</div>}
        </Card>

        <Card title="Connected UEs" icon={<Users size={22} color={COLORS.dl} fill={COLORS.dl} />} className="flush" right={<span>Total: {snap.ues.length}</span>}>
          <div style={{ paddingTop: 10 }}><UETable ues={sorted} limit={5} onSelect={() => go('subscribers')} /></div>
        </Card>

        <Card title="Traffic Distribution" icon={<PieChart size={22} color={COLORS.dl} />}>
          <SliceDonut slices={snap.slices} totalBytes={k.totalBytes} />
        </Card>
      </div>

      <div className="grid g-ov3 mt">
        <Events events={snap.events} limit={4} onViewAll={() => go('core')} />
        <CityScape />
      </div>
    </>
  )
}
