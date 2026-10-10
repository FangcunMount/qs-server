package evaluation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"sort"
	"strings"
	"time"

	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
)

var (
	ErrSQLCrossStoreInvalid  = errors.New("sql_cross_store_responsibility_invalid")
	ErrSQLCrossStoreBounds   = errors.New("sql_cross_store_responsibility_budget_exceeded")
	ErrSQLCrossStoreConflict = errors.New("sql_cross_store_responsibility_snapshot_conflict")
	ErrSQLCrossStoreSchema   = errors.New("sql_cross_store_responsibility_schema_unsupported")
)

// These are resource bounds, never authority to omit an unresolved row.
type SQLCrossStoreLimits struct {
	PageRows, MaxPageRows                      int
	MaxKeys, MaxKeyRetainedBytes, MaxPageBytes uint64
	MaxDuration                                time.Duration
}

func DefaultSQLCrossStoreLimits() SQLCrossStoreLimits {
	return SQLCrossStoreLimits{PageRows: 256, MaxPageRows: 32768, MaxKeys: 2_000_000, MaxKeyRetainedBytes: 1 << 30, MaxPageBytes: 64 << 20, MaxDuration: time.Minute}
}

func (l SQLCrossStoreLimits) valid() bool {
	return l.PageRows > 0 && l.PageRows <= 512 && l.MaxPageRows > 0 && l.MaxPageRows <= 131072 && l.MaxKeys > 0 && l.MaxKeys <= 10_000_000 && l.MaxKeyRetainedBytes > 0 && l.MaxKeyRetainedBytes <= 4<<30 && l.MaxPageBytes > 0 && l.MaxPageBytes <= 256<<20 && l.MaxDuration > 0 && l.MaxDuration <= 2*time.Minute
}

// One catalog per complete cycle. The frozen cycle retains key hashes, not
// recoverable keys. A second PRIMARY-only bounded read authenticates original
// keys in the same real snapshot; it does not read another full message body.
type SQLHistoricalCrossStoreCatalog struct {
	cycle                     *SQLHistoricalResponsibilityCycle
	keys                      map[int][]string
	byRequest, requestParents map[string][]int
	requestOrganizations      map[uint64][]int
	byGovernanceOrg           map[uint64][]int
	byMongoOwner              map[string][]int
	limits                    SQLCrossStoreLimits
	report                    SQLCrossStoreCatalogReport
}

type SQLCrossStoreCatalogReport struct {
	Version, CycleID, DatabaseIdentitySHA256                                                              string
	Keys, KeyBytes, KeyQueries                                                                            uint64
	Complete, SourceAuthenticationRequired, BusinessQualificationRequired, WriterFenceRequired, DropReady bool
	Ledgers                                                                                               []SQLCrossStoreKeyLedgerReport
}

// Boundary digests describe actual server-ordered primary keys from this
// one complete epoch. Empty ledgers have no lower/upper/last observed key;
// these hashes never expose raw business identifiers or message payloads.
type SQLCrossStoreKeyLedgerReport struct {
	Store, FirstObservedKeySHA256, FixedUpperKeySHA256, LastObservedKeySHA256, KeysSHA256, SchemaSHA256, PrimaryKeySHA256 string
	Keys, Queries                                                                                                         uint64
}

func (*SQLHistoricalCrossStoreCatalog) MarshalJSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalCrossStoreCatalog) MarshalBSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalCrossStoreCatalog) String() string {
	return "private SQL cross-store catalog; selectors are not closure proof"
}
func (c *SQLHistoricalCrossStoreCatalog) GoString() string { return c.String() }
func (c *SQLHistoricalCrossStoreCatalog) Report() SQLCrossStoreCatalogReport {
	if c == nil {
		return SQLCrossStoreCatalogReport{SourceAuthenticationRequired: true, BusinessQualificationRequired: true, WriterFenceRequired: true}
	}
	v := c.report
	v.Ledgers = append([]SQLCrossStoreKeyLedgerReport(nil), v.Ledgers...)
	return v
}

func PrepareSQLHistoricalCrossStoreCatalog(ctx context.Context, cycle *SQLHistoricalResponsibilityCycle, limits SQLCrossStoreLimits) (*SQLHistoricalCrossStoreCatalog, error) {
	if ctx == nil || cycle == nil || cycle.report.CompletedAt.IsZero() || len(cycle.ledgers) != len(sqlResponsibilityTables) || !limits.valid() || uint64(len(cycle.observations)) > limits.MaxKeys {
		return nil, ErrSQLCrossStoreInvalid
	}
	if err := cycle.ValidateBorrowedSnapshot(ctx); err != nil {
		return nil, err
	}
	tx, err := historicalTx(ctx)
	if err != nil {
		return nil, err
	}
	c := &SQLHistoricalCrossStoreCatalog{cycle: cycle, keys: map[int][]string{}, byRequest: map[string][]int{}, requestParents: map[string][]int{}, requestOrganizations: map[uint64][]int{}, byGovernanceOrg: map[uint64][]int{}, byMongoOwner: map[string][]int{}, limits: limits}
	c.report = SQLCrossStoreCatalogReport{Version: "sql-cross-store-catalog/v1", CycleID: cycle.report.CycleID, DatabaseIdentitySHA256: cycle.report.DatabaseIdentitySHA256, SourceAuthenticationRequired: true, BusinessQualificationRequired: true, WriterFenceRequired: true}
	byKey := map[string]int{}
	for i, v := range cycle.observations {
		key := v.Store + ":" + v.PrimaryKeySHA256
		if _, exists := byKey[key]; exists {
			return nil, ErrSQLCrossStoreConflict
		}
		byKey[key] = i
		if v.OwnerKind == "AnswerSheet" || v.OwnerKind == "ReportGeneration" {
			c.byMongoOwner[v.OwnerKind+":"+v.OwnerID] = append(c.byMongoOwner[v.OwnerKind+":"+v.OwnerID], i)
		}
		switch v.Store {
		case "qs_rm_replay_requests":
			c.requestParents[cyclePair(v.OrgID, v.link.requestID)] = append(c.requestParents[cyclePair(v.OrgID, v.link.requestID)], i)
			c.requestOrganizations[v.OrgID] = append(c.requestOrganizations[v.OrgID], i)
		case "qs_rm_replay_items":
			c.byRequest[cyclePair(v.OrgID, v.link.requestID)] = append(c.byRequest[cyclePair(v.OrgID, v.link.requestID)], i)
		case "system_governance_action_runs":
			c.byGovernanceOrg[v.OrgID] = append(c.byGovernanceOrg[v.OrgID], i)
		}
	}
	for n, ledger := range cycle.ledgers {
		if ledger.spec.name != sqlResponsibilityTables[n].name {
			return nil, ErrSQLCrossStoreConflict
		}
		columns, schema, pk, e := cycleSchema(tx, ledger.spec)
		if e != nil || schema != ledger.report.SchemaSHA256 || pk != ledger.report.PrimaryKeySHA256 {
			return nil, ErrSQLCrossStoreConflict
		}
		if !sqlCrossStoreSupportedColumns(ledger.spec.name, columns) {
			return nil, ErrSQLCrossStoreSchema
		}
		var after []string
		var count uint64
		bound := SQLCrossStoreKeyLedgerReport{Store: ledger.spec.name, SchemaSHA256: schema, PrimaryKeySHA256: pk}
		if ledger.upper != nil {
			bound.FixedUpperKeySHA256 = cycleKeyDigest(ledger.upper)
		}
		keysHash := sha256.New()
		cycleFrame(keysHash, []byte("sql-cross-store-server-ordered-keys/v1"), true)
		cycleFrame(keysHash, []byte(ledger.spec.name), true)
		for ledger.upper != nil {
			predicate, args := cycleKeyPredicate(ledger.spec, ledger.upper, true)
			if after != nil {
				lower, values := cycleKeyPredicate(ledger.spec, after, false)
				predicate += " AND " + lower
				args = append(args, values...)
			}
			args = append(args, limits.PageRows)
			rows, columns, _, e := cycleQuery(tx, "SELECT "+strings.Join(ledger.spec.keys, ",")+" FROM `"+ledger.spec.name+"` FORCE INDEX (PRIMARY) WHERE "+predicate+" ORDER BY "+strings.Join(ledger.spec.keys, ",")+" LIMIT ?", limits.PageRows, args...)
			if e != nil {
				return nil, e
			}
			c.report.KeyQueries++
			bound.Queries++
			if !reflect.DeepEqual(columns, ledger.spec.keys) {
				return nil, ErrSQLCrossStoreConflict
			}
			if len(rows) == 0 {
				break
			}
			for _, row := range rows {
				key, e := cycleKey(ledger.spec, row)
				if e != nil {
					return nil, e
				}
				if after != nil && cycleCompare(ledger.spec, key, after) <= 0 || cycleCompare(ledger.spec, key, ledger.upper) > 0 {
					return nil, ErrSQLCrossStoreConflict
				}
				after = key
				digest := cycleKeyDigest(key)
				if count == 0 {
					bound.FirstObservedKeySHA256 = digest
				}
				bound.LastObservedKeySHA256 = digest
				cycleFrame(keysHash, []byte(digest), true)
				i, ok := byKey[ledger.spec.name+":"+cycleKeyDigest(key)]
				if !ok || c.keys[i] != nil {
					return nil, ErrSQLCrossStoreConflict
				}
				cost := uint64(160)
				for _, v := range key {
					cost += uint64(len(v) + 16)
				}
				if cost > limits.MaxKeyRetainedBytes-c.report.KeyBytes || c.report.Keys >= limits.MaxKeys {
					return nil, ErrSQLCrossStoreBounds
				}
				c.keys[i] = append([]string(nil), key...)
				c.report.KeyBytes += cost
				c.report.Keys++
				count++
			}
		}
		if count != ledger.report.Rows || count > 0 && cycleCompare(ledger.spec, after, ledger.upper) != 0 {
			return nil, ErrSQLCrossStoreConflict
		}
		_, schema, pk, e = cycleSchema(tx, ledger.spec)
		if e != nil || schema != ledger.report.SchemaSHA256 || pk != ledger.report.PrimaryKeySHA256 {
			return nil, ErrSQLCrossStoreConflict
		}
		bound.Keys = count
		bound.KeysSHA256 = hex.EncodeToString(keysHash.Sum(nil))
		c.report.Ledgers = append(c.report.Ledgers, bound)
	}
	if len(c.keys) != len(cycle.observations) {
		return nil, ErrSQLCrossStoreConflict
	}
	if err := cycle.ValidateBorrowedSnapshot(ctx); err != nil {
		return nil, err
	}
	c.report.Complete = true
	return c, nil
}

// Current eight-ledger schemas are defined by migrations 48/49/50/84/86/89/90.
// Whole-column hashing detects drift, but cannot interpret future columns.
// This separate allowlist rejects an unsupported schema even for empty tables.
func sqlCrossStoreSupportedColumns(store string, columns []string) bool {
	var expected string
	switch store {
	case "rm_outbox":
		expected = "id producer message_id destination event_type schema_version scope content_type occurred_at payload fingerprint state next_attempt_at claim_token lease_until version attempt_count failure_count last_error_code transport_confirmed_at manual_replay_request_id manual_replay_version created_at updated_at"
	case "retry_event_hold":
		expected = "id event_id message_id org_id provider topic_name channel_name payload_json original_delivery_attempt blocked_reason blocked_at status retry_disposition replay_attempt_count next_attempt_at claim_token claim_expires_at last_error manual_replay_request_id replayed_at created_at updated_at"
	case "event_delivery_dead_letter":
		expected = "id message_id transport_message_id event_id org_id provider topic_name channel_name delivery_attempts payload_json last_error retry_disposition replay_request_id replayed_at failed_at created_at updated_at"
	case "qs_rm_evaluation_request_ref":
		expected = "event_id assessment_id org_id created_at"
	case "qs_rm_gap_recovery_request":
		expected = "org_id request_id actor_id assessment_id event_id expected_version reason submitted_before input_hash result_code authorized outbox_version_before outbox_version_after created_at updated_at"
	case "qs_rm_replay_requests":
		expected = "org_id request_id store_name reason input_hash created_at"
	case "qs_rm_replay_items":
		expected = "org_id request_id ordinal event_id expected_failure_count authorized reason"
	case "system_governance_action_runs":
		expected = "id request_id action_id org_id actor_user_id component target_instance input_json status result_json started_at finished_at created_at updated_at"
	default:
		return false
	}
	known := strings.Fields(expected)
	if len(known) != len(columns) {
		return false
	}
	set := map[string]bool{}
	for _, name := range known {
		set[name] = true
	}
	for _, name := range columns {
		if !set[name] {
			return false
		}
		delete(set, name)
	}
	return len(set) == 0
}

// These are untrusted selectors. No business terminal, Mongo ownership, source
// authentication or closure boolean is accepted here. The maintenance layer
// derives them exclusively from its verified source and opaque owner batches.
type SQLCrossStoreOwnerReference struct{ Kind, ID string }
type SQLCrossStoreSelectors struct {
	EventIDs                       []string
	AssessmentIDs, OrganizationIDs []uint64
	MongoOwners                    []SQLCrossStoreOwnerReference
}

type SQLCrossStoreRow struct {
	Observation                          SQLResponsibilityObservation
	Inner                                *domainwire.Envelope
	LegacyContentSHA256, InnerDataSHA256 string
	Replay                               *SQLCrossStoreReplay
}
type SQLCrossStoreReplay struct {
	OrganizationID                uint64
	RequestID, Store, InputSHA256 string
	FingerprintVerified           bool
	Items                         []SQLCrossStoreReplayItem
}
type SQLCrossStoreReplayItem struct {
	EventID, Reason      string
	ExpectedFailureCount uint64
	Authorized           bool
}

func (SQLCrossStoreRow) MarshalJSON() ([]byte, error) { return nil, ErrSQLHistoricalFactsSerialization }
func (SQLCrossStoreRow) MarshalBSON() ([]byte, error) { return nil, ErrSQLHistoricalFactsSerialization }
func (SQLCrossStoreRow) String() string               { return "private cross-store row; no source authentication" }
func (v SQLCrossStoreRow) GoString() string           { return v.String() }

type SQLHistoricalCrossStorePage struct {
	catalog     *SQLHistoricalCrossStoreCatalog
	batch       *SQLHistoricalOwnerBatch
	rows        map[int]historicalSQLRow
	facts       map[int]SQLCrossStoreRow
	byEvent     map[string][]int
	byOwner     map[uint64][]int
	byOrg       map[uint64][]int
	replayPairs map[string]bool
	report      SQLCrossStorePageReport
	started     time.Time
	selectors   SQLCrossStoreSelectors
}
type SQLCrossStorePageReport struct {
	Version, CycleID, DatabaseIdentitySHA256, RowsSHA256                                                                                     string
	Rows, Bytes, Queries                                                                                                                     uint64
	GlobalUnknown, GlobalBlocking                                                                                                            uint64
	Complete, SourceAuthenticationRequired, MongoQualificationRequired, AIInboxCoverageRequired, WriterFenceRequired, CASRequired, DropReady bool
}

func (*SQLHistoricalCrossStorePage) MarshalJSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalCrossStorePage) MarshalBSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalCrossStorePage) String() string {
	return "private exact SQL cross-store page; no retirement approval"
}
func (p *SQLHistoricalCrossStorePage) GoString() string { return p.String() }
func (p *SQLHistoricalCrossStorePage) Report() SQLCrossStorePageReport {
	if p == nil {
		return SQLCrossStorePageReport{SourceAuthenticationRequired: true, MongoQualificationRequired: true, AIInboxCoverageRequired: true, WriterFenceRequired: true, CASRequired: true}
	}
	return p.report
}

func PrepareSQLHistoricalCrossStorePage(ctx context.Context, catalog *SQLHistoricalCrossStoreCatalog, batch *SQLHistoricalOwnerBatch, selectors SQLCrossStoreSelectors) (*SQLHistoricalCrossStorePage, error) {
	if ctx == nil || catalog == nil || !catalog.report.Complete || batch == nil || !batch.report.Complete || batch.cycle != catalog.cycle || len(selectors.EventIDs) == 0 || len(selectors.EventIDs) > 512 || len(selectors.AssessmentIDs)+len(selectors.OrganizationIDs) > 1024 || len(selectors.MongoOwners) > 32768 {
		return nil, ErrSQLCrossStoreInvalid
	}
	if err := batch.ValidateBorrowedSnapshot(ctx); err != nil {
		return nil, err
	}
	selected := map[int]bool{}
	add := func(ids []int) {
		for _, i := range ids {
			selected[i] = true
		}
	}
	for _, id := range selectors.EventIDs {
		if strings.TrimSpace(id) == "" || len(id) > 128 {
			return nil, ErrSQLCrossStoreInvalid
		}
		add(catalog.cycle.byEvent[id])
	}
	for _, id := range selectors.AssessmentIDs {
		if _, err := batch.OwnerByAssessment(id); err != nil {
			return nil, err
		}
		add(catalog.cycle.byOwner[id])
	}
	for _, org := range selectors.OrganizationIDs {
		if org == 0 {
			return nil, ErrSQLCrossStoreInvalid
		}
		add(catalog.byGovernanceOrg[org])
	}
	for _, owner := range selectors.MongoOwners {
		if owner.Kind != "AnswerSheet" && owner.Kind != "ReportGeneration" || cyclePayloadID(owner.ID) == 0 {
			return nil, ErrSQLCrossStoreInvalid
		}
		add(catalog.byMongoOwner[owner.Kind+":"+owner.ID])
	}
	replayPairs := map[string]bool{}
	for i := range selected {
		v := catalog.cycle.observations[i]
		if v.Store == "qs_rm_replay_items" {
			pair := cyclePair(v.OrgID, v.link.requestID)
			replayPairs[pair] = true
			add(catalog.requestParents[pair])
			add(catalog.byRequest[pair])
		}
	}
	if len(selected) > catalog.limits.MaxPageRows {
		return nil, ErrSQLCrossStoreBounds
	}
	p := &SQLHistoricalCrossStorePage{catalog: catalog, batch: batch, rows: map[int]historicalSQLRow{}, facts: map[int]SQLCrossStoreRow{}, byEvent: map[string][]int{}, byOwner: map[uint64][]int{}, byOrg: map[uint64][]int{}, replayPairs: replayPairs, started: time.Now(), selectors: SQLCrossStoreSelectors{EventIDs: append([]string(nil), selectors.EventIDs...), AssessmentIDs: append([]uint64(nil), selectors.AssessmentIDs...), OrganizationIDs: append([]uint64(nil), selectors.OrganizationIDs...), MongoOwners: append([]SQLCrossStoreOwnerReference(nil), selectors.MongoOwners...)}}
	p.report = SQLCrossStorePageReport{Version: "sql-cross-store-page/v1", CycleID: catalog.report.CycleID, DatabaseIdentitySHA256: catalog.report.DatabaseIdentitySHA256, SourceAuthenticationRequired: true, MongoQualificationRequired: true, AIInboxCoverageRequired: true, WriterFenceRequired: true, CASRequired: true}
	p.report.GlobalUnknown, p.report.GlobalBlocking = catalog.cycle.report.Unknown, catalog.cycle.report.Blocking
	if err := p.read(ctx, selected); err != nil {
		return nil, err
	}
	for i, row := range p.rows {
		v := catalog.cycle.observations[i]
		f := SQLCrossStoreRow{Observation: v}
		f.Observation.Reasons = append([]string(nil), v.Reasons...)
		if v.Store == "rm_outbox" || v.Store == "retry_event_hold" || v.Store == "event_delivery_dead_letter" {
			raw := mustCycleInner(row, v.Store)
			var inner domainwire.Envelope
			if sqlHistoricalStrictJSON(raw, &inner) == nil {
				f.Inner = &inner
				sum := sha256.Sum256(raw)
				f.LegacyContentSHA256 = hex.EncodeToString(sum[:])
				sum = sha256.Sum256(inner.Data)
				f.InnerDataSHA256 = hex.EncodeToString(sum[:])
			}
		}
		p.facts[i] = f
		if v.EventID != "" {
			p.byEvent[v.EventID] = append(p.byEvent[v.EventID], i)
		}
		if v.AssessmentID != 0 {
			p.byOwner[v.AssessmentID] = append(p.byOwner[v.AssessmentID], i)
		}
		if v.Store == "system_governance_action_runs" {
			p.byOrg[v.OrgID] = append(p.byOrg[v.OrgID], i)
		}
	}
	if err := p.bindReplay(); err != nil {
		return nil, err
	}
	parts := make([]string, 0, len(p.rows))
	for i := range p.rows {
		v := catalog.cycle.observations[i]
		parts = append(parts, v.Store+":"+v.PrimaryKeySHA256+":"+v.RowSHA256)
	}
	sort.Strings(parts)
	h := sha256.New()
	cycleFrame(h, []byte("sql-cross-store-page/v1"), true)
	for _, part := range parts {
		cycleFrame(h, []byte(part), true)
	}
	p.report.RowsSHA256 = hex.EncodeToString(h.Sum(nil))
	if err := batch.ValidateBorrowedSnapshot(ctx); err != nil {
		return nil, err
	}
	p.report.Complete = true
	return p, nil
}

func (p *SQLHistoricalCrossStorePage) read(ctx context.Context, selected map[int]bool) error {
	tx, err := historicalTx(ctx)
	if err != nil {
		return err
	}
	for _, ledger := range p.catalog.cycle.ledgers {
		var ids []int
		for i := range selected {
			if p.catalog.cycle.observations[i].Store == ledger.spec.name {
				ids = append(ids, i)
			}
		}
		sort.Ints(ids)
		for offset := 0; offset < len(ids); offset += p.catalog.limits.PageRows {
			if time.Since(p.started) > p.catalog.limits.MaxDuration {
				return ErrSQLCrossStoreBounds
			}
			end := offset + p.catalog.limits.PageRows
			if end > len(ids) {
				end = len(ids)
			}
			terms := []string{}
			args := []any{}
			expected := map[string]int{}
			for _, i := range ids[offset:end] {
				key := p.catalog.keys[i]
				if len(key) != len(ledger.spec.keys) {
					return ErrSQLCrossStoreConflict
				}
				clauses := []string{}
				for _, field := range ledger.spec.keys {
					clauses = append(clauses, "`"+field+"`=?")
				}
				terms = append(terms, "("+strings.Join(clauses, " AND ")+")")
				args = append(args, cycleArgs(ledger.spec, key)...)
				expected[cycleKeyDigest(key)] = i
			}
			args = append(args, len(expected)+1)
			rows, columns, size, e := cycleQuery(tx, "SELECT * FROM `"+ledger.spec.name+"` FORCE INDEX (PRIMARY) WHERE "+strings.Join(terms, " OR ")+" ORDER BY "+strings.Join(ledger.spec.keys, ",")+" LIMIT ?", len(expected)+1, args...)
			if e != nil {
				return e
			}
			p.report.Queries++
			if !reflect.DeepEqual(columns, ledger.columns) || len(rows) != len(expected) {
				return ErrSQLCrossStoreConflict
			}
			if size > p.catalog.limits.MaxPageBytes-p.report.Bytes {
				return ErrSQLCrossStoreBounds
			}
			p.report.Bytes += size
			for _, row := range rows {
				key, e := cycleKey(ledger.spec, row)
				if e != nil {
					return e
				}
				i, ok := expected[cycleKeyDigest(key)]
				if !ok || p.rows[i] != nil || cycleRowDigest(columns, row) != p.catalog.cycle.observations[i].RowSHA256 {
					return ErrSQLCrossStoreConflict
				}
				p.rows[i] = row
				p.report.Rows++
			}
		}
	}
	return nil
}

func (p *SQLHistoricalCrossStorePage) bindReplay() error {
	for pair := range p.replayPairs {
		parents := p.catalog.requestParents[pair]
		var touched bool
		for _, i := range p.catalog.byRequest[pair] {
			if p.rows[i] != nil {
				touched = true
			}
		}
		if !touched {
			continue
		}
		if len(parents) != 1 || p.rows[parents[0]] == nil {
			return ErrSQLCrossStoreConflict
		}
		header := p.rows[parents[0]]
		org := cyclePositive(header, "org_id")
		if org == 0 || org > 1<<63-1 {
			return ErrSQLCrossStoreConflict
		}
		input := standard.ReplayRequest{OrgID: int64(org), RequestID: valueOrEmpty(header["request_id"]), Store: valueOrEmpty(header["store_name"]), Reason: valueOrEmpty(header["reason"])}
		r := &SQLCrossStoreReplay{OrganizationID: org, RequestID: input.RequestID, Store: input.Store, InputSHA256: hex.EncodeToString([]byte(valueOrEmpty(header["input_hash"])))}
		items := append([]int(nil), p.catalog.byRequest[pair]...)
		sort.Slice(items, func(i, j int) bool {
			return p.catalog.cycle.observations[items[i]].link.ordinal < p.catalog.cycle.observations[items[j]].link.ordinal
		})
		for ordinal, i := range items {
			row := p.rows[i]
			if row == nil {
				return ErrSQLCrossStoreConflict
			}
			index, ok := cycleSafeUint(row, "ordinal")
			expected, ok2 := cycleSafeUint(row, "expected_failure_count")
			if !ok || !ok2 || index != uint64(ordinal) || cyclePositive(row, "org_id") != org || valueOrEmpty(row["request_id"]) != input.RequestID {
				return ErrSQLCrossStoreConflict
			}
			item := SQLCrossStoreReplayItem{EventID: valueOrEmpty(row["event_id"]), ExpectedFailureCount: expected, Authorized: valueOrEmpty(row["authorized"]) == "1", Reason: valueOrEmpty(row["reason"])}
			r.Items = append(r.Items, item)
			input.Targets = append(input.Targets, standard.ReplayTarget{EventID: item.EventID, ExpectedFailureCount: expected})
		}
		hash, e := input.Fingerprint()
		r.FingerprintVerified = e == nil && bytes.Equal(hash[:], []byte(valueOrEmpty(header["input_hash"])))
		for _, i := range append(items, parents...) {
			f := p.facts[i]
			f.Replay = r
			p.facts[i] = f
		}
	}
	return nil
}

// Defensive cached copies are observations, not source/owner approval. The
// caller must run real boundary validation and independent fresh rechecks.
func (p *SQLHistoricalCrossStorePage) Lookup(eventID string, assessmentID, organizationID uint64) ([]SQLCrossStoreRow, error) {
	if p == nil || !p.report.Complete || eventID == "" || organizationID == 0 || time.Since(p.started) > p.catalog.limits.MaxDuration {
		return nil, ErrSQLCrossStoreInvalid
	}
	seen := map[int]bool{}
	var indices []int
	for _, list := range [][]int{p.byEvent[eventID], p.byOwner[assessmentID], p.byOrg[organizationID]} {
		for _, i := range list {
			if !seen[i] {
				seen[i] = true
				indices = append(indices, i)
			}
		}
	}
	sort.Ints(indices)
	var out []SQLCrossStoreRow
	for _, i := range indices {
		v := p.facts[i]
		v.Observation.Reasons = append([]string(nil), v.Observation.Reasons...)
		if v.Inner != nil {
			c := *v.Inner
			c.Data = append([]byte(nil), c.Data...)
			v.Inner = &c
		}
		if v.Replay != nil {
			c := *v.Replay
			c.Items = append([]SQLCrossStoreReplayItem(nil), c.Items...)
			v.Replay = &c
		}
		out = append(out, v)
	}
	return out, nil
}

func (p *SQLHistoricalCrossStorePage) ValidateBorrowedSnapshot(ctx context.Context) error {
	if p == nil || !p.report.Complete {
		return ErrSQLCrossStoreInvalid
	}
	return p.batch.ValidateBorrowedSnapshot(ctx)
}
func (c *SQLHistoricalCrossStoreCatalog) ValidateBorrowedSnapshot(ctx context.Context) error {
	if c == nil || !c.report.Complete {
		return ErrSQLCrossStoreInvalid
	}
	return c.cycle.ValidateBorrowedSnapshot(ctx)
}
