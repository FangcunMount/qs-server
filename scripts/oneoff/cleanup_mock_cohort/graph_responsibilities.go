package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"

	standardoutbox "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	eventevidence "github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/reliable-messaging/message"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	"go.mongodb.org/mongo-driver/bson"
)

func graphSQLBytes(r record, name string) []byte {
	for _, c := range r.Columns {
		if c.Name == name && !c.Null {
			return c.Raw
		}
	}
	return nil
}
func (p *cohortPlanner) standardSQL(r record, ref eventevidence.StandardReference) error {
	in := message.Input{ID: string(graphSQLBytes(r, "message_id")), Producer: string(graphSQLBytes(r, "producer")), Destination: string(graphSQLBytes(r, "destination")), EventType: string(graphSQLBytes(r, "event_type")), SchemaVersion: string(graphSQLBytes(r, "schema_version")), Scope: string(graphSQLBytes(r, "scope")), ContentType: string(graphSQLBytes(r, "content_type")), OccurredAt: string(graphSQLBytes(r, "occurred_at")), Payload: graphSQLBytes(r, "payload")}
	if _, e := standardoutbox.VerifyReference(in, hex.EncodeToString(graphSQLBytes(r, "fingerprint")), ref); e != nil {
		return errors.New("original standard message does not match its frozen business reference")
	}
	o, e := p.sqlObject(r)
	if e != nil {
		return e
	}
	o.RootIDs = graphRoots(p.referenceOwners[referenceKey(ref.Producer, ref.EventID, ref.Destination)])
	if o.Links["state"] != "published" || o.Links["claim_token"] != "" || o.Links["lease_until"] != "" {
		p.action("settle_mysql_outbox:" + o.Links["state"])
		p.holdObject(o, "mysql/rm_outbox:unsettled_publish_responsibility")
	}
	p.writer.index.Retained["mysql/rm_outbox"] = "original full wire/state retained; separate frozen consumer/manual replay settlement is still required"
	return p.writer.add(o, 0)
}
func (p *cohortPlanner) standardMongo(r record, ref eventevidence.StandardReference) error {
	raw := bson.Raw(r.BSON)
	in := message.Input{ID: graphMongoString(raw, "message_id"), Producer: graphMongoString(raw, "producer"), Destination: graphMongoString(raw, "destination"), EventType: graphMongoString(raw, "event_type"), SchemaVersion: graphMongoString(raw, "schema_version"), Scope: graphMongoString(raw, "scope"), ContentType: graphMongoString(raw, "content_type"), OccurredAt: graphMongoString(raw, "occurred_at")}
	v := raw.Lookup("payload")
	if v.Type == bson.TypeBinary {
		_, in.Payload = v.Binary()
	} else {
		return errors.New("original Mongo outbox payload type changed")
	}
	fp := raw.Lookup("fingerprint")
	if fp.Type != bson.TypeBinary {
		return errors.New("original Mongo outbox fingerprint type changed")
	}
	_, fingerprint := fp.Binary()
	if _, e := standardoutbox.VerifyReference(in, hex.EncodeToString(fingerprint), ref); e != nil {
		return errors.New("original Mongo standard message conflicts with frozen reference")
	}
	o, e := p.mongoObject(r)
	if e != nil {
		return e
	}
	o.RootIDs = graphRoots(p.referenceOwners[referenceKey(ref.Producer, ref.EventID, ref.Destination)])
	if o.Links["state"] != "published" || graphMongoString(raw, "claim_token") != "" || raw.Lookup("lease_until").Type != 0 && raw.Lookup("lease_until").Type != bson.TypeNull {
		p.action("settle_mongo_outbox:" + o.Links["state"])
		p.holdObject(o, "mongo/rm_outbox:unsettled_publish_responsibility")
	}
	p.writer.index.Retained["mongo/rm_outbox"] = "original full wire/state retained; separate frozen consumer/manual replay settlement is still required"
	return p.writer.add(o, 0)
}
func referenceKey(producer, id, destination string) string {
	return digest(encode([]string{producer, id, destination}))
}

// Historical delivery rows are selected by their concrete payload's typed
// Testee/Assessment ownership, never by archived_mock, a numeric event ID, topic
// name or date alone. Unrecognized payloads remain retained and explicitly held.
type deliveryCandidate struct {
	links                         map[string]string
	r                             record
	assessmentID, testeeID, orgID string
	recognized                    bool
}

func payloadID(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var value string
	if raw[0] == '"' {
		if json.Unmarshal(raw, &value) != nil {
			return "", false
		}
	} else {
		value = string(raw)
	}
	return value, graphDecimal(value)
}
func deliveryPayload(r record) (deliveryCandidate, error) {
	c := deliveryCandidate{r: r, links: map[string]string{}}
	raw := graphSQLBytes(r, "payload_json")
	var original map[string]json.RawMessage
	if e := strictDecode(raw, &original); e != nil {
		return c, nil
	}
	envelope, e := domainwire.DecodeEnvelope(raw)
	if e != nil || envelope.ID == "" {
		return c, nil
	}
	eventID, _ := graphSQLValue(r, "event_id")
	if eventID != "" && eventID != envelope.ID {
		return c, errors.New("delivery payload event identity conflicts with physical row")
	}
	switch envelope.EventType {
	case "answersheet.submitted", "evaluation.requested", "evaluation.retry.requested", "evaluation.outcome.committed", "evaluation.failed", "interpretation.report.generated", "interpretation.report.failed", "interpretation.retry.requested", "task.opened", "task.opened.reminder.requested", "task.completed", "task.expired", "task.canceled":
	default:
		return c, nil
	}
	var data map[string]json.RawMessage
	if json.Unmarshal(envelope.Data, &data) != nil {
		return c, nil
	}
	var ok bool
	c.orgID, ok = payloadID(data["org_id"])
	if !ok {
		return c, nil
	}
	c.testeeID, _ = payloadID(data["testee_id"])
	c.assessmentID, _ = payloadID(data["assessment_id"])
	for _, key := range []string{"testee_id", "assessment_id"} {
		if value, exists := data[key]; exists {
			if _, ok := payloadID(value); !ok {
				return c, nil
			}
		}
	}
	for _, key := range []string{"outcome_id", "generation_id", "report_id", "run_id", "task_id", "enrollment_id", "plan_id", "answersheet_id"} {
		if raw, exists := data[key]; exists {
			v, ok := payloadID(raw)
			if !ok {
				return c, nil
			}
			c.links[key] = v
		}
	}
	c.recognized = c.testeeID != "" || c.assessmentID != ""
	return c, nil
}
func (p *cohortPlanner) classifyDeliveries(ctx context.Context, records []record, roots map[string]bool) error {
	candidates := make([]deliveryCandidate, 0, len(records))
	assessments := []string{}
	seen := map[string]bool{}
	for _, r := range records {
		c, e := deliveryPayload(r)
		if e != nil {
			return e
		}
		candidates = append(candidates, c)
		if c.assessmentID != "" && !seen[c.assessmentID] {
			seen[c.assessmentID] = true
			assessments = append(assessments, c.assessmentID)
		}
	}
	owners := map[string]string{}
	if len(assessments) > 0 {
		slots, args := placeholders(assessments)
		if e := p.reader.sqlPages(ctx, "assessment", "x.id IN ("+slots+")", args, func(r record) error {
			id, _ := graphSQLValue(r, "id")
			testee, _ := graphSQLValue(r, "testee_id")
			org, _ := graphSQLValue(r, "org_id")
			if org == p.selector.Selection.OrgID {
				owners[id] = testee
			}
			return nil
		}); e != nil {
			return e
		}
	}
	if e := p.deliveryCorrelations(ctx, candidates, roots, owners); e != nil {
		return e
	}
	for _, c := range candidates {
		rowOrg, _ := graphSQLValue(c.r, "org_id")
		if !c.recognized || c.orgID != p.selector.Selection.OrgID || rowOrg != c.orgID {
			p.writer.index.Retained[c.r.Object.Name+"/unassigned"] = "unrecognized/different-domain payload retained, outside proved target ownership"
			p.writer.index.Counts["retained/"+c.r.Object.Name+"/unassigned_payload"]++
			continue
		}
		if c.testeeID != "" && !roots[c.testeeID] {
			if owner, ok := owners[c.assessmentID]; ok && roots[owner] {
				return errors.New("delivery payload crosses retained Testee and selected Assessment")
			}
			p.writer.index.Retained[c.r.Object.Name+"/non_target_or_missing_root"] = "canonical Testee is outside fixed current roots; no expansion from historical wire"
			p.writer.index.Counts["retained/"+c.r.Object.Name+"/non_target_or_missing_root"]++
			continue
		}
		owned := c.testeeID != "" && roots[c.testeeID]
		if c.assessmentID != "" {
			owner, exists := owners[c.assessmentID]
			if exists {
				if c.testeeID != "" && owner != c.testeeID {
					return errors.New("delivery payload testee/assessment ownership conflicts")
				}
				owned = roots[owner]
			} else {
				if owned {
					p.holdRecord(c.r, []string{c.testeeID}, c.r.Object.Name+":missing_assessment")
				} else {
					p.writer.index.Counts["retained/"+c.r.Object.Name+"/missing_assessment_without_current_root"]++
				}
				continue
			}
		}
		if !owned {
			continue
		}
		o, e := p.sqlObject(c.r)
		if e != nil {
			return e
		}
		root := c.testeeID
		if root == "" {
			root = owners[c.assessmentID]
		}
		o.RootIDs = []string{root}
		if e := p.writer.add(o, graphSQLTables[c.r.Object.Name]); e != nil {
			return e
		}
	}
	return nil
}
