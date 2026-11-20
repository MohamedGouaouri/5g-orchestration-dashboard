# 6. REST / SSE API reference

Base URL: the dashboard origin (e.g. `http://host:8088`). All responses are JSON unless noted. There's no authentication (see [08 § Security](08-management.md#84-security-model)).

| Method | Path | Purpose | Requires control |
|--------|------|---------|------------------|
| GET | `/api/health` | Liveness and mode | – |
| GET | `/api/snapshot` | Full current state | – |
| GET | `/api/stream` | SSE: a snapshot every second | – |
| GET | `/api/traffic/history?seconds=N` | Per-second traffic history | – |
| GET | `/api/subscribers` | Subscriber list | – |
| POST | `/api/subscribers` | Provision a subscriber | ✔ |
| DELETE | `/api/subscribers/{imsi}` | Remove a subscriber | ✔ |
| POST | `/api/nf/{container}/restart` | Restart an NF container | ✔ |
| GET | `/*` | SPA static files (fallback `index.html`) | – |

"Requires control" means the backend runs in live mode with `ENABLE_CONTROL=true`. Otherwise the endpoint returns **403** `{"error":"management actions disabled (set ENABLE_CONTROL=true in live mode)"}`.

---

## GET /api/health

```json
{ "ok": true, "mode": "live" }
```

## GET /api/snapshot

Returns a `Snapshot`. The same object is pushed by `/api/stream`.

```jsonc
{
  "mode": "live",                       // "live" | "mock"
  "timestamp": "2026-11-20T12:07:53Z",
  "network": "online",                  // "online" | "degraded" | "offline"
  "control": false,                     // management actions enabled?
  "kpis": {
    "subscribers": 1, "registeredUEs": 1, "activeUEs": 0, "pduSessions": 1,
    "dlMbps": 0, "ulMbps": 0, "dlPps": 0, "ulPps": 0, "lossPct": 0,
    "totalBytes": 0, "dlBytes": 0, "ulBytes": 0,
    "avgBlerDl": 0.0206, "avgBlerUl": 0.5217        // fractions 0..1
  },
  "ues": [{
    "id": "UE-001", "imsi": "262990100016069", "ip": "12.1.1.233",
    "dnn": "oai", "slice": "eMBB",
    "state": "5GMM-REGISTERED",          // raw AMF state
    "status": "Idle",                    // Active | Idle | Deregistered (derived)
    "cellId": "0000e0000", "gnb": "gNB-oai",
    "ranUeId": "0x01", "amfUeId": "0x34",
    "rnti": 60946, "mcsDl": 0, "mcsUl": 0,
    "blerDl": 0.0206, "blerUl": 0.5217,  // fractions 0..1
    "prbDl": 0, "prbUl": 5,              // peak PRBs in one slot over the last xApp interval
    "dlMbps": 0, "ulMbps": 0, "dlBytes": 0, "ulBytes": 0,
    "estimated": false,                  // per-UE throughput apportioned?
    "radioStale": true                   // radio metrics from a non-advancing xApp report?
  }],
  "core": [{
    "name": "AMF", "kind": "core",       // core | db | ran | ric | other
    "container": "oai-amf", "image": "oaisoftwarealliance/oai-amf:v2.1.10",
    "status": "online", "health": "healthy",
    "startedAt": "2026-11-19T15:42:40.1Z", "uptimeSec": 73500,
    "cpuPct": 0.2, "memMB": 9.3,
    "ips": "oai-public-net=192.168.71.132",
    "detail": "1 UEs · 1 gNB"
  }],
  "gnbs": [{
    "id": "0x0E00", "name": "gNB-oai", "status": "online", "plmn": "262,99", "connectedUEs": 1,
    "config": {
      "cellId": "12345678", "pci": 0, "tac": 1, "mcc": "262", "mnc": "99", "sst": 1, "sd": "0xffffff",
      "band": 78, "arfcnSsb": 640320, "arfcnPointA": 640032, "freqMHz": 3604.8,
      "scsKHz": 30, "prbs": 24, "bandwidthMHz": 10,
      "duplex": "TDD", "tddPeriodMs": 5, "dlSlots": 7, "ulSlots": 2,
      "nbTx": 1, "nbRx": 1, "ssbPower": -25, "prachIndex": 98,
      "amfAddr": "192.168.71.132", "ngAddr": "192.168.71.140", "maxRxGain": 114,
      "rawAvailable": true
    }
  }],
  "traffic": [                           // last 180 s, oldest first
    { "t": 1795175631726, "dl": 0, "ul": 0, "dlPps": 0, "ulPps": 0, "loss": 0, "drops": 0 }
  ],
  "slices": [ { "name": "eMBB", "pct": 0, "bytes": 0 }, { "name": "URLLC", … }, { "name": "mMTC", … } ],
  "events": [ { "t": 1795175624049, "level": "info", "msg": "Dashboard collector started (live mode)" } ],  // newest first, max 50
  "sources": {
    "docker": "ok", "amf": "ok", "smf": "ok", "upf": "ok", "gnb-config": "ok",
    "xapp": "ok",
    "mysql": "disabled (MYSQL_DSN not set) — showing IMSIs observed in AMF/SMF"
  }
}
```

Units: throughput in **Mbps** (10⁶ bit/s), packet rates in **packets/s**, loss in **percent**, BLER as a **fraction**, bytes as **bytes**, times as **Unix milliseconds** (except `timestamp` and `startedAt`, which are RFC 3339).

## GET /api/stream

`Content-Type: text/event-stream`. Every second:

```
data: {"mode":"live","timestamp":"…", …}

```

Browser usage:

```js
const es = new EventSource('/api/stream')
es.onmessage = (e) => render(JSON.parse(e.data))
```

## GET /api/traffic/history?seconds=N

Returns `TrafficPoint[]`, the last N seconds (default 600, max 3600 = buffer size), oldest first.

```bash
curl -s 'localhost:8088/api/traffic/history?seconds=60' | jq '.[-1]'
```

## GET /api/subscribers

```json
[
  { "imsi": "262990100016069", "authMethod": "5G_AKA", "sqn": "000000000020",
    "dnn": "oai", "slice": "eMBB", "registered": true, "source": "db" }
]
```

`source` is `db` (from MySQL), `observed` (seen in the AMF/SMF, no DB configured) or `mock`. Secret keys are never returned.

## POST /api/subscribers

Request:

```json
{ "imsi": "262990100016070", "key": "0C0A34601D4F07677303652C0462535B",
  "opc": "63bfa50ee6523365ff14c1f45f88737d", "dnn": "oai", "sst": 1, "sd": "FFFFFF" }
```

| Field | Rules | Default |
|-------|-------|---------|
| `imsi` | exactly 15 digits | required |
| `key`, `opc` | exactly 32 hex characters (lower-cased before storage) | required |
| `dnn` | string | `oai` |
| `sst` | 1, 2 or 3 | 1 |
| `sd` | hex string | `FFFFFF` |

Responses: `201 {"ok":true}`, `400 {"error":"…"}` (validation, duplicate IMSI, missing DSN), `403` (control disabled).

## DELETE /api/subscribers/{imsi}

`200 {"ok":true}` or `400 {"error":"invalid IMSI"}`.

## POST /api/nf/{container}/restart

`{container}` must be one of the containers currently listed in `snapshot.core`; anything else returns **404**. The call blocks until Docker has restarted the container (10 s stop grace, 60 s overall timeout).

```bash
curl -X POST localhost:8088/api/nf/oai-smf/restart
```
