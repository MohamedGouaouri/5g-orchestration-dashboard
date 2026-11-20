package main

import "time"

// Snapshot is the full state pushed to the UI every tick.
type Snapshot struct {
	Mode      string            `json:"mode"` // live | mock
	Timestamp time.Time         `json:"timestamp"`
	Network   string            `json:"network"` // online | degraded | offline
	KPIs      KPIs              `json:"kpis"`
	UEs       []UE              `json:"ues"`
	Core      []NF              `json:"core"`
	GNBs      []GNB             `json:"gnbs"`
	Traffic   []TrafficPoint    `json:"traffic"`
	Slices    []SliceShare      `json:"slices"`
	Events    []Event           `json:"events"`
	Sources   map[string]string `json:"sources"`
	Control   bool              `json:"control"` // management actions enabled
}

type KPIs struct {
	Subscribers   int     `json:"subscribers"`
	RegisteredUEs int     `json:"registeredUEs"`
	ActiveUEs     int     `json:"activeUEs"`
	PDUSessions   int     `json:"pduSessions"`
	DLMbps        float64 `json:"dlMbps"`
	ULMbps        float64 `json:"ulMbps"`
	DLPps         float64 `json:"dlPps"`
	ULPps         float64 `json:"ulPps"`
	LossPct       float64 `json:"lossPct"`
	TotalBytes    uint64  `json:"totalBytes"`
	DLBytes       uint64  `json:"dlBytes"`
	ULBytes       uint64  `json:"ulBytes"`
	AvgBLERDL     float64 `json:"avgBlerDl"`
	AvgBLERUL     float64 `json:"avgBlerUl"`
}

type UE struct {
	ID         string  `json:"id"`
	IMSI       string  `json:"imsi"`
	IP         string  `json:"ip"`
	DNN        string  `json:"dnn"`
	Slice      string  `json:"slice"`
	State      string  `json:"state"`  // 5GMM state from AMF
	Status     string  `json:"status"` // Active | Idle | Deregistered
	CellID     string  `json:"cellId"`
	GNB        string  `json:"gnb"`
	RANUEID    string  `json:"ranUeId"`
	AMFUEID    string  `json:"amfUeId"`
	RNTI       int     `json:"rnti"`
	MCSDL      int     `json:"mcsDl"`
	MCSUL      int     `json:"mcsUl"`
	BLERDL     float64 `json:"blerDl"`
	BLERUL     float64 `json:"blerUl"`
	PRBDL      int     `json:"prbDl"`
	PRBUL      int     `json:"prbUl"`
	DLMbps     float64 `json:"dlMbps"`
	ULMbps     float64 `json:"ulMbps"`
	DLBytes    uint64  `json:"dlBytes"`
	ULBytes    uint64  `json:"ulBytes"`
	Estimated  bool    `json:"estimated"` // per-UE throughput is apportioned, not measured
	RadioStale bool    `json:"radioStale"`
}

type NF struct {
	Name      string  `json:"name"`
	Kind      string  `json:"kind"` // core | ran | ric | db | other
	Container string  `json:"container"`
	Image     string  `json:"image"`
	Status    string  `json:"status"` // online | degraded | offline
	Health    string  `json:"health"`
	StartedAt string  `json:"startedAt"`
	UptimeSec int64   `json:"uptimeSec"`
	CPUPct    float64 `json:"cpuPct"`
	MemMB     float64 `json:"memMB"`
	IPs       string  `json:"ips"`
	Detail    string  `json:"detail"`
}

type GNB struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Status       string    `json:"status"`
	PLMN         string    `json:"plmn"`
	ConnectedUEs int       `json:"connectedUEs"`
	Config       GNBConfig `json:"config"`
}

type GNBConfig struct {
	CellID         string  `json:"cellId"`
	PCI            int     `json:"pci"`
	TAC            int     `json:"tac"`
	MCC            string  `json:"mcc"`
	MNC            string  `json:"mnc"`
	SST            int     `json:"sst"`
	SD             string  `json:"sd"`
	Band           int     `json:"band"`
	ARFCNSSB       int     `json:"arfcnSsb"`
	ARFCNPointA    int     `json:"arfcnPointA"`
	FreqMHz        float64 `json:"freqMHz"`
	SCSkHz         int     `json:"scsKHz"`
	PRBs           int     `json:"prbs"`
	BandwidthMHz   int     `json:"bandwidthMHz"`
	Duplex         string  `json:"duplex"`
	TDDPeriodMs    float64 `json:"tddPeriodMs"`
	DLSlots        int     `json:"dlSlots"`
	ULSlots        int     `json:"ulSlots"`
	NbTx           int     `json:"nbTx"`
	NbRx           int     `json:"nbRx"`
	SSBPower       int     `json:"ssbPower"`
	PRACHIndex     int     `json:"prachIndex"`
	AMFAddr        string  `json:"amfAddr"`
	NGAddr         string  `json:"ngAddr"`
	MaxRxGain      int     `json:"maxRxGain"`
	RawAvailable   bool    `json:"rawAvailable"`
}

type TrafficPoint struct {
	T       int64   `json:"t"` // unix ms
	DLMbps  float64 `json:"dl"`
	ULMbps  float64 `json:"ul"`
	DLPps   float64 `json:"dlPps"`
	ULPps   float64 `json:"ulPps"`
	LossPct float64 `json:"loss"`
	Drops   uint64  `json:"drops"`
}

type SliceShare struct {
	Name  string  `json:"name"`
	Pct   float64 `json:"pct"`
	Bytes uint64  `json:"bytes"`
}

type Event struct {
	T     int64  `json:"t"`
	Level string `json:"level"` // info | success | warning | error
	Msg   string `json:"msg"`
}

type Subscriber struct {
	IMSI       string `json:"imsi"`
	AuthMethod string `json:"authMethod"`
	SQN        string `json:"sqn,omitempty"`
	DNN        string `json:"dnn,omitempty"`
	Slice      string `json:"slice,omitempty"`
	Registered bool   `json:"registered"`
	Source     string `json:"source"` // db | observed | mock
}
