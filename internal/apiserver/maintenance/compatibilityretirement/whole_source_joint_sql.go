package retirement

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"sort"
	"strconv"

	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventevidencebinding"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	sheetmongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	interpretmongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	eventoutcome "github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/reliable-messaging/message"
	"go.mongodb.org/mongo-driver/bson"
)

// Reuse the original typed row decoder for the already captured owner ranges.
// A one-row decoder has no transaction or Complete capability and is never
// returned. It cannot prove any absent, unbound or global responsibility.
func historicalComponentSelectedMongoTerminal(indexes map[string]map[string]map[uint64][]bson.Raw) error {
	for _, name := range mongoBatchBusinessCollections {
		for _, rows := range indexes[name]["domain_id"] {
			if len(rows) != 1 {
				return ErrMongoOwnerConflict
			}
			s := &MongoResponsibilitySnapshot{limits: MongoResponsibilityLimits{MaxGraphEntries: 131072, MaxGraphBytes: 256 << 20}, byEvent: map[string][]int{}, bySheet: map[string][]int{}, byAssessment: map[string][]int{}, byGeneration: map[string][]int{}, byOutcome: map[string][]int{}, report: MongoResponsibilityCycleReport{ClassCounts: map[string]uint64{}}}
			s.graph.initialize()
			if s.classifyRow(name, rows[0]) != nil || len(s.observations) != 1 {
				return ErrMongoOwnerConflict
			}
			observed := s.observations[0]
			if observed.Invalid || observed.Unfinished || observed.LeasePresent {
				return ErrCoordinatorCASQualification
			}
		}
	}
	return nil
}

// SQL-source owners also have downstream Mongo messages and execution runs.
// The original organization/original-ID reader checks real native rows,
// fingerprint, reverse business ownership, and complete replay headers/items.
// Its row/byte/query bounds and this component's absolute deadline remain.
func historicalComponentSQLMongoResponsibilities(ctx context.Context, o *HistoricalComponentSourceObservation, rows []qualifiedCASRow, indexes map[string]map[string]map[uint64][]bson.Raw) error {
	views := map[string]*sqlevaluation.SQLHistoricalComponentSemanticView{}
	for i, observer := range o.sql {
		view, err := observer.SemanticView(ctx)
		if err != nil {
			return err
		}
		ids, err := o.component.inputs[i].sqlRecipe.SourceEventIDs()
		if err != nil {
			return err
		}
		for _, id := range ids {
			if views[id] == nil {
				views[id] = view
			}
		}
	}
	for _, row := range rows {
		source := row.facts
		if source.Source.Database != "mysql" {
			continue // the common business factory already checks Mongo originals
		}
		assessment, err := sqlSourceAssessment(source)
		view := views[source.EventID]
		if err != nil || view == nil {
			return ErrSQLMongoCrossStoreConflict
		}
		owner, err := view.OwnerByAssessment(ctx, assessment)
		if err != nil || owner.Owner.OrgID != source.OrgID {
			return ErrSQLMongoCrossStoreConflict
		}
		// The authenticated SQL source is immutable here. The native Mongo
		// responsibility reader only reads its identity and owner selectors;
		// it must not reinterpret it as a Mongo legacy source.
		r := &MongoOwnerResolution{db: o.mongo.db, source: source, businessReader: &historicalComponentMongoReader{o, source, indexes}, sqlFacts: &historicalComponentSQLOwnerReader{o, view, ctx, assessment, nil}}
		r.local = MongoLocalResolution{EventID: source.EventID, EventType: source.EventType, OrgID: owner.Owner.OrgID, TesteeID: owner.Owner.TesteeID, AssessmentID: assessment, AnswerSheetID: owner.Owner.AnswerSheetID}
		if len(owner.Outcomes) == 1 {
			r.local.OutcomeID = owner.Outcomes[0].ID
		}
		if err = r.readMongoResponsibilities(ctx); err != nil {
			return err
		}
		if len(r.local.BlockingReasons) != 0 {
			return ErrCoordinatorCASQualification
		}
		if _, collision := r.currentStandard[source.EventID]; collision {
			return ErrSQLMongoCrossStoreConflict // SQL event identity cannot move stores
		}
		if err = historicalComponentSQLMongoReferences(r, owner, indexes); err != nil {
			return err
		}
	}
	return o.ValidateBorrowedObservation(ctx)
}

func historicalComponentSQLMongoReferences(r *MongoOwnerResolution, owner sqlevaluation.SQLHistoricalFactsSnapshot, indexes map[string]map[string]map[uint64][]bson.Raw) error {
	for _, raw := range indexes["answersheets"]["domain_id"][owner.Owner.AnswerSheetID] {
		var sheet sheetmongo.AnswerSheetPO
		if bson.Unmarshal(raw, &sheet) != nil {
			return ErrMongoOwnerConflict
		}
		if sheet.DurableAcceptance != nil {
			if err := historicalComponentCurrentMongoReference(r, sheet.DurableAcceptance.EventID, "answersheet.submitted", sheet.DurableAcceptance.EventEvidence); err != nil {
				return err
			}
		}
	}
	for _, outcome := range owner.Outcomes {
		for _, raw := range indexes["report_generations"]["outcome_id"][outcome.ID] {
			var generation interpretmongo.ReportGenerationPO
			if bson.Unmarshal(raw, &generation) != nil {
				return ErrMongoOwnerConflict
			}
			if generation.TransactionSchemaVersion == 1 && generation.Status == "generated" {
				if err := historicalComponentCurrentMongoReference(r, generation.GeneratedEventID, "interpretation.report.generated", generation.GeneratedEventEvidence); err != nil {
					return err
				}
			}
			for _, runRaw := range indexes["interpretation_runs"]["generation_id"][uint64(generation.DomainID)] {
				var run interpretmongo.InterpretationRunPO
				if bson.Unmarshal(runRaw, &run) != nil {
					return ErrMongoOwnerConflict
				}
				if run.RetryEventID != "" {
					if err := historicalComponentCurrentMongoReference(r, run.RetryEventID, "interpretation.retry.requested", run.RetryEventEvidence); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func historicalComponentCurrentMongoReference(r *MongoOwnerResolution, id, eventType string, proof *evidence.EventEvidenceV1) error {
	row, found := r.currentStandard[id]
	if id == "" || !found || row.EventType != eventType {
		return ErrMongoOwnerConflict
	}
	if proof == nil {
		return nil // the native reverse reader still verified the actual full body
	}
	inner, err := row.envelope()
	if err != nil {
		return err
	}
	var binding string
	switch eventType {
	case "answersheet.submitted":
		var payload eventpayload.AnswerSheetSubmittedData
		if strictTyped(inner.Data, &payload) != nil {
			return ErrMongoOwnerConflict
		}
		binding, err = eventevidencebinding.AnswerSheet(payload)
	case "interpretation.report.generated":
		var payload eventoutcome.ReportGeneratedPayload
		if strictTyped(inner.Data, &payload) != nil {
			return ErrMongoOwnerConflict
		}
		binding, err = eventevidencebinding.Generated(payload)
	case "interpretation.retry.requested":
		var payload eventoutcome.InterpretationRetryRequestedPayload
		if strictTyped(inner.Data, &payload) != nil {
			return ErrMongoOwnerConflict
		}
		binding, err = eventevidencebinding.Retry(payload)
	default:
		return ErrSourceEventType
	}
	msg, messageErr := message.New(row.input())
	if err != nil || messageErr != nil || mongoCycleProofMatches(proof, standard.ReferenceFromMessage(msg), binding, true) != nil {
		return ErrMongoOwnerConflict
	}
	return nil
}

func (p *WholeSourceJointPage) originalMongoGraph(ctx context.Context, source *DecodedSourceEvent, key verifiedSourceKey, resolved map[string]sqlevaluation.SQLResponsibilityObservation) (*MongoHistoricalBatchOwnerQualification, error) {
	initial := p.cross.qualification[key]
	if initial == nil {
		return nil, ErrWholeSourceJoint
	}
	r := &MongoOwnerResolution{db: p.mongo.global.db, config: p.mongo.global.config, source: source, businessReader: p.mongo}
	if actual := p.mongo.sqlOwners[key]; actual != nil {
		r.sqlFacts = &sqlMongoResolvedOwnerReader{actual: actual, resolved: resolved}
	}
	r.local = MongoLocalResolution{EventID: source.EventID, EventType: source.EventType, OrgID: source.OrgID, SourceAuthenticationRequired: true, SQLCrossClosureRequired: true, SQLResponsibilityRequired: true, GlobalUnboundResponsibilityCoverageRequired: true}
	var err error
	if source.Submitted != nil {
		err = r.readSubmission(ctx)
	} else if source.Generated != nil {
		err = r.readGenerated(ctx)
	} else {
		return nil, ErrSourceEventType
	}
	if err != nil {
		return nil, err
	}
	for _, reason := range initial.responsibilities.BlockingReasons {
		r.block(reason)
	}
	for _, o := range initial.responsibilities.Observations {
		if o.Collection == "rm_outbox" {
			r.local.CurrentResponsibilityCount++
		}
	}
	for _, expected := range r.expectedStandard {
		actual, exists := p.mongo.global.graph.messages[expected.EventID]
		if !exists {
			r.block("mongo_live_standard_reference_absent")
		} else if actual.reference != expected {
			return nil, ErrMongoOwnerConflict
		}
	}
	if source.Submitted != nil && r.local.FrozenAdmissionPurpose == "independent_questionnaire" {
		if !p.mongo.sqlAbsent[key] {
			return nil, ErrMongoOwnerConflict
		}
		var gaps []string
		for _, gap := range r.local.Gaps {
			if gap != "independent_admission_sql_absence_and_global_responsibility_not_checked" {
				gaps = append(gaps, gap)
			}
		}
		r.local.Gaps = append(gaps, "independent_admission_unique_sql_absence_observed_external_coverage_required")
	}
	old := initial.Local()
	if r.local.BusinessBindingSHA256 != old.BusinessBindingSHA256 || !reflect.DeepEqual(r.local.OriginalRun, old.OriginalRun) {
		return nil, ErrSQLMongoCrossStoreConflict
	}
	return &MongoHistoricalBatchOwnerQualification{batch: p.mongo, local: r.Local(), responsibilities: initial.responsibilities}, nil
}

// Construct a simultaneous proof set only from individually exact original
// source+wire+physical row identities. Then rerun every original business graph
// with that set and monotonically REMOVE rows whose graph is not closed. A
// sibling graph failure never grants immunity to its own or another row.
func (p *WholeSourceJointPage) proveOriginalMongoOwners(ctx context.Context) error {
	tentative := map[string]sqlevaluation.SQLResponsibilityObservation{}
	rowSources := map[string]verifiedSourceKey{}
	rowsByKey := map[string]sqlevaluation.SQLCrossStoreRow{}
	for _, handle := range p.cross.sources {
		source, key, err := p.mongo.source(handle)
		if err != nil {
			return err
		}
		local := p.cross.qualification[key].Local()
		rows, err := p.cross.page.Lookup(source.EventID, local.AssessmentID, source.OrgID)
		if err != nil {
			return err
		}
		actual := p.mongo.sqlOwners[key]
		if actual == nil {
			continue
		}
		for _, row := range rows {
			o := row.Observation
			if (o.Store != "retry_event_hold" && o.Store != "event_delivery_dead_letter") || o.EventID != source.EventID || o.EventType != source.EventType || !o.OwnerUnproven || o.Invalid || o.Unfinished || o.LeasePresent || o.ScopeClass != "retirement_related" || len(o.Reasons) != 0 {
				continue
			}
			var view SQLMongoCrossStoreResponsibilityView
			if err = p.cross.verifyMongoMessage(&view, source, local, row); err != nil {
				return err
			}
			if len(view.BlockingReasons) != 0 || len(view.OwnerBoundObservationKeys) != 1 {
				return ErrSQLMongoCrossStoreConflict
			}
			observationKey := o.Store + ":" + o.PrimaryKeySHA256
			if prior, exists := rowsByKey[observationKey]; exists && !reflect.DeepEqual(prior, row) {
				return ErrSQLMongoCrossStoreConflict
			}
			// The original SQL decoder's Invalid flag is retained. This is
			// only its specifically declared Mongo owner-unproven cause.
			owner := actual.Snapshot().Owner
			if owner.AssessmentID != local.AssessmentID || owner.OrgID != source.OrgID || owner.TesteeID != local.TesteeID || source.Submitted != nil && !actual.HasVerifiedAnswerSheetAssociation(local.AnswerSheetID) {
				return ErrSQLMongoCrossStoreConflict
			}
			for _, current := range actual.Snapshot().Responsibilities {
				if current.Store == o.Store && current.ID == o.PrimaryKeySHA256 && !sqlMongoProvisionalMatches(current, o) {
					return ErrSQLMongoCrossStoreConflict
				}
			}
			// A Submitted wire has no Assessment ID, so its own full cycle
			// observation need not be in ForAssessment. Its real unique
			// AnswerSheet association above provides that missing ownership.
			// The original full observation key is still retained; absence
			// from the compact owner snapshot is not used as closure proof.
			tentative[observationKey] = o
			rowSources[observationKey] = key
			rowsByKey[observationKey] = row
		}
	}
	for {
		removed := false
		qualified := map[verifiedSourceKey]*MongoHistoricalBatchOwnerQualification{}
		for _, handle := range p.cross.sources {
			source, key, err := p.mongo.source(handle)
			if err != nil {
				return err
			}
			q, err := p.originalMongoGraph(ctx, source, key, tentative)
			if err != nil {
				return err
			}
			qualified[key] = q
			if !q.Local().OwnerLocalTerminal || len(q.Local().BlockingReasons) != 0 {
				for observationKey, owner := range rowSources {
					if owner == key {
						if _, exists := tentative[observationKey]; exists {
							delete(tentative, observationKey)
							removed = true
						}
					}
				}
			}
		}
		if removed {
			continue
		}
		p.cross.qualification = qualified
		break
	}
	p.resolved = tentative
	keys := make([]string, 0, len(tentative))
	for key := range tentative {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		row := rowsByKey[key]
		sourceKey := rowSources[key]
		q := p.cross.qualification[sourceKey]
		if q == nil || !q.Local().OwnerLocalTerminal || len(q.Local().BlockingReasons) != 0 {
			return ErrWholeSourceJoint
		}
		source := p.mongo.sources[sourceKey]
		entry := p.index.entries[source.EventID]
		p.bindings = append(p.bindings, WholeSourceJointObservationBinding{ObservationKey: key, OriginalObservationRowSHA256: row.Observation.RowSHA256, OriginalID: source.EventID, EventType: source.EventType, OrganizationID: strconv.FormatUint(source.OrgID, 10), SourcePrimaryKeySHA256: source.Source.PrimaryKeySHA256, SourceRowSHA256: source.Source.Digest.SHA256, SourceFactsSHA256: hex.EncodeToString(entry.FactsSHA256[:]), LegacyContentSHA256: source.ContentDigest.SHA256, InnerWireDataSHA256: row.InnerDataSHA256, SDKFingerprintSHA256: row.Observation.SDKFingerprintSHA256, BusinessBindingSHA256: q.Local().BusinessBindingSHA256, ActualOriginalRun: q.Local().OriginalRun})
	}
	return nil
}

func wholeSourceJointBindingsHash(bindings []WholeSourceJointObservationBinding) (string, error) {
	h := sha256.New()
	sourceFrame(h, []byte("whole-source-joint-observation-bindings/v1"), false)
	for _, binding := range bindings {
		row, err := privateFactsSHA(binding)
		if err != nil {
			return "", err
		}
		sourceFrame(h, row[:], false)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (p *WholeSourceJointPage) resolveSQL(ctx context.Context, source *DecodedSourceEvent) (SQLLocalResolution, error) {
	id, err := sqlSourceAssessment(source)
	if err != nil {
		return SQLLocalResolution{}, err
	}
	actual, err := p.sql.OwnerByAssessment(id)
	if err != nil {
		return SQLLocalResolution{}, err
	}
	reader := &sqlMongoResolvedOwnerReader{actual: actual, resolved: p.resolved}
	local, err := resolveSQLLocalFacts(source, reader.Snapshot())
	if err != nil {
		return local, err
	}
	seen := map[string]bool{}
	var rows []sqlevaluation.SQLResponsibilityObservation
	add := func(in []sqlevaluation.SQLResponsibilityObservation) {
		for _, o := range in {
			key := o.Store + ":" + o.PrimaryKeySHA256
			if !seen[key] {
				seen[key] = true
				rows = append(rows, o)
			}
		}
	}
	add(p.sql.responsibility.cycle.ForAssessment(id))
	add(p.sql.responsibility.cycle.ForEvent(source.EventID))
	add(p.sql.responsibility.cycle.ForOrganizationActions(source.OrgID))
	local.CurrentResponsibilityCount = len(rows)
	for _, o := range rows {
		key := o.Store + ":" + o.PrimaryKeySHA256
		if sqlResponsibilitySourceCollision(o, source) {
			local.BlockingReasons = append(local.BlockingReasons, "current_event_reuses_original_source_identity")
		}
		if o.Invalid || o.ScopeClass == "retirement_related" && (o.Unfinished || o.LeasePresent) || o.OrgID != source.OrgID || o.AssessmentID != 0 && o.AssessmentID != id {
			local.BlockingReasons = append(local.BlockingReasons, "current_sql_responsibility_unclosed_or_conflicting")
		}
		proved, exists := p.resolved[key]
		if exists && !reflect.DeepEqual(proved, o) {
			return local, ErrSQLMongoCrossStoreConflict
		}
		if (o.OwnerUnproven && !exists) || o.ScopeClass == "coordination_required" {
			local.BlockingReasons = append(local.BlockingReasons, "external_owner_or_responsibility_coverage_required")
		}
	}
	physical, err := p.cross.page.Lookup(source.EventID, id, source.OrgID)
	if err != nil {
		return local, err
	}
	for _, row := range physical {
		if row.Observation.EventID == source.EventID && row.Inner != nil {
			if err = p.verifyOriginalSQLWire(source, row); err != nil {
				return local, err
			}
		}
	}
	if err = p.validateRelatedSourceRows(ctx, physical, source.OrgID, id); err != nil {
		return local, err
	}
	local.BlockingReasons = uniqueMongoCycleStrings(local.BlockingReasons)
	local.OwnerLocalTerminal = local.OwnerLocalTerminal && len(local.BlockingReasons) == 0
	return local, nil
}

func (p *WholeSourceJointPage) verifyOriginalSQLWire(source *DecodedSourceEvent, row sqlevaluation.SQLCrossStoreRow) error {
	return verifyOriginalSQLWire(source, row)
}

// Shared exact source/wire rule; this grants no current or global closure.
func verifyOriginalSQLWire(source *DecodedSourceEvent, row sqlevaluation.SQLCrossStoreRow) error {
	o := row.Observation
	inner := row.Inner
	if inner == nil || inner.ID != source.EventID || inner.EventType != source.EventType || inner.AggregateType != source.AggregateType || inner.AggregateID != source.AggregateID || !inner.OccurredAt.Equal(source.OccurredAt) || o.OrgID != source.OrgID || row.LegacyContentSHA256 != source.ContentDigest.SHA256 {
		return ErrSQLMongoCrossStoreConflict
	}
	switch source.EventType {
	case "evaluation.requested", "evaluation.retry.requested":
		var body eventpayload.EvaluationRequestedData
		if strictTyped(inner.Data, &body) != nil || !reflect.DeepEqual(&body, source.Requested) {
			return ErrSQLMongoCrossStoreConflict
		}
	case "evaluation.failed":
		var body eventpayload.EvaluationFailedData
		if strictTyped(inner.Data, &body) != nil || !reflect.DeepEqual(&body, source.Failed) {
			return ErrSQLMongoCrossStoreConflict
		}
	case "evaluation.outcome.committed":
		var body eventpayload.EvaluationOutcomeCommittedData
		if strictTyped(inner.Data, &body) != nil || !reflect.DeepEqual(&body, source.OutcomeCommitted) {
			return ErrSQLMongoCrossStoreConflict
		}
	default:
		return ErrSourceEventType
	}
	return nil
}

func (p *WholeSourceJointPage) validateRelatedSourceRows(ctx context.Context, rows []sqlevaluation.SQLCrossStoreRow, org, assessment uint64) error {
	for _, row := range rows {
		o := row.Observation
		entry, exists := p.index.entries[o.EventID]
		if !exists {
			continue
		} // no assertion that absent source means closed
		if entry.OrgID != org || entry.EventType != o.EventType {
			return ErrSQLMongoCrossStoreConflict
		}
		if row.Inner == nil {
			continue
		} // replay remains its own fingerprint gate
		if entry.Key.object == 0 {
			handle, err := p.index.event(ctx, o.EventID)
			if err != nil {
				return err
			}
			source, err := handle.Facts()
			if err != nil {
				return err
			}
			if entry.AssessmentID != assessment {
				return ErrSQLMongoCrossStoreConflict
			}
			if err = p.verifyOriginalSQLWire(source, row); err != nil {
				return err
			}
		}
	}
	return nil
}

// Every different old Mongo event is verified against ITS OWN complete source
// and original graph. It is never made equivalent to the current event or to
// the absent SDK row. Current SDK references and replay claims stay separate.
func (p *WholeSourceJointPage) jointMongoSQLView(ctx context.Context, source *DecodedSourceEvent, local MongoLocalResolution) (SQLMongoCrossStoreResponsibilityView, error) {
	v := SQLMongoCrossStoreResponsibilityView{EventID: source.EventID, EventType: source.EventType, SourceRowSHA256: source.Source.Digest.SHA256, LegacyContentSHA256: source.ContentDigest.SHA256, BusinessBindingSHA256: local.BusinessBindingSHA256, OrganizationID: source.OrgID, AssessmentID: local.AssessmentID, SourceCopyFactsBound: true, CurrentSQLCoverageObserved: true, ExternalOriginAuthenticationRequired: true, GlobalFreshRequired: true, AIInboxCoverageRequired: true, WriterFenceRequired: true, CASRequired: true, GlobalUnboundSQLCoverageRequired: true}
	rows, err := p.cross.page.Lookup(source.EventID, local.AssessmentID, source.OrgID)
	if err != nil {
		return v, err
	}
	if err = p.validateRelatedSourceRows(ctx, rows, source.OrgID, local.AssessmentID); err != nil {
		return v, err
	}
	for _, row := range rows {
		o := row.Observation
		key := o.Store + ":" + o.PrimaryKeySHA256
		if o.Invalid || o.OrgID != source.OrgID || o.AssessmentID != 0 && o.AssessmentID != local.AssessmentID {
			return v, ErrSQLMongoCrossStoreConflict
		}
		switch o.Store {
		case "rm_outbox", "retry_event_hold", "event_delivery_dead_letter":
			if o.EventType == "answersheet.submitted" || o.EventType == "interpretation.report.generated" {
				entry, found := p.index.entries[o.EventID]
				q := p.cross.qualification[entry.Key]
				if !found || entry.Key.object != 3 || q == nil || p.mongo.sources[entry.Key] == nil {
					v.BlockingReasons = append(v.BlockingReasons, "whole_source_related_original_mongo_source_or_graph_not_covered")
				} else {
					own := p.mongo.sources[entry.Key]
					ownLocal := q.Local()
					if ownLocal.OrgID != local.OrgID || ownLocal.TesteeID != local.TesteeID || ownLocal.AssessmentID != local.AssessmentID {
						return v, ErrSQLMongoCrossStoreConflict
					}
					if err = p.cross.verifyMongoMessage(&v, own, ownLocal, row); err != nil {
						return v, err
					}
					if !ownLocal.OwnerLocalTerminal || len(ownLocal.BlockingReasons) != 0 {
						v.BlockingReasons = append(v.BlockingReasons, "whole_source_related_original_business_unclosed")
					}
					if o.OwnerUnproven {
						if proved, ok := p.resolved[key]; !ok || !reflect.DeepEqual(proved, o) {
							v.BlockingReasons = append(v.BlockingReasons, "whole_source_original_mongo_owner_still_unproven")
						}
					}
				}
			} else if o.OwnerUnproven || o.AssessmentID == 0 || o.ScopeClass == "coordination_required" {
				v.BlockingReasons = append(v.BlockingReasons, "cross_store_related_sql_owner_unproven")
			}
			if o.Unfinished || o.LeasePresent {
				v.BlockingReasons = append(v.BlockingReasons, "cross_store_sql_message_responsibility_unclosed")
			}
		case "qs_rm_replay_items":
			if err = p.cross.verifyReplay(&v, source, local, row); err != nil {
				return v, err
			}
		case "system_governance_action_runs":
			if o.ScopeClass != "scope_outside_retirement" && (o.Unfinished || o.OwnerUnproven || o.ScopeClass == "coordination_required") {
				v.BlockingReasons = append(v.BlockingReasons, "cross_store_governance_responsibility_unclosed_or_unknown")
			}
		case "qs_rm_evaluation_request_ref", "qs_rm_gap_recovery_request":
			if o.EventID == source.EventID {
				return v, ErrSQLMongoCrossStoreConflict
			}
			if o.Unfinished || o.LeasePresent || o.OwnerUnproven {
				v.BlockingReasons = append(v.BlockingReasons, "cross_store_sql_owner_reference_unclosed_or_unknown")
			}
		case "qs_rm_replay_requests":
			v.BlockingReasons = append(v.BlockingReasons, "cross_store_replay_header_without_original_item")
		default:
			return v, ErrSQLMongoCrossStoreConflict
		}
		v.Observations = append(v.Observations, row)
	}
	v.BlockingReasons = uniqueMongoCycleStrings(v.BlockingReasons)
	return v, nil
}
