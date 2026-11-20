package main

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Mode          string // auto | live | mock
	DockerSock    string
	AMFContainer  string
	SMFContainer  string
	UPFContainer  string
	GNBContainer  string
	GNBConfPath   string
	UPFTunIface   string
	UPFN3Iface    string
	XAppEndpoint  string
	MySQLDSN      string
	EnableControl bool
	HistorySize   int
}

// Source is anything that can produce snapshots.
type Source interface {
	Snapshot() Snapshot
	History(n int) []TrafficPoint
	Subscribers(ctx context.Context) ([]Subscriber, error)
}

// ring buffer of traffic points
type history struct {
	mu  sync.RWMutex
	buf []TrafficPoint
	max int
}

func (h *history) add(p TrafficPoint) {
	h.mu.Lock()
	h.buf = append(h.buf, p)
	if len(h.buf) > h.max {
		h.buf = h.buf[len(h.buf)-h.max:]
	}
	h.mu.Unlock()
}

func (h *history) last(n int) []TrafficPoint {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if n <= 0 || n > len(h.buf) {
		n = len(h.buf)
	}
	out := make([]TrafficPoint, n)
	copy(out, h.buf[len(h.buf)-n:])
	return out
}

type eventLog struct {
	mu  sync.RWMutex
	evs []Event
}

func (e *eventLog) add(level, format string, a ...any) {
	e.mu.Lock()
	e.evs = append(e.evs, Event{T: time.Now().UnixMilli(), Level: level, Msg: fmt.Sprintf(format, a...)})
	if len(e.evs) > 300 {
		e.evs = e.evs[len(e.evs)-300:]
	}
	e.mu.Unlock()
}

func (e *eventLog) recent(n int) []Event {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := []Event{}
	for i := len(e.evs) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, e.evs[i])
	}
	return out
}

// ---------------------------------------------------------------------------

type Live struct {
	cfg  Config
	dk   *Docker
	xapp *XApp
	db   *SubscriberDB
	hist *history
	ev   *eventLog

	mu       sync.RWMutex
	sources  map[string]string
	nfs      []NF
	amfGNBs  []amfGNB
	amfUEs   []amfUE
	amfOK    bool
	smf      *smfTracker
	gnbConf  GNBConfig
	subCount int

	// UPF counter state
	prev       *ifCounters
	prevN3     *ifCounters
	prevT      time.Time
	cur        TrafficPoint
	totDL      uint64
	totUL      uint64
	baseDL     uint64
	baseUL     uint64
	ueBytes    map[string][2]uint64 // imsi -> dl, ul (apportioned)
	sliceBytes map[string]uint64

	prevUEState map[string]string
	prevNF      map[string]string
	prevGNB     map[string]string
}

func NewLive(cfg Config) *Live {
	l := &Live{
		cfg:         cfg,
		dk:          NewDocker(cfg.DockerSock),
		hist:        &history{max: cfg.HistorySize},
		ev:          &eventLog{},
		sources:     map[string]string{},
		smf:         newSMFTracker(),
		ueBytes:     map[string][2]uint64{},
		sliceBytes:  map[string]uint64{},
		prevUEState: map[string]string{},
		prevNF:      map[string]string{},
		prevGNB:     map[string]string{},
	}
	if cfg.XAppEndpoint != "" {
		l.xapp = NewXApp(cfg.XAppEndpoint)
	} else {
		l.sources["xapp"] = "disabled (XAPP_ZMQ not set)"
	}
	if cfg.MySQLDSN != "" {
		db, err := OpenSubscriberDB(cfg.MySQLDSN)
		if err != nil {
			l.sources["mysql"] = "error: " + err.Error()
		} else {
			l.db = db
		}
	} else {
		l.sources["mysql"] = "disabled (MYSQL_DSN not set) — showing IMSIs observed in AMF/SMF"
	}
	return l
}

func (l *Live) setSource(k, v string) {
	l.mu.Lock()
	l.sources[k] = v
	l.mu.Unlock()
}

func (l *Live) Run(ctx context.Context) {
	if l.xapp != nil {
		go l.xapp.Run(ctx)
	}
	l.ev.add("info", "Dashboard collector started (live mode)")
	l.pollContainers(ctx)
	l.pollAMF(ctx)
	l.pollSMF(ctx, false)
	l.pollGNBConf(ctx)
	l.pollSubscribers(ctx)

	every := func(d time.Duration, f func(context.Context)) {
		go func() {
			t := time.NewTicker(d)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					c, cancel := context.WithTimeout(ctx, d*3)
					f(c)
					cancel()
				}
			}
		}()
	}
	every(1*time.Second, l.pollUPF)
	every(3*time.Second, l.pollAMF)
	every(3*time.Second, func(c context.Context) { l.pollSMF(c, true) })
	every(5*time.Second, l.pollContainers)
	every(30*time.Second, l.pollGNBConf)
	every(15*time.Second, l.pollSubscribers)
	<-ctx.Done()
}

// ---------------- pollers ----------------

var nfPattern = regexp.MustCompile(`(?i)(amf|smf|upf|nrf|ausf|udm|udr|pcf|nssf|nef|lmf|mysql|gnb|ric|xapp|ext-dn)`)

func nfKind(n string) string {
	switch n {
	case "AMF", "SMF", "UPF", "NRF", "AUSF", "UDM", "UDR", "PCF", "NSSF", "NEF", "LMF":
		return "core"
	case "MYSQL":
		return "db"
	case "GNB":
		return "ran"
	case "RIC", "XAPP":
		return "ric"
	}
	return "other"
}

func (l *Live) pollContainers(ctx context.Context) {
	list, err := l.dk.List(ctx)
	if err != nil {
		l.setSource("docker", "error: "+err.Error())
		return
	}
	l.setSource("docker", "ok")
	var nfs []NF
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, c := range list {
		m := nfPattern.FindString(c.Name())
		if m == "" {
			continue
		}
		c := c
		wg.Add(1)
		go func() {
			defer wg.Done()
			nf := NF{Name: strings.ToUpper(m), Container: c.Name(), Image: c.Image}
			switch nf.Name {
			case "RIC":
				nf.Name = "Near-RT RIC"
			case "XAPP":
				nf.Name = "Monitor xApp"
			case "EXT-DN":
				nf.Name = "Ext-DN"
			case "MYSQL":
				nf.Name = "UDR DB"
			case "GNB":
				nf.Name = "gNB"
			}
			nf.Kind = nfKind(strings.ToUpper(m))
			var ips []string
			for k, v := range c.NetworkSettings.Networks {
				if v.IPAddress != "" {
					ips = append(ips, k+"="+v.IPAddress)
				}
			}
			sort.Strings(ips)
			nf.IPs = strings.Join(ips, ", ")
			ins, err := l.dk.Inspect(ctx, c.ID)
			if err == nil {
				nf.StartedAt = ins.State.StartedAt
				if t, err := time.Parse(time.RFC3339Nano, ins.State.StartedAt); err == nil && ins.State.Running {
					nf.UptimeSec = int64(time.Since(t).Seconds())
				}
				nf.Health = ins.State.Status
				if ins.State.Health != nil {
					nf.Health = ins.State.Health.Status
				}
			}
			switch {
			case c.State != "running":
				nf.Status = "offline"
			case nf.Health == "unhealthy":
				nf.Status = "degraded"
			default:
				nf.Status = "online"
			}
			if c.State == "running" {
				if cpu, mem, err := l.dk.Stats(ctx, c.ID); err == nil {
					nf.CPUPct = math.Round(cpu*10) / 10
					nf.MemMB = math.Round(mem*10) / 10
				}
			}
			mu.Lock()
			nfs = append(nfs, nf)
			mu.Unlock()
		}()
	}
	wg.Wait()
	order := map[string]int{"core": 0, "db": 1, "ran": 2, "ric": 3, "other": 4}
	sort.Slice(nfs, func(i, j int) bool {
		if order[nfs[i].Kind] != order[nfs[j].Kind] {
			return order[nfs[i].Kind] < order[nfs[j].Kind]
		}
		return nfs[i].Name < nfs[j].Name
	})
	l.mu.Lock()
	for _, nf := range nfs {
		if p, ok := l.prevNF[nf.Container]; ok && p != nf.Status {
			lvl := map[string]string{"online": "success", "degraded": "warning", "offline": "error"}[nf.Status]
			l.ev.add(lvl, "%s (%s) is now %s", nf.Name, nf.Container, nf.Status)
		}
		l.prevNF[nf.Container] = nf.Status
	}
	l.nfs = nfs
	l.mu.Unlock()
}

func (l *Live) pollAMF(ctx context.Context) {
	text, err := l.dk.Logs(ctx, l.cfg.AMFContainer, 0, 400)
	if err != nil {
		l.setSource("amf", "error: "+err.Error())
		return
	}
	gnbs, ues, ok := parseAMF(text)
	if !ok {
		l.setSource("amf", "no UE/gNB tables found in recent AMF logs")
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sources["amf"] = "ok"
	seen := map[string]bool{}
	for _, u := range ues {
		seen[u.IMSI] = true
		if p, ok := l.prevUEState[u.IMSI]; !ok || p != u.State {
			if ok || l.amfOK {
				if strings.Contains(u.State, "DEREGISTERED") {
					l.ev.add("warning", "UE imsi-%s deregistered", u.IMSI)
				} else {
					l.ev.add("info", "UE imsi-%s %s (cell %s)", u.IMSI, strings.ToLower(u.State), u.CellID)
				}
			}
		}
		l.prevUEState[u.IMSI] = u.State
	}
	for imsi := range l.prevUEState {
		if !seen[imsi] {
			l.ev.add("warning", "UE imsi-%s removed from AMF context", imsi)
			delete(l.prevUEState, imsi)
		}
	}
	for _, g := range gnbs {
		if p, ok := l.prevGNB[g.GlobalID]; ok && p != g.Status {
			l.ev.add(map[bool]string{true: "success", false: "error"}[g.Status == "Connected"], "gNB %s (%s) %s", g.Name, g.GlobalID, strings.ToLower(g.Status))
		} else if !ok && l.amfOK {
			l.ev.add("success", "gNB %s (%s) connected to AMF", g.Name, g.GlobalID)
		}
		l.prevGNB[g.GlobalID] = g.Status
	}
	l.amfGNBs, l.amfUEs, l.amfOK = gnbs, ues, true
}

func (l *Live) pollSMF(ctx context.Context, emit bool) {
	since := int64(0)
	l.mu.RLock()
	if l.smf.lastTS != "" {
		if t, err := time.Parse(time.RFC3339Nano, l.smf.lastTS); err == nil {
			since = t.Unix()
		}
	}
	l.mu.RUnlock()
	text, err := l.dk.Logs(ctx, l.cfg.SMFContainer, since, 0)
	if err != nil {
		l.setSource("smf", "error: "+err.Error())
		return
	}
	l.mu.Lock()
	l.smf.feed(text, emit)
	for _, e := range l.smf.pending {
		if e.kind == "pdu" {
			l.ev.add("success", "PDU session established for imsi-%s (IP %s, DNN %s)", e.imsi, e.ip, e.dnn)
		} else {
			l.ev.add("info", "PDU session released for imsi-%s (IP %s)", e.imsi, e.ip)
		}
	}
	l.smf.pending = nil
	l.sources["smf"] = "ok"
	l.mu.Unlock()
}

func (l *Live) pollGNBConf(ctx context.Context) {
	text, err := l.dk.Exec(ctx, l.cfg.GNBContainer, "cat", l.cfg.GNBConfPath)
	if err != nil || strings.Contains(text, "No such file") {
		if err == nil {
			err = fmt.Errorf("%s", strings.TrimSpace(text))
		}
		l.setSource("gnb-config", "error: "+err.Error())
		return
	}
	c := parseGNBConf(text)
	l.mu.Lock()
	l.gnbConf = c
	l.sources["gnb-config"] = "ok"
	l.mu.Unlock()
}

func (l *Live) pollSubscribers(ctx context.Context) {
	if l.db == nil {
		return
	}
	subs, err := l.db.List(ctx)
	if err != nil {
		l.setSource("mysql", "error: "+err.Error())
		return
	}
	l.mu.Lock()
	l.subCount = len(subs)
	l.sources["mysql"] = "ok"
	l.mu.Unlock()
}

func (l *Live) pollUPF(ctx context.Context) {
	text, err := l.dk.Exec(ctx, l.cfg.UPFContainer, "cat", "/proc/net/dev")
	now := time.Now()
	if err != nil {
		l.setSource("upf", "error: "+err.Error())
		l.hist.add(TrafficPoint{T: now.UnixMilli()})
		return
	}
	ifs := parseNetDev(text)
	tun, ok := ifs[l.cfg.UPFTunIface]
	if !ok {
		l.setSource("upf", "interface "+l.cfg.UPFTunIface+" not found in UPF")
		return
	}
	n3 := ifs[l.cfg.UPFN3Iface]

	l.mu.Lock()
	defer l.mu.Unlock()
	l.sources["upf"] = "ok"
	if l.prev == nil {
		l.baseDL, l.baseUL = tun.TxBytes, tun.RxBytes
		l.prev, l.prevN3, l.prevT = &tun, &n3, now
		return
	}
	dt := now.Sub(l.prevT).Seconds()
	if dt <= 0 {
		return
	}
	d := func(a, b uint64) uint64 { // counter delta, tolerant of resets
		if a >= b {
			return a - b
		}
		return a
	}
	// On the UPF tun interface: RX = packets coming from UEs (uplink), TX = towards UEs (downlink).
	dlB, ulB := d(tun.TxBytes, l.prev.TxBytes), d(tun.RxBytes, l.prev.RxBytes)
	dlP, ulP := d(tun.TxPkts, l.prev.TxPkts), d(tun.RxPkts, l.prev.RxPkts)
	drops := d(tun.RxDrop+tun.TxDrop+tun.RxErrs+tun.TxErrs, l.prev.RxDrop+l.prev.TxDrop+l.prev.RxErrs+l.prev.TxErrs) +
		d(n3.RxDrop+n3.RxErrs+n3.TxDrop+n3.TxErrs, l.prevN3.RxDrop+l.prevN3.RxErrs+l.prevN3.TxDrop+l.prevN3.TxErrs)
	loss := 0.0
	if tot := dlP + ulP + drops; tot > 0 {
		loss = float64(drops) / float64(tot) * 100
	}
	l.cur = TrafficPoint{
		T: now.UnixMilli(), DLMbps: r2(float64(dlB) * 8 / dt / 1e6), ULMbps: r2(float64(ulB) * 8 / dt / 1e6),
		DLPps: r2(float64(dlP) / dt), ULPps: r2(float64(ulP) / dt), LossPct: r2(loss), Drops: drops,
	}
	l.totDL, l.totUL = l.totDL+dlB, l.totUL+ulB
	l.prev, l.prevN3, l.prevT = &tun, &n3, now
	l.hist.add(l.cur)
	l.apportion(dlB, ulB)
}

// apportion splits measured UPF bytes across active UEs by their PRB share (xApp), or evenly.
func (l *Live) apportion(dlB, ulB uint64) {
	ues := l.activeIMSIs()
	if len(ues) == 0 {
		return
	}
	var radio []xappUE
	if l.xapp != nil {
		if r, fresh, _ := l.xapp.Get(); fresh {
			radio = r
		}
	}
	wDL, wUL := make([]float64, len(ues)), make([]float64, len(ues))
	var sDL, sUL float64
	for i := range ues {
		if i < len(radio) {
			wDL[i], wUL[i] = float64(radio[i].DeltaPRBsDown), float64(radio[i].DeltaPRBsUp)
		}
		sDL, sUL = sDL+wDL[i], sUL+wUL[i]
	}
	for i, u := range ues {
		fd, fu := 1/float64(len(ues)), 1/float64(len(ues))
		if sDL > 0 {
			fd = wDL[i] / sDL
		}
		if sUL > 0 {
			fu = wUL[i] / sUL
		}
		b := l.ueBytes[u.IMSI]
		b[0] += uint64(float64(dlB) * fd)
		b[1] += uint64(float64(ulB) * fu)
		l.ueBytes[u.IMSI] = b
		slice := "eMBB"
		if s := l.smf.sess[u.IMSI]; s != nil && s.Slice != "" {
			slice = s.Slice
		}
		l.sliceBytes[slice] += uint64(float64(dlB)*fd + float64(ulB)*fu)
	}
}

func (l *Live) activeIMSIs() []amfUE {
	var out []amfUE
	for _, u := range l.amfUEs {
		if strings.Contains(u.State, "REGISTERED") && !strings.Contains(u.State, "DEREGISTERED") {
			out = append(out, u)
		}
	}
	return out
}

func r2(v float64) float64 { return math.Round(v*100) / 100 }

// ---------------- snapshot ----------------

func (l *Live) History(n int) []TrafficPoint { return l.hist.last(n) }

func (l *Live) Snapshot() Snapshot {
	var radio []xappUE
	radioFresh := false
	xstat := ""
	if l.xapp != nil {
		radio, radioFresh, xstat = l.xapp.Get()
	}

	l.mu.RLock()
	defer l.mu.RUnlock()
	s := Snapshot{
		Mode: "live", Timestamp: time.Now(), Sources: map[string]string{}, Control: l.cfg.EnableControl,
		UEs: []UE{}, Core: append([]NF{}, l.nfs...), GNBs: []GNB{}, Slices: []SliceShare{},
	}
	for k, v := range l.sources {
		s.Sources[k] = v
	}
	if l.xapp != nil {
		s.Sources["xapp"] = xstat
	}

	// UEs: AMF context + SMF session + xApp radio stats (matched in order; RNTI<->NGAP ID is not exposed)
	active := 0
	ri := 0
	gnbName := ""
	if len(l.amfGNBs) > 0 {
		gnbName = l.amfGNBs[0].Name
	}
	var sumBlerDL, sumBlerUL float64
	nActive := len(l.activeIMSIs())
	for i, a := range l.amfUEs {
		u := UE{ID: fmt.Sprintf("UE-%03d", i+1), IMSI: a.IMSI, State: a.State, CellID: a.CellID, RANUEID: a.RANUEID, AMFUEID: a.AMFUEID, GNB: gnbName, Slice: "eMBB"}
		if ss := l.smf.sess[a.IMSI]; ss != nil {
			u.IP, u.DNN = ss.IP, ss.DNN
			if ss.Slice != "" {
				u.Slice = ss.Slice
			}
		}
		registered := strings.Contains(a.State, "REGISTERED") && !strings.Contains(a.State, "DEREGISTERED")
		myRadio := -1
		if registered && ri < len(radio) {
			x := radio[ri]
			myRadio = ri
			ri++
			u.RNTI, u.MCSDL, u.MCSUL, u.BLERDL, u.BLERUL = x.RNTI, x.MCSDown, x.MCSUp, r4(x.BLERDown), r4(x.BLERUp)
			u.PRBDL, u.PRBUL = x.NumPRBsDown, x.NumPRBsUp
			u.RadioStale = !radioFresh
			sumBlerDL += x.BLERDown
			sumBlerUL += x.BLERUp
		}
		switch {
		case !registered:
			u.Status = "Deregistered"
		case u.RNTI != 0 && radioFresh, u.IP != "" && l.cur.DLPps+l.cur.ULPps > 0:
			u.Status = "Active"
		default:
			u.Status = "Idle"
		}
		if registered && nActive > 0 {
			b := l.ueBytes[a.IMSI]
			u.DLBytes, u.ULBytes = b[0], b[1]
			share := 1 / float64(nActive)
			wD, wU := share, share
			if radioFresh && myRadio >= 0 {
				var td, tu float64
				for _, x := range radio {
					td += float64(x.DeltaPRBsDown)
					tu += float64(x.DeltaPRBsUp)
				}
				if td > 0 {
					wD = float64(radio[myRadio].DeltaPRBsDown) / td
				}
				if tu > 0 {
					wU = float64(radio[myRadio].DeltaPRBsUp) / tu
				}
			}
			u.DLMbps, u.ULMbps = r2(l.cur.DLMbps*wD), r2(l.cur.ULMbps*wU)
			u.Estimated = nActive > 1
		}
		if u.Status == "Active" {
			active++
		}
		s.UEs = append(s.UEs, u)
	}

	pdu := 0
	for _, ss := range l.smf.sess {
		if ss.IP != "" && l.prevUEState[ss.IMSI] != "" {
			pdu++
		}
	}
	subs := l.subCount
	if l.db == nil {
		subs = len(l.observedIMSIs())
	}
	s.KPIs = KPIs{
		Subscribers: subs, RegisteredUEs: nActive, ActiveUEs: active, PDUSessions: pdu,
		DLMbps: l.cur.DLMbps, ULMbps: l.cur.ULMbps, DLPps: l.cur.DLPps, ULPps: l.cur.ULPps, LossPct: l.cur.LossPct,
		DLBytes: l.totDL, ULBytes: l.totUL, TotalBytes: l.totDL + l.totUL,
	}
	if ri > 0 {
		s.KPIs.AvgBLERDL, s.KPIs.AvgBLERUL = r4(sumBlerDL/float64(ri)), r4(sumBlerUL/float64(ri))
	}
	for i := range s.Core {
		switch s.Core[i].Name {
		case "AMF":
			s.Core[i].Detail = fmt.Sprintf("%d UEs · %d gNB", nActive, len(l.amfGNBs))
		case "SMF":
			s.Core[i].Detail = fmt.Sprintf("%d sessions", pdu)
		case "UPF":
			s.Core[i].Detail = fmt.Sprintf("%.1f Mbps", l.cur.DLMbps+l.cur.ULMbps)
		case "gNB":
			s.Core[i].Detail = fmt.Sprintf("%d UEs", nActive)
		case "UDR DB":
			if l.db != nil {
				s.Core[i].Detail = fmt.Sprintf("%d subscribers", l.subCount)
			}
		}
	}

	// gNBs
	gnbContainerUp := false
	for _, nf := range l.nfs {
		if nf.Kind == "ran" && nf.Status != "offline" {
			gnbContainerUp = true
		}
	}
	for _, g := range l.amfGNBs {
		st := "offline"
		if g.Status == "Connected" {
			st = "online"
		}
		s.GNBs = append(s.GNBs, GNB{ID: g.GlobalID, Name: g.Name, Status: st, PLMN: g.PLMN, ConnectedUEs: nActive, Config: l.gnbConf})
	}
	if len(s.GNBs) == 0 && l.gnbConf.RawAvailable {
		st := "offline"
		if gnbContainerUp {
			st = "degraded"
		}
		s.GNBs = append(s.GNBs, GNB{ID: "-", Name: l.cfg.GNBContainer, Status: st, Config: l.gnbConf})
	}

	// slices
	var tot uint64
	for _, b := range l.sliceBytes {
		tot += b
	}
	for _, name := range []string{"eMBB", "URLLC", "mMTC"} {
		b := l.sliceBytes[name]
		pct := 0.0
		if tot > 0 {
			pct = r2(float64(b) / float64(tot) * 100)
		}
		s.Slices = append(s.Slices, SliceShare{Name: name, Pct: pct, Bytes: b})
	}

	// overall status
	s.Network = "online"
	for _, nf := range l.nfs {
		if nf.Kind == "core" && nf.Status == "offline" {
			s.Network = "offline"
			break
		}
		if nf.Kind == "core" && nf.Status == "degraded" {
			s.Network = "degraded"
		}
	}
	if len(l.nfs) == 0 {
		s.Network = "offline"
	}
	s.Events = l.ev.recent(50)
	s.Traffic = l.hist.last(180)
	return s
}

func (l *Live) observedIMSIs() map[string]bool {
	m := map[string]bool{}
	for k := range l.smf.sess {
		m[k] = true
	}
	for _, u := range l.amfUEs {
		m[u.IMSI] = true
	}
	return m
}

func (l *Live) Subscribers(ctx context.Context) ([]Subscriber, error) {
	l.mu.RLock()
	reg := map[string]bool{}
	for _, u := range l.activeIMSIs() {
		reg[u.IMSI] = true
	}
	obs := l.observedIMSIs()
	sess := map[string]smfSession{}
	for k, v := range l.smf.sess {
		sess[k] = *v
	}
	l.mu.RUnlock()

	if l.db != nil {
		subs, err := l.db.List(ctx)
		if err != nil {
			return nil, err
		}
		for i := range subs {
			subs[i].Registered = reg[subs[i].IMSI]
		}
		return subs, nil
	}
	var out []Subscriber
	for imsi := range obs {
		s := Subscriber{IMSI: imsi, AuthMethod: "5G_AKA", Registered: reg[imsi], Source: "observed"}
		if ss, ok := sess[imsi]; ok {
			s.DNN, s.Slice = ss.DNN, ss.Slice
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].IMSI < out[j].IMSI })
	return out, nil
}

func r4(v float64) float64 { return math.Round(v*10000) / 10000 }
