package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
)

func frozenRuleSQLKeys(ids ...string) *sqlmock.Rows {
	rows := sqlmock.NewRowsWithColumnDefinition(sqlmock.NewColumn("id").OfType("UNSIGNED BIGINT", ""))
	for _, id := range ids {
		rows.AddRow(id)
	}
	return rows
}

func TestFrozenRuleSQLDeleteUsesLockedKeysAndKeepsUnknownCommit(t *testing.T) {
	for _, outcome := range []string{"committed", "unknown_commit", "missing_key", "affected_count_changed"} {
		t.Run(outcome, func(t *testing.T) {
			s, mock := graphBatchMock(t)
			ctx := graphFrozenRuleDeleteContext(t.Context(), true)
			original := graphBatchRow("1", "complete original body remains in the archive")
			graphBatchExpectDeletePreflight(mock)
			mock.ExpectBegin()
			graphBatchExpectShape(mock, "assessment_score", "id", "bigint", "bigint unsigned")
			keys := frozenRuleSQLKeys("1")
			if outcome == "missing_key" {
				keys = frozenRuleSQLKeys()
			}
			// No original/current business body is selected inside this explicit
			// direct-rule transaction; default-mode CAS is tested separately.
			mock.ExpectQuery("SELECT x.`id` FROM `assessment_score` x WHERE (`id` = ?) FOR UPDATE").WithArgs([]byte("1")).WillReturnRows(keys)
			commitUnknown := errors.New("commit acknowledgement unavailable")
			if outcome == "missing_key" {
				mock.ExpectRollback()
			} else {
				graphBatchExpectNoSideEffects(mock)
				graphBatchExpectPacket(mock, 64*1024*1024)
				count := int64(1)
				if outcome == "affected_count_changed" {
					count = 0
				}
				mock.ExpectExec("DELETE FROM `assessment_score` WHERE (`id` = ?)").WithArgs([]byte("1")).WillReturnResult(sqlmock.NewResult(0, count))
				if outcome == "affected_count_changed" {
					mock.ExpectRollback()
				} else {
					mock.ExpectQuery("SELECT x.`id` FROM `assessment_score` x WHERE (`id` = ?) FOR UPDATE").WithArgs([]byte("1")).WillReturnRows(frozenRuleSQLKeys())
					if outcome == "unknown_commit" {
						mock.ExpectCommit().WillReturnError(commitUnknown)
					} else {
						mock.ExpectCommit()
					}
				}
			}
			err := s.mutateExactBatch(ctx, []record{original}, false)
			if outcome == "committed" && err != nil || outcome != "committed" && err == nil || outcome == "unknown_commit" && !errors.Is(err, commitUnknown) {
				t.Fatal("lost exact-key/count/unknown-commit boundary:", err)
			}
		})
	}
}

func TestFrozenRuleCaptureSkipsUniqueProbeButPreservesRestoreShape(t *testing.T) {
	s, mock := graphBatchMock(t)
	r := graphBatchRow("1", "full raw original")
	graphBatchExpectShape(mock, "assessment_score", "id", "bigint", "bigint unsigned")
	graphBatchExpectNoSideEffects(mock)
	mock.ExpectQuery("SELECT * FROM `assessment_score` LIMIT 0").WillReturnRows(graphBatchRows())
	mock.ExpectQuery("SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND GENERATION_EXPRESSION IS NOT NULL AND GENERATION_EXPRESSION<>''").WithArgs("assessment_score").WillReturnRows(sqlmock.NewRows([]string{"COLUMN_NAME"}))
	graphBatchExpectPacket(mock, 64*1024*1024)
	ctx := graphFrozenRuleDeleteContext(t.Context(), true)
	if err := s.preflightExactBatch(ctx, []record{r}, true); err != nil {
		t.Fatal(err)
	}
	// Actual restore with this context is rejected before any SQL request.
	if err := s.mutateExactBatch(ctx, []record{r}, true); err == nil {
		t.Fatal("direct-rule context admitted restore")
	}
}

func TestFrozenRuleCaptureBudgetSplitDoesNotRereadOriginalBodies(t *testing.T) {
	s, _, _, _, _, _, _ := graphEngineFixture(t)
	s.preflightLimit = 1
	s.preflights, s.readBatches = 0, 0
	one, two := graphBatchRow("1", "first original"), graphBatchRow("2", "second original")
	s.rows[objectID(one.Object)], s.rows[objectID(two.Object)] = one, two
	captured := []record{}
	ctx := graphFrozenRuleDeleteContext(t.Context(), true)
	if err := graphReadBounded(ctx, s, []object{one.Object, two.Object}, true, func(records []record) error {
		captured = append(captured, records...)
		return nil
	}); err != nil || len(captured) != 2 || !sameRecord(captured[0], one) || !sameRecord(captured[1], two) || s.readBatches != 1 || s.preflights != 3 {
		t.Fatal("budget split refetched or changed original bodies:", err, s.readBatches, s.preflights)
	}
}

func TestFrozenRuleMongoKeyProjectionStillRejectsNumericAlias(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	for _, alias := range []bool{false, true} {
		name := "exact_int64"
		if alias {
			name = "int32_alias"
		}
		mt.Run(name, func(mt *mtest.T) {
			definition := []byte("frozen collection definition")
			originalRaw, err := bson.Marshal(bson.D{{Key: "_id", Value: int64(42)}, {Key: "answers", Value: strings.Repeat("original", 1000)}})
			if err != nil {
				mt.Fatal(err)
			}
			original, err := graphMongoObject("answersheets", originalRaw, definition)
			if err != nil {
				mt.Fatal(err)
			}
			var actual any = int64(42)
			if alias {
				actual = int32(42)
			}
			mt.AddMockResponses(mtest.CreateCursorResponse(0, "owned.answersheets", mtest.FirstBatch, bson.D{{Key: "_id", Value: actual}}))
			s := newGraphStore(&liveStore{mongo: mt.Client, mongoDB: "owned"})
			ctx := graphFrozenRuleDeleteContext(mt.Context(), true)
			current, err := s.graphMutationMongoRead(ctx, []object{original.Object}, definition)
			if alias {
				if err == nil {
					mt.Fatal("numeric BSON alias passed the projected physical key check")
				}
			} else {
				if err != nil || len(current) != 1 || !current[0].Exists || len(current[0].Record.BSON) >= len(original.BSON) {
					mt.Fatal("exact-key projection failed:", err)
				}
				if _, err = graphMutationPreimages(ctx, []record{original}, current, false); err != nil {
					mt.Fatal("projected exact key did not admit explicit direct delete:", err)
				}
				if _, err = graphBatchPreimages([]record{original}, current, false); err == nil {
					mt.Fatal("default full-preimage CAS was weakened")
				}
			}
			event := mt.GetStartedEvent()
			if event == nil || event.CommandName != "find" || event.Command.Lookup("projection", "_id").Int32() != 1 || event.Command.Lookup("collation", "locale").StringValue() != "simple" || mt.GetStartedEvent() != nil {
				mt.Fatal("projection/simple-collation/single request missing")
			}
		})
	}
}

func TestFrozenRuleFreshPrecheckKeepsProtectionAndUnknownIntent(t *testing.T) {
	s, _, _, _, m, _, dir := graphEngineFixture(t)
	j, err := openGraphJournal(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.close() }()
	ctx := graphFrozenRuleDeleteContext(t.Context(), true)
	s.preflights = 0
	if err = graphPrecheck(ctx, s, m, dir, j, false); err != nil || s.preflights != 0 {
		t.Fatal("fresh direct precheck repeated target admission:", err)
	}
	j.latest[graphChunkID(m.Targets[0])] = journalEntry{State: "unknown"}
	if err = graphPrecheck(ctx, s, m, dir, j, false); err == nil {
		t.Fatal("fresh optimization bypassed unknown journal responsibility")
	}
	delete(j.latest, graphChunkID(m.Targets[0]))
	guard, err := readGraphArchivePage(dir, m, m.Guards[0])
	if err != nil {
		t.Fatal(err)
	}
	delete(s.rows, objectID(guard[0].Object))
	if err = graphPrecheck(ctx, s, m, dir, j, false); err == nil {
		t.Fatal("fresh optimization bypassed protected originals")
	}
}
