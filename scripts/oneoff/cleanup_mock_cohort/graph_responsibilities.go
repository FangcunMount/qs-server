package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

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
	if o.Links["state"] != "published" {
		p.action("settle_mysql_outbox:" + o.Links["state"])
	}
	return p.writer.add(o, graphSQLTables["rm_outbox"])
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
	if o.Links["state"] != "published" {
		p.action("settle_mongo_outbox:" + o.Links["state"])
	}
	return p.writer.add(o, graphMongoTables["rm_outbox"])
}
func referenceKey(producer, id, destination string) string {
	return digest(encode([]string{producer, id, destination}))
}
func (p *cohortPlanner) standardBatch(ctx context.Context, refs []eventevidence.StandardReference) error {
	if len(refs) == 0 {
		return nil
	}
	if len(refs) > 500 {
		return errors.New("standard message query exceeds fixed 500 references")
	}
	expected := map[string]eventevidence.StandardReference{}
	matches := map[string]int{}
	parts := []string{}
	args := []any{}
	filters := bson.A{}
	for _, ref := range refs {
		key := referenceKey(ref.Producer, ref.EventID, ref.Destination)
		expected[key] = ref
		parts = append(parts, "(BINARY x.producer=BINARY ? AND BINARY x.message_id=BINARY ? AND BINARY x.destination=BINARY ?)")
		args = append(args, ref.Producer, ref.EventID, ref.Destination)
		filters = append(filters, bson.D{{Key: "producer", Value: ref.Producer}, {Key: "message_id", Value: ref.EventID}, {Key: "destination", Value: ref.Destination}})
	}
	if e := p.reader.sqlPages(ctx, "rm_outbox", strings.Join(parts, " OR "), args, func(r record) error {
		key := referenceKey(string(graphSQLBytes(r, "producer")), string(graphSQLBytes(r, "message_id")), string(graphSQLBytes(r, "destination")))
		ref, ok := expected[key]
		if !ok {
			return errors.New("SQL message query matched foreign physical reference")
		}
		matches[key]++
		return p.standardSQL(r, ref)
	}); e != nil {
		return e
	}
	if e := p.reader.mongoPages(ctx, "rm_outbox", bson.D{{Key: "$or", Value: filters}}, func(r record) error {
		raw := bson.Raw(r.BSON)
		key := referenceKey(graphMongoString(raw, "producer"), graphMongoString(raw, "message_id"), graphMongoString(raw, "destination"))
		ref, ok := expected[key]
		if !ok {
			return errors.New("mongo message query matched foreign physical reference")
		}
		matches[key]++
		return p.standardMongo(r, ref)
	}); e != nil {
		return e
	}
	for key := range expected {
		if matches[key] != 1 {
			p.holdObject(p.referenceOwners[key], "standard_reference:missing_or_multiple_durable_owner")
		}
	}
	return p.writer.flush()
}
func (p *cohortPlanner) discoverStandard(ctx context.Context) error {
	// A bounded identity-only set prevents repeated references from selecting a
	// row twice. Raw records remain page-local; larger reference sets require an
	// explicitly smaller execution subset rather than more memory or time.
	seen := map[string]bool{}
	batch := []eventevidence.StandardReference{}
	shards := append([]graphShard{}, p.writer.index.Shards...)
	for _, shard := range shards {
		page, e := readGraphPage(p.dir, shard)
		if e != nil {
			return e
		}
		for _, obj := range page.Objects {
			for _, ref := range obj.References {
				key := referenceKey(ref.Producer, ref.EventID, ref.Destination)
				if seen[key] {
					continue
				}
				if len(seen) >= 100000 {
					return errors.New("message reference subset exceeds fixed 100000 identity bound")
				}
				seen[key] = true
				if p.referenceOwners == nil {
					p.referenceOwners = map[string]graphObject{}
				}
				p.referenceOwners[key] = obj
				batch = append(batch, ref)
				if len(batch) == 500 {
					if e = p.standardBatch(ctx, batch); e != nil {
						return e
					}
					batch = nil
				}
			}
		}
	}
	return p.standardBatch(ctx, batch)
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
				p.writer.index.Holds[c.r.Object.Name+":missing_assessment"]++
				continue
			}
		}
		if !owned {
			continue
		}
		if e := p.emitSQL(c.r); e != nil {
			return e
		}
	}
	return nil
}
func (p *cohortPlanner) discoverResponsibilities(ctx context.Context) error {
	if e := p.discoverStandard(ctx); e != nil {
		return e
	}
	roots := map[string]bool{}
	for _, id := range p.selector.TesteeIDs {
		roots[id] = true
	}
	for _, name := range []string{"event_delivery_dead_letter", "retry_event_hold"} {
		p.writer.index.Counts["mysql/"+name] = 0
		batch := []record{}
		flush := func() error {
			if len(batch) == 0 {
				return nil
			}
			e := p.classifyDeliveries(ctx, batch, roots)
			batch = nil
			return e
		}
		if e := p.reader.sqlPages(ctx, name, "x.org_id=?", []any{p.selector.Selection.OrgID}, func(r record) error {
			batch = append(batch, r)
			if len(batch) == 1000 {
				return flush()
			}
			return nil
		}); e != nil {
			return e
		}
		if e := flush(); e != nil {
			return e
		}
		if e := p.writer.flush(); e != nil {
			return e
		}
	}
	// AI scope is intentionally read-only and zero-only for this confirmed batch.
	// Any target reference prevents a claim of a complete zero AI range; it does
	// not silently authorize an AI purge or infer ownership of its real sessions.
	p.writer.index.Counts["mysql/ai_bridge_requests_target"] = 0
	if e := rootBatches(p.selector.TesteeIDs, 1000, func(ids []string) error {
		slots, args := placeholders(ids)
		args = append(args, p.selector.Selection.OrgID)
		where := "x.organization_id=? AND (x.testee_id IN (" + slots + ") OR EXISTS (SELECT 1 FROM ai_bridge_request_assessments ra JOIN assessment a ON a.id=ra.assessment_id WHERE ra.request_id=x.request_id AND a.testee_id IN (" + slots + ") AND a.org_id=?))"
		baseArgs := append([]any{}, args[:len(ids)]...)
		args = append([]any{p.selector.Selection.OrgID}, baseArgs...)
		args = append(args, baseArgs...)
		args = append(args, p.selector.Selection.OrgID)
		return p.reader.sqlPages(ctx, "ai_bridge_requests", where, args, func(r record) error {
			p.writer.index.Counts["mysql/ai_bridge_requests_target"]++
			p.holdRecord(r, nil, "ai_bridge_requests:requires_separate_shared_reference_resolution")
			return nil
		})
	}); e != nil {
		return e
	}
	p.writer.index.Retained["ai_bridge_requests/ai_messaging/qs-ai"] = "no purge implemented; real requests, sessions and their message graph remain protected; target intersection must be zero"
	// Replay headers cannot be owned from an equal request string alone. Their
	// local item/header relationship and all members require separate resolution.
	for _, name := range []string{"qs_rm_replay_requests", "qs_rm_replay_items"} {
		if e := p.reader.sqlPages(ctx, name, "x.org_id=?", []any{p.selector.Selection.OrgID}, func(r record) error {
			p.writer.index.Retained[name] = "request is retained unless every typed item has proved target ownership"
			p.holdRecord(r, nil, name+":manual_replay_responsibility")
			return nil
		}); e != nil {
			return e
		}
	}
	return nil
}
