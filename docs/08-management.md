# 8. Management & orchestration

Besides monitoring, the platform can act on the network. These actions are **disabled by default**. They're enabled only when **both** conditions hold:

1. The backend runs in **live** mode, and
2. `ENABLE_CONTROL=true` is set.

The snapshot field `control` tells the UI whether to show the buttons. The backend enforces the same rule on every management endpoint and returns 403 otherwise, so hiding a button is never the only protection.

> These actions were implemented but **not exercised against the live testbed**: restarting an NF interrupts service, and provisioning needs database credentials. Test them on a non-production setup first.

---

## 8.1 Restarting a network function

**UI:** *5G Core → Network Function Inventory → Restart* (with a confirmation dialog).
**API:** `POST /api/nf/{container}/restart`

How it works:
1. The container name must be one of the NFs currently discovered (`snapshot.core`). This stops the endpoint being used to restart arbitrary containers on the host.
2. The backend calls the Docker API `POST /containers/{name}/restart?t=10`: SIGTERM, up to 10 s grace, then SIGKILL, then start.
3. An event *"Operator restarted oai-smf"* (warning) is logged. The next container polls show the status going offline → online, with matching events.

What to expect in OAI:

| Restarted NF | Impact |
|--------------|--------|
| AMF | All UE contexts are lost. The gNB's NG association drops (OAI gNB usually needs a restart to reconnect). UEs must re-register |
| SMF | Existing PDU sessions are lost on the control side. The UPF may keep forwarding stale rules until it's re-associated. UEs usually need to re-establish sessions |
| UPF | User traffic stops. PFCP association with the SMF must be re-established. `tun0` counters reset (the dashboard handles counter resets) |
| MySQL | Authentication fails during the restart |
| gNB | All UEs drop. The radio restarts |
| RIC / xApp | Monitoring only. The dashboard reconnects to the xApp automatically |

**Order matters** after a full restart in OAI: MySQL → AMF → SMF → UPF → gNB → RIC → xApp. The dashboard restarts only the container you ask for. It doesn't orchestrate dependencies.

---

## 8.2 Provisioning a subscriber

**UI:** *Subscribers & UEs → Subscriber Database → Add subscriber* (needs `MYSQL_DSN`).
**API:** `POST /api/subscribers`

### What a subscriber needs in OAI
For a UE to register and get a session, three things must exist for its IMSI:

1. **Authentication data**: the long-term key **K** and the operator key **OPc**, used by the Milenage algorithm in **5G-AKA**. They must match what's on the SIM. A sequence number (**SQN**) is also stored for replay protection.
2. **Access & mobility subscription**: which slices (S-NSSAIs) the UE may use.
3. **Session management subscription**: which DNNs it may use and their QoS profile (5QI, ARP, session AMBR).

### What the backend inserts
`SubscriberDB.Add` validates the input (IMSI = 15 digits; K and OPc = 32 hex) and runs one **transaction**:

```sql
INSERT INTO AuthenticationSubscription
  (ueid, authenticationMethod, encPermanentKey, protectionParameterId, sequenceNumber,
   authenticationManagementField, algorithmId, encOpcKey, encTopcKey,
   vectorGenerationInHss, n5gcAuthMethod, rgAuthenticationInd, supi)
VALUES (:imsi, '5G_AKA', :k, :k,
   '{"sqn": "000000000020", "sqnScheme": "NON_TIME_BASED", "lastIndexes": {"ausf": 0}}',
   '8000', 'milenage', :opc, NULL, NULL, NULL, NULL, :imsi);

INSERT INTO AccessAndMobilitySubscriptionData (ueid, servingPlmnid, nssai)
VALUES (:imsi, :first5digits, '{"defaultSingleNssais": [{"sst": :sst, "sd": ":sd"}]}');

INSERT INTO SessionManagementSubscriptionData (ueid, servingPlmnid, singleNssai, dnnConfigurations)
VALUES (:imsi, :first5digits, '{"sst": :sst, "sd": ":sd"}',
  '{":dnn": {"pduSessionTypes": {"defaultSessionType": "IPV4"},
             "sscModes": {"defaultSscMode": "SSC_MODE_1"},
             "5gQosProfile": {"5qi": 9, "arp": {"priorityLevel": 15, "preemptCap": "NOT_PREEMPT",
                              "preemptVuln": "PREEMPTABLE"}, "priorityLevel": 1},
             "sessionAmbr": {"uplink": "1000Mbps", "downlink": "1000Mbps"}}}');
```

This mirrors the sample rows in OAI CN5G's `oai_db.sql`. `servingPlmnid` uses the first 5 digits of the IMSI (MCC + 2-digit MNC). Adjust the code if your MNC has 3 digits. If any insert fails, for example because the IMSI already exists (primary key), the whole transaction is rolled back and the error is returned.

After provisioning, the UE can register. The SMF must also be configured for the requested DNN and slice, and the gNB must advertise the slice, otherwise the session or registration is rejected.

---

## 8.3 Removing a subscriber

**UI:** the trash button on a subscriber row (with a confirmation dialog).
**API:** `DELETE /api/subscribers/{imsi}`

One transaction deletes the IMSI from `SessionManagementSubscriptionData`, `AccessAndMobilitySubscriptionData` and `AuthenticationSubscription`. A UE that's **already registered stays registered** until it deregisters or its context expires. Deleting the subscription only prevents future registrations and authentications.

---

## 8.4 Security model

| Aspect | Current state | Recommendation |
|--------|---------------|----------------|
| Docker socket | Mounted read-write. Required for exec, logs and restart. Access to the socket is equivalent to **root on the host** | Run on a trusted management network. For monitor-only deployments, put a Docker socket proxy in front (e.g. one allowing only `GET /containers`, `logs`, `exec`) |
| HTTP authentication | None | Put the dashboard behind a reverse proxy with authentication (basic auth / OIDC) before exposing port 8088, especially with `ENABLE_CONTROL=true` |
| Management actions | Off by default, enforced server-side, limited to discovered NFs, input validated, SQL parameterised | Keep off on wall displays and read-only screens |
| Secrets | K and OPc are accepted on input but never returned by any GET. `MYSQL_DSN` is read from the environment and never exposed | Pass the DSN via an `.env` file excluded from version control (`.gitignore` already lists `.env`) |
| Container exec | Only fixed commands: `cat /proc/net/dev` and `cat <GNB_CONF_PATH>`. No user input reaches exec | – |
| Logs | The backend reads NF logs, which contain IMSIs. IMSIs are shown in the UI | Treat the dashboard as holding subscriber-identifying data |
