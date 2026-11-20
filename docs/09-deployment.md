# 9. Deployment & operations

## 9.1 Quick start

Prerequisites: Docker with the Compose plugin on the same host as the OAI containers, and the OAI networks `oai-public-net` and `oai-e2-net` existing.

```bash
cd 5gdashboard-demo

# optional
export MYSQL_DSN='user:password@tcp(oai-mysql:3306)/oai_db'
export ENABLE_CONTROL=false

docker compose up -d --build
docker logs -f 5g-dashboard        # "mode=live listen=:8080 control=false", "xapp: subscribed to …"
```

Open **http://\<host\>:8088**. Check *Settings → Data Sources*: every source should be `ok`, apart from `mysql` if you didn't configure it.

Demo without OAI:

```bash
docker run --rm -p 8088:8080 -e MODE=mock 5g-dashboard:latest
```

---

## 9.2 The container image

`Dockerfile` is a three-stage build:

| Stage | Base | Does |
|-------|------|------|
| `web` | `node:20-alpine` | `npm install`, `npm run build` → `/web/dist` |
| `api` | `golang:1.23-alpine` | `go mod download`, `CGO_ENABLED=0 go build -trimpath -ldflags="-s -w"` → static binary |
| runtime | `alpine:3.20` + ca-certificates, tzdata | `/app/dashboard` + `/app/web`, `EXPOSE 8080` |

The final image holds only the binary and static files (about 17 MB). `.dockerignore` excludes `node_modules`, `dist`, test binaries and `design.png`.

---

## 9.3 docker-compose and networking

```yaml
services:
  dashboard:
    build: .
    image: 5g-dashboard:latest
    container_name: 5g-dashboard
    restart: unless-stopped
    ports: ["8088:8080"]
    environment: { MODE: live, XAPP_ZMQ: tcp://monitor_xapp:5555, … }
    volumes: ["/var/run/docker.sock:/var/run/docker.sock"]
    networks: [oai-public-net, oai-e2-net]
networks:
  oai-public-net: { external: true }
  oai-e2-net:     { external: true }
```

Why each attachment exists:

| Network / mount | Needed for |
|-----------------|-----------|
| `oai-e2-net` | Reaching `monitor_xapp:5555` (ZeroMQ). The xApp port is also published on the host as `0.0.0.0:5555`, so `tcp://host.docker.internal:5555` or the host IP would work too |
| `oai-public-net` | Reaching `oai-mysql:3306`, which isn't published on the host |
| `/var/run/docker.sock` | Container discovery, health, stats, logs, exec, restart. AMF, SMF, UPF and gNB data all go through it, so no extra network access to those NFs is needed |

The dashboard takes a dynamic IP on each OAI network. The OAI subnets are `/26` (62 hosts), which leaves plenty of room.

**Deploying on another host:** the Docker socket must be the one on the host running OAI. You could expose the Docker API over TLS (`DOCKER_HOST`), but the client only supports a unix socket today. Run the dashboard on the OAI host.

---

## 9.4 Configuration reference

| Variable | Default | Description |
|----------|---------|-------------|
| `MODE` | `auto` | `live`, `mock`, or `auto` (live if `DOCKER_SOCK` exists) |
| `LISTEN` | `:8080` | HTTP listen address inside the container |
| `STATIC_DIR` | `./web` (image: `/app/web`) | Directory with the built SPA |
| `DOCKER_SOCK` | `/var/run/docker.sock` | Docker Engine unix socket |
| `AMF_CONTAINER` | `oai-amf` | AMF container name |
| `SMF_CONTAINER` | `oai-smf` | SMF container name |
| `UPF_CONTAINER` | `oai-upf` | UPF container name |
| `UPF_TUN_IFACE` | `tun0` | UE-side tunnel interface inside the UPF |
| `UPF_N3_IFACE` | `eth0` | N3 interface inside the UPF (drops counted) |
| `GNB_CONTAINER` | `oai-gnb` | gNB container name |
| `GNB_CONF_PATH` | `/opt/oai-gnb/etc/gnb.conf` | Config path inside the gNB container |
| `XAPP_ZMQ` | *(empty = disabled)* | xApp publisher endpoint, e.g. `tcp://monitor_xapp:5555` |
| `MYSQL_DSN` | *(empty = disabled)* | Go MySQL DSN, e.g. `user:pass@tcp(oai-mysql:3306)/oai_db` |
| `ENABLE_CONTROL` | `false` | `true` enables NF restart and subscriber add/delete |

Fixed in code (change in `main.go` / `collector.go` if needed): history size 3600 points, snapshot traffic window 180 points, poll periods (1/3/3/5/15/30 s), AMF log tail of 400 lines, event ring of 300.

---

## 9.5 Development workflow

Without Go or Node on the host (as on the testbed), use containers:

```bash
# backend: vet + build
docker run --rm -u $(id -u):$(id -g) -e GOCACHE=/tmp/gc -e GOPATH=/tmp/gp \
  -v "$PWD/backend":/src -w /src golang:1.23-alpine sh -c 'go vet ./... && go build -o /src/.bin-test .'

# frontend: install + build
docker run --rm -u $(id -u):$(id -g) -e npm_config_cache=/tmp/npmc \
  -v "$PWD/frontend":/web -w /web node:20-alpine sh -c 'npm install && npm run build'
```

With local toolchains:

```bash
cd backend  && MODE=mock go run .                        # API on :8080
cd frontend && npm install && npm run dev                # UI on :5173, proxies /api
```

To develop against the live network from the host, run the backend with access to the socket. Note that `monitor_xapp` is published on `localhost:5555`, but MySQL is reachable only from inside `oai-public-net`:

```bash
cd backend && MODE=live XAPP_ZMQ=tcp://127.0.0.1:5555 go run .
```

Rebuild and redeploy: `docker compose up -d --build`.

---

## 9.6 Operations

| Task | Command |
|------|---------|
| Logs | `docker logs -f 5g-dashboard` |
| Health | `curl localhost:8088/api/health` |
| Source status | `curl -s localhost:8088/api/snapshot \| jq .sources` |
| Current KPIs | `curl -s localhost:8088/api/snapshot \| jq .kpis` |
| Stop | `docker compose down` (only removes the dashboard; OAI is untouched) |
| Generate traffic to see charts move | from the UE: `iperf3 -c 192.168.72.135 -t 60` (UL) or `-R` (DL); or `ping 192.168.72.135` |

Resource use: the backend is idle most of the time. The 1 Hz `docker exec` into the UPF and the 5 s stats sampling are its main costs, a few percent of one core.

---

## 9.7 Troubleshooting

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| UI says *Demo data* although OAI is running | `MODE=auto` and the socket isn't mounted | Mount `/var/run/docker.sock` or set `MODE=live` |
| `sources.docker: error … permission denied` | Container user can't access the socket | Run as root (the default image does) or add the socket's group |
| `sources.amf: no UE/gNB tables found` | AMF log level too low, or AMF just started | Wait 20 s; make sure the AMF logs at `info` or above |
| `sources.amf: error … No such container` | Different container name | Set `AMF_CONTAINER` (same for SMF/UPF/GNB) |
| UE shows no IP | SMF log rotated or truncated before the session was created, or a different log format | Re-establish the PDU session; check the SMF logs contain `PAA, Ipv4 Address` |
| Throughput always 0 | No user traffic; wrong `UPF_TUN_IFACE` | Generate traffic; `docker exec oai-upf cat /proc/net/dev` to check the interface name |
| `sources.upf: interface tun0 not found` | UPF uses a different datapath (e.g. eBPF/VPP) | Set `UPF_TUN_IFACE` to the right interface; with VPP, counters aren't in `/proc/net/dev` |
| `sources.xapp: dial …` / `no messages` | xApp down, wrong endpoint, dashboard not on `oai-e2-net` | `docker logs monitor_xapp`; check `XAPP_ZMQ`; check the compose networks |
| `sources.xapp: stale …`, UE shows *stale* badge, UE *Idle* | E2 indications stopped: UE out of coverage, gNB E2 agent or RIC issue | `docker logs nearRT-RIC`, `docker logs oai-gnb`; restart `monitor_xapp` |
| `sources.mysql: error … Access denied` / `unknown database` | Wrong DSN | Check the user, password and database (`oai_db`) |
| Add subscriber → `Duplicate entry` | IMSI already exists | Delete it first or use another IMSI |
| Management buttons missing | `ENABLE_CONTROL` not `true`, or mock mode | Set it and redeploy |
| UI shows *Backend Disconnected* | Backend down or proxy blocking SSE | `docker ps`; behind nginx, set `proxy_buffering off` (the backend already sends `X-Accel-Buffering: no`) |
| gNB card *degraded* | gNB container running but not in the AMF gNB table | Check N2 connectivity (`amf_ip_address` in gnb.conf, SCTP 38412) |
