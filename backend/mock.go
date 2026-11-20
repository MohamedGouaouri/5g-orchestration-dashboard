package main

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"
)

// Mock generates a realistic synthetic 5G network for demos.
type Mock struct {
	mu     sync.RWMutex
	rng    *rand.Rand
	hist   *history
	ev     *eventLog
	ues    []UE
	subs   []Subscriber
	totDL  uint64
	totUL  uint64
	slices map[string]uint64
	start  time.Time
	cur    TrafficPoint
}

func NewMock(histSize int) *Mock {
	m := &Mock{rng: rand.New(rand.NewSource(42)), hist: &history{max: histSize}, ev: &eventLog{}, slices: map[string]uint64{}, start: time.Now()}
	dnns := []string{"internet", "internet", "internet", "iot", "ims"}
	for i := 0; i < 80; i++ {
		imsi := fmt.Sprintf("0010101%08d", i+1)
		m.subs = append(m.subs, Subscriber{IMSI: imsi, AuthMethod: "5G_AKA", DNN: dnns[i%5], Slice: []string{"eMBB", "eMBB", "URLLC", "mMTC", "eMBB"}[i%5], Source: "mock"})
		if i < 72 {
			cell := "Cell-1"
			if i%3 == 2 {
				cell = "Cell-2"
			}
			u := UE{ID: fmt.Sprintf("UE-%03d", i+1), IMSI: imsi, IP: fmt.Sprintf("10.45.0.%d", i+2), DNN: dnns[i%5], Slice: m.subs[i].Slice,
				State: "5GMM-REGISTERED", Status: "Active", CellID: cell, GNB: "gNB-001", RANUEID: fmt.Sprintf("0x%02x", i+1), AMFUEID: fmt.Sprintf("0x%02x", i+0x20), RNTI: 0x4600 + i*7}
			if i%9 == 3 {
				u.Status = "Idle"
			}
			m.ues = append(m.ues, u)
			m.subs[i].Registered = true
		}
	}
	now := time.Now()
	for i := histSize; i > 0; i-- {
		m.step(now.Add(-time.Duration(i) * time.Second))
	}
	m.ev.add("success", "gNB-001 connected to AMF (NG Setup complete)")
	m.ev.add("info", "Mock data mode — no OAI stack connected")
	return m
}

func (m *Mock) Run(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			m.step(now)
			m.randomEvent()
		}
	}
}

func (m *Mock) step(now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	x := float64(now.Unix())
	dl := 780 + 90*math.Sin(x/90) + 40*math.Sin(x/13) + m.rng.Float64()*40
	ul := 220 + 30*math.Sin(x/70+1) + 15*math.Sin(x/9) + m.rng.Float64()*15
	loss := math.Max(0, 0.08+0.05*math.Sin(x/40)+m.rng.NormFloat64()*0.03)
	m.cur = TrafficPoint{T: now.UnixMilli(), DLMbps: r2(dl), ULMbps: r2(ul), DLPps: r2(dl * 1e6 / 8 / 1200), ULPps: r2(ul * 1e6 / 8 / 900), LossPct: r2(loss)}
	m.cur.Drops = uint64((m.cur.DLPps + m.cur.ULPps) * loss / 100)
	m.hist.add(m.cur)
	dlB, ulB := uint64(dl*1e6/8), uint64(ul*1e6/8)
	m.totDL += dlB
	m.totUL += ulB
	// weights per UE
	var wsum float64
	w := make([]float64, len(m.ues))
	for i, u := range m.ues {
		if u.Status == "Active" {
			w[i] = 0.3 + math.Abs(math.Sin(float64(i)*1.7+x/60))
			if u.Slice == "mMTC" {
				w[i] *= 0.1
			}
			wsum += w[i]
		}
	}
	for i := range m.ues {
		u := &m.ues[i]
		f := 0.0
		if wsum > 0 {
			f = w[i] / wsum
		}
		u.DLMbps, u.ULMbps = r2(dl*f), r2(ul*f)
		u.DLBytes += uint64(float64(dlB) * f)
		u.ULBytes += uint64(float64(ulB) * f)
		m.slices[u.Slice] += uint64(float64(dlB+ulB) * f)
		if u.Status == "Active" {
			u.MCSDL = 18 + int(8*math.Abs(math.Sin(float64(i)+x/30)))
			u.MCSUL = 12 + int(8*math.Abs(math.Sin(float64(i)*0.7+x/25)))
			u.BLERDL = r4(math.Abs(0.02 + 0.03*math.Sin(float64(i)+x/20)))
			u.BLERUL = r4(math.Abs(0.04 + 0.04*math.Sin(float64(i)*1.3+x/17)))
			u.PRBDL = int(273 * f * 8)
			u.PRBUL = int(273 * f * 5)
		} else {
			u.MCSDL, u.MCSUL, u.PRBDL, u.PRBUL = 0, 0, 0, 0
		}
	}
}

func (m *Mock) randomEvent() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rng.Intn(6) != 0 {
		return
	}
	i := m.rng.Intn(len(m.ues))
	u := &m.ues[i]
	switch m.rng.Intn(4) {
	case 0:
		m.ev.add("success", "PDU session established for %s (DNN: %s)", u.ID, u.DNN)
	case 1:
		from := u.CellID
		if from == "Cell-1" {
			u.CellID = "Cell-2"
		} else {
			u.CellID = "Cell-1"
		}
		m.ev.add("success", "Handover completed: %s (%s → %s)", u.ID, from, u.CellID)
	case 2:
		if u.Status == "Active" {
			u.Status = "Idle"
			m.ev.add("warning", "%s moved to CM-IDLE", u.ID)
		} else {
			u.Status = "Active"
			m.ev.add("info", "%s service request (CM-CONNECTED)", u.ID)
		}
	case 3:
		m.ev.add("error", "QoS flow modified for %s (5QI: %d)", u.ID, []int{7, 8, 9}[m.rng.Intn(3)])
	}
}

func (m *Mock) History(n int) []TrafficPoint { return m.hist.last(n) }

func (m *Mock) Snapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	up := int64(time.Since(m.start).Seconds()) + 86400*3
	nf := func(name, kind, detail string, cpu, mem float64) NF {
		return NF{Name: name, Kind: kind, Container: "mock-" + name, Image: "mock/" + name, Status: "online", Health: "healthy", UptimeSec: up, CPUPct: r2(cpu + m.rng.Float64()), MemMB: mem, Detail: detail}
	}
	active, sumD, sumU := 0, 0.0, 0.0
	for _, u := range m.ues {
		if u.Status == "Active" {
			active++
			sumD += u.BLERDL
			sumU += u.BLERUL
		}
	}
	ues := append([]UE{}, m.ues...)
	var tot uint64
	for _, b := range m.slices {
		tot += b
	}
	var slices []SliceShare
	for _, n := range []string{"eMBB", "URLLC", "mMTC"} {
		slices = append(slices, SliceShare{Name: n, Bytes: m.slices[n], Pct: r2(float64(m.slices[n]) / float64(tot) * 100)})
	}
	cfg := GNBConfig{CellID: "1", PCI: 12, TAC: 1, MCC: "001", MNC: "01", SST: 1, SD: "0xffffff", Band: 78, ARFCNSSB: 641280, ARFCNPointA: 640008,
		FreqMHz: 3619.2, SCSkHz: 30, PRBs: 273, BandwidthMHz: 100, Duplex: "TDD", TDDPeriodMs: 5, DLSlots: 7, ULSlots: 2, NbTx: 2, NbRx: 2,
		SSBPower: -25, PRACHIndex: 159, AMFAddr: "10.0.0.2", NGAddr: "10.0.0.10", MaxRxGain: 114, RawAvailable: true}
	return Snapshot{
		Mode: "mock", Timestamp: time.Now(), Network: "online", Control: false,
		KPIs: KPIs{Subscribers: len(m.subs), RegisteredUEs: len(m.ues), ActiveUEs: active, PDUSessions: len(m.ues),
			DLMbps: m.cur.DLMbps, ULMbps: m.cur.ULMbps, DLPps: m.cur.DLPps, ULPps: m.cur.ULPps, LossPct: m.cur.LossPct,
			DLBytes: m.totDL + 9e11, ULBytes: m.totUL + 3e11, TotalBytes: m.totDL + m.totUL + 1.2e12,
			AvgBLERDL: r4(sumD / float64(max(active, 1))), AvgBLERUL: r4(sumU / float64(max(active, 1)))},
		UEs: ues,
		Core: []NF{
			nf("AMF", "core", fmt.Sprintf("%d UEs", len(m.ues)), 4, 210), nf("SMF", "core", fmt.Sprintf("%d sessions", len(m.ues)), 3, 180),
			nf("UPF", "core", fmt.Sprintf("%.1f Gbps", (m.cur.DLMbps+m.cur.ULMbps)/1000), 22, 640), nf("AUSF", "core", "", 1, 90),
			nf("UDM", "core", "", 1, 95), nf("NRF", "core", "6 NFs registered", 1, 80), nf("UDR DB", "db", "", 2, 420),
			nf("gNB", "ran", "", 180, 2400),
		},
		GNBs: []GNB{
			{ID: "0x0001", Name: "gNB-001", Status: "online", PLMN: "001,01", ConnectedUEs: len(m.ues), Config: cfg},
		},
		Traffic: m.hist.last(180), Slices: slices, Events: m.ev.recent(50),
		Sources: map[string]string{"mock": "synthetic data generator"},
	}
}

func (m *Mock) Subscribers(context.Context) ([]Subscriber, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]Subscriber{}, m.subs...), nil
}
