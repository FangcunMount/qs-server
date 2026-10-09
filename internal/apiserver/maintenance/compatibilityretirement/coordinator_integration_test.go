//go:build integration

package retirement

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"testing"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

func coordinatorNativeFixtureCopies(t *testing.T, sqlEvents, mongoEvents []event.DomainEvent) authFixture {
	t.Helper()
	var f authFixture
	var sqlRows [][][]byte
	var mongoRows [][]byte
	for i, evt := range sqlEvents {
		body, err := domainwire.EncodeEvent(evt)
		if err != nil {
			t.Fatal(err)
		}
		row := fixtureSQLRow(t, body, strconv.Itoa(i+1))
		row[5] = []byte("7")
		sqlRows = append(sqlRows, row)
	}
	f.raw[0], f.expected[0] = fixtureSQLCopy(t, sqlRows, nil)
	f.raw[1], f.expected[1] = aiFixtureCopy(t, AIBridgeCommandSource, nil, nil)
	f.raw[2], f.expected[2] = aiFixtureCopy(t, AILegacyCommandSource, nil, nil)
	for i, evt := range mongoEvents {
		body, err := domainwire.EncodeEvent(evt)
		if err != nil {
			t.Fatal(err)
		}
		row := fixtureMongoRow(t, body, int64(i+1))
		row = setMongoField(row, "org_id", int64(7))
		raw, err := bson.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		mongoRows = append(mongoRows, raw)
	}
	f.raw[3], f.expected[3] = fixtureMongoCopy(t, mongoRows)
	return f
}

func TestHistoricalCoordinatorNativeSixTypesActualBusinessAndBlockedExternalGates(t *testing.T) {
	sqlDB := mongoLocalSQLFixture(t)
	client, db, config := mongoCycleNativeDB(t)
	generated, _ := mongoLocalGeneratedFixture(t, db)
	mongoBatchNativeIndexes(t, db)
	sheet := mongoBatchNativeAssessmentSheet()
	insertMongoLocalSheet(t, db, sheet)
	submitted, err := mongoSubmissionPayload(sheet)
	if err != nil {
		t.Fatal(err)
	}
	at := sheet.FilledAt
	requested := eventpayload.EvaluationRequestedData{OrgID: 7, AssessmentID: 42, TesteeID: 21, QuestionnaireCode: "Q", QuestionnaireVer: "1.0", AnswerSheetID: "10042", ModelKind: "scale", ModelCode: "M", ModelVersion: "1.0", RequestedAt: at}
	retry := requested
	retry.ExpectedAttempt = 1
	retry.AttemptOrigin = "automatic"
	retry.Mode = "next_attempt"
	failed := eventpayload.EvaluationFailedData{OrgID: 7, AssessmentID: 42, TesteeID: 21, FailedAt: at, Reason: "synthetic old failure with missing original Run"}
	committed := event.Event[eventpayload.EvaluationOutcomeCommittedData]{BaseEvent: event.BaseEvent{ID: "native-coordinator-committed", EventTypeValue: "evaluation.outcome.committed", AggregateTypeValue: "Evaluation", AggregateIDValue: "42", OccurredAtValue: at}, Data: eventpayload.EvaluationOutcomeCommittedData{OrgID: 7, AssessmentID: 42, TesteeID: 21, OutcomeID: "9001", EvaluationRunID: "42:1", CommittedAt: at}}
	f := coordinatorNativeFixtureCopies(t, []event.DomainEvent{mongoBatchNativeEvent(t, requested, "evaluation.requested", "native-coordinator-requested"), mongoBatchNativeEvent(t, retry, "evaluation.retry.requested", "native-coordinator-retry"), mongoBatchNativeEvent(t, failed, "evaluation.failed", "native-coordinator-failed"), committed}, []event.DomainEvent{mongoBatchNativeEvent(t, submitted, "answersheet.submitted", "native-coordinator-submitted"), mongoBatchNativeEvent(t, generated, "interpretation.report.generated", "native-coordinator-generated")})
	c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits())
	if err != nil {
		t.Fatal(err)
	}
	p, err := c.NextPage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	handles, err := p.Events()
	if err != nil || len(handles) != 6 {
		t.Fatal("six genuine source types not selected", err)
	}
	var uuid, database string
	if err = sqlDB.Raw("SELECT @@server_uuid,DATABASE()").Row().Scan(&uuid, &database); err != nil {
		t.Fatal(err)
	}
	expected := mongoOwnerHashParts("mysql_database_identity_v1", uuid, database)
	if err = mongoCycleNativeTx(t, client, func(mongoCtx mongo.SessionContext) error {
		return sqlDB.Transaction(func(tx *gorm.DB) error {
			ctx := mongo.NewSessionContext(hostmysql.WithTx(mongoCtx, tx), mongo.SessionFromContext(mongoCtx))
			cycle, e := PrepareSQLResponsibilitySnapshot(ctx, expected, sqlevaluation.DefaultSQLResponsibilityLimits())
			if e != nil {
				return e
			}
			selectors, e := MongoHistoricalSQLBatchSelectors(handles)
			if e != nil {
				return e
			}
			sqlBatch, e := PrepareSQLBusinessOwnerBatch(ctx, cycle, selectors, sqlevaluation.DefaultSQLHistoricalOwnerBatchLimits())
			if e != nil {
				return e
			}
			global, e := PrepareMongoResponsibilitySnapshot(ctx, db, config, mongoCycleTestLimits())
			if e != nil {
				return e
			}
			mongoBatch, e := PrepareMongoHistoricalOwnerBatch(ctx, global, sqlBatch.facts, handles, DefaultMongoHistoricalOwnerBatchLimits())
			if e != nil {
				return e
			}
			if e = c.QualifyPage(context.Background(), p, sqlBatch, mongoBatch); e == nil {
				return ErrCoordinatorInvalid
			}
			if e = c.QualifyPage(ctx, p, sqlBatch, mongoBatch); e != nil {
				return e
			}
			// No candidate can escape before clean second EOF, even after actual local
			// business qualification. Current Run never fills absent source OriginalRun.
			if _, e = c.CandidateRange(0, 128); e != ErrCoordinatorIncomplete {
				return ErrCoordinatorInvalid
			}
			return nil
		}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	}); err != nil {
		t.Fatal(err)
	}
	if page, e := c.NextPage(t.Context()); page != nil || e != io.EOF {
		t.Fatal("missing full second EOF", e)
	}
	rows, err := c.CandidateRange(0, 128)
	if err != nil || len(rows) != 6 {
		t.Fatal(err)
	}
	byType := map[string]HistoricalCandidate{}
	for _, r := range rows {
		byType[r.EventType] = r
		if len(r.RequiredAdapters) == 0 {
			t.Fatal("native candidate lost remaining gates")
		}
	}
	if !byType["evaluation.requested"].LocalQualified || !byType["evaluation.outcome.committed"].LocalQualified || !byType["answersheet.submitted"].LocalQualified || !byType["interpretation.report.generated"].LocalQualified {
		t.Fatal("actual bound original business lost local qualification")
	}
	// These fixture old failure/retry rows deliberately lack a qualifying actual
	// original failure. All six types are covered without hiding real ambiguity.
	if actual := byType["evaluation.outcome.committed"].ActualOriginalRun; actual == nil || actual.RunID != "42:1" || actual.Attempt != 1 {
		t.Fatal("precise actual original Run/attempt lost")
	}
	if byType["evaluation.requested"].ActualOriginalRun != nil || byType["evaluation.failed"].ActualOriginalRun != nil {
		t.Fatal("source omitted Run inferred from current winner")
	}
	if byType["evaluation.retry.requested"].LocalQualified || byType["evaluation.failed"].OriginalRunID != "" {
		t.Fatal("missing old Run fabricated")
	}
	receipt := c.Receipt()
	if !receipt.SourceCoverageComplete || receipt.CandidateCount != 6 || receipt.CASComplete || receipt.DropReady {
		t.Fatal("source coverage confused with stored proof")
	}
	t.Logf("native_types=6 local_qualified=%d local_blocked=%d source_coverage_complete=true cas_complete=false drop_ready=false", receipt.LocallyQualifiedCount, receipt.BlockedLocalCount)
}

func coordinatorNativeAICopy(t *testing.T, table string, rows [][][]byte, columns SQLColumns, boundary SourceBoundary) ([]byte, SourceCopyExpectation) {
	t.Helper()
	raw, expected := aiFixtureCopy(t, table, rows, columns)
	expected.Boundary = boundary
	lineEnd := bytes.IndexByte(raw, '\n')
	if lineEnd < 0 {
		t.Fatal("native private source framing")
	}
	header, err := json.Marshal(sqlSourceHeader{Protocol: SQLSourceProtocol, Columns: columns, Boundary: boundary})
	if err != nil {
		t.Fatal(err)
	}
	return append(append(header, '\n'), raw[lineEnd+1:]...), expected
}

func TestHistoricalCoordinatorNativeEightDeliveredSTARTAndMappedPair(t *testing.T) {
	for _, mapped := range []bool{false, true} {
		t.Run(strconv.FormatBool(mapped), func(t *testing.T) {
			fixture := newAINativeFixture(t, mapped)
			if !mapped {
				for i := 2; i <= 8; i++ {
					request := fixture.graph.request
					request.RequestID = fmt.Sprintf("10000000-0000-4000-8000-%012d", i)
					projection := fixture.graph.projection
					projection.RequestID = request.RequestID
					projection.SessionID = fmt.Sprintf("30000000-0000-4000-8000-%012d", i)
					projection.EventID = fmt.Sprintf("50000000-0000-4000-8000-%012d", i)
					payload, e := json.Marshal(request)
					if e != nil {
						t.Fatal(e)
					}
					projected, e := json.Marshal(projection)
					if e != nil {
						t.Fatal(e)
					}
					aiNativeExec(t, fixture.db, `INSERT INTO ai_bridge_requests(request_id,request_hash,payload,session_id,version,status,projection,organization_id,subject_id,testee_id,created_at,updated_at) VALUES(?,?,CONVERT(? USING utf8mb4),?,?,?,CONVERT(? USING utf8mb4),?,?,?,'2026-10-08 11:12:12.123456','2026-10-08 11:12:14.123456')`, request.RequestID, sourceSHA(payload), payload, projection.SessionID, projection.Version, projection.Status, projected, request.Actor.OrgID, request.Actor.SubjectID, request.TesteeID)
					aiNativeExec(t, fixture.db, `INSERT INTO ai_bridge_commands(command_id,request_id,kind,payload,payload_hash,delivered,attempts,available_at) SELECT request_id,request_id,'start',payload,request_hash,1,3,'2026-10-08 11:12:13.123456' FROM ai_bridge_requests WHERE request_id=?`, request.RequestID)
					for _, id := range request.AssessmentIDs {
						aiNativeExec(t, fixture.db, "INSERT INTO ai_bridge_request_assessments VALUES(?,?)", request.RequestID, id)
					}
					aiNativeExec(t, fixture.db, "INSERT INTO ai_bridge_events VALUES(?,?,?,?)", projection.EventID, request.RequestID, projection.Version, sourceSHA(projected))
				}
			}
			binding := aiNativeBinding(t, fixture.db)
			reader := &AILocalResolver{pool: fixture.db}
			var bridges, legacy [][][]byte
			count := 8
			if mapped {
				count = 1
			}
			for i := 1; i <= count; i++ {
				id := fmt.Sprintf("10000000-0000-4000-8000-%012d", i)
				actual, e := reader.read(t.Context(), aiBridgeQuery, id, id)
				if e != nil || len(actual) != 1 {
					t.Fatal("actual bridge row read")
				}
				bridges = append(bridges, actual[0])
				if mapped {
					actual, e = reader.read(t.Context(), aiLegacyQuery, id, id)
					if e != nil || len(actual) != 1 {
						t.Fatal("actual legacy row read")
					}
					legacy = append(legacy, actual[0])
				}
			}
			var f authFixture
			f.raw[0], f.expected[0] = fixtureSQLCopy(t, nil, nil)
			f.raw[3], f.expected[3] = fixtureMongoCopy(t, nil)
			f.raw[1], f.expected[1] = coordinatorNativeAICopy(t, AIBridgeCommandSource, bridges, binding.BridgeColumns, binding.BridgeBoundary)
			f.raw[2], f.expected[2] = coordinatorNativeAICopy(t, AILegacyCommandSource, legacy, binding.LegacyColumns, binding.LegacyBoundary)
			c, e := PrepareHistoricalCoordinator(t.Context(), HistoricalCoordinatorBinding{binding.SourceSHA, binding.OperationID}, f.inputs(), DefaultHistoricalCoordinatorLimits())
			if e != nil {
				t.Fatal(e)
			}
			page, e := c.NextPage(t.Context())
			if e != nil {
				t.Fatal(e)
			}
			tx := aiNativeTx(t, fixture.db)
			resolver, e := NewAILocalResolver(t.Context(), aiLocalGORM(tx), binding)
			if e != nil {
				t.Fatal(e)
			}
			if e = c.QualifyAIPage(t.Context(), page, resolver); e != nil {
				t.Fatal(e)
			}
			if next, e := c.NextPage(t.Context()); next != nil || e != io.EOF {
				t.Fatal("missing complete AI two-source EOF", e)
			}
			values, e := c.CandidateRange(0, 128)
			if e != nil {
				t.Fatal(e)
			}
			for _, v := range values {
				if v.AIRequestID != v.OriginalID || v.AIResourceID != v.OriginalID || v.AISubjectID == "" || v.AISourceAttempts != 3 || v.AIHandoffBudgetFloor != 3 || v.AIAdmissionRevision != 7 || !evidenceHash(v.ContentDigest.SHA256) || !evidenceHash(v.WriterPayloadDigest.SHA256) {
					t.Fatal("native original identity/budget/digest layers lost")
				}
				if !v.LocalQualified || v.LocalClassification != "candidate_historical_gap_requires_joint_closure" || len(v.HistoricalGaps) < 4 || len(v.RequiredAdapters) < 10 {
					t.Fatal("AI local actual graph either lost or became external acceptance")
				}
			}
			r := c.Receipt()
			want := [4]uint64{0, uint64(count), 0, 0}
			if mapped {
				want[2] = 1
			}
			if r.ConsumedRecords != want || r.CandidateCount != want[1]+want[2] || !r.SourceCoverageComplete || r.CASComplete || r.DropReady {
				t.Fatal("AI source rows collapsed or delivered became accepted")
			}
			if e = tx.Rollback(); e != nil {
				t.Fatal("host transaction no longer owned")
			}
			var retired int
			if e = fixture.db.QueryRow("SELECT COUNT(*) FROM ai_messaging_operations WHERE retired<>0").Scan(&retired); e != nil || retired != 0 {
				t.Fatal("read coordinator wrote retirement")
			}
			t.Logf("native_bridge=%d native_legacy=%d local_qualified=%d external_closure=unknown cas_complete=false drop_ready=false", r.ConsumedRecords[1], r.ConsumedRecords[2], r.LocallyQualifiedCount)
		})
	}
}

func TestHistoricalCoordinatorNativeSourceMutationAndTTL(t *testing.T) {
	// This source test deliberately shares the native-tag contract without DB
	// writes, exercising exact whole-copy framing alongside native business gates.
	TestHistoricalCoordinatorSecondPassTruncationAndModificationNeverComplete(t)
	TestHistoricalCoordinatorPageReplayForeignAndExpired(t)
}
