package main

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// splitTS splits a docker log line ("<RFC3339Nano> <msg>") into timestamp and message.
func splitTS(line string) (string, string) {
	if i := strings.IndexByte(line, ' '); i > 20 && i < 40 && line[4] == '-' {
		return line[:i], line[i+1:]
	}
	return "", line
}

// ---------------- AMF ----------------

type amfUE struct {
	State, IMSI, GUTI, RANUEID, AMFUEID, PLMN, CellID string
}
type amfGNB struct {
	Status, GlobalID, Name, PLMN string
}

func tableRows(lines []string, start int) [][]string {
	var rows [][]string
	for i := start + 1; i < len(lines); i++ {
		_, l := splitTS(lines[i])
		l = strings.TrimSpace(l)
		if l == "" {
			if len(rows) > 0 || i > start+3 {
				break
			}
			continue
		}
		if strings.HasPrefix(l, "|---") || strings.HasPrefix(l, "|  Index") {
			if strings.HasPrefix(l, "|---") && len(rows) > 0 {
				break
			}
			continue
		}
		if !strings.HasPrefix(l, "|") {
			break
		}
		parts := strings.Split(strings.Trim(l, "|"), "|")
		for j := range parts {
			parts[j] = strings.TrimSpace(parts[j])
		}
		rows = append(rows, parts)
	}
	return rows
}

// parseAMF extracts the latest gNB and UE tables printed periodically by OAI AMF.
func parseAMF(text string) ([]amfGNB, []amfUE, bool) {
	lines := strings.Split(text, "\n")
	gi, ui := -1, -1
	for i := len(lines) - 1; i >= 0; i-- {
		if ui < 0 && strings.Contains(lines[i], "UEs' Information") {
			ui = i
		}
		if gi < 0 && strings.Contains(lines[i], "gNBs' Information") {
			gi = i
		}
		if gi >= 0 && ui >= 0 {
			break
		}
	}
	if gi < 0 && ui < 0 {
		return nil, nil, false
	}
	var gnbs []amfGNB
	if gi >= 0 {
		for _, r := range tableRows(lines, gi) {
			if len(r) >= 5 {
				gnbs = append(gnbs, amfGNB{Status: r[1], GlobalID: r[2], Name: r[3], PLMN: r[4]})
			}
		}
	}
	var ues []amfUE
	if ui >= 0 {
		for _, r := range tableRows(lines, ui) {
			if len(r) >= 8 {
				ues = append(ues, amfUE{State: r[1], IMSI: r[2], GUTI: r[3], RANUEID: r[4], AMFUEID: r[5], PLMN: r[6], CellID: r[7]})
			}
		}
	}
	return gnbs, ues, true
}

// ---------------- SMF ----------------

var (
	reSMFCreate  = regexp.MustCompile(`PDU Session Create SM Context Request message from AMF, SUPI imsi-(\d+)`)
	reSMFDNN     = regexp.MustCompile(`Requested DNN: (\S+)`)
	reSMFPAA     = regexp.MustCompile(`PAA, Ipv4 Address: ([0-9.]+)`)
	reSMFRelease = regexp.MustCompile(`(?i)release.*imsi-(\d+)`)
	reSMFSST     = regexp.MustCompile(`SST[:=]\s*(\d+)`)
)

type smfSession struct {
	IMSI, IP, DNN, Slice string
	Sessions             int
}

// smfTracker incrementally follows SMF logs to map SUPI -> UE IP / DNN.
type smfTracker struct {
	lastTS  string
	cur     string
	sess    map[string]*smfSession
	pending []smfEvent
}

type smfEvent struct {
	kind, imsi, ip, dnn string
}

func newSMFTracker() *smfTracker { return &smfTracker{sess: map[string]*smfSession{}} }

func (t *smfTracker) feed(text string, emit bool) {
	for _, line := range strings.Split(text, "\n") {
		ts, msg := splitTS(line)
		if ts != "" {
			if ts <= t.lastTS {
				continue
			}
			t.lastTS = ts
		}
		if m := reSMFCreate.FindStringSubmatch(msg); m != nil {
			t.cur = m[1]
			if t.sess[t.cur] == nil {
				t.sess[t.cur] = &smfSession{IMSI: t.cur, Slice: "eMBB"}
			}
			continue
		}
		if t.cur == "" {
			continue
		}
		if m := reSMFDNN.FindStringSubmatch(msg); m != nil {
			t.sess[t.cur].DNN = m[1]
		} else if m := reSMFSST.FindStringSubmatch(msg); m != nil {
			t.sess[t.cur].Slice = sliceName(atoi(m[1]))
		} else if m := reSMFPAA.FindStringSubmatch(msg); m != nil {
			s := t.sess[t.cur]
			s.IP = m[1]
			s.Sessions++
			if emit {
				t.pending = append(t.pending, smfEvent{"pdu", s.IMSI, s.IP, s.DNN})
			}
		} else if m := reSMFRelease.FindStringSubmatch(msg); m != nil && strings.Contains(msg, "PDU") {
			if s := t.sess[m[1]]; s != nil && s.IP != "" {
				if emit {
					t.pending = append(t.pending, smfEvent{"release", s.IMSI, s.IP, s.DNN})
				}
				s.IP = ""
			}
		}
	}
}

func sliceName(sst int) string {
	switch sst {
	case 2:
		return "URLLC"
	case 3:
		return "mMTC"
	default:
		return "eMBB"
	}
}

// ---------------- UPF (/proc/net/dev) ----------------

type ifCounters struct {
	RxBytes, RxPkts, RxErrs, RxDrop uint64
	TxBytes, TxPkts, TxErrs, TxDrop uint64
}

func parseNetDev(text string) map[string]ifCounters {
	out := map[string]ifCounters{}
	for _, line := range strings.Split(text, "\n") {
		i := strings.IndexByte(line, ':')
		if i < 0 {
			continue
		}
		name := strings.TrimSpace(line[:i])
		f := strings.Fields(line[i+1:])
		if len(f) < 12 {
			continue
		}
		u := func(k int) uint64 { v, _ := strconv.ParseUint(f[k], 10, 64); return v }
		out[name] = ifCounters{u(0), u(1), u(2), u(3), u(8), u(9), u(10), u(11)}
	}
	return out
}

// ---------------- gNB config ----------------

func confVal(text, key string) string {
	re := regexp.MustCompile(`(?m)^[^#\n]*\b` + regexp.QuoteMeta(key) + `\s*=\s*([^;,#\n})]+)`)
	m := re.FindStringSubmatch(text)
	if m == nil {
		return ""
	}
	v := strings.TrimSpace(m[1])
	v = strings.Trim(v, `"`)
	return strings.TrimSuffix(v, "L")
}

func atoi(s string) int {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		v, _ := strconv.ParseInt(s[2:], 16, 64)
		return int(v)
	}
	v, _ := strconv.Atoi(s)
	return v
}

// nrarfcnToMHz converts an NR-ARFCN to frequency (TS 38.104 §5.4.2.1).
func nrarfcnToMHz(n int) float64 {
	switch {
	case n < 600000:
		return float64(n) * 0.005
	case n < 2016667:
		return 3000 + float64(n-600000)*0.015
	default:
		return 24250.08 + float64(n-2016667)*0.06
	}
}

var bwTable = map[int]map[int]int{ // scs kHz -> PRBs -> MHz (TS 38.101-1 Table 5.3.2-1)
	15:  {25: 5, 52: 10, 79: 15, 106: 20, 133: 25, 160: 30, 216: 40, 270: 50},
	30:  {11: 5, 24: 10, 38: 15, 51: 20, 65: 25, 78: 30, 106: 40, 133: 50, 162: 60, 189: 70, 217: 80, 245: 90, 273: 100},
	60:  {11: 10, 18: 15, 24: 20, 31: 25, 38: 30, 51: 40, 65: 50, 79: 60, 93: 70, 107: 80, 121: 90, 135: 100},
	120: {66: 50, 132: 100, 264: 200, 528: 400},
}

var tddPeriods = []float64{0.5, 0.625, 1, 1.25, 2, 2.5, 5, 10, 3, 4}

func parseGNBConf(text string) GNBConfig {
	c := GNBConfig{RawAvailable: text != ""}
	if text == "" {
		return c
	}
	c.CellID = confVal(text, "nr_cellid")
	c.PCI = atoi(confVal(text, "physCellId"))
	c.TAC = atoi(confVal(text, "tracking_area_code"))
	c.MCC = confVal(text, "mcc")
	c.MNC = confVal(text, "mnc")
	c.SST = atoi(confVal(text, "sst"))
	c.SD = confVal(text, "sd")
	c.Band = atoi(confVal(text, "dl_frequencyBand"))
	c.ARFCNSSB = atoi(confVal(text, "absoluteFrequencySSB"))
	c.ARFCNPointA = atoi(confVal(text, "dl_absoluteFrequencyPointA"))
	if c.ARFCNSSB > 0 {
		c.FreqMHz = math.Round(nrarfcnToMHz(c.ARFCNSSB)*100) / 100
	}
	scsIdx := atoi(confVal(text, "dl_subcarrierSpacing"))
	c.SCSkHz = 15 << scsIdx
	c.PRBs = atoi(confVal(text, "dl_carrierBandwidth"))
	if m, ok := bwTable[c.SCSkHz]; ok {
		c.BandwidthMHz = m[c.PRBs]
	}
	if c.BandwidthMHz == 0 && c.PRBs > 0 {
		c.BandwidthMHz = int(math.Ceil(float64(c.PRBs*12*c.SCSkHz) / 1000))
	}
	// Bands n77/n78/n79/n38/n40/n41/n48 etc. are TDD in practice; presence of a TDD pattern is the real signal.
	if p := confVal(text, "dl_UL_TransmissionPeriodicity"); p != "" {
		c.Duplex = "TDD"
		if i := atoi(p); i >= 0 && i < len(tddPeriods) {
			c.TDDPeriodMs = tddPeriods[i]
		}
		c.DLSlots = atoi(confVal(text, "nrofDownlinkSlots"))
		c.ULSlots = atoi(confVal(text, "nrofUplinkSlots"))
	} else {
		c.Duplex = "FDD"
	}
	c.NbTx = atoi(confVal(text, "nb_tx"))
	c.NbRx = atoi(confVal(text, "nb_rx"))
	c.SSBPower = atoi(confVal(text, "ssPBCH_BlockPower"))
	c.PRACHIndex = atoi(confVal(text, "prach_ConfigurationIndex"))
	c.MaxRxGain = atoi(confVal(text, "max_rxgain"))
	c.NGAddr = confVal(text, "GNB_IPV4_ADDRESS_FOR_NG_AMF")
	if m := regexp.MustCompile(`amf_ip_address\s*=\s*\(\s*\{\s*ipv4\s*=\s*"([^"]+)"`).FindStringSubmatch(text); m != nil {
		c.AMFAddr = m[1]
	}
	return c
}
