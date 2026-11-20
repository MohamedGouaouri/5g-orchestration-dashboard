# 5G Network Dashboard — Documentation

This folder documents the 5G orchestration and monitoring platform end to end: what it shows, where every number comes from, how the backend and frontend are built, and the 5G / O-RAN concepts you need to read the screens correctly.

## Reading order

| # | Document | What you will learn |
|---|----------|---------------------|
| 1 | [Architecture overview](01-architecture.md) | The big picture: components, the OAI testbed it monitors, how data flows from the network to the browser |
| 2 | [5G & O-RAN concepts](02-concepts.md) | A primer on every term shown in the UI: SUPI/IMSI, 5GMM states, PDU sessions, DNN, slices, NR-ARFCN, numerology, PRB, TDD, MCS, BLER, HARQ, RNTI, near-RT RIC, E2, xApps |
| 3 | [Data collection in depth](03-data-collection.md) | Source by source: Docker API, AMF/SMF log parsing, UPF counters, gnb.conf parsing, the FlexRIC xApp pipeline (how BLER/MCS/PRB reach the dashboard), MySQL |
| 4 | [Data correlation & derived metrics](04-correlation.md) | How the sources are merged into one UE view, how status/throughput/loss/slices are computed, staleness detection, events |
| 5 | [Backend internals](05-backend.md) | Go code walkthrough: files, types, goroutines, locking, the Docker client, SSE streaming, mock mode |
| 6 | [REST / SSE API reference](06-api.md) | Every endpoint, request/response schemas, examples |
| 7 | [Frontend internals](07-frontend.md) | React structure, live-update hook, pages, components, charts, styling, responsive behaviour |
| 8 | [Management & orchestration](08-management.md) | NF restarts, subscriber provisioning (exact SQL), safety model |
| 9 | [Deployment & operations](09-deployment.md) | docker-compose, networks, environment variables, development workflow, troubleshooting |
| 10 | [Limitations & roadmap](10-limitations.md) | What is approximated, known caveats, and concrete next steps |

## One-paragraph summary

The platform is a single Go binary that also serves a React single-page app. In **live mode** it observes an OpenAirInterface (OAI) 5G network running in Docker: it asks the **Docker Engine API** for container health and resource usage, reads the **AMF** and **SMF** logs to learn which UEs are registered and which IP addresses they got, samples the **UPF**'s tunnel interface counters every second to compute throughput, packet rate and loss, reads the **gNB configuration file** for cell parameters, and subscribes to a **FlexRIC xApp** over ZeroMQ to receive per-UE MAC statistics (MCS, BLER, PRBs) that originate in the gNB scheduler and travel over the O-RAN **E2** interface. Optionally it connects to the **UDR MySQL** database to list and provision subscribers. Everything is merged into one JSON *snapshot* that is pushed to the browser once per second with Server-Sent Events. In **mock mode** a synthetic generator produces the same snapshot shape for demos.

## Quick links

- Run it: [09-deployment.md § Quick start](09-deployment.md#91-quick-start)
- "Why is my UE *Idle*?": [04-correlation.md § UE status](04-correlation.md#42-ue-status)
- "Where does BLER come from?": [03-data-collection.md § FlexRIC xApp](03-data-collection.md#6-flexric-monitor-xapp-mcs-bler-prbs)
- "Why is per-UE throughput marked *estimated*?": [04-correlation.md § Per-UE throughput](04-correlation.md#43-per-ue-throughput-apportioning)
