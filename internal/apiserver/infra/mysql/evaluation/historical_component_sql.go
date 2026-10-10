package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

var ErrSQLHistoricalComponent = errors.New("sql_historical_component_rejected")

// A recipe is a copy of actual original reads, never a renewed qualification.
// It can only be produced while the original native owner/page is still alive.
type SQLHistoricalComponentRecipe struct {
	self           *SQLHistoricalComponentRecipe
	plan           *SQLHistoricalBatchCASPlan
	selectors      SQLCrossStoreSelectors
	responsibility sqlHistoricalCASImage
	anchors        map[string]string
	limits         SQLCrossStoreLimits
	oldPool        gorm.ConnPool
	seal           string
	input          *SQLHistoricalCASFrozenInput
}

// The host owns every transaction and its commit. This observer grants only a
// short-lived SQL component read, not source, Mongo, AI or retirement authority.
type SQLHistoricalComponentObservation struct {
	self        *SQLHistoricalComponentObservation
	recipe      *SQLHistoricalComponentRecipe
	pool        gorm.ConnPool
	transaction sqlResponsibilityTransaction
	expires     time.Time
	writable    bool
	used        bool
	seal        string
}

type SQLHistoricalComponentStatement struct {
	self        *SQLHistoricalComponentStatement
	observation *SQLHistoricalComponentObservation
	statement   *SQLHistoricalBatchCASStatement
	seal        string
}

type SQLHistoricalComponentReadReport struct {
	BusinessMatched, ResponsibilitiesMatched, IndependentPersistedReadMatched              bool
	FullSourcesRequired, MongoQualificationRequired, AIClosureRequired, HostCommitRequired bool
	HostCommitVerified, WholeRetirementComplete, DropReady                                 bool
}

func (*SQLHistoricalComponentRecipe) MarshalJSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalComponentRecipe) MarshalBSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalComponentRecipe) UnmarshalJSON([]byte) error {
	return ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalComponentRecipe) UnmarshalBSON([]byte) error {
	return ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalComponentObservation) MarshalJSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalComponentObservation) MarshalBSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalComponentStatement) MarshalJSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalComponentStatement) MarshalBSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}

func componentCopySelectors(v SQLCrossStoreSelectors) SQLCrossStoreSelectors {
	return SQLCrossStoreSelectors{EventIDs: slices.Clone(v.EventIDs), AssessmentIDs: slices.Clone(v.AssessmentIDs), OrganizationIDs: slices.Clone(v.OrganizationIDs), MongoOwners: slices.Clone(v.MongoOwners)}
}

func (r *SQLHistoricalComponentRecipe) digest() string {
	if r == nil || r.plan == nil || r.input == nil || r.input.self != r.input || r.input.seal == "" || r.input.seal != r.input.digest() {
		return ""
	}
	parts := []string{"sql-historical-component-input/v1", r.input.seal, r.plan.identity, casImageHash(r.plan.before), casImageHash(r.responsibility), sqlHistoricalProvenancePoolToken(r.oldPool)}
	parts = append(parts, r.selectors.EventIDs...)
	for _, id := range r.selectors.AssessmentIDs {
		parts = append(parts, "assessment:"+strconv.FormatUint(id, 10))
	}
	for _, id := range r.selectors.OrganizationIDs {
		parts = append(parts, "organization:"+strconv.FormatUint(id, 10))
	}
	for _, owner := range r.selectors.MongoOwners {
		parts = append(parts, owner.Kind+":"+owner.ID)
	}
	keys := make([]string, 0, len(r.anchors))
	for key := range r.anchors {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts = append(parts, key, r.anchors[key])
	}
	// This private codec also freezes attachments, groups, negative selectors,
	// original native transaction and limits. No exported import is provided.
	record := componentPlanRecord(r.plan)
	raw, err := json.Marshal(record)
	if err != nil {
		return ""
	}
	parts = append(parts, sqlSpoolSHA(raw), strconv.Itoa(r.limits.MaxPageRows), strconv.FormatUint(r.limits.MaxPageBytes, 10), r.limits.MaxDuration.String())
	return cycleKeyDigest(parts)
}
func (r *SQLHistoricalComponentRecipe) intact() bool {
	return r != nil && r.self == r && r.seal != "" && r.seal == r.digest()
}

// Input hashes/dependencies describe frozen reads and writes, never permission.
func (r *SQLHistoricalComponentRecipe) InputSHA256() (string, error) {
	if !r.intact() {
		return "", ErrSQLHistoricalComponent
	}
	return r.seal, nil
}
func (r *SQLHistoricalComponentRecipe) RowDependencies(visit func(string, uint64, string, uint64, bool) error) error {
	if !r.intact() {
		return ErrSQLHistoricalComponent
	}
	return r.input.RowDependencies(visit)
}

func componentPlanRecord(p *SQLHistoricalBatchCASPlan) sqlSpoolPlan {
	r := sqlSpoolPlan{Version: 1, Identity: p.identity, Server: p.server, Database: p.database, Old: [3]uint64{p.oldTransaction.connection, p.oldTransaction.thread, p.oldTransaction.event}, Request: p.request, Limits: p.limits, Before: sqlSpoolImageOut(p.before), Attachments: p.attachments, Missing: p.missingOriginalRunIDs}
	for _, g := range p.groups {
		r.Groups = append(r.Groups, sqlSpoolGroup{g.table, g.column, g.id, g.entries, g.set})
	}
	return r
}

func FreezeSQLHistoricalComponentRecipe(ctx context.Context, original *SQLHistoricalOwnerBatch, cross *SQLHistoricalCrossStorePage, provenance *SQLHistoricalCASProvenance, unchanged *SQLHistoricalCASReadBaseline) (*SQLHistoricalComponentRecipe, error) {
	if ctx == nil || ctx.Err() != nil || original == nil || cross == nil || cross.batch != original || !cross.report.Complete || cross.catalog == nil || cross.catalog.cycle != original.cycle || len(cross.selectors.EventIDs) == 0 || (provenance == nil) == (unchanged == nil) || original.ValidateBorrowedSnapshot(ctx) != nil {
		return nil, ErrSQLHistoricalComponent
	}
	input, err := FreezeSQLHistoricalCASInput(ctx, original, provenance, unchanged)
	if err != nil {
		return nil, err
	}
	tx, err := historicalTx(ctx)
	if err != nil {
		return nil, err
	}
	server, database, err := historicalDatabase(tx)
	if err != nil {
		return nil, err
	}
	var plan *SQLHistoricalBatchCASPlan
	if provenance != nil {
		plan = provenance.plan
	} else {
		plan = &SQLHistoricalBatchCASPlan{identity: unchanged.identity, server: server, database: database, oldTransaction: original.cycle.transaction, request: original.request, limits: original.limits, before: unchanged.before}
		for _, id := range original.originalOutcomeRunAbsence {
			plan.missingOriginalRunIDs = append(plan.missingOriginalRunIDs, id)
		}
		sort.Strings(plan.missingOriginalRunIDs)
		plan.missingOriginalRunIDs = slices.Compact(plan.missingOriginalRunIDs)
	}
	// Deep-copy through the existing private NULL/binary-preserving codec.
	raw, err := sqlSpoolEncode(componentPlanRecord(plan))
	if err != nil {
		return nil, err
	}
	var record sqlSpoolPlan
	if err = sqlSpoolDecode(raw, &record); err != nil {
		return nil, err
	}
	plan, err = sqlSpoolReconstruct(record, sqlSpoolImageIn(record.Before))
	if err != nil {
		return nil, err
	}
	r := &SQLHistoricalComponentRecipe{plan: plan, selectors: componentCopySelectors(cross.selectors), limits: cross.catalog.limits, oldPool: tx.Statement.ConnPool, input: input}
	r.responsibility, r.anchors, err = r.captureResponsibility(tx)
	if err != nil {
		return nil, err
	}
	// Every expanded row must also have been observed in the same genuine full
	// original cycle. The factory cannot smuggle in guessed/expected raw rows.
	known := map[string]string{}
	for _, v := range cross.catalog.cycle.observations {
		known[v.Store+":"+v.PrimaryKeySHA256] = v.RowSHA256
	}
	for _, spec := range sqlResponsibilityTables {
		for _, row := range r.responsibility.rows[spec.name] {
			key, e := cycleKey(spec, row)
			if e != nil || known[spec.name+":"+cycleKeyDigest(key)] != cycleRowDigest(r.responsibility.columns[spec.name], row) {
				return nil, ErrSQLHistoricalComponent
			}
		}
	}
	if original.ValidateBorrowedSnapshot(ctx) != nil {
		return nil, ErrSQLHistoricalComponent
	}
	r.self = r
	r.seal = r.digest()
	if !r.intact() {
		return nil, ErrSQLHistoricalComponent
	}
	return r, nil
}

// These expressions are selectors only. Exact original bytes are subsequently
// decoded with the real domain/legacy readers; invalid JSON is never discarded.
func componentMessageDocument(column string) (string, string) {
	// Native message bodies are binary columns. JSON_EXTRACT refuses MySQL's
	// binary character set; conversion is used only for selectors, while every
	// retained body and its fingerprint still use the exact original bytes.
	text := "CONVERT(`" + column + "` USING utf8mb4)"
	outer := "(CASE WHEN JSON_VALID(" + text + ") THEN " + text + " ELSE '{}' END)"
	inner := "(CASE WHEN JSON_UNQUOTE(JSON_EXTRACT(" + outer + ",'$.type'))='component-base.messaging.message.v1' THEN CONVERT(FROM_BASE64(JSON_UNQUOTE(JSON_EXTRACT(" + outer + ",'$.payload'))) USING utf8mb4) ELSE " + text + " END)"
	safe := "(CASE WHEN JSON_VALID(" + inner + ") THEN " + inner + " ELSE '{}' END)"
	return inner, safe
}

func componentPredicate(spec sqlResponsibilityTable, s SQLCrossStoreSelectors, events []string) (string, []any, error) {
	terms := []string{}
	args := []any{}
	add := func(q string, v any) { terms = append(terms, q); args = append(args, v) }
	switch spec.name {
	case "rm_outbox", "retry_event_hold", "event_delivery_dead_letter":
		column, identity := "payload_json", "event_id"
		if spec.name == "rm_outbox" {
			column, identity = "payload", "message_id"
		}
		inner, doc := componentMessageDocument(column)
		terms = append(terms, "COALESCE(JSON_VALID("+inner+"),0)=0")
		add("CAST(`"+identity+"` AS BINARY) IN ?", events)
		add("CAST(JSON_UNQUOTE(JSON_EXTRACT("+doc+",'$.id')) AS BINARY) IN ?", events)
		if len(s.AssessmentIDs) > 0 {
			ids := make([]string, len(s.AssessmentIDs))
			for i, id := range s.AssessmentIDs {
				ids[i] = strconv.FormatUint(id, 10)
			}
			add("CAST(JSON_UNQUOTE(JSON_EXTRACT("+doc+",'$.data.assessment_id')) AS BINARY) IN ?", ids)
			add("(JSON_UNQUOTE(JSON_EXTRACT("+doc+",'$.aggregateType'))='Evaluation' AND CAST(JSON_UNQUOTE(JSON_EXTRACT("+doc+",'$.aggregateID')) AS BINARY) IN ?)", ids)
		}
		for _, o := range s.MongoOwners {
			terms = append(terms, "(CAST(JSON_UNQUOTE(JSON_EXTRACT("+doc+",'$.aggregateType')) AS BINARY)=CAST(? AS BINARY) AND CAST(JSON_UNQUOTE(JSON_EXTRACT("+doc+",'$.aggregateID')) AS BINARY)=CAST(? AS BINARY))")
			args = append(args, o.Kind, o.ID)
		}
	case "qs_rm_evaluation_request_ref", "qs_rm_gap_recovery_request":
		add("CAST(event_id AS BINARY) IN ?", events)
		if len(s.AssessmentIDs) > 0 {
			add("assessment_id IN ?", s.AssessmentIDs)
		}
	case "qs_rm_replay_items":
		add("CAST(event_id AS BINARY) IN ?", events)
	case "qs_rm_replay_requests":
		// An orphan organization request cannot be hidden merely because no
		// event-bearing item yet exists. Complete known pairs are added below.
		if len(s.OrganizationIDs) == 0 {
			return "0=1", nil, nil
		}
		return "(org_id IN ? AND NOT EXISTS (SELECT 1 FROM qs_rm_replay_items component_item WHERE component_item.org_id=qs_rm_replay_requests.org_id AND component_item.request_id=qs_rm_replay_requests.request_id))", []any{s.OrganizationIDs}, nil
	case "system_governance_action_runs":
		if len(s.OrganizationIDs) > 0 {
			add("org_id IN ?", s.OrganizationIDs)
		} else {
			return "0=1", nil, nil
		}
	default:
		return "", nil, ErrSQLHistoricalComponent
	}
	return "(" + strings.Join(terms, " OR ") + ")", args, nil
}

// Bounded fixed-point closure includes every member of a replay pair and then
// every event referenced by those members. It reads new matching rows, rather
// than re-reading only the old primary keys. A growing/oversized graph fails.
func (r *SQLHistoricalComponentRecipe) captureResponsibility(tx *gorm.DB) (sqlHistoricalCASImage, map[string]string, error) {
	image := sqlHistoricalCASImage{rows: map[string][]historicalSQLRow{}, schema: map[string]string{}, columns: map[string][]string{}}
	started := time.Now()
	events := map[string]bool{}
	for _, id := range r.selectors.EventIDs {
		events[id] = true
	}
	pairs := map[string]SQLResponsibilityObservation{}
	retained := map[string]map[string]historicalSQLRow{}
	var total int
	var bytes uint64
	for _, spec := range sqlResponsibilityTables {
		cols, hash, _, err := cycleSchema(tx, spec)
		if err != nil || !sqlCrossStoreSupportedColumns(spec.name, cols) {
			return image, nil, ErrSQLHistoricalComponent
		}
		image.schema[spec.name], image.columns[spec.name] = hash, cols
		retained[spec.name] = map[string]historicalSQLRow{}
	}
	for round := 0; round <= 512; round++ {
		if time.Since(started) > r.limits.MaxDuration {
			return image, nil, ErrSQLHistoricalComponent
		}
		beforeEvents, beforePairs := len(events), len(pairs)
		ids := make([]string, 0, len(events))
		for id := range events {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		if len(ids) == 0 || len(ids) > 512 || len(pairs) > 512 {
			return image, nil, ErrSQLHistoricalComponent
		}
		for _, spec := range sqlResponsibilityTables {
			predicate, args, err := componentPredicate(spec, r.selectors, ids)
			if err != nil {
				return image, nil, err
			}
			if spec.name == "qs_rm_replay_items" || spec.name == "qs_rm_replay_requests" {
				keys := make([]string, 0, len(pairs))
				for key := range pairs {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				for _, key := range keys {
					v := pairs[key]
					predicate += " OR (org_id=? AND CAST(request_id AS BINARY)=CAST(? AS BINARY))"
					args = append(args, v.OrgID, v.link.requestID)
				}
			}
			rows, cols, size, err := cycleQuery(tx, "SELECT * FROM `"+spec.name+"` WHERE "+predicate+" ORDER BY "+strings.Join(spec.keys, ",")+" LIMIT ?", r.limits.MaxPageRows, append(args, r.limits.MaxPageRows+1)...)
			if err != nil {
				return image, nil, err
			}
			if !reflect.DeepEqual(cols, image.columns[spec.name]) || size > r.limits.MaxPageBytes {
				return image, nil, ErrSQLHistoricalComponent
			}
			for _, row := range rows {
				key, err := cycleKey(spec, row)
				if err != nil {
					return image, nil, err
				}
				digest := cycleKeyDigest(key)
				if previous := retained[spec.name][digest]; previous != nil {
					if !reflect.DeepEqual(previous, row) {
						return image, nil, ErrSQLHistoricalComponent
					}
					continue
				}
				total++
				bytes += uint64(len(cycleRowDigest(cols, row)))
				for _, cell := range row {
					if cell != nil {
						bytes += uint64(len(*cell))
					}
				}
				if total > r.limits.MaxPageRows || bytes > r.limits.MaxPageBytes {
					return image, nil, ErrSQLHistoricalComponent
				}
				retained[spec.name][digest] = row
				v := cycleDecode(spec.name, row)
				if v.EventID != "" {
					events[v.EventID] = true
				}
				if spec.name == "qs_rm_replay_items" || spec.name == "qs_rm_replay_requests" {
					if v.OrgID == 0 || v.link.requestID == "" {
						return image, nil, ErrSQLHistoricalComponent
					}
					pairs[cyclePair(v.OrgID, v.link.requestID)] = v
				}
			}
		}
		if len(events) == beforeEvents && len(pairs) == beforePairs {
			break
		}
		if round == 512 {
			return image, nil, ErrSQLHistoricalComponent
		}
	}
	c := &SQLHistoricalResponsibilityCycle{owners: map[uint64]sqlResponsibilityOwner{}, anchorDigests: map[string]string{}, byOwner: map[uint64][]int{}, byEvent: map[string][]int{}, byOrgActions: map[uint64][]int{}}
	for _, spec := range sqlResponsibilityTables {
		for _, row := range retained[spec.name] {
			image.rows[spec.name] = append(image.rows[spec.name], row)
		}
		if image.rows[spec.name] == nil {
			image.rows[spec.name] = []historicalSQLRow{}
		}
		sort.Slice(image.rows[spec.name], func(i, j int) bool {
			a, _ := cycleKey(spec, image.rows[spec.name][i])
			b, _ := cycleKey(spec, image.rows[spec.name][j])
			return cycleCompare(spec, a, b) < 0
		})
		for _, row := range image.rows[spec.name] {
			c.observations = append(c.observations, cycleDecode(spec.name, row))
		}
		_, hash, _, err := cycleSchema(tx, spec)
		if err != nil || hash != image.schema[spec.name] {
			return image, nil, ErrSQLHistoricalComponent
		}
	}
	if err := c.checkPageOwners(tx, 0); err != nil {
		return image, nil, err
	}
	c.checkReverse()
	for _, v := range c.observations {
		if v.Invalid || v.ScopeClass == "retirement_related" && (v.Unfinished || v.LeasePresent) {
			return image, nil, ErrSQLHistoricalComponent
		}
	}
	return image, c.anchorDigests, nil
}

func (o *SQLHistoricalComponentObservation) digest() string {
	if o == nil || !o.recipe.intact() {
		return ""
	}
	return cycleKeyDigest([]string{o.recipe.seal, sqlHistoricalProvenancePoolToken(o.pool), strconv.FormatUint(o.transaction.connection, 10), strconv.FormatUint(o.transaction.thread, 10), strconv.FormatUint(o.transaction.event, 10), o.expires.UTC().Format(time.RFC3339Nano), strconv.FormatBool(o.writable)})
}
func (o *SQLHistoricalComponentObservation) live(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil || o == nil || o.self != o || o.seal == "" || o.seal != o.digest() || time.Now().After(o.expires) {
		return ErrSQLHistoricalComponent
	}
	tx, err := historicalTx(ctx)
	if err != nil || tx.Statement.ConnPool != o.pool {
		return ErrSQLHistoricalComponent
	}
	var actual sqlResponsibilityTransaction
	if o.writable {
		actual, err = casActualRW(tx)
	} else {
		actual, err = cycleActualTransaction(tx)
	}
	if err != nil || actual != o.transaction {
		return ErrSQLHistoricalComponent
	}
	return nil
}

// writable selects native RW instrumentation; it is not a permission boolean.
// Both modes perform the same genuine fresh full component reads before return.
func PrepareSQLHistoricalComponentObservation(ctx context.Context, r *SQLHistoricalComponentRecipe, budget time.Duration, writable bool) (*SQLHistoricalComponentObservation, error) {
	if ctx == nil || ctx.Err() != nil || !r.intact() || budget <= 0 || budget > 20*time.Second {
		return nil, ErrSQLHistoricalComponent
	}
	started := time.Now()
	bounded, cancel := context.WithDeadline(ctx, started.Add(budget))
	defer cancel()
	tx, err := historicalTx(bounded)
	if err != nil || tx.Statement.ConnPool == r.oldPool {
		return nil, ErrSQLHistoricalComponent
	}
	server, database, err := historicalDatabase(tx)
	if err != nil || server != r.plan.server || database != r.plan.database {
		return nil, ErrSQLHistoricalComponent
	}
	image, err := r.plan.capture(tx, writable)
	if err != nil || !reflect.DeepEqual(image, r.plan.before) {
		return nil, fmt.Errorf("%w: business_read_or_baseline", ErrSQLHistoricalComponent)
	}
	var actual sqlResponsibilityTransaction
	if writable {
		actual, err = casActualRW(tx)
	} else {
		actual, err = cycleActualTransaction(tx)
	}
	if err != nil || actual == r.plan.oldTransaction {
		return nil, ErrSQLHistoricalComponent
	}
	responsibility, anchors, err := r.captureResponsibility(tx)
	if err != nil {
		return nil, fmt.Errorf("%w: responsibility_read", ErrSQLHistoricalComponent)
	}
	if !reflect.DeepEqual(responsibility, r.responsibility) {
		return nil, fmt.Errorf("%w: responsibility_baseline", ErrSQLHistoricalComponent)
	}
	if !reflect.DeepEqual(anchors, r.anchors) {
		return nil, fmt.Errorf("%w: responsibility_anchors", ErrSQLHistoricalComponent)
	}
	o := &SQLHistoricalComponentObservation{recipe: r, pool: tx.Statement.ConnPool, transaction: actual, expires: started.Add(budget), writable: writable}
	o.self = o
	o.seal = o.digest()
	if o.live(bounded) != nil {
		return nil, ErrSQLHistoricalComponent
	}
	return o, nil
}

func (o *SQLHistoricalComponentObservation) Report() SQLHistoricalComponentReadReport {
	return SQLHistoricalComponentReadReport{BusinessMatched: o != nil && o.self == o && o.seal != "" && o.seal == o.digest(), ResponsibilitiesMatched: o != nil && o.self == o && o.seal != "" && o.seal == o.digest(), FullSourcesRequired: true, MongoQualificationRequired: true, AIClosureRequired: true, HostCommitRequired: true}
}

// The physical native CAS primitive is private. Frozen input and physical
// observations cannot reach a public production write API. A real fresh
// source/Mongo/AI-qualified aggregate must be composed before a public caller
// is added. Native regressions exercise this primitive without granting one.
func (o *SQLHistoricalComponentObservation) apply(ctx context.Context) (*SQLHistoricalComponentStatement, error) {
	if o.live(ctx) != nil || !o.writable || o.used || len(o.recipe.plan.groups) == 0 {
		return nil, ErrSQLHistoricalComponent
	}
	o.used = true
	statement, err := o.recipe.plan.Apply(ctx)
	if err != nil {
		return nil, err
	}
	if statement.transaction != o.transaction || o.live(ctx) != nil {
		return nil, ErrSQLHistoricalComponent
	}
	s := &SQLHistoricalComponentStatement{observation: o, statement: statement}
	s.self = s
	s.seal = s.digest()
	return s, nil
}
func (s *SQLHistoricalComponentStatement) digest() string {
	if s == nil || s.observation == nil || s.statement == nil || s.statement.plan != s.observation.recipe.plan {
		return ""
	}
	return cycleKeyDigest([]string{s.observation.seal, casImageHash(s.statement.expected), strconv.FormatUint(s.statement.transaction.event, 10)})
}

// A different actual RR-RO transaction must read the committed server bytes.
// A rollback/partial/unknown write cannot be converted to expected JSON input.
// SQL readback alone deliberately never certifies the paired host commits.
func (s *SQLHistoricalComponentStatement) VerifyIndependentPersisted(ctx context.Context, budget time.Duration) (SQLHistoricalComponentReadReport, error) {
	r := SQLHistoricalComponentReadReport{FullSourcesRequired: true, MongoQualificationRequired: true, AIClosureRequired: true, HostCommitRequired: true}
	if ctx == nil || ctx.Err() != nil || s == nil || s.self != s || s.seal == "" || s.seal != s.digest() || !s.observation.recipe.intact() || budget <= 0 || budget > 20*time.Second {
		return r, ErrSQLHistoricalComponent
	}
	bounded, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	tx, err := historicalTx(bounded)
	if err != nil || tx.Statement.ConnPool == s.observation.pool || tx.Statement.ConnPool == s.observation.recipe.oldPool || sqlHistoricalEndedPool(bounded, s.observation.pool) != nil {
		return r, ErrSQLHistoricalComponent
	}
	server, database, err := historicalDatabase(tx)
	p := s.observation.recipe.plan
	if err != nil || server != p.server || database != p.database {
		return r, ErrSQLHistoricalComponent
	}
	image, err := p.capture(tx, false)
	if err != nil || !reflect.DeepEqual(image, s.statement.expected) {
		return r, ErrSQLHistoricalComponent
	}
	actual, err := cycleActualTransaction(tx)
	if err != nil || actual == s.statement.transaction || actual == p.oldTransaction {
		return r, ErrSQLHistoricalComponent
	}
	scoped, anchors, err := s.observation.recipe.captureResponsibility(tx)
	if err != nil || !reflect.DeepEqual(scoped, s.observation.recipe.responsibility) || !reflect.DeepEqual(anchors, s.observation.recipe.anchors) {
		return r, ErrSQLHistoricalComponent
	}
	after, err := cycleActualTransaction(tx)
	if err != nil || after != actual || bounded.Err() != nil {
		return r, ErrSQLHistoricalComponent
	}
	r.BusinessMatched, r.ResponsibilitiesMatched, r.IndependentPersistedReadMatched = true, true, true
	return r, nil
}
