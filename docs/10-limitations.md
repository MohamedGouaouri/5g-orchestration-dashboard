# 10. Limitations & roadmap

The dashboard was built to be non-intrusive: it observes an unmodified OAI deployment. Most limitations follow from that choice. Each one below lists its effect and a concrete way to remove it.

## 10.1 Known limitations

| # | Limitation | Effect | Root cause |
|---|------------|--------|------------|
| 1 | **Radio ↔ core UE matching is order-based** | With several UEs, radio metrics (MCS/BLER/PRB) may sit next to the wrong IMSI | The E2 MAC SM reports RNTI; the AMF reports IMSI/NGAP ID; no exported mapping between them |
| 2 | **Per-UE throughput is estimated** | With several UEs, rates are split by PRB share, not measured | OAI UPF (simple switch) only has per-interface counters |
| 3 | **"Active" can be over-reported** | With stale radio data, any UE with an IP looks Active while *any* traffic flows | The fallback activity test uses total UPF packets |
| 4 | **Log parsing depends on OAI log formats** | A different OAI version may print other strings, and AMF/SMF data stops updating | No management API for UE/session lists in OAI CN5G v2.1 |
| 5 | **AMF state lags up to ~20 s** | Registrations and deregistrations appear with a delay | The AMF prints its table every 20 s |
| 6 | **Totals and events reset on backend restart** | Traffic volume counts from backend start; event history is lost | In-memory state only |
| 7 | **"Packet loss" is UPF kernel drops only** | Doesn't reflect radio, UPF-internal or end-to-end loss | Only interface counters are sampled |
| 8 | **Slice attribution is best-effort** | Slice comes from SMF logs and defaults to eMBB | SST isn't always printed in a parseable form |
| 9 | **Single cell assumed in places** | All UEs get the first gNB's name; RAN view shows one gNB at a time | The testbed has one gNB |
| 10 | **No authentication** | Anyone who reaches port 8088 can view, and act if control is enabled | Scope of the prototype |
| 11 | **NF restart doesn't orchestrate dependencies** | Restarting AMF/SMF may leave the gNB or UPF in a bad state | Restart is a single-container action |
| 12 | **Docker-only** | Doesn't work for Kubernetes or bare-metal deployments | Uses the Docker Engine API |

## 10.2 Roadmap

### Accuracy
- **Exact RNTI ↔ IMSI mapping** (fixes #1). Options:
  - Enable the xApp's `rnti_imsi_map.json` mechanism (`CHECK_RNTI`), fed by parsing gNB logs (the gNB logs RNTI with RAN UE NGAP ID at RRC setup), which in turn maps to IMSI through the AMF table.
  - Or use the gNB telnet server (`--telnetsrv` is already enabled on the testbed) to query UE contexts.
- **True per-UE throughput** (fixes #2): forward `dl_aggr_tbs` / `ul_aggr_tbs` from the MAC SM in the xApp. Their deltas give per-UE MAC-layer bytes, measured at the gNB. Alternatively, subscribe to FlexRIC's **GTP** or **PDCP** service models for per-bearer byte counters.
- **More radio KPIs:** CQI (`wb_cqi`), PUSCH/PUCCH SNR, power headroom, buffer status, HARQ retransmission counts. They're already in the MAC SM record and only need forwarding (see [03 § 6](03-data-collection.md#extending-the-radio-metrics)).
- **Per-UE activity test** (fixes #3): once per-UE bytes exist, base *Active* on the UE's own traffic.
- **UPF-internal statistics:** where the UPF datapath supports them (e.g. eBPF maps or a UPF API), read per-PDR counters and real drop reasons.

### Robustness
- **Unit tests** for `parsers.go`, using captured AMF/SMF logs, `/proc/net/dev` and `gnb.conf` fixtures.
- **Version adapters** for log formats (fixes #4), selected from the NF image tag.
- **Persistence** (fixes #6): store traffic history and events in SQLite or a time-series DB, or expose a `/metrics` endpoint for **Prometheus** and keep long-term history in Grafana.

### Management
- **Authentication and roles** (fixes #10): viewer vs operator, with operator required for control endpoints.
- **Dependency-aware orchestration** (fixes #11): ordered restart of the whole stack, health-gated (wait for `healthy` before the next step).
- **Subscriber editing:** change DNN/slice/AMBR, bulk import from CSV, reset SQN.
- **gNB configuration editing:** edit selected `gnb.conf` parameters (PCI, bandwidth, TDD pattern) with validation, then restart the gNB. Needs care, since a bad config takes the cell down.
- **RIC control:** use FlexRIC's SLICE or RC service models to create and resize RAN slices from the UI. That turns the platform from monitoring into closed-loop RAN orchestration.

### Platform
- **Kubernetes support** (fixes #12): replace the Docker client with the Kubernetes API (pod status, logs, exec), for OAI Helm deployments.
- **Multi-gNB / multi-cell** (fixes #9): per-gNB UE assignment from the AMF table (via RAN UE NGAP IDs and cell IDs), and a cell map.
- **Alerting:** thresholds on BLER, loss and NF health, with notifications.
