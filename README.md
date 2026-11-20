# 5G Network Dashboard

A monitoring and orchestration dashboard for an OpenAirInterface (OAI) 5G deployment: a 5G core (CN5G) plus a gNB, with an optional FlexRIC near-RT RIC.

- **Backend:** Go (standard library, plus a MySQL driver and a pure-Go ZeroMQ client)
- **Frontend:** React, Vite and Recharts

Full documentation: [docs/](docs/README.md).

The UI follows `design.png` and has six views: Overview, Subscribers & UEs, Traffic Analytics, 5G Core, RAN & gNB, and Settings.

## What it shows and where the data comes from

| View / metric | Live source |
|---|---|
| NF health, uptime, CPU and memory | Docker Engine API (container state and healthcheck) |
| Registered UEs, 5GMM state, cell, NGAP IDs, connected gNBs | The UE and gNB tables that the AMF prints periodically in its logs |
| PDU sessions, UE IP, DNN | SMF logs (`Create SM Context` → `PAA, Ipv4 Address`) |
| DL/UL throughput, packet rate, packet loss | `/proc/net/dev` of the UPF tunnel interface (`tun0`: RX = uplink, TX = downlink), plus drops on N3 |
| MCS, BLER and PRBs per UE | FlexRIC monitor xApp, through its ZeroMQ PUB feed (`ue_stats`) |
| Cell configuration (PCI, TAC, band, ARFCN, bandwidth, TDD pattern, and more) | The gNB's `gnb.conf`, read from the container |
| Subscriber database (list / add / remove) | UDR MySQL `oai_db` (optional) |
| Events | Detected changes: UE registered or deregistered, PDU session, gNB connected, NF status changes |

Notes:
- OAI UPF has no per-UE counters. When several UEs are active, each UE's throughput is estimated by splitting the UPF total in proportion to the PRBs scheduled for that UE. The UI marks these values as estimated.
- Throughput and volume totals start counting when the backend starts.
- If `MYSQL_DSN` is not set, the subscriber list shows only the IMSIs seen in the AMF and SMF logs.

## Run (next to the OAI docker-compose stack)

```bash
# optional: subscriber DB access and management actions
export MYSQL_DSN='user:password@tcp(oai-mysql:3306)/oai_db'
export ENABLE_CONTROL=true        # allow NF restarts and subscriber provisioning

docker compose up -d --build
# open http://<host>:8088
```

The container joins the existing `oai-public-net` and `oai-e2-net` networks. It mounts `/var/run/docker.sock`, which gives it full control of Docker on the host, so only expose port 8088 on trusted networks.

### Demo mode (no OAI stack)

```bash
docker run --rm -p 8088:8080 -e MODE=mock 5g-dashboard:latest
```

## Configuration (environment variables)

| Variable | Default | Purpose |
|---|---|---|
| `MODE` | `auto` | `live`, `mock`, or `auto` (live if the Docker socket exists) |
| `AMF_CONTAINER` / `SMF_CONTAINER` / `UPF_CONTAINER` / `GNB_CONTAINER` | `oai-amf` / `oai-smf` / `oai-upf` / `oai-gnb` | Container names |
| `GNB_CONF_PATH` | `/opt/oai-gnb/etc/gnb.conf` | gNB config path inside its container |
| `UPF_TUN_IFACE` / `UPF_N3_IFACE` | `tun0` / `eth0` | UPF interfaces |
| `XAPP_ZMQ` | *(empty)* | e.g. `tcp://monitor_xapp:5555` |
| `MYSQL_DSN` | *(empty)* | Go MySQL DSN for `oai_db` |
| `ENABLE_CONTROL` | `false` | Enables `POST /api/nf/{container}/restart` and `POST`/`DELETE /api/subscribers` |
| `LISTEN` / `STATIC_DIR` | `:8080` / `./web` | HTTP server |

## API

- `GET /api/snapshot`: full state (KPIs, UEs, NFs, gNBs, traffic, slices, events, source status)
- `GET /api/stream`: Server-Sent Events stream that sends a snapshot every second
- `GET /api/traffic/history?seconds=N`: per-second traffic history (up to 1 h)
- `GET /api/subscribers`, `POST /api/subscribers`, `DELETE /api/subscribers/{imsi}`
- `POST /api/nf/{container}/restart`

## Development

```bash
cd backend && MODE=mock go run .            # :8080
cd frontend && npm install && npm run dev   # :5173, proxies /api to :8080
```

## Layout

```
backend/   main.go (HTTP/SSE/API) · collector.go (live sources) · parsers.go (AMF/SMF/UPF/gnb.conf)
           docker.go (Docker API client) · xapp.go (ZMQ) · subscribers.go (MySQL) · mock.go
frontend/  src/App.jsx · src/pages/* · src/components/common.jsx · src/styles.css
```
