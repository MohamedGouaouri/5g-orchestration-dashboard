package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// SubscriberDB reads/writes the OAI UDR MySQL database (oai_db).
type SubscriberDB struct {
	db *sql.DB
}

func OpenSubscriberDB(dsn string) (*SubscriberDB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetConnMaxLifetime(5 * time.Minute)
	return &SubscriberDB{db: db}, nil
}

func (s *SubscriberDB) List(ctx context.Context) ([]Subscriber, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT a.ueid, a.authenticationMethod, COALESCE(JSON_UNQUOTE(JSON_EXTRACT(a.sequenceNumber,'$.sqn')),''),
		COALESCE(CAST(sm.dnnConfigurations AS CHAR),''), COALESCE(CAST(sm.singleNssai AS CHAR),'')
		FROM AuthenticationSubscription a LEFT JOIN SessionManagementSubscriptionData sm ON sm.ueid = a.ueid
		ORDER BY a.ueid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]int{}
	var out []Subscriber
	for rows.Next() {
		var sub Subscriber
		var dnnCfg, nssai string
		if err := rows.Scan(&sub.IMSI, &sub.AuthMethod, &sub.SQN, &dnnCfg, &nssai); err != nil {
			return nil, err
		}
		sub.Source = "db"
		sub.DNN = firstJSONKey(dnnCfg)
		var sn struct {
			SST int `json:"sst"`
		}
		if json.Unmarshal([]byte(nssai), &sn) == nil && sn.SST > 0 {
			sub.Slice = sliceName(sn.SST)
		}
		if i, ok := seen[sub.IMSI]; ok { // multiple SM rows: keep first, append DNN
			if sub.DNN != "" && out[i].DNN != sub.DNN {
				out[i].DNN += ", " + sub.DNN
			}
			continue
		}
		seen[sub.IMSI] = len(out)
		out = append(out, sub)
	}
	return out, rows.Err()
}

func firstJSONKey(s string) string {
	var m map[string]any
	if json.Unmarshal([]byte(s), &m) != nil {
		return ""
	}
	for k := range m {
		return k
	}
	return ""
}

var (
	reIMSI = regexp.MustCompile(`^\d{15}$`)
	reHex  = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)
)

type NewSubscriber struct {
	IMSI string `json:"imsi"`
	Key  string `json:"key"`
	OPc  string `json:"opc"`
	DNN  string `json:"dnn"`
	SST  int    `json:"sst"`
	SD   string `json:"sd"`
}

// Add provisions a subscriber in the same shape as the OAI CN5G sample database.
func (s *SubscriberDB) Add(ctx context.Context, n NewSubscriber) error {
	if !reIMSI.MatchString(n.IMSI) {
		return fmt.Errorf("IMSI must be 15 digits")
	}
	if !reHex.MatchString(n.Key) || !reHex.MatchString(n.OPc) {
		return fmt.Errorf("key and OPc must be 32 hex characters")
	}
	if n.DNN == "" {
		n.DNN = "oai"
	}
	if n.SST == 0 {
		n.SST = 1
	}
	if n.SD == "" {
		n.SD = "FFFFFF"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO AuthenticationSubscription
		(ueid, authenticationMethod, encPermanentKey, protectionParameterId, sequenceNumber, authenticationManagementField, algorithmId, encOpcKey, encTopcKey, vectorGenerationInHss, n5gcAuthMethod, rgAuthenticationInd, supi)
		VALUES (?, '5G_AKA', ?, ?, '{"sqn": "000000000020", "sqnScheme": "NON_TIME_BASED", "lastIndexes": {"ausf": 0}}', '8000', 'milenage', ?, NULL, NULL, NULL, NULL, ?)`,
		n.IMSI, n.Key, n.Key, n.OPc, n.IMSI)
	if err != nil {
		return err
	}
	nssai := fmt.Sprintf(`{"sst": %d, "sd": "%s"}`, n.SST, n.SD)
	dnn := fmt.Sprintf(`{"%s":{"pduSessionTypes":{ "defaultSessionType": "IPV4"},"sscModes": {"defaultSscMode": "SSC_MODE_1"},"5gQosProfile": {"5qi": 9,"arp":{"priorityLevel": 15,"preemptCap": "NOT_PREEMPT","preemptVuln":"PREEMPTABLE"},"priorityLevel":1},"sessionAmbr":{"uplink":"1000Mbps", "downlink":"1000Mbps"}}}`, n.DNN)
	if _, err = tx.ExecContext(ctx, `INSERT INTO AccessAndMobilitySubscriptionData (ueid, servingPlmnid, nssai) VALUES (?, ?, ?)`,
		n.IMSI, n.IMSI[:5], fmt.Sprintf(`{"defaultSingleNssais": [%s]}`, nssai)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO SessionManagementSubscriptionData (ueid, servingPlmnid, singleNssai, dnnConfigurations) VALUES (?, ?, ?, ?)`,
		n.IMSI, n.IMSI[:5], nssai, dnn); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SubscriberDB) Delete(ctx context.Context, imsi string) error {
	if !reIMSI.MatchString(imsi) {
		return fmt.Errorf("invalid IMSI")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, t := range []string{"SessionManagementSubscriptionData", "AccessAndMobilitySubscriptionData", "AuthenticationSubscription"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+t+" WHERE ueid = ?", imsi); err != nil {
			return err
		}
	}
	return tx.Commit()
}
