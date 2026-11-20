# 4. Data correlation & derived metrics

No single source knows everything about a UE:

| Source | Knows | Key |
|--------|-------|-----|
| AMF | IMSI, 5GMM state, cell, NGAP IDs | IMSI |
| SMF | IP, DNN, slice | IMSI |
| xApp | MCS, BLER, PRBs | **RNTI** |
| UPF | Bytes and packets | **None**: aggregate for all UEs |
| MySQL | Provisioned subscribers | IMSI |

`Live.Snapshot()` in `backend/collector.go` merges them. This chapter explains every merge rule and every derived number.

---

## 4.1 Building the UE list

The **AMF UE table is the master list**. A UE exists in the dashboard if and only if the AMF has a context for it. For each AMF row, in table order:

1. **Identity.** `id = UE-001, UE-002, …` (row position), plus IMSI, 5GMM state, cell, RAN/AMF NGAP IDs. The gNB name comes from the first AMF gNB row.
2. **Session.** Look up `smf.sess[IMSI]` to get the IP, DNN and slice (default `eMBB`).
3. **Radio.** If the UE is registered, take the **next unused xApp record** (`radio[ri]`, `ri++`) to get the RNTI, MCS, BLER and PRBs, and set `radioStale = !fresh`.

### Why order-based radio matching
The xApp reports UEs by **RNTI**. The AMF knows them by **IMSI / NGAP ID**. Linking the two would need either:
- the gNB's internal RNTI ↔ RAN-UE-NGAP-ID mapping (in gNB logs or telnet, not exported over E2 MAC SM), or
- the xApp's `rnti_imsi_map.json`, which is disabled in this deployment (`CHECK_RNTI = False`).

So the dashboard pairs registered UEs with radio records **in order**. That's **exact with one UE** (the testbed case). With several UEs the order may not match, so the IMSI shown next to a given set of radio metrics may be swapped. The RNTI shown is always correct for its radio metrics. A robust mapping is on the roadmap ([10 § Correlation](10-limitations.md)).

---

## 4.2 UE status

The `status` column is derived. It's not a 3GPP state.

```
if 5GMM state is not REGISTERED (or is DEREGISTERED)           → Deregistered
else if (UE has an RNTI from a FRESH xApp report)
     or (UE has an IP and the UPF moved packets in the last second) → Active
else                                                             → Idle
```

What this means:

- **Active**: the UE is registered and visibly using the radio or the user plane right now.
- **Idle**: registered in the core but with no fresh radio activity and no traffic. Typical causes: the UE is in CM-IDLE (radio asleep), the xApp feed is stale, or the UE has lost radio coverage but its AMF context hasn't timed out yet.
- **Deregistered**: the AMF still lists the IMSI, but in a deregistered state.

> Caveat: the traffic test uses the **total** UPF packet count. With several UEs, one UE with traffic can make every UE that holds an IP look *Active* when radio data is stale. When the xApp feed is fresh, the RNTI test is per-UE and accurate.

**Registered UEs** KPI = number of AMF rows in a REGISTERED state.
**Active UEs** KPI = number of UEs with status *Active*.

---

## 4.3 Per-UE throughput (apportioning)

OAI UPF counts traffic per interface, not per UE (see [03 § 4](03-data-collection.md#4-upf-tunnel-interface-throughput-packet-rate-loss)). Per-UE values are therefore **apportioned** from the measured total:

```
for each registered UE i:
    if the xApp report is fresh and has a record for UE i:
        wDL_i = delta_prbs_down_i / Σ delta_prbs_down      (fallback to 1/N if Σ = 0)
        wUL_i = delta_prbs_up_i   / Σ delta_prbs_up
    else:
        wDL_i = wUL_i = 1 / N                               (N = registered UEs)

    UE_i.dlMbps  = total DL Mbps × wDL_i
    UE_i.ulMbps  = total UL Mbps × wUL_i
    UE_i.dlBytes += Δ DL bytes × wDL_i      (accumulated every UPF sample)
    UE_i.ulBytes += Δ UL bytes × wUL_i
```

**Why PRB share?** The PRBs a UE gets are the radio resources the scheduler gave it, which is a good proxy for its share of the cell's throughput. It's an approximation: UEs with a higher MCS carry more bytes per PRB, so they're under-estimated, and vice versa.

- With **one** registered UE, the per-UE value equals the measured total. It's exact and `estimated = false`.
- With **more than one**, `estimated = true` and the UI shows a note under the UE table.

The byte accumulation runs inside `pollUPF` → `apportion()`. The instantaneous rates are computed in `Snapshot()` from the latest UPF point.

---

## 4.4 Traffic by slice

During apportioning, each UE's bytes are also added to `sliceBytes[slice]`, where the slice comes from the SMF session (default eMBB). The donut shows `pct = sliceBytes / Σ sliceBytes` for eMBB, URLLC and mMTC. With no traffic yet, all slices show 0% and the donut is grey.

---

## 4.5 KPIs

| KPI | Formula |
|-----|---------|
| `subscribers` | MySQL row count if `MYSQL_DSN` is set, otherwise the number of distinct IMSIs seen in the AMF and SMF |
| `registeredUEs` | AMF rows in a REGISTERED state |
| `activeUEs` | UEs with status Active |
| `pduSessions` | SMF sessions with an IP whose IMSI is currently known to the AMF |
| `dlMbps`, `ulMbps`, `dlPps`, `ulPps`, `lossPct` | Latest UPF sample ([03 § 4](03-data-collection.md#4-upf-tunnel-interface-throughput-packet-rate-loss)) |
| `dlBytes`, `ulBytes`, `totalBytes` | Sums since backend start |
| `avgBlerDl`, `avgBlerUl` | Mean BLER over the UEs that received a radio record (stale ones included) |

KPI card subtitles in the UI: "x% of subs" = registered ÷ subscribers; "y%" = active ÷ registered.

---

## 4.6 NF detail strings

Short live summaries are added to the NF cards:

| NF | Detail |
|----|--------|
| AMF | `<registered UEs> UEs · <gNBs> gNB` |
| SMF | `<pduSessions> sessions` |
| UPF | `<DL+UL> Mbps` |
| gNB | `<registered UEs> UEs` |
| UDR DB | `<n> subscribers` (only with MySQL) |

---

## 4.7 Overall network status

The header pill:

```
any core NF offline   → offline   ("Network Offline", red)
any core NF degraded  → degraded  ("Network Degraded", orange)
no NF discovered      → offline
otherwise             → online    ("Network Online", green)
```

Only `core` kind NFs (AMF, SMF, UPF, NRF, …) count. A stopped xApp doesn't make the network "offline". On the browser side, if the SSE stream breaks, the pill switches to **"Backend Disconnected"** whatever the last snapshot said.

---

## 4.8 Freshness and staleness, summarised

| Data | Max age shown | Staleness handling |
|------|---------------|--------------------|
| Throughput / packets | ~1 s | A failed exec produces a zero point and an error in `sources.upf` |
| Registered UEs | ≤ 20 s (AMF print interval) + 3 s | The last good table is kept if parsing fails |
| PDU sessions / IPs | ~3 s after the SMF log line | Incremental, no loss |
| NF health / CPU | ~5–7 s | Error in `sources.docker` |
| Cell config | 30 s | The last good value is kept |
| Radio (MCS/BLER/PRB) | ~0.5–1 s when fresh | Report timestamp frozen for ≥ 10 s → `stale` badge; no message for > 10 s → radio data hidden |
| Subscriber count | 15 s | Error in `sources.mysql` |

---

## 4.9 Events

Events are kept in an in-memory ring of 300 entries (`eventLog`). The newest 50 are included in every snapshot.

| Event | Level | Trigger |
|-------|-------|---------|
| Collector started | info | Backend start |
| UE registered / state change | info | AMF table diff |
| UE deregistered / removed from AMF | warning | AMF table diff |
| gNB connected | success | New gNB in the AMF table |
| gNB disconnected | error | gNB status changed away from Connected |
| PDU session established | success | SMF `PAA` line |
| PDU session released | info | SMF release line |
| NF status changed | success / warning / error | Container status diff |
| Operator restarted NF | warning | Management action |
| Subscriber provisioned / removed | success / warning | Management action |

Events aren't persisted, so a backend restart clears them. The SMF tracker deliberately doesn't replay historical sessions as events at startup.
