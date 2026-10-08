package retirement

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// This fixture is independently authenticated before its consumption stream is
// changed. Every changed stream keeps the original approved expectation.
func authenticatedConsumptionFixture(t *testing.T) (authFixture, [][][]byte, [][]byte) {
	t.Helper()
	f := coordinatorFixture(t, 2, true)
	var sqlRows [][][]byte
	for i, kind := range []string{"evaluation.requested", "evaluation.retry.requested", "evaluation.failed", "evaluation.outcome.committed"} {
		body := bytes.ReplaceAll(wireFixture(t, kind), []byte("original-event-1"), []byte(fmt.Sprintf("coordinator-sql-%d", i)))
		sqlRows = append(sqlRows, fixtureSQLRow(t, body, fmt.Sprint(i+1)))
	}
	var mongoRows [][]byte
	for i, kind := range []string{"answersheet.submitted", "interpretation.report.generated"} {
		body := bytes.ReplaceAll(wireFixture(t, kind), []byte("original-event-1"), []byte(fmt.Sprintf("coordinator-mongo-%d", i)))
		mongoRows = append(mongoRows, marshalBSON(t, fixtureMongoRow(t, body, int64(fixtureLargeID+int64(i)))))
	}
	return f, sqlRows, mongoRows
}

func authenticatedSQLStream(t *testing.T, f authFixture, rows [][][]byte) []byte {
	t.Helper()
	raw, _ := fixtureSQLCopy(t, rows, nil)
	// The original header, rather than the changed stream's observed upper,
	// remains the independently approved boundary.
	return append(append([]byte(nil), f.raw[0][:bytes.IndexByte(f.raw[0], '\n')+1]...), raw[bytes.IndexByte(raw, '\n')+1:]...)
}

func authenticatedConsumptionError(t *testing.T, f authFixture, object int, alternate []byte) error {
	t.Helper()
	inputs := f.inputs()
	inputs[object].Input = &coordinatorSeekMutation{Reader: bytes.NewReader(f.raw[object]), alternate: alternate}
	c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), inputs, DefaultHistoricalCoordinatorLimits())
	if err != nil {
		t.Fatal("valid initial authentication failed", err)
	}
	for {
		p, err := c.NextPage(t.Context())
		if err != nil {
			if err == io.EOF || c.Receipt().SourceCoverageComplete || c.Receipt().DropReady {
				t.Fatal("changed consumption acquired complete coverage")
			}
			if _, exportErr := c.CandidateRange(0, 32); !errors.Is(exportErr, ErrCoordinatorIncomplete) {
				t.Fatal("failed stream exported candidates", exportErr)
			}
			return err
		}
		events, err := p.Events()
		if err != nil {
			t.Fatal(err)
		}
		if len(events) > 0 {
			err = c.QualifyPage(t.Context(), p, nil, nil)
		} else {
			err = c.QualifyAIPage(t.Context(), p, nil)
		}
		if err != nil {
			t.Fatal("unexpected qualification error", err)
		}
	}
}

func TestAuthenticatedConsumptionBaselineErrorContract(t *testing.T) {
	f, sqlRows, mongoRows := authenticatedConsumptionFixture(t)
	duplicateSQL := append([][][]byte(nil), sqlRows...)
	duplicateSQL[1] = append([][]byte(nil), sqlRows[1]...)
	duplicateSQL[1][1] = []byte("coordinator-sql-0")
	duplicateSQL[1][7] = bytes.ReplaceAll(sqlRows[1][7], []byte("coordinator-sql-1"), []byte("coordinator-sql-0"))
	changedSQL := append([][][]byte(nil), sqlRows...)
	changedSQL[0] = append([][]byte(nil), sqlRows[0]...)
	changedSQL[0][8] = []byte("failed")
	reorderedSQL := append([][][]byte(nil), sqlRows...)
	reorderedSQL[0], reorderedSQL[1] = reorderedSQL[1], reorderedSQL[0]
	repeatedSQL := append([][][]byte(nil), sqlRows...)
	repeatedSQL[1] = sqlRows[0]
	duplicateMongo := append([][]byte(nil), mongoRows...)
	duplicateMongo[1] = bytes.ReplaceAll(mongoRows[1], []byte("coordinator-mongo-1"), []byte("coordinator-mongo-0"))
	changedMongo := append([][]byte(nil), mongoRows...)
	var changedDoc bson.D
	if bson.Unmarshal(mongoRows[0], &changedDoc) != nil {
		t.Fatal("invalid fixture")
	}
	changedMongo[0] = marshalBSON(t, setMongoField(changedDoc, "attempt_count", int64(1)))
	for _, tc := range []struct {
		name   string
		object int
		raw    []byte
		want   error
	}{
		{"sql_duplicate_original_id", 0, authenticatedSQLStream(t, f, duplicateSQL), ErrSourceDigest},
		{"sql_full_fact_mutation", 0, authenticatedSQLStream(t, f, changedSQL), ErrSourceAuthentication},
		{"sql_reordered_authenticated_rows", 0, authenticatedSQLStream(t, f, reorderedSQL), ErrSourceOrder},
		{"sql_repeated_authenticated_pk", 0, authenticatedSQLStream(t, f, repeatedSQL), ErrSourceOrder},
		{"sql_missing_final_row", 0, authenticatedSQLStream(t, f, sqlRows[:3]), ErrSourceIncomplete},
		{"mongo_duplicate_original_id", 3, mongoFixtureStream(t, duplicateMongo), ErrSourceDigest},
		{"mongo_full_fact_mutation", 3, mongoFixtureStream(t, changedMongo), ErrSourceAuthentication},
		{"mongo_reordered_authenticated_rows", 3, mongoFixtureStream(t, [][]byte{mongoRows[1], mongoRows[0]}), ErrSourceOrder},
		{"mongo_repeated_authenticated_pk", 3, mongoFixtureStream(t, [][]byte{mongoRows[0], mongoRows[0]}), ErrSourceOrder},
		{"mongo_missing_final_row", 3, mongoFixtureStream(t, mongoRows[:1]), ErrSourceIncomplete},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := authenticatedConsumptionError(t, f, tc.object, tc.raw); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func mongoFixtureStream(t *testing.T, rows [][]byte) []byte {
	t.Helper()
	raw, _ := fixtureMongoCopy(t, rows)
	return raw
}

func TestAuthenticatedConsumptionBaselineFullGoldenAndIndependentAuthentication(t *testing.T) {
	f, _, _ := authenticatedConsumptionFixture(t)
	wantHashes := [4]string{
		"6f29c5343ecfc5140126298b186b2229acd4ee7297e8fd9c55ffe60632559907",
		"e5d9ba749ac7e97642d35a551c4a8c7d3ed6e8f560f44276ec2dec00fe3f8041",
		"ac9c3515a0434729fe811589a615118137e1f283254c69482456d3501c7aa972",
		"a004d3fc9f9376c7e0b10387d367808a72f5c877a058771e6150192d038af339",
	}
	wantBytes := [4]uint64{2607, 1238, 1480, 1906}
	for epoch := 0; epoch < 2; epoch++ {
		limits := DefaultHistoricalCoordinatorLimits()
		limits.MaxPageRecords = 2
		c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), limits)
		if err != nil {
			t.Fatal(err)
		}
		coordinatorDrain(t, c)
		r := c.Receipt()
		if !r.SourceCoverageComplete || r.ConsumedRecords != [4]uint64{4, 2, 2, 2} || r.CandidateCount != 10 || r.DropReady || r.CASComplete || c.SQLCASReadiness().CASAuthorized || c.MongoCASReadiness().CASAuthorized {
			t.Fatal("whole-source consumption or false authority changed")
		}
		if r.CandidateSHA256 != "5c312bb5056e8b0c510d454a3f17519373c0430a632aa7b0e68fb1f7ed38f4c9" {
			t.Fatal("complete original candidate hash changed")
		}
		for i, receipt := range r.SecondPassCopies {
			if receipt.DataHash != wantHashes[i] || receipt.Bytes != wantBytes[i] || !receipt.Complete || receipt.BusinessClosureVerified || receipt.DropReady {
				t.Fatal("original full-copy digest or byte coverage changed", i)
			}
		}
	}
	// A new epoch must independently authenticate all four copies; the earlier
	// successful epoch cannot approve a cross-store original EventID collision.
	bad := sourceAuthFixture(t, false, false)
	if c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), bad.inputs(), DefaultHistoricalCoordinatorLimits()); c != nil || err != ErrSourceIdentity {
		t.Fatal("new initial authentication inherited prior ID authority", err)
	}
}

func TestAuthenticatedConsumptionBaselineRemainingCannotBeReused(t *testing.T) {
	f, _, _ := authenticatedConsumptionFixture(t)
	c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits())
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewSQLSourceReader(bytes.NewReader(f.raw[0]), f.expected[0])
	if err != nil {
		t.Fatal(err)
	}
	event, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	key, err := sourceAuthKey(event.Source.Database, event.Source.Object, event.Source.PrimaryKeySHA256)
	if err != nil {
		t.Fatal(err)
	}
	delete(c.remaining, key)
	if p, err := c.NextPage(t.Context()); p != nil || err != ErrSourceIdentity || c.Receipt().SourceCoverageComplete || c.Receipt().DropReady {
		t.Fatal("authenticated row reused a consumed physical source key", err)
	}
}

func TestAuthenticatedReaderOnlyOwnSealedCopyAndCompactIDs(t *testing.T) {
	f, _, _ := authenticatedConsumptionFixture(t)
	for _, object := range []int{0, 3} {
		t.Run(fmt.Sprint(object), func(t *testing.T) {
			c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits())
			if err != nil {
				t.Fatal(err)
			}
			var next func() (*DecodedSourceEvent, error)
			var receipt func() SourceCopyReceipt
			var acc *sourceAccumulator
			if object == 0 {
				r, err := c.newAuthenticatedSQLSourceReader()
				if err != nil {
					t.Fatal(err)
				}
				next, receipt, acc = r.Next, r.Receipt, &r.acc
			} else {
				r, err := c.newAuthenticatedMongoSourceReader()
				if err != nil {
					t.Fatal(err)
				}
				next, receipt, acc = r.Next, r.Receipt, &r.acc
			}
			if acc.authenticated == nil || acc.authenticated.owner != c || acc.authenticated.auth != c.authenticated || acc.authenticated.expected != f.expected[object] {
				t.Fatal("private reader did not bind its actual coordinator and approved copy")
			}
			for {
				event, err := next()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				if _, err := c.authenticated.BindEvent(event); err != nil {
					t.Fatal("accepted row did not match complete typed authentication", err)
				}
			}
			if acc.identities.seen != nil || uint64(len(acc.authenticated.seen)) != f.expected[object].Records || receipt() != c.authenticated.receipts[object] {
				t.Fatal("private reader retained full references or changed EOF receipt")
			}
		})
	}
	// Independent public readers still perform their own original-reference
	// tracking. The opaque authentication index is not a public bypass option.
	sql, err := NewSQLSourceReader(bytes.NewReader(f.raw[0]), f.expected[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sql.Next(); err != nil || sql.acc.authenticated != nil || len(sql.acc.identities.seen) != 1 {
		t.Fatal("public SQL reader changed its default identity checks", err)
	}
	mgo, err := NewMongoSourceReader(bytes.NewReader(f.raw[3]), f.expected[3])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgo.Next(); err != nil || mgo.acc.authenticated != nil || len(mgo.acc.identities.seen) != 1 {
		t.Fatal("public Mongo reader changed its default identity checks", err)
	}
}

func TestAuthenticatedReaderRejectsIncompleteForeignAndChangedBinding(t *testing.T) {
	for _, object := range []uint8{0, 3} {
		for _, tc := range []struct {
			name   string
			change func(*HistoricalCoordinator)
		}{
			{"missing_auth", func(c *HistoricalCoordinator) { c.authenticated = nil }},
			{"incomplete_auth", func(c *HistoricalCoordinator) { c.authenticated.complete = false }},
			{"empty_forged_auth", func(c *HistoricalCoordinator) { c.authenticated = &VerifiedSourceCopies{complete: true} }},
			{"missing_rows", func(c *HistoricalCoordinator) { c.authenticated.rows = nil }},
			{"wrong_namespace", func(c *HistoricalCoordinator) { c.copies[object].Expected.Boundary.Name = "rm_outbox" }},
			{"wrong_count", func(c *HistoricalCoordinator) { c.copies[object].Expected.Records++ }},
			{"wrong_bytes", func(c *HistoricalCoordinator) { c.copies[object].Expected.Bytes++ }},
			{"wrong_full_hash", func(c *HistoricalCoordinator) { c.copies[object].Expected.DataHash = sourceSHA([]byte("another-copy")) }},
			{"incomplete_receipt", func(c *HistoricalCoordinator) { c.authenticated.receipts[object].Complete = false }},
			{"authority_receipt", func(c *HistoricalCoordinator) { c.authenticated.receipts[object].DropReady = true }},
			{"closure_receipt", func(c *HistoricalCoordinator) { c.authenticated.receipts[object].BusinessClosureVerified = true }},
			{"different_receipt_protocol", func(c *HistoricalCoordinator) { c.authenticated.receipts[object].Protocol = "unapproved" }},
			{"missing_seeker", func(c *HistoricalCoordinator) { c.seekers[object] = nil }},
		} {
			t.Run(fmt.Sprintf("%d/%s", object, tc.name), func(t *testing.T) {
				f, _, _ := authenticatedConsumptionFixture(t)
				c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits())
				if err != nil {
					t.Fatal(err)
				}
				tc.change(c)
				if o, err := c.newAuthenticatedObserver(object); o != nil || err != ErrSourceAuthentication {
					t.Fatal("unsealed or different copy acquired compact observation", err)
				}
			})
		}
	}
	for _, c := range []*HistoricalCoordinator{nil, {}} {
		if o, err := c.newAuthenticatedObserver(0); o != nil || err != ErrSourceAuthentication {
			t.Fatal("zero/nil coordinator created private observer", err)
		}
	}
	for _, object := range []uint8{1, 2, 255} {
		f, _, _ := authenticatedConsumptionFixture(t)
		c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits())
		if err != nil {
			t.Fatal(err)
		}
		if o, err := c.newAuthenticatedObserver(object); o != nil || err != ErrSourceAuthentication {
			t.Fatal("non-event copy acquired compact observation", err)
		}
	}
}

func TestAuthenticatedReaderRejectsBindingChangesBeforeNextRawRow(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*HistoricalCoordinator, *authenticatedSourceObserver)
	}{
		{"new_actual_auth", func(c *HistoricalCoordinator, _ *authenticatedSourceObserver) {
			f, _, _ := authenticatedConsumptionFixture(t)
			other, err := VerifySourceCopies(t.Context(), f.inputs())
			if err != nil {
				t.Fatal(err)
			}
			c.authenticated = other
		}},
		{"changed_expected_identity", func(c *HistoricalCoordinator, _ *authenticatedSourceObserver) {
			c.copies[0].Expected.Boundary.IdentityHash = sourceSHA([]byte("different-database"))
		}},
		{"changed_receipt", func(c *HistoricalCoordinator, _ *authenticatedSourceObserver) { c.authenticated.receipts[0].Bytes++ }},
		{"incomplete_after_construction", func(c *HistoricalCoordinator, _ *authenticatedSourceObserver) { c.authenticated.complete = false }},
		{"foreign_owner", func(_ *HistoricalCoordinator, o *authenticatedSourceObserver) { o.owner = &HistoricalCoordinator{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, _, _ := authenticatedConsumptionFixture(t)
			c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits())
			if err != nil {
				t.Fatal(err)
			}
			r, err := c.newAuthenticatedSQLSourceReader()
			if err != nil {
				t.Fatal(err)
			}
			tc.change(c, r.acc.authenticated)
			if event, err := r.Next(); event != nil || err != ErrSourceAuthentication || r.Receipt().Complete || len(r.acc.authenticated.seen) != 0 || r.acc.identities.seen != nil {
				t.Fatal("changed opaque owner/authentication was accepted", err)
			}
			if _, err := r.Next(); err != ErrSourceIncomplete {
				t.Fatal("failed private reader resumed after rejection", err)
			}
		})
	}
}

func TestAuthenticatedObserverUsesAllTypedFactsNotOnlySourceDigest(t *testing.T) {
	f, _, _ := authenticatedConsumptionFixture(t)
	c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits())
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewSQLSourceReader(bytes.NewReader(f.raw[0]), f.expected[0])
	if err != nil {
		t.Fatal(err)
	}
	event, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	bound, err := c.authenticated.BindEvent(event)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(*DecodedSourceEvent)
	}{
		{"original_event_id", func(v *DecodedSourceEvent) { v.EventID = "another-original" }},
		{"original_event_type", func(v *DecodedSourceEvent) { v.EventType = "evaluation.failed" }},
		{"organization", func(v *DecodedSourceEvent) { v.OrgID++ }},
		{"business_clock", func(v *DecodedSourceEvent) { v.BusinessAt = v.BusinessAt.Add(time.Millisecond) }},
		{"source_pk_token", func(v *DecodedSourceEvent) { v.PrimaryKeyToken = "Mg==" }},
		{"schema", func(v *DecodedSourceEvent) { v.SupportedSchema = "future-v1" }},
		{"immutable_content_sha", func(v *DecodedSourceEvent) { v.ContentDigest.SHA256 = sourceSHA([]byte("another-wire")) }},
		{"original_run", func(v *DecodedSourceEvent) { v.OriginalRun.RunID = "inferred-latest-run" }},
		{"business_ids", func(v *DecodedSourceEvent) { v.BusinessIDs["unapproved"] = "9007199254740993" }},
		{"transport_attempt", func(v *DecodedSourceEvent) { v.Transport.AttemptCount++ }},
		{"null_vs_empty_transport", func(v *DecodedSourceEvent) { empty := ""; v.Transport.Fields["last_error"] = &empty }},
		{"missing_payload", func(v *DecodedSourceEvent) { v.Requested = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v, err := bound.Facts()
			if err != nil {
				t.Fatal(err)
			}
			tc.change(v)
			if v.Source != event.Source {
				t.Fatal("adversarial test changed original SourceRef")
			}
			o, err := c.newAuthenticatedObserver(0)
			if err != nil {
				t.Fatal(err)
			}
			if err := o.observe(v); err != ErrSourceAuthentication || len(o.seen) != 0 {
				t.Fatal("same raw SourceRef admitted changed complete typed facts", err)
			}
		})
	}
}
