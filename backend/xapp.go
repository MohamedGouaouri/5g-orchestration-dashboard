package main

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/go-zeromq/zmq4"
)

type xappUE struct {
	UEID          string  `json:"ue_id"`
	IMSI          string  `json:"imsi"`
	RNTI          int     `json:"rnti"`
	MCSUp         int     `json:"mcs_up"`
	MCSDown       int     `json:"mcs_down"`
	BLERUp        float64 `json:"bler_up"`
	BLERDown      float64 `json:"bler_down"`
	AggrPRBsDown  uint64  `json:"aggr_prbs_down"`
	AggrPRBsUp    uint64  `json:"aggr_prbs_up"`
	DeltaPRBsDown uint64  `json:"delta_prbs_down"`
	DeltaPRBsUp   uint64  `json:"delta_prbs_up"`
	NumPRBsDown   int     `json:"num_prbs_down"`
	NumPRBsUp     int     `json:"num_prbs_up"`
}

type xappMsg struct {
	Timestamp int64    `json:"timestamp"`
	UEs       []xappUE `json:"UE_stats"`
}

// XApp subscribes to the FlexRIC monitoring xApp ZeroMQ PUB feed.
type XApp struct {
	endpoint string
	mu       sync.RWMutex
	latest   xappMsg
	lastRecv time.Time // last message received at all
	lastNew  time.Time // last time the report timestamp advanced
	err      string
}

func NewXApp(endpoint string) *XApp { return &XApp{endpoint: endpoint, err: "connecting"} }

func (x *XApp) Run(ctx context.Context) {
	for ctx.Err() == nil {
		x.session(ctx)
		select {
		case <-ctx.Done():
		case <-time.After(3 * time.Second):
		}
	}
}

func (x *XApp) session(ctx context.Context) {
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	sub := zmq4.NewSub(sctx, zmq4.WithAutomaticReconnect(true))
	defer sub.Close()
	if err := sub.Dial(x.endpoint); err != nil {
		x.setErr("dial: " + err.Error())
		return
	}
	_ = sub.SetOption(zmq4.OptionSubscribe, "")
	log.Printf("xapp: subscribed to %s", x.endpoint)
	for {
		msg, err := sub.Recv()
		if err != nil {
			x.setErr("recv: " + err.Error())
			return
		}
		if len(msg.Frames) == 0 {
			continue
		}
		var m xappMsg
		if err := json.Unmarshal(msg.Frames[len(msg.Frames)-1], &m); err != nil {
			continue
		}
		x.mu.Lock()
		now := time.Now()
		if m.Timestamp != x.latest.Timestamp {
			x.lastNew = now
		}
		x.latest, x.lastRecv, x.err = m, now, ""
		x.mu.Unlock()
	}
}

func (x *XApp) setErr(e string) {
	x.mu.Lock()
	x.err = e
	x.mu.Unlock()
}

// Get returns the latest report, whether it is fresh, and a status string.
func (x *XApp) Get() ([]xappUE, bool, string) {
	x.mu.RLock()
	defer x.mu.RUnlock()
	if x.lastRecv.IsZero() {
		return nil, false, x.err
	}
	if time.Since(x.lastRecv) > 10*time.Second {
		return nil, false, "no messages for " + time.Since(x.lastRecv).Truncate(time.Second).String()
	}
	fresh := time.Since(x.lastNew) < 10*time.Second
	status := "ok"
	if !fresh {
		status = "stale (report timestamp not advancing — E2/MAC indications stopped?)"
	}
	return x.latest.UEs, fresh, status
}
