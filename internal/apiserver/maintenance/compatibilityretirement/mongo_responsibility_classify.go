package retirement

import (
	"bytes"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventevidencebinding"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	sheetmongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	interpretmongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	eventoutcome "github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/reliable-messaging/message"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"go.mongodb.org/mongo-driver/bson"
)

// Only compact identity/clock/reference facts survive a streamed row. Full
// source BSON is hashed as received and discarded; no body archive is written.
type mongoCycleSheet struct {
	org, testee    uint64
	binding, event string
	proof          *evidence.EventEvidenceV1
	history        *evidence.HistoricalReferenceSetV1
}
type mongoCycleGeneration struct {
	outcome, run, artifact uint64
	status, event          string
	reportType, template   string
	schema                 uint32
	proof                  *evidence.EventEvidenceV1
	history                *evidence.HistoricalReferenceSetV1
}
type mongoCycleRun struct {
	generation                 uint64
	attempt                    int
	status, event, disposition string
	origin, action             string
	lease, finished            *time.Time
	next                       *time.Time
	failure                    *interpretmongo.InterpretationFailurePO
	proof                      *evidence.EventEvidenceV1
}
type mongoCycleArtifact struct {
	generation, outcome, run, assessment, testee uint64
	org                                          int64
	reportType, template                         string
	generated                                    time.Time
	binding                                      string
}
type mongoCycleMessage struct {
	row         mongoOwnerStandardRow
	observation int
	reference   evidence.StandardReference
	payloadIDs  map[string]string
	generated   *eventoutcome.ReportGeneratedPayload
	failed      *eventoutcome.ReportFailedPayload
	retry       *eventoutcome.InterpretationRetryRequestedPayload
}
type mongoCycleReplayItem struct {
	event      string
	count      uint64
	authorized bool
	reason     string
}
type mongoCycleReplay struct {
	org         uint64
	request     string
	items       []mongoCycleReplayItem
	observation int
}
type mongoCycleGraph struct {
	sheets      map[uint64]mongoCycleSheet
	generations map[uint64]mongoCycleGeneration
	runs        map[uint64]mongoCycleRun
	artifacts   map[uint64]mongoCycleArtifact
	messages    map[string]mongoCycleMessage
	replays     []mongoCycleReplay
	originals   map[string]string
}

func (g *mongoCycleGraph) initialize() {
	g.sheets = map[uint64]mongoCycleSheet{}
	g.generations = map[uint64]mongoCycleGeneration{}
	g.runs = map[uint64]mongoCycleRun{}
	g.artifacts = map[uint64]mongoCycleArtifact{}
	g.messages = map[string]mongoCycleMessage{}
	g.originals = map[string]string{}
}

func (s *MongoResponsibilitySnapshot) reserveGraph(bytes uint64) error {
	s.report.GraphEntries++
	s.report.GraphBytes += bytes + 256
	if s.report.GraphEntries > s.limits.MaxGraphEntries || s.report.GraphBytes > s.limits.MaxGraphBytes {
		return ErrMongoCycleBounds
	}
	return nil
}
func (s *MongoResponsibilitySnapshot) addObservation(o MongoResponsibilityObservation) error {
	if err := s.reserveGraph(uint64(len(o.EventID) + len(o.EventType) + len(o.OwnerKind) + len(o.OwnerID) + len(o.BindingSHA256) + len(o.ContentSHA256) + len(o.PrimaryKeySHA256) + len(o.RowSHA256))); err != nil {
		return err
	}
	i := len(s.observations)
	s.observations = append(s.observations, o)
	s.report.ClassifiedRows++
	s.report.ClassCounts[o.Class]++
	if o.EventID != "" {
		s.byEvent[o.EventID] = append(s.byEvent[o.EventID], i)
	}
	for _, v := range []struct {
		id    uint64
		index map[string][]int
	}{{o.AnswerSheetID, s.bySheet}, {o.AssessmentID, s.byAssessment}, {o.GenerationID, s.byGeneration}, {o.OutcomeID, s.byOutcome}} {
		if v.id != 0 {
			k := mongoCycleKey(v.id)
			v.index[k] = append(v.index[k], i)
		}
	}
	if o.Invalid {
		s.block("invalid_or_orphan_global_mongo_responsibility")
	}
	if o.OwnerUnproven {
		s.gap("global_mongo_owner_or_external_graph_unproven")
	}
	return nil
}
func mongoCycleID(fields map[string]bson.RawValue, key string) (uint64, error) {
	n, ok := mongoExactInteger(fields[key])
	if !ok || n <= 0 {
		return 0, ErrMongoCycleSchema
	}
	return uint64(n), nil
}
func mongoCycleStringID(value string) (uint64, error) {
	n, e := strconv.ParseUint(value, 10, 63)
	if e != nil || n == 0 || strconv.FormatUint(n, 10) != value {
		return 0, ErrMongoCycleSchema
	}
	return n, nil
}
func (s *MongoResponsibilitySnapshot) registerHistory(set *evidence.HistoricalReferenceSetV1, eventType, owner string) error {
	if set == nil {
		return nil
	}
	if set.Validate() != nil {
		return ErrMongoCycleSchema
	}
	for _, e := range set.Entries {
		if e.EventType != eventType {
			return ErrMongoCycleSchema
		}
		if err := s.reserveGraph(uint64(len(e.EventID) + len(e.Source.PrimaryKeySHA256) + 512)); err != nil {
			return err
		}
		key := e.EventID
		if _, ok := s.graph.originals[key]; ok {
			return ErrMongoCycleConflict
		}
		s.graph.originals[key] = owner
		s.byEvent[key] = append(s.byEvent[key], len(s.observations))
		// A claimed history proof remains a business/source-coordinator input;
		// its presence cannot authenticate the source copy or fence writers.
	}
	return nil
}

func (s *MongoResponsibilitySnapshot) classifyRow(name string, raw bson.Raw) error {
	f, err := exactBSONFields(raw)
	if err != nil {
		return ErrMongoCycleSchema
	}
	o := MongoResponsibilityObservation{Collection: name, PrimaryKeySHA256: mongoCycleToken(f["_id"]), RowSHA256: sourceSHA(raw), Class: "current_business_fact"}
	typ := reflect.TypeOf(sheetmongo.AnswerSheetPO{})
	switch name {
	case "answersheets":
	case "report_generations":
		typ = reflect.TypeOf(interpretmongo.ReportGenerationPO{})
	case "interpretation_runs":
		typ = reflect.TypeOf(interpretmongo.InterpretationRunPO{})
	case "interpret_report_artifacts":
		typ = reflect.TypeOf(interpretmongo.InterpretReportPO{})
	default:
		typ = nil
	}
	if typ != nil {
		if mongoPOShape(raw, typ) != nil {
			return ErrMongoCycleSchema
		}
		if _, err = mongoCycleID(f, "domain_id"); err != nil {
			return err
		}
	}
	switch name {
	case "answersheets":
		var p sheetmongo.AnswerSheetPO
		if bson.Unmarshal(raw, &p) != nil {
			return ErrMongoCycleSchema
		}
		id := uint64(p.DomainID)
		if _, ok := s.graph.sheets[id]; ok {
			return ErrMongoCycleConflict
		}
		o.AnswerSheetID, o.OrgID = id, p.OrgID
		o.OwnerKind, o.OwnerID = "AnswerSheet", mongoCycleKey(id)
		g := mongoCycleSheet{org: p.OrgID, testee: p.TesteeID, history: p.LegacySubmissionEvidence}
		payload, e := mongoSubmissionPayload(p)
		if e == nil {
			g.binding, e = eventevidencebinding.AnswerSheet(payload)
		}
		if e != nil {
			o.OwnerUnproven = true
			o.Class = "original_business_verification_required"
		}
		if p.DurableAcceptance != nil {
			g.event = p.DurableAcceptance.EventID
			g.proof = p.DurableAcceptance.EventEvidence
			o.EventID, o.EventType = g.event, "answersheet.submitted"
			if p.DurableAcceptance.SchemaVersion != 1 || g.event == "" || !BusinessTimeEqual(p.DurableAcceptance.AcceptedAt, p.FilledAt) {
				o.Invalid = true
			}
		}
		if err = s.registerHistory(p.LegacySubmissionEvidence, "answersheet.submitted", "answersheets:"+mongoCycleKey(id)); err != nil {
			return err
		}
		// Keep only bounded reference/identity data; answers and attribution
		// body are not retained in this private global graph.
		if err = s.reserveGraph(uint64(128+len(g.event)+len(g.binding)) + mongoCycleHistoryBytes(g.history) + mongoCycleProofBytes(g.proof)); err != nil {
			return err
		}
		s.graph.sheets[id] = g
		o.BindingSHA256 = g.binding
	case "report_generations":
		var p interpretmongo.ReportGenerationPO
		if bson.Unmarshal(raw, &p) != nil {
			return ErrMongoCycleSchema
		}
		id := uint64(p.DomainID)
		if _, ok := s.graph.generations[id]; ok {
			return ErrMongoCycleConflict
		}
		if p.OutcomeID == 0 || p.Version == 0 {
			return ErrMongoCycleSchema
		}
		if p.TransactionSchemaVersion > 1 {
			return ErrMongoCycleSchema
		}
		o.GenerationID, o.OutcomeID, o.RunID = id, p.OutcomeID, p.LatestRunID
		o.OwnerKind, o.OwnerID = "ReportGeneration", mongoCycleKey(id)
		o.EventID, o.EventType = p.GeneratedEventID, "interpretation.report.generated"
		switch p.Status {
		case "generated", "failed":
		case "pending", "generating":
			o.Unfinished = true
		default:
			return ErrMongoCycleSchema
		}
		g := mongoCycleGeneration{outcome: p.OutcomeID, run: p.LatestRunID, artifact: p.ReportID, status: p.Status, event: p.GeneratedEventID, reportType: p.ReportType, template: p.TemplateVersion, schema: p.TransactionSchemaVersion, proof: p.GeneratedEventEvidence, history: p.HistoricalGeneratedEvidence}
		if err = s.registerHistory(g.history, "interpretation.report.generated", "report_generations:"+mongoCycleKey(id)); err != nil {
			return err
		}
		if err = s.reserveGraph(uint64(128+len(g.event)) + mongoCycleHistoryBytes(g.history) + mongoCycleProofBytes(g.proof)); err != nil {
			return err
		}
		s.graph.generations[id] = g
	case "interpretation_runs":
		var p interpretmongo.InterpretationRunPO
		if bson.Unmarshal(raw, &p) != nil {
			return ErrMongoCycleSchema
		}
		id := uint64(p.DomainID)
		if _, ok := s.graph.runs[id]; ok {
			return ErrMongoCycleConflict
		}
		if p.GenerationID == 0 || p.Attempt <= 0 {
			return ErrMongoCycleSchema
		}
		o.GenerationID, o.RunID = p.GenerationID, id
		o.OwnerKind, o.OwnerID = "ReportGeneration", mongoCycleKey(p.GenerationID)
		o.EventID, o.EventType = p.RetryEventID, "interpretation.retry.requested"
		switch p.Status {
		case "succeeded", "failed":
			if p.FinishedAt == nil {
				o.Invalid = true
			}
		case "pending", "running":
			o.Unfinished = true
		default:
			return ErrMongoCycleSchema
		}
		// A persisted future retry/manual wait or active lease is responsibility.
		if p.RetryDisposition == "automatic" || p.RetryDisposition == "manual_required" || p.NextAttemptAt != nil || p.LeaseExpiresAt != nil {
			o.Unfinished = true
		}
		o.LeasePresent = p.LeaseExpiresAt != nil
		r := mongoCycleRun{generation: p.GenerationID, attempt: p.Attempt, status: p.Status, event: p.RetryEventID, disposition: p.RetryDisposition, origin: p.AttemptOrigin, action: p.ActionRequestID, next: p.NextAttemptAt, lease: p.LeaseExpiresAt, finished: p.FinishedAt, failure: p.Failure, proof: p.RetryEventEvidence}
		if err = s.reserveGraph(uint64(256+len(r.event)+len(r.disposition)) + mongoCycleProofBytes(r.proof)); err != nil {
			return err
		}
		s.graph.runs[id] = r
	case "interpret_report_artifacts":
		var p interpretmongo.InterpretReportPO
		if bson.Unmarshal(raw, &p) != nil {
			return ErrMongoCycleSchema
		}
		id := uint64(p.DomainID)
		if _, ok := s.graph.artifacts[id]; ok {
			return ErrMongoCycleConflict
		}
		if p.GenerationID == 0 || p.OutcomeID == 0 || p.InterpretationRunID == 0 || p.OrgID <= 0 || p.AssessmentID == 0 || p.TesteeID == 0 || p.GeneratedAt.IsZero() {
			return ErrMongoCycleSchema
		}
		o.GenerationID, o.OutcomeID, o.RunID, o.AssessmentID, o.OrgID = p.GenerationID, p.OutcomeID, p.InterpretationRunID, p.AssessmentID, uint64(p.OrgID)
		o.OwnerKind, o.OwnerID = "ReportGeneration", mongoCycleKey(p.GenerationID)
		a := mongoCycleArtifact{generation: p.GenerationID, outcome: p.OutcomeID, run: p.InterpretationRunID, assessment: p.AssessmentID, testee: p.TesteeID, org: p.OrgID, reportType: p.ReportType, template: p.TemplateVersion, generated: p.GeneratedAt}
		if r, ok := s.graph.runs[p.InterpretationRunID]; ok && p.Model != nil {
			payload := eventoutcome.ReportGeneratedPayload{OrgID: p.OrgID, GenerationID: mongoCycleKey(p.GenerationID), RunID: mongoCycleKey(p.InterpretationRunID), ReportID: mongoCycleKey(id), AssessmentID: mongoCycleKey(p.AssessmentID), OutcomeID: mongoCycleKey(p.OutcomeID), TesteeID: p.TesteeID, Attempt: uint(r.attempt), ReportType: p.ReportType, TemplateVersion: p.TemplateVersion, BuilderIdentity: p.BuilderIdentity, ContentSchemaVersion: p.ContentSchemaVersion, GeneratedAt: p.GeneratedAt, Model: eventoutcome.ModelIdentity{Kind: p.Model.Kind, Algorithm: p.Model.Algorithm, Code: p.Model.Code, Version: p.Model.Version, Title: p.Model.Title}}
			if v := p.PrimaryScore; v != nil {
				payload.PrimaryScore = &eventoutcome.ScoreValue{Kind: v.Kind, Value: v.Value, Label: v.Label, Max: v.Max}
			}
			if v := p.Level; v != nil {
				payload.Level = &eventoutcome.ResultLevel{Code: v.Code, Label: v.Label, Severity: v.Severity}
			}
			var bindingErr error
			a.binding, bindingErr = eventevidencebinding.Generated(payload)
			if bindingErr != nil {
				o.OwnerUnproven = true
			}
		} else {
			o.OwnerUnproven = true
		}
		if err = s.reserveGraph(uint64(256 + len(a.reportType) + len(a.template))); err != nil {
			return err
		}
		s.graph.artifacts[id] = a
	case "report_query_catalog":
		// PO omits Mongo's automatic _id; explicitly admit exactly that field.
		var d bson.D
		if bson.Unmarshal(raw, &d) != nil {
			return ErrMongoCycleSchema
		}
		fields := bson.D{}
		for _, v := range d {
			if v.Key != "_id" {
				fields = append(fields, v)
			}
		}
		body, e := bson.Marshal(fields)
		if e != nil || mongoPOShape(body, reflect.TypeOf(interpretmongo.ReportCatalogPO{})) != nil {
			return ErrMongoCycleSchema
		}
		var p interpretmongo.ReportCatalogPO
		if bson.Unmarshal(raw, &p) != nil || p.OrgID <= 0 || p.AssessmentID == 0 || p.GenerationID == 0 || p.SourceID == 0 || p.SourceKind != "artifact" {
			return ErrMongoCycleSchema
		}
		o.AssessmentID, o.GenerationID, o.OutcomeID, o.OrgID = p.AssessmentID, p.GenerationID, p.OutcomeID, uint64(p.OrgID)
		o.OwnerID = mongoCycleKey(p.SourceID)
		o.OwnerKind = "ArtifactCatalog"
		o.Class = "current_query_projection"
	case "rm_outbox":
		return s.classifyRM(raw, o)
	case "qs_rm_replay_requests":
		return s.classifyReplay(raw, o)
	case "interpretation_catalog_repair_plans":
		// This is an actual mutating maintenance authorization, not an Inbox.
		// Until its original dry-run/reconcile contract is jointly resolved it
		// must remain explicit unknown, never silently treated as unrelated.
		o.Class = "maintenance_repair_authorization_unknown"
		o.OwnerUnproven = true
		s.gap("catalog_repair_plan_execution_coverage_required")
	case "evaluation_acceptance_failure_claims", "interpretation_acceptance_failure_claims", "qrcode_acceptance_failure_claims":
		return s.classifyClaim(name, f, o)
	default:
		return ErrMongoCycleSchema
	}
	return s.addObservation(o)
}

func mongoCycleHistoryBytes(s *evidence.HistoricalReferenceSetV1) uint64 {
	if s == nil {
		return 0
	}
	raw, _ := bson.Marshal(s)
	return uint64(len(raw))
}
func mongoCycleProofBytes(p *evidence.EventEvidenceV1) uint64 {
	if p == nil {
		return 0
	}
	raw, _ := bson.Marshal(p)
	return uint64(len(raw))
}

func (s *MongoResponsibilitySnapshot) classifyRM(raw bson.Raw, o MongoResponsibilityObservation) error {
	f, e := exactBSONFields(raw)
	if e != nil {
		return ErrMongoCycleSchema
	}
	allowed := strings.Fields("_id producer message_id destination event_type schema_version scope content_type occurred_at payload fingerprint state next_attempt_at created_at updated_at version attempt_count failure_count claim_token lease_until last_error_code transport_confirmed_at manual_replay_request_id manual_replay_version")
	set := map[string]bool{}
	for _, k := range allowed {
		set[k] = true
	}
	for k := range f {
		if !set[k] {
			return ErrMongoCycleSchema
		}
	}
	for _, k := range []string{"producer", "message_id", "destination", "event_type", "schema_version", "scope", "content_type", "occurred_at", "state"} {
		if f[k].Type != bson.TypeString {
			return ErrMongoCycleSchema
		}
	}
	for _, k := range []string{"payload", "fingerprint"} {
		if f[k].Type != bson.TypeBinary {
			return ErrMongoCycleSchema
		}
	}
	for _, k := range []string{"version", "attempt_count", "failure_count", "manual_replay_version"} {
		if v, exists := f[k]; exists {
			n, ok := mongoExactInteger(v)
			if !ok || n < 0 {
				return ErrMongoCycleSchema
			}
		}
	}
	for _, k := range []string{"next_attempt_at", "created_at", "updated_at", "lease_until", "transport_confirmed_at"} {
		if v, ok := f[k]; ok && v.Type != bson.TypeDateTime {
			return ErrMongoCycleSchema
		}
	}
	for _, k := range []string{"claim_token", "last_error_code", "manual_replay_request_id"} {
		if v, ok := f[k]; ok && v.Type != bson.TypeString {
			return ErrMongoCycleSchema
		}
	}
	var row mongoOwnerStandardRow
	if bson.Unmarshal(raw, &row) != nil {
		return ErrMongoCycleSchema
	}
	// The SDK key is ordered and exact. Maps or reordered compound keys are
	// not the persisted key emitted by the pinned SDK producer.
	elements, e := row.ID.Elements()
	if e != nil || len(elements) != 3 {
		return ErrMongoCycleSchema
	}
	for i, k := range []string{"producer", "message_id", "destination"} {
		if elements[i].Key() != k {
			return ErrMongoCycleSchema
		}
	}
	inner, e := row.envelope()
	if e != nil {
		return ErrMongoCycleSchema
	}
	outer, recognized, e := legacy.Decode(row.Payload)
	var exactEnvelope domainwire.Envelope
	if e != nil || !recognized || strictTyped(outer.Payload, &exactEnvelope) != nil {
		return ErrMongoCycleSchema
	}
	if len(outer.Metadata) != 5 {
		return ErrMongoCycleSchema
	}
	for key := range outer.Metadata {
		switch key {
		case "event_type", "aggregate_type", "aggregate_id", "occurred_at", "source":
		default:
			return ErrMongoCycleSchema
		}
	}
	org, e := strconv.ParseUint(strings.TrimPrefix(row.Scope, "org:"), 10, 63)
	if e != nil || org == 0 || row.Scope != "org:"+mongoCycleKey(org) {
		return ErrMongoCycleSchema
	}
	if _, exists := s.graph.messages[row.MessageID]; exists {
		return ErrMongoCycleConflict
	}
	o.EventID, o.EventType, o.OrgID, o.OwnerKind, o.OwnerID = row.MessageID, row.EventType, org, inner.AggregateType, inner.AggregateID
	o.ContentSHA256 = sourceSHA(outer.Payload)
	o.Class = "current_live_standard_message"
	_, o.LeasePresent = f["lease_until"]
	if _, ok := f["claim_token"]; ok {
		o.LeasePresent = true
	}
	switch row.State {
	case "pending", "retry_wait", "publishing", "quarantined":
		o.Unfinished = true
	case "published":
		if row.Version == 0 || row.TransportConfirmedAt == nil || row.TransportConfirmedAt.IsZero() {
			return ErrMongoCycleSchema
		}
	default:
		return ErrMongoCycleSchema
	}
	ids := map[string]string{}
	for _, k := range []string{"answer_sheet_id", "answersheet_id", "assessment_id", "generation_id", "outcome_id", "run_id", "report_id"} {
		v, err := mongoPayloadID(inner.Data, k)
		if err != nil {
			return ErrMongoCycleSchema
		}
		if v != "" {
			ids[k] = v
		}
	}
	for k, v := range ids {
		n, _ := mongoCycleStringID(v)
		switch k {
		case "answer_sheet_id", "answersheet_id":
			o.AnswerSheetID = n
		case "assessment_id":
			o.AssessmentID = n
		case "generation_id":
			o.GenerationID = n
		case "outcome_id":
			o.OutcomeID = n
		case "run_id":
			o.RunID = n
		}
	}
	var generated *eventoutcome.ReportGeneratedPayload
	var failed *eventoutcome.ReportFailedPayload
	var retry *eventoutcome.InterpretationRetryRequestedPayload
	switch inner.EventType {
	case "answersheet.submitted":
		var p eventpayload.AnswerSheetSubmittedData
		if strictTyped(inner.Data, &p) != nil || p.OrgID != org || p.TesteeID == 0 || inner.AggregateType != "AnswerSheet" || inner.AggregateID != p.AnswerSheetID {
			return ErrMongoCycleSchema
		}
		o.BindingSHA256, e = eventevidencebinding.AnswerSheet(p)
	case "interpretation.report.generated":
		var p eventoutcome.ReportGeneratedPayload
		if strictTyped(inner.Data, &p) != nil || p.OrgID != int64(org) || inner.AggregateType != "ReportGeneration" || inner.AggregateID != p.GenerationID {
			return ErrMongoCycleSchema
		}
		o.BindingSHA256, e = eventevidencebinding.Generated(p)
		generated = &p
	case "interpretation.retry.requested":
		var p eventoutcome.InterpretationRetryRequestedPayload
		if strictTyped(inner.Data, &p) != nil || p.OrgID != int64(org) || inner.AggregateType != "ReportGeneration" || inner.AggregateID != p.GenerationID {
			return ErrMongoCycleSchema
		}
		o.BindingSHA256, e = eventevidencebinding.Retry(p)
		retry = &p
	case "interpretation.report.failed":
		var p eventoutcome.ReportFailedPayload
		if strictTyped(inner.Data, &p) != nil || p.OrgID != int64(org) || inner.AggregateType != "ReportGeneration" || inner.AggregateID != p.GenerationID {
			return ErrMongoCycleSchema
		}
		failed = &p
	case "evaluation.requested", "evaluation.retry.requested", "evaluation.outcome.committed", "evaluation.failed", "task.opened.reminder.requested":
		o.Class = "sql_owner_coordination_required"
		o.OwnerUnproven = true
	default:
		o.Class = "unsupported_standard_message_type"
		o.OwnerUnproven = true
		s.gap("unknown_standard_event_type_global_coverage_required")
	}
	if e != nil {
		return ErrMongoCycleSchema
	}
	msg, e := message.New(row.input())
	if e != nil {
		return ErrMongoCycleSchema
	}
	if err := s.reserveGraph(uint64(len(row.MessageID) + len(row.State) + len(row.ManualReplayRequestID) + len(inner.Data) + 1024)); err != nil {
		return err
	}
	// Do not retain the payload in row. References and typed local bindings
	// are enough for indexed responsibility/owner checking.
	row.Payload = nil
	row.ID = nil
	row.Fingerprint = nil
	s.graph.messages[row.MessageID] = mongoCycleMessage{row: row, observation: len(s.observations), reference: standard.ReferenceFromMessage(msg), payloadIDs: ids, generated: generated, failed: failed, retry: retry}
	return s.addObservation(o)
}

func (s *MongoResponsibilitySnapshot) classifyReplay(raw bson.Raw, o MongoResponsibilityObservation) error {
	f, e := exactBSONFields(raw)
	if e != nil {
		return ErrMongoCycleSchema
	}
	allowed := map[string]bool{"_id": true, "org_id": true, "request_id": true, "store_name": true, "reason": true, "input_hash": true, "items": true, "created_at": true}
	for k := range f {
		if !allowed[k] {
			return ErrMongoCycleSchema
		}
	}
	for _, k := range []string{"_id", "request_id", "store_name", "reason"} {
		if f[k].Type != bson.TypeString {
			return ErrMongoCycleSchema
		}
	}
	org, e := mongoCycleID(f, "org_id")
	if e != nil || f["items"].Type != bson.TypeArray || f["input_hash"].Type != bson.TypeBinary {
		return ErrMongoCycleSchema
	}
	request := standard.ReplayRequest{OrgID: int64(org), RequestID: f["request_id"].StringValue(), Store: f["store_name"].StringValue(), Reason: f["reason"].StringValue()}
	if f["_id"].StringValue() != mongoCycleKey(org)+":"+request.RequestID || request.Store != "mongo-domain-events" {
		return ErrMongoCycleSchema
	}
	if v, ok := f["created_at"]; ok && v.Type != bson.TypeDateTime {
		return ErrMongoCycleSchema
	}
	values, e := f["items"].Array().Values()
	if e != nil || len(values) == 0 || len(values) > 100 {
		return ErrMongoCycleSchema
	}
	r := mongoCycleReplay{org: org, request: request.RequestID, observation: len(s.observations)}
	for _, v := range values {
		if v.Type != bson.TypeEmbeddedDocument {
			return ErrMongoCycleSchema
		}
		item, e := exactBSONFields(v.Document())
		if e != nil || len(item) != 4 || item["event_id"].Type != bson.TypeString || item["authorized"].Type != bson.TypeBoolean || item["reason"].Type != bson.TypeString {
			return ErrMongoCycleSchema
		}
		count, e := mongoCycleID(item, "expected_failure_count")
		if e != nil {
			return e
		}
		id := item["event_id"].StringValue()
		reason := item["reason"].StringValue()
		authorized := item["authorized"].Boolean()
		if id == "" || reason == "" {
			return ErrMongoCycleSchema
		}
		request.Targets = append(request.Targets, standard.ReplayTarget{EventID: id, ExpectedFailureCount: count})
		r.items = append(r.items, mongoCycleReplayItem{event: id, count: count, authorized: authorized, reason: reason})
	}
	h, e := request.Fingerprint()
	_, input := f["input_hash"].Binary()
	if e != nil || !bytes.Equal(h[:], input) {
		return ErrMongoCycleSchema
	}
	o.OrgID = org
	o.Class = "durable_replay_ledger"
	if err := s.reserveGraph(uint64(len(raw))); err != nil {
		return err
	}
	s.graph.replays = append(s.graph.replays, r)
	return s.addObservation(o)
}

func (s *MongoResponsibilitySnapshot) classifyClaim(name string, f map[string]bson.RawValue, o MongoResponsibilityObservation) error {
	base := []string{"_id", "claimed_at", "expires_at"}
	specific := []string{"org_id", "testee_id", "model_code", "assessment_id", "run_id"}
	switch name {
	case "evaluation_acceptance_failure_claims":
		specific = append(specific, "input_snapshot_ref")
	case "interpretation_acceptance_failure_claims":
		specific = append(specific, "outcome_id")
	case "qrcode_acceptance_failure_claims":
		specific = []string{"kind", "code", "version", "external_call_started"}
	}
	allowed := map[string]bool{}
	for _, k := range append(base, specific...) {
		allowed[k] = true
	}
	for k := range f {
		if !allowed[k] {
			return ErrMongoCycleSchema
		}
	}
	for _, k := range base {
		if k == "_id" {
			if f[k].Type != bson.TypeString {
				return ErrMongoCycleSchema
			}
		} else if f[k].Type != bson.TypeDateTime {
			return ErrMongoCycleSchema
		}
	}
	for _, k := range specific {
		if _, ok := f[k]; !ok {
			return ErrMongoCycleSchema
		}
	}
	if name == "qrcode_acceptance_failure_claims" {
		for _, k := range []string{"kind", "code", "version"} {
			if f[k].Type != bson.TypeString {
				return ErrMongoCycleSchema
			}
		}
		if f["external_call_started"].Type != bson.TypeBoolean || f["external_call_started"].Boolean() {
			o.Invalid = true
		}
	} else {
		org, e := mongoCycleID(f, "org_id")
		if e != nil {
			return e
		}
		if _, e = mongoCycleID(f, "testee_id"); e != nil {
			return e
		}
		o.OrgID = org
		for _, k := range []string{"assessment_id", "run_id", "outcome_id"} {
			if v, ok := f[k]; ok {
				if v.Type != bson.TypeString {
					return ErrMongoCycleSchema
				}
				id, e := mongoCycleStringID(v.StringValue())
				if e != nil {
					return e
				}
				switch k {
				case "assessment_id":
					o.AssessmentID = id
				case "run_id":
					o.RunID = id
				case "outcome_id":
					o.OutcomeID = id
				}
			}
		}
		for _, k := range []string{"model_code", "input_snapshot_ref"} {
			if v, ok := f[k]; ok && v.Type != bson.TypeString {
				return ErrMongoCycleSchema
			}
		}
	}
	o.Class = "failure_injection_claim_audit_fact"
	return s.addObservation(o)
}

func (s *MongoResponsibilitySnapshot) classifyGraph() error {
	// Resolve all inverse relationships from global indexes. The original
	// Outcome graph and AI/Inbox responsibility remain SQL-coordinator duties.
	artifactByGeneration := map[uint64]uint64{}
	attempts := map[string]uint64{}
	for id, r := range s.graph.runs {
		key := mongoCycleKey(r.generation) + ":" + strconv.Itoa(r.attempt)
		if _, ok := attempts[key]; ok {
			s.block("duplicate_generation_attempt_identity")
		}
		attempts[key] = id
	}
	for id, a := range s.graph.artifacts {
		g, gok := s.graph.generations[a.generation]
		r, rok := s.graph.runs[a.run]
		if !gok || !rok || r.generation != a.generation || g.outcome != a.outcome || g.reportType != a.reportType || g.template != a.template || r.status != "succeeded" || r.finished == nil || !BusinessTimeEqual(*r.finished, a.generated) {
			s.block("orphan_or_conflicting_artifact_original_run_graph")
		}
		if _, exists := artifactByGeneration[a.generation]; exists {
			s.block("multiple_artifacts_for_generation")
		}
		artifactByGeneration[a.generation] = id
	}
	for id, g := range s.graph.generations {
		if g.status == "generated" && g.schema == 1 && g.event == "" {
			s.block("native_generated_event_reference_absent")
		}
		if g.run != 0 {
			r, ok := s.graph.runs[g.run]
			if !ok || r.generation != id {
				s.block("generation_latest_run_owner_conflict")
			}
		}
		if g.status == "generated" {
			a, ok := s.graph.artifacts[g.artifact]
			if !ok || a.generation != id || a.outcome != g.outcome || g.run != a.run {
				s.block("generated_owner_artifact_missing_or_conflicting")
			}
		}
		if g.event != "" {
			m, ok := s.graph.messages[g.event]
			if g.schema == 1 {
				if !ok || m.row.EventType != "interpretation.report.generated" || s.observations[m.observation].GenerationID != id {
					s.block("standard_generated_reference_orphan")
				}
				binding := s.graph.artifacts[g.artifact].binding
				if g.proof != nil && mongoCycleProofMatches(g.proof, m.reference, binding, true) != nil {
					s.block("standard_generated_evidence_conflict")
				}
			} else {
				s.gap("historical_generation_single_slot_requires_original_source_verification")
			}
		}
	}
	for _, r := range s.graph.runs {
		if _, ok := s.graph.generations[r.generation]; !ok {
			s.block("orphan_interpretation_run")
		}
		if r.event != "" {
			m, ok := s.graph.messages[r.event]
			if !ok || m.row.EventType != "interpretation.retry.requested" {
				s.block("retry_reference_orphan")
			}
			binding := s.observations[m.observation].BindingSHA256
			if r.proof != nil && mongoCycleProofMatches(r.proof, m.reference, binding, true) != nil {
				s.block("retry_evidence_conflict")
			}
		}
	}
	for id, g := range s.graph.sheets {
		if g.event != "" {
			m, ok := s.graph.messages[g.event]
			if !ok || m.row.EventType != "answersheet.submitted" || s.observations[m.observation].AnswerSheetID != id {
				s.block("standard_submission_reference_orphan")
			}
			if g.proof != nil && mongoCycleProofMatches(g.proof, m.reference, g.binding, true) != nil {
				s.block("standard_submission_evidence_conflict")
			}
		}
	}
	for event, m := range s.graph.messages {
		o := &s.observations[m.observation]
		switch m.row.EventType {
		case "answersheet.submitted":
			g, ok := s.graph.sheets[o.AnswerSheetID]
			if !ok || g.org != o.OrgID || g.event != event || g.binding == "" || g.binding != o.BindingSHA256 {
				o.Invalid = true
				o.Reasons = append(o.Reasons, "standard_submission_owner_binding_conflict")
			}
		case "interpretation.report.generated", "interpretation.report.failed", "interpretation.retry.requested":
			g, gok := s.graph.generations[o.GenerationID]
			r, rok := s.graph.runs[o.RunID]
			if !gok || !rok || r.generation != o.GenerationID || g.outcome != o.OutcomeID {
				o.Invalid = true
				o.Reasons = append(o.Reasons, "standard_report_run_owner_conflict")
			}
			if m.row.EventType == "interpretation.report.generated" {
				aid, e := mongoCycleStringID(m.payloadIDs["report_id"])
				a, ok := s.graph.artifacts[aid]
				if e != nil || !ok || a.generation != o.GenerationID || a.run != o.RunID || a.assessment != o.AssessmentID || uint64(a.org) != o.OrgID || g.event != event || g.schema != 1 || a.binding == "" || a.binding != o.BindingSHA256 || m.generated == nil || m.generated.Attempt != uint(r.attempt) || !BusinessTimeEqual(m.generated.GeneratedAt, a.generated) {
					o.Invalid = true
				}
				o.OwnerUnproven = true
				s.gap("standard_generated_complete_payload_requires_original_sql_outcome")
			}
			if m.row.EventType == "interpretation.retry.requested" {
				p := m.retry
				if r.event != event || p == nil || p.ExpectedAttempt != r.attempt || p.ActionRequestID != r.action || p.Mode != "next_attempt" || r.next == nil || !BusinessTimeEqual(p.RequestedAt, *r.next) || r.status != "failed" {
					o.Invalid = true
				}
				if p != nil {
					prefix := "interpret-retry:" + mongoCycleKey(r.generation) + ":" + strconv.Itoa(r.attempt) + ":"
					expected := prefix + p.AttemptOrigin
					if p.ActionRequestID != "" {
						expected += ":" + p.ActionRequestID
					}
					if expected != event {
						o.Invalid = true
					}
				}
				o.OwnerUnproven = true
				s.gap("retry_sql_outcome_and_original_execution_coverage_required")
			}
			if m.row.EventType == "interpretation.report.failed" {
				p := m.failed
				if p == nil || r.status != "failed" || r.failure == nil || r.finished == nil || p.Attempt != uint(r.attempt) || p.FailureKind != r.failure.Kind || p.FailureCode != r.failure.Code || p.Retryable != r.failure.Retryable || !BusinessTimeEqual(p.FailedAt, *r.finished) || p.ReportType != g.reportType || p.TemplateVersion != g.template {
					o.Invalid = true
				}
				o.OwnerUnproven = true
				s.gap("failed_report_sql_outcome_ownership_coverage_required")
			}
		}
		if o.Invalid {
			s.block("reverse_standard_message_owner_missing_or_conflicting")
		}
	}
	for _, r := range s.graph.replays {
		for _, item := range r.items {
			m, exists := s.graph.messages[item.event]
			i := r.observation
			s.byEvent[item.event] = append(s.byEvent[item.event], i)
			if !exists {
				if item.authorized {
					s.observations[i].Invalid = true
					s.block("authorized_replay_outbox_orphan")
				} else {
					s.gap("denied_replay_original_identity_requires_source_lookup")
				}
				continue
			}
			o := s.observations[m.observation]
			if o.OrgID != r.org {
				s.observations[i].Invalid = true
				s.block("replay_cross_organization_conflict")
			}
			for _, v := range []struct {
				id    uint64
				index map[string][]int
			}{{o.AnswerSheetID, s.bySheet}, {o.AssessmentID, s.byAssessment}, {o.GenerationID, s.byGeneration}, {o.OutcomeID, s.byOutcome}} {
				if v.id != 0 {
					k := mongoCycleKey(v.id)
					v.index[k] = append(v.index[k], i)
				}
			}
			if item.authorized && (m.row.State != "published" || m.row.ManualReplayRequestID != r.request || m.row.ManualReplayVersion == 0 || m.row.Version < m.row.ManualReplayVersion+2 || m.row.FailureCount < item.count) {
				s.observations[i].Unfinished = true
				s.observations[i].Reasons = append(s.observations[i].Reasons, "authorized_replay_execution_unclosed")
			}
		}
	}
	for i, o := range s.observations {
		if o.Collection == "report_query_catalog" {
			id, e := mongoCycleStringID(o.OwnerID)
			a, ok := s.graph.artifacts[id]
			if e != nil || !ok || a.assessment != o.AssessmentID || a.generation != o.GenerationID || a.outcome != o.OutcomeID || uint64(a.org) != o.OrgID {
				s.observations[i].Invalid = true
				s.block("query_catalog_artifact_orphan_or_cross_owner")
			}
		}
		if o.Collection == "report_generations" || o.Collection == "interpretation_runs" {
			if id := o.GenerationID; id != 0 {
				if g, ok := s.graph.generations[id]; ok && o.OutcomeID == 0 {
					s.observations[i].OutcomeID = g.outcome
					s.byOutcome[mongoCycleKey(g.outcome)] = append(s.byOutcome[mongoCycleKey(g.outcome)], i)
				}
				if aid, ok := artifactByGeneration[id]; ok {
					a := s.graph.artifacts[aid]
					s.observations[i].AssessmentID, s.observations[i].OrgID = a.assessment, uint64(a.org)
					s.byAssessment[mongoCycleKey(a.assessment)] = append(s.byAssessment[mongoCycleKey(a.assessment)], i)
				} else {
					s.observations[i].OwnerUnproven = true
					s.gap("generation_run_sql_outcome_organization_reverse_binding_required")
				}
			}
		}
	}
	return nil
}

func mongoCycleProofMatches(p *evidence.EventEvidenceV1, reference evidence.StandardReference, binding string, exactBinding bool) error {
	if p == nil || p.Validate() != nil {
		return ErrMongoCycleSchema
	}
	if p.Class != evidence.StandardReferenceClass {
		return ErrMongoCycleSchema
	}
	if p.Reference == nil || *p.Reference != reference || exactBinding && (binding == "" || p.BusinessBindingSHA256 != binding) {
		return ErrMongoCycleConflict
	}
	return nil
}
