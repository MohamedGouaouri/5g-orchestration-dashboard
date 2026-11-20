package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	cfg := Config{
		Mode:          env("MODE", "auto"),
		DockerSock:    env("DOCKER_SOCK", "/var/run/docker.sock"),
		AMFContainer:  env("AMF_CONTAINER", "oai-amf"),
		SMFContainer:  env("SMF_CONTAINER", "oai-smf"),
		UPFContainer:  env("UPF_CONTAINER", "oai-upf"),
		GNBContainer:  env("GNB_CONTAINER", "oai-gnb"),
		GNBConfPath:   env("GNB_CONF_PATH", "/opt/oai-gnb/etc/gnb.conf"),
		UPFTunIface:   env("UPF_TUN_IFACE", "tun0"),
		UPFN3Iface:    env("UPF_N3_IFACE", "eth0"),
		XAppEndpoint:  env("XAPP_ZMQ", ""),
		MySQLDSN:      env("MYSQL_DSN", ""),
		EnableControl: env("ENABLE_CONTROL", "false") == "true",
		HistorySize:   3600,
	}
	addr := env("LISTEN", ":8080")
	static := env("STATIC_DIR", "./web")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mode := cfg.Mode
	if mode == "auto" {
		if _, err := os.Stat(cfg.DockerSock); err == nil {
			mode = "live"
		} else {
			mode = "mock"
		}
	}
	var src Source
	var live *Live
	if mode == "live" {
		live = NewLive(cfg)
		go live.Run(ctx)
		src = live
	} else {
		m := NewMock(cfg.HistorySize)
		go m.Run(ctx)
		src = m
	}
	log.Printf("5G dashboard backend: mode=%s listen=%s control=%v", mode, addr, cfg.EnableControl)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"ok": true, "mode": mode})
	})
	mux.HandleFunc("GET /api/snapshot", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, src.Snapshot())
	})
	mux.HandleFunc("GET /api/traffic/history", func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(r.URL.Query().Get("seconds"))
		if n <= 0 {
			n = 600
		}
		writeJSON(w, 200, src.History(n))
	})
	mux.HandleFunc("GET /api/subscribers", func(w http.ResponseWriter, r *http.Request) {
		subs, err := src.Subscribers(r.Context())
		if err != nil {
			writeErr(w, 502, err)
			return
		}
		if subs == nil {
			subs = []Subscriber{}
		}
		writeJSON(w, 200, subs)
	})
	// Server-Sent Events: push a snapshot every second.
	mux.HandleFunc("GET /api/stream", func(w http.ResponseWriter, r *http.Request) {
		fl, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", 500)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			b, _ := json.Marshal(src.Snapshot())
			if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
				return
			}
			fl.Flush()
			select {
			case <-r.Context().Done():
				return
			case <-t.C:
			}
		}
	})

	// ---- management actions (opt-in) ----
	control := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !cfg.EnableControl || live == nil {
				writeErr(w, 403, fmt.Errorf("management actions disabled (set ENABLE_CONTROL=true in live mode)"))
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("POST /api/nf/{container}/restart", control(func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("container")
		known := false
		for _, nf := range live.Snapshot().Core {
			if nf.Container == name {
				known = true
			}
		}
		if !known {
			writeErr(w, 404, fmt.Errorf("unknown network function container %q", name))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		if err := live.dk.Restart(ctx, name); err != nil {
			writeErr(w, 502, err)
			return
		}
		live.ev.add("warning", "Operator restarted %s", name)
		writeJSON(w, 200, map[string]any{"ok": true})
	}))
	mux.HandleFunc("POST /api/subscribers", control(func(w http.ResponseWriter, r *http.Request) {
		if live.db == nil {
			writeErr(w, 400, fmt.Errorf("MYSQL_DSN not configured"))
			return
		}
		var n NewSubscriber
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&n); err != nil {
			writeErr(w, 400, err)
			return
		}
		n.Key, n.OPc = strings.ToLower(n.Key), strings.ToLower(n.OPc)
		if err := live.db.Add(r.Context(), n); err != nil {
			writeErr(w, 400, err)
			return
		}
		live.ev.add("success", "Subscriber imsi-%s provisioned", n.IMSI)
		live.pollSubscribers(r.Context())
		writeJSON(w, 201, map[string]any{"ok": true})
	}))
	mux.HandleFunc("DELETE /api/subscribers/{imsi}", control(func(w http.ResponseWriter, r *http.Request) {
		if live.db == nil {
			writeErr(w, 400, fmt.Errorf("MYSQL_DSN not configured"))
			return
		}
		imsi := r.PathValue("imsi")
		if err := live.db.Delete(r.Context(), imsi); err != nil {
			writeErr(w, 400, err)
			return
		}
		live.ev.add("warning", "Subscriber imsi-%s removed", imsi)
		live.pollSubscribers(r.Context())
		writeJSON(w, 200, map[string]any{"ok": true})
	}))

	// static SPA
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		p := filepath.Join(static, filepath.Clean("/"+r.URL.Path))
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			http.ServeFile(w, r, p)
			return
		}
		http.ServeFile(w, r, filepath.Join(static, "index.html"))
	})

	srv := &http.Server{Addr: addr, Handler: logReq(mux), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(c)
	}()
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func logReq(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			log.Printf("%s %s", r.Method, r.URL.Path)
		}
		h.ServeHTTP(w, r)
	})
}
