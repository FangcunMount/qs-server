package statistics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	statisticsDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gorm.io/gorm"
)

const collectorBatchSize = 500

func scanStableBatches[T any](
	from time.Time,
	rows *[]T,
	fetch func(lastAt time.Time, lastID uint64) error,
	cursor func(T) (time.Time, uint64),
	handle func([]T) error,
) error {
	lastAt, lastID := from, uint64(0)
	for {
		*rows = (*rows)[:0]
		if err := fetch(lastAt, lastID); err != nil {
			return err
		}
		if len(*rows) == 0 {
			return nil
		}
		batch := append([]T(nil), (*rows)...)
		if err := handle(batch); err != nil {
			return err
		}
		lastAt, lastID = cursor(batch[len(batch)-1])
		if len(batch) < collectorBatchSize {
			return nil
		}
	}
}

func hashCore(values map[string]any) string {
	copyValues := make(map[string]any, len(values))
	for key, value := range values {
		if key != "payload_json" && key != "created_at" && key != "core_hash" {
			copyValues[key] = value
		}
	}
	payload, _ := json.Marshal(copyValues)
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func baseFact(orgID int64, key, factType string, occurredAt time.Time, sourceType, sourceRef string) map[string]any {
	return map[string]any{"org_id": orgID, "fact_key": key, "fact_type": factType, "occurred_at": occurredAt, "stat_date": statisticsDomain.BusinessDate(occurredAt), "source_type": sourceType, "source_ref": sourceRef, "schema_version": 1}
}

type AccessFactCollector struct {
	db     *gorm.DB
	writer factWriter
}

func NewAccessFactCollector(db *gorm.DB) *AccessFactCollector {
	return &AccessFactCollector{db: db, writer: factWriter{db}}
}
func (*AccessFactCollector) Name() string { return "access" }

func (c *AccessFactCollector) Collect(ctx context.Context, req statisticsDomain.CollectRequest) (statisticsDomain.CollectResult, error) {
	result := statisticsDomain.CollectResult{Collector: c.Name(), FactTypeCounts: map[string]int64{}}
	type resolveRow struct {
		ID, ClinicianID, EntryID uint64
		ResolvedAt               time.Time
	}
	var resolves []resolveRow
	if err := scanStableBatches(req.Window.From, &resolves, func(lastAt time.Time, lastID uint64) error {
		return c.db.WithContext(ctx).Table("assessment_entry_resolve_log").
			Select("id,clinician_id,entry_id,resolved_at").
			Where("org_id=? AND resolved_at>=? AND resolved_at<? AND deleted_at IS NULL", req.OrgID, req.Window.From, req.Window.To).
			Where("(resolved_at>? OR (resolved_at=? AND id>?))", lastAt, lastAt, lastID).
			Order("resolved_at,id").Limit(collectorBatchSize).Find(&resolves).Error
	}, func(row resolveRow) (time.Time, uint64) { return row.ResolvedAt, row.ID }, func(batch []resolveRow) error {
		candidates := make([]factCandidate, 0, len(batch))
		for _, row := range batch {
			fact := baseFact(req.OrgID, fmt.Sprintf("entry_resolve:%d:entry_opened", row.ID), "entry_opened", row.ResolvedAt, "entry_resolve", strconv.FormatUint(row.ID, 10))
			fact["clinician_id"] = row.ClinicianID
			fact["entry_id"] = row.EntryID
			candidates = append(candidates, factCandidate{SourceID: row.ID, FactType: "entry_opened", Values: fact})
		}
		return writeFactCandidates(ctx, c.writer, "statistics_access_fact", candidates, req.Mode == statisticsDomain.CollectModeValidate, &result)
	}); err != nil {
		return result, err
	}
	type intakeRow struct {
		ID, ClinicianID, EntryID, TesteeID uint64
		TesteeCreated, AssignmentCreated   bool
		IntakeAt                           time.Time
	}
	var intakes []intakeRow
	if err := scanStableBatches(req.Window.From, &intakes, func(lastAt time.Time, lastID uint64) error {
		return c.db.WithContext(ctx).Table("assessment_entry_intake_log").
			Select("id,clinician_id,entry_id,testee_id,testee_created,assignment_created,intake_at").
			Where("org_id=? AND intake_at>=? AND intake_at<? AND deleted_at IS NULL", req.OrgID, req.Window.From, req.Window.To).
			Where("(intake_at>? OR (intake_at=? AND id>?))", lastAt, lastAt, lastID).
			Order("intake_at,id").Limit(collectorBatchSize).Find(&intakes).Error
	}, func(row intakeRow) (time.Time, uint64) { return row.IntakeAt, row.ID }, func(batch []intakeRow) error {
		candidates := make([]factCandidate, 0, len(batch)*2)
		for _, row := range batch {
			types := []string{"intake_confirmed"}
			if row.TesteeCreated {
				types = append(types, "testee_created")
			}
			if row.AssignmentCreated {
				types = append(types, "care_relationship_established")
			}
			for _, typ := range types {
				fact := baseFact(req.OrgID, fmt.Sprintf("entry_intake:%d:%s", row.ID, typ), typ, row.IntakeAt, "entry_intake", strconv.FormatUint(row.ID, 10))
				fact["clinician_id"] = row.ClinicianID
				fact["entry_id"] = row.EntryID
				fact["testee_id"] = row.TesteeID
				candidates = append(candidates, factCandidate{SourceID: row.ID, FactType: typ, Values: fact})
			}
		}
		return writeFactCandidates(ctx, c.writer, "statistics_access_fact", candidates, req.Mode == statisticsDomain.CollectModeValidate, &result)
	}); err != nil {
		return result, err
	}
	type transferRow struct {
		ID, ClinicianID, TesteeID, SourceClinicianID uint64
		BoundAt                                      time.Time
	}
	lastTransferAt, lastTransferID := req.Window.From, uint64(0)
	for {
		var transfers []transferRow
		if err := c.db.WithContext(ctx).Raw(`SELECT r.id,r.clinician_id,r.testee_id,r.bound_at,
		(SELECT old.clinician_id FROM clinician_relation old
		 WHERE old.org_id=r.org_id AND old.testee_id=r.testee_id AND old.relation_type=r.relation_type
		   AND old.clinician_id<>r.clinician_id AND old.unbound_at IS NOT NULL AND old.unbound_at<=r.bound_at
		 ORDER BY old.unbound_at DESC,old.id DESC LIMIT 1) source_clinician_id
		FROM clinician_relation r
		WHERE r.org_id=? AND r.bound_at>=? AND r.bound_at<? AND r.deleted_at IS NULL
		  AND (r.bound_at>? OR (r.bound_at=? AND r.id>?))
		HAVING source_clinician_id IS NOT NULL
		ORDER BY r.bound_at,r.id LIMIT ?`, req.OrgID, req.Window.From, req.Window.To, lastTransferAt, lastTransferAt, lastTransferID, collectorBatchSize).Scan(&transfers).Error; err != nil {
			return result, err
		}
		if len(transfers) == 0 {
			break
		}
		candidates := make([]factCandidate, 0, len(transfers))
		for _, row := range transfers {
			fact := baseFact(req.OrgID, fmt.Sprintf("clinician_relation:%d:transferred", row.ID), "care_relationship_transferred", row.BoundAt, "clinician_relation", strconv.FormatUint(row.ID, 10))
			fact["clinician_id"], fact["source_clinician_id"], fact["testee_id"] = row.ClinicianID, row.SourceClinicianID, row.TesteeID
			candidates = append(candidates, factCandidate{SourceID: row.ID, FactType: "care_relationship_transferred", Values: fact})
		}
		if err := writeFactCandidates(ctx, c.writer, "statistics_access_fact", candidates, req.Mode == statisticsDomain.CollectModeValidate, &result); err != nil {
			return result, err
		}
		last := transfers[len(transfers)-1]
		lastTransferAt, lastTransferID = last.BoundAt, last.ID
		if len(transfers) < collectorBatchSize {
			break
		}
	}
	return result, nil
}

type PlanFactCollector struct {
	db     *gorm.DB
	writer factWriter
}

func NewPlanFactCollector(db *gorm.DB) *PlanFactCollector {
	return &PlanFactCollector{db: db, writer: factWriter{db}}
}
func (*PlanFactCollector) Name() string { return "plan" }

type enrollmentEventRow struct {
	ID, PlanID, TesteeID uint64
	OccurredAt           time.Time
}

type taskEventRow struct {
	ID, PlanID, EnrollmentID, TesteeID uint64
	Seq                                int
	ScaleCode                          string
	PlannedAt                          time.Time
	ExpireAt                           *time.Time
	OccurredAt                         time.Time
}

type lifecycleSource struct {
	factType  string
	timeField string
}

func scanLifecycleRows[T any](
	ctx context.Context,
	db *gorm.DB,
	table, selectColumns, timeField string,
	req statisticsDomain.CollectRequest,
	rows *[]T,
	cursor func(T) (time.Time, uint64),
	handle func([]T) error,
) error {
	return scanStableBatches(req.Window.From, rows, func(lastAt time.Time, lastID uint64) error {
		return db.WithContext(ctx).Table(table).
			Select(selectColumns+", "+timeField+" AS occurred_at").
			Where("org_id=? AND deleted_at IS NULL AND "+timeField+">=? AND "+timeField+"<?", req.OrgID, req.Window.From, req.Window.To).
			Where("("+timeField+">? OR ("+timeField+"=? AND id>?))", lastAt, lastAt, lastID).
			Order(timeField + ",id").Limit(collectorBatchSize).Find(rows).Error
	}, cursor, handle)
}

func (c *PlanFactCollector) Collect(ctx context.Context, req statisticsDomain.CollectRequest) (statisticsDomain.CollectResult, error) {
	result := statisticsDomain.CollectResult{Collector: c.Name(), FactTypeCounts: map[string]int64{}}
	for _, source := range []lifecycleSource{
		{factType: "enrollment_joined", timeField: "joined_at"},
		{factType: "enrollment_closed", timeField: "closed_at"},
		{factType: "enrollment_terminated", timeField: "terminated_at"},
	} {
		var rows []enrollmentEventRow
		err := scanLifecycleRows(ctx, c.db, "plan_enrollment", "id,plan_id,testee_id", source.timeField, req, &rows,
			func(row enrollmentEventRow) (time.Time, uint64) { return row.OccurredAt, row.ID },
			func(batch []enrollmentEventRow) error {
				candidates := make([]factCandidate, 0, len(batch))
				for _, row := range batch {
					fact := baseFact(req.OrgID, fmt.Sprintf("enrollment:%d:%s", row.ID, source.factType), source.factType, row.OccurredAt, "plan_enrollment", strconv.FormatUint(row.ID, 10))
					fact["plan_id"], fact["enrollment_id"], fact["testee_id"] = row.PlanID, row.ID, row.TesteeID
					candidates = append(candidates, factCandidate{SourceID: row.ID, FactType: source.factType, Values: fact})
				}
				return writeFactCandidates(ctx, c.writer, "statistics_plan_fact", candidates, req.Mode == statisticsDomain.CollectModeValidate, &result)
			})
		if err != nil {
			return result, err
		}
	}

	for _, source := range []lifecycleSource{
		{factType: "task_created", timeField: "COALESCE(business_created_at,created_at)"},
		{factType: "task_opened", timeField: "open_at"},
		{factType: "task_completed", timeField: "completed_at"},
		{factType: "task_expired", timeField: "expired_at"},
		{factType: "task_canceled", timeField: "canceled_at"},
	} {
		var rows []taskEventRow
		err := scanLifecycleRows(ctx, c.db, "assessment_task", "id,plan_id,enrollment_id,testee_id,seq,scale_code,planned_at,expire_at", source.timeField, req, &rows,
			func(row taskEventRow) (time.Time, uint64) { return row.OccurredAt, row.ID },
			func(batch []taskEventRow) error {
				candidates := make([]factCandidate, 0, len(batch))
				for _, row := range batch {
					fact := baseFact(req.OrgID, fmt.Sprintf("task:%d:%s", row.ID, source.factType), source.factType, row.OccurredAt, "assessment_task", strconv.FormatUint(row.ID, 10))
					fact["plan_id"], fact["enrollment_id"], fact["testee_id"] = row.PlanID, row.EnrollmentID, row.TesteeID
					fact["task_id"], fact["task_seq"], fact["scale_code"] = row.ID, row.Seq, row.ScaleCode
					fact["planned_at"], fact["due_at"] = row.PlannedAt, row.ExpireAt
					applyTaskLifecycleFields(fact, source.factType, &row.OccurredAt)
					candidates = append(candidates, factCandidate{SourceID: row.ID, FactType: source.factType, Values: fact})
				}
				return writeFactCandidates(ctx, c.writer, "statistics_plan_fact", candidates, req.Mode == statisticsDomain.CollectModeValidate, &result)
			})
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

func applyTaskLifecycleFields(fact map[string]any, eventType string, eventAt *time.Time) {
	switch eventType {
	case "task_completed":
		fact["completed_at"] = eventAt
		fact["task_status"] = "completed"
	case "task_expired":
		fact["task_status"] = "expired"
	case "task_canceled":
		fact["task_status"] = "canceled"
	}
}

type AssessmentFactCollector struct {
	db     *gorm.DB
	mongo  *mongo.Database
	writer factWriter
}

type assessmentFactParent struct {
	AnswerSheetID                           uint64
	QuestionnaireCode, QuestionnaireVersion string
}

type assessmentCollectionState struct {
	collector    *AssessmentFactCollector
	orgID        int64
	attributions map[uint64]frozenAnswerSheetAttribution
	assessments  map[uint64]assessmentFactParent
}

func newAssessmentCollectionState(collector *AssessmentFactCollector, orgID int64) *assessmentCollectionState {
	return &assessmentCollectionState{
		collector: collector, orgID: orgID,
		attributions: make(map[uint64]frozenAnswerSheetAttribution),
		assessments:  make(map[uint64]assessmentFactParent),
	}
}

func (s *assessmentCollectionState) rememberAttribution(answerSheetID uint64, value frozenAnswerSheetAttribution) {
	s.attributions[answerSheetID] = value
}

func (s *assessmentCollectionState) loadAttribution(ctx context.Context, answerSheetID uint64) (frozenAnswerSheetAttribution, error) {
	if value, exists := s.attributions[answerSheetID]; exists {
		return value, nil
	}
	value, err := s.collector.loadAnswerSheetAttribution(ctx, s.orgID, answerSheetID)
	if err != nil {
		return frozenAnswerSheetAttribution{}, err
	}
	s.rememberAttribution(answerSheetID, value)
	return value, nil
}

func (s *assessmentCollectionState) rememberAssessment(assessmentID uint64, value assessmentFactParent) {
	s.assessments[assessmentID] = value
}

func (s *assessmentCollectionState) loadAssessment(ctx context.Context, assessmentID uint64) (assessmentFactParent, error) {
	if value, exists := s.assessments[assessmentID]; exists {
		return value, nil
	}
	var value assessmentFactParent
	if err := s.collector.db.WithContext(ctx).Table("assessment").
		Select("answer_sheet_id,questionnaire_code,questionnaire_version").
		Where("org_id=? AND id=?", s.orgID, assessmentID).
		Take(&value).Error; err != nil {
		return assessmentFactParent{}, err
	}
	s.rememberAssessment(assessmentID, value)
	return value, nil
}

type frozenAnswerSheetAttribution struct {
	OriginType, OriginID, ClinicianID, EntryID, PlanID, EnrollmentID, TaskID, Mode string
}

func (c *AssessmentFactCollector) loadAnswerSheetAttribution(ctx context.Context, orgID int64, answerSheetID uint64) (frozenAnswerSheetAttribution, error) {
	var row struct {
		TaskID      string `bson:"task_id"`
		Attribution *struct {
			OriginType   string `bson:"origin_type"`
			OriginID     string `bson:"origin_id"`
			ClinicianID  string `bson:"clinician_id"`
			EntryID      string `bson:"entry_id"`
			PlanID       string `bson:"plan_id"`
			EnrollmentID string `bson:"enrollment_id"`
			TaskID       string `bson:"task_id"`
			Mode         string `bson:"mode"`
		} `bson:"attribution"`
	}
	err := c.mongo.Collection("answersheets").FindOne(ctx, bson.M{"org_id": uint64(orgID), "domain_id": answerSheetID, "deleted_at": nil}, options.FindOne().SetProjection(bson.M{"attribution": 1, "task_id": 1})).Decode(&row)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return frozenAnswerSheetAttribution{Mode: "unknown"}, nil
		}
		return frozenAnswerSheetAttribution{}, err
	}
	if row.Attribution == nil {
		return c.deriveLegacyAttribution(ctx, orgID, row.TaskID)
	}
	return frozenAnswerSheetAttribution{OriginType: row.Attribution.OriginType, OriginID: row.Attribution.OriginID, ClinicianID: row.Attribution.ClinicianID, EntryID: row.Attribution.EntryID, PlanID: row.Attribution.PlanID, EnrollmentID: row.Attribution.EnrollmentID, TaskID: row.Attribution.TaskID, Mode: row.Attribution.Mode}, nil
}

// deriveLegacyAttribution only copies relationships that are directly proven
// by the historical task record. It deliberately leaves Entry and Clinician
// unknown because resolving them from current Actor state would fabricate a
// historical fact. Historical bias is explicit through derived_legacy.
func (c *AssessmentFactCollector) deriveLegacyAttribution(ctx context.Context, orgID int64, rawTaskID string) (frozenAnswerSheetAttribution, error) {
	value := frozenAnswerSheetAttribution{Mode: "derived_legacy"}
	taskID, err := strconv.ParseUint(rawTaskID, 10, 64)
	if err != nil || taskID == 0 {
		return value, nil
	}
	var row struct {
		ID, PlanID, EnrollmentID uint64
	}
	err = c.db.WithContext(ctx).Table("assessment_task").Select("id,plan_id,enrollment_id").Where("org_id=? AND id=?", orgID, taskID).Take(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return value, nil
		}
		return frozenAnswerSheetAttribution{}, err
	}
	value.OriginType = "plan_task"
	value.OriginID = strconv.FormatUint(row.ID, 10)
	value.TaskID = strconv.FormatUint(row.ID, 10)
	value.PlanID = strconv.FormatUint(row.PlanID, 10)
	if row.EnrollmentID != 0 {
		value.EnrollmentID = strconv.FormatUint(row.EnrollmentID, 10)
	}
	return value, nil
}

func applyFrozenAttribution(fact map[string]any, value frozenAnswerSheetAttribution) {
	if value.OriginType != "" {
		fact["origin_type"] = value.OriginType
	}
	if value.OriginID != "" {
		fact["origin_id"] = value.OriginID
	}
	fact["clinician_id"] = parseNullableID(value.ClinicianID)
	fact["entry_id"] = parseNullableID(value.EntryID)
	fact["plan_id"] = parseNullableID(value.PlanID)
	fact["enrollment_id"] = parseNullableID(value.EnrollmentID)
	fact["task_id"] = parseNullableID(value.TaskID)
	if value.Mode != "" {
		fact["attribution_mode"] = value.Mode
	}
}

func NewAssessmentFactCollector(db *gorm.DB, mongoDB *mongo.Database) *AssessmentFactCollector {
	return &AssessmentFactCollector{db: db, mongo: mongoDB, writer: factWriter{db}}
}
func (*AssessmentFactCollector) Name() string { return "assessment" }
func (c *AssessmentFactCollector) Collect(ctx context.Context, req statisticsDomain.CollectRequest) (statisticsDomain.CollectResult, error) {
	result := statisticsDomain.CollectResult{Collector: c.Name(), FactTypeCounts: map[string]int64{}}
	if c.mongo == nil {
		return result, fmt.Errorf("mongo database is required")
	}
	state := newAssessmentCollectionState(c, req.OrgID)
	cursor, err := c.mongo.Collection("answersheets").Find(ctx, bson.M{"org_id": uint64(req.OrgID), "deleted_at": nil, "filled_at": bson.M{"$gte": req.Window.From, "$lt": req.Window.To}}, options.Find().SetSort(bson.D{{Key: "filled_at", Value: 1}, {Key: "domain_id", Value: 1}}).SetBatchSize(collectorBatchSize))
	if err != nil {
		return result, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	candidates := make([]factCandidate, 0, collectorBatchSize)
	flushCandidates := func() error {
		err := writeFactCandidates(ctx, c.writer, "statistics_assessment_fact", candidates, req.Mode == statisticsDomain.CollectModeValidate, &result)
		candidates = candidates[:0]
		return err
	}
	for cursor.Next(ctx) {
		var row struct {
			DomainID             uint64    `bson:"domain_id"`
			TesteeID             uint64    `bson:"testee_id"`
			FillerID             int64     `bson:"filler_id"`
			QuestionnaireCode    string    `bson:"questionnaire_code"`
			QuestionnaireVersion string    `bson:"questionnaire_version"`
			FilledAt             time.Time `bson:"filled_at"`
			TaskID               string    `bson:"task_id"`
			Admission            *struct {
				ModelKind    string `bson:"model_kind"`
				ModelCode    string `bson:"model_code"`
				ModelVersion string `bson:"model_version"`
			} `bson:"admission"`
			Attribution *struct {
				OriginType   string    `bson:"origin_type"`
				OriginID     string    `bson:"origin_id"`
				ClinicianID  string    `bson:"clinician_id"`
				EntryID      string    `bson:"entry_id"`
				PlanID       string    `bson:"plan_id"`
				EnrollmentID string    `bson:"enrollment_id"`
				TaskID       string    `bson:"task_id"`
				Mode         string    `bson:"mode"`
				CapturedAt   time.Time `bson:"captured_at"`
			} `bson:"attribution"`
		}
		if err := cursor.Decode(&row); err != nil {
			if writeErr := flushCandidates(); writeErr != nil {
				return result, writeErr
			}
			return result, err
		}
		fact := baseFact(req.OrgID, fmt.Sprintf("answersheet:%d:submitted", row.DomainID), "answersheet_submitted", row.FilledAt, "answersheet", strconv.FormatUint(row.DomainID, 10))
		fact["answersheet_id"] = row.DomainID
		fact["testee_id"] = row.TesteeID
		fact["filler_id"] = uint64(row.FillerID)
		fact["questionnaire_code"] = row.QuestionnaireCode
		fact["questionnaire_version"] = row.QuestionnaireVersion
		if row.Admission != nil {
			fact["model_kind"] = row.Admission.ModelKind
			fact["model_code"] = row.Admission.ModelCode
			fact["model_version"] = row.Admission.ModelVersion
		}
		var attribution frozenAnswerSheetAttribution
		if row.Attribution != nil {
			attribution = frozenAnswerSheetAttribution{
				OriginType: row.Attribution.OriginType, OriginID: row.Attribution.OriginID,
				ClinicianID: row.Attribution.ClinicianID, EntryID: row.Attribution.EntryID,
				PlanID: row.Attribution.PlanID, EnrollmentID: row.Attribution.EnrollmentID,
				TaskID: row.Attribution.TaskID, Mode: row.Attribution.Mode,
			}
			fact["origin_type"] = row.Attribution.OriginType
			fact["origin_id"] = row.Attribution.OriginID
			fact["clinician_id"] = parseNullableID(row.Attribution.ClinicianID)
			fact["entry_id"] = parseNullableID(row.Attribution.EntryID)
			fact["plan_id"] = parseNullableID(row.Attribution.PlanID)
			fact["enrollment_id"] = parseNullableID(row.Attribution.EnrollmentID)
			fact["task_id"] = parseNullableID(row.Attribution.TaskID)
			fact["attribution_mode"] = row.Attribution.Mode
		} else {
			var deriveErr error
			attribution, deriveErr = c.deriveLegacyAttribution(ctx, req.OrgID, row.TaskID)
			if deriveErr != nil {
				if writeErr := flushCandidates(); writeErr != nil {
					return result, writeErr
				}
				return result, deriveErr
			}
			applyFrozenAttribution(fact, attribution)
		}
		state.rememberAttribution(row.DomainID, attribution)
		candidates = append(candidates, factCandidate{SourceID: row.DomainID, FactType: "answersheet_submitted", Values: fact})
		if len(candidates) == collectorBatchSize {
			if err := flushCandidates(); err != nil {
				return result, err
			}
		}
	}
	if err := flushCandidates(); err != nil {
		return result, err
	}
	if err := cursor.Err(); err != nil {
		return result, err
	}
	if err := c.collectAssessmentMySQL(ctx, req, &result, state); err != nil {
		return result, err
	}
	if err := c.collectReports(ctx, req, &result, state); err != nil {
		return result, err
	}
	if err := c.collectReportFailures(ctx, req, &result, state); err != nil {
		return result, err
	}
	return result, nil
}

func (c *AssessmentFactCollector) collectAssessmentMySQL(ctx context.Context, req statisticsDomain.CollectRequest, result *statisticsDomain.CollectResult, state *assessmentCollectionState) error {
	type assessmentRow struct {
		ID, TesteeID, AnswerSheetID                         uint64
		QuestionnaireCode, QuestionnaireVersion, OriginType string
		OriginID, ModelKind, ModelCode, ModelVersion        *string
		OccurredAt                                          time.Time
	}
	for _, source := range []lifecycleSource{
		{factType: "assessment_created", timeField: "created_at"},
		{factType: "assessment_failed", timeField: "failed_at"},
	} {
		var rows []assessmentRow
		err := scanLifecycleRows(ctx, c.db, "assessment", "id,testee_id,answer_sheet_id,questionnaire_code,questionnaire_version,origin_type,origin_id,evaluation_model_kind model_kind,evaluation_model_code model_code,evaluation_model_version model_version", source.timeField, req, &rows,
			func(row assessmentRow) (time.Time, uint64) { return row.OccurredAt, row.ID },
			func(batch []assessmentRow) error {
				candidates := make([]factCandidate, 0, len(batch))
				for _, row := range batch {
					state.rememberAssessment(row.ID, assessmentFactParent{
						AnswerSheetID: row.AnswerSheetID, QuestionnaireCode: row.QuestionnaireCode,
						QuestionnaireVersion: row.QuestionnaireVersion,
					})
					attribution, loadErr := state.loadAttribution(ctx, row.AnswerSheetID)
					if loadErr != nil {
						return loadErr
					}
					fact := baseFact(req.OrgID, fmt.Sprintf("assessment:%d:%s", row.ID, source.factType), source.factType, row.OccurredAt, "assessment", strconv.FormatUint(row.ID, 10))
					fact["assessment_id"] = row.ID
					fact["answersheet_id"] = row.AnswerSheetID
					fact["testee_id"] = row.TesteeID
					fact["questionnaire_code"] = row.QuestionnaireCode
					fact["questionnaire_version"] = row.QuestionnaireVersion
					fact["origin_type"] = row.OriginType
					fact["origin_id"] = row.OriginID
					fact["model_kind"] = row.ModelKind
					fact["model_code"] = row.ModelCode
					fact["model_version"] = row.ModelVersion
					applyFrozenAttribution(fact, attribution)
					candidates = append(candidates, factCandidate{SourceID: row.ID, FactType: source.factType, Values: fact})
				}
				return writeFactCandidates(ctx, c.writer, "statistics_assessment_fact", candidates, req.Mode == statisticsDomain.CollectModeValidate, result)
			})
		if err != nil {
			return err
		}
	}
	type outcomeRow struct {
		ID, AssessmentID, TesteeID, AnswerSheetID uint64
		ModelKind, ModelCode, ModelVersion        string
		QuestionnaireCode, QuestionnaireVersion   string
		EvaluatedAt                               time.Time
	}
	var outcomes []outcomeRow
	if err := scanStableBatches(req.Window.From, &outcomes, func(lastAt time.Time, lastID uint64) error {
		return c.db.WithContext(ctx).Table("evaluation_outcome o").
			Select("o.id,o.assessment_id,o.testee_id,o.model_kind,o.model_code,o.model_version,o.evaluated_at,a.answer_sheet_id,a.questionnaire_code,a.questionnaire_version").
			Joins("JOIN assessment a ON a.id=o.assessment_id AND a.org_id=o.org_id").
			Where("o.org_id=? AND o.evaluated_at>=? AND o.evaluated_at<?", req.OrgID, req.Window.From, req.Window.To).
			Where("(o.evaluated_at>? OR (o.evaluated_at=? AND o.id>?))", lastAt, lastAt, lastID).
			Order("o.evaluated_at,o.id").Limit(collectorBatchSize).Find(&outcomes).Error
	}, func(row outcomeRow) (time.Time, uint64) { return row.EvaluatedAt, row.ID }, func(batch []outcomeRow) error {
		candidates := make([]factCandidate, 0, len(batch))
		for _, row := range batch {
			state.rememberAssessment(row.AssessmentID, assessmentFactParent{
				AnswerSheetID: row.AnswerSheetID, QuestionnaireCode: row.QuestionnaireCode,
				QuestionnaireVersion: row.QuestionnaireVersion,
			})
			attribution, err := state.loadAttribution(ctx, row.AnswerSheetID)
			if err != nil {
				return err
			}
			fact := baseFact(req.OrgID, fmt.Sprintf("outcome:%d:committed", row.ID), "outcome_committed", row.EvaluatedAt, "evaluation_outcome", strconv.FormatUint(row.ID, 10))
			fact["outcome_id"] = row.ID
			fact["assessment_id"] = row.AssessmentID
			fact["testee_id"] = row.TesteeID
			fact["model_kind"] = row.ModelKind
			fact["model_code"] = row.ModelCode
			fact["model_version"] = row.ModelVersion
			fact["answersheet_id"] = row.AnswerSheetID
			fact["questionnaire_code"] = row.QuestionnaireCode
			fact["questionnaire_version"] = row.QuestionnaireVersion
			applyFrozenAttribution(fact, attribution)
			candidates = append(candidates, factCandidate{SourceID: row.ID, FactType: "outcome_committed", Values: fact})
		}
		return writeFactCandidates(ctx, c.writer, "statistics_assessment_fact", candidates, req.Mode == statisticsDomain.CollectModeValidate, result)
	}); err != nil {
		return err
	}
	return nil
}

func (c *AssessmentFactCollector) collectReports(ctx context.Context, req statisticsDomain.CollectRequest, result *statisticsDomain.CollectResult, state *assessmentCollectionState) error {
	cursor, err := c.mongo.Collection("interpret_report_artifacts").Find(ctx, bson.M{"org_id": req.OrgID, "generated_at": bson.M{"$gte": req.Window.From, "$lt": req.Window.To}}, options.Find().SetSort(bson.D{{Key: "generated_at", Value: 1}, {Key: "domain_id", Value: 1}}).SetBatchSize(collectorBatchSize))
	if err != nil {
		return err
	}
	defer func() { _ = cursor.Close(ctx) }()
	candidates := make([]factCandidate, 0, collectorBatchSize)
	flushCandidates := func() error {
		err := writeFactCandidates(ctx, c.writer, "statistics_assessment_fact", candidates, req.Mode == statisticsDomain.CollectModeValidate, result)
		candidates = candidates[:0]
		return err
	}
	for cursor.Next(ctx) {
		var row struct {
			DomainID     uint64    `bson:"domain_id"`
			OutcomeID    uint64    `bson:"outcome_id"`
			AssessmentID uint64    `bson:"assessment_id"`
			TesteeID     uint64    `bson:"testee_id"`
			GeneratedAt  time.Time `bson:"generated_at"`
			Model        *struct {
				Kind    string `bson:"kind"`
				Code    string `bson:"code"`
				Version string `bson:"version"`
			} `bson:"model"`
		}
		if err := cursor.Decode(&row); err != nil {
			if writeErr := flushCandidates(); writeErr != nil {
				return writeErr
			}
			return err
		}
		assessment, err := state.loadAssessment(ctx, row.AssessmentID)
		if err != nil {
			if writeErr := flushCandidates(); writeErr != nil {
				return writeErr
			}
			return err
		}
		attribution, err := state.loadAttribution(ctx, assessment.AnswerSheetID)
		if err != nil {
			if writeErr := flushCandidates(); writeErr != nil {
				return writeErr
			}
			return err
		}
		fact := baseFact(req.OrgID, fmt.Sprintf("report:%d:generated", row.DomainID), "report_generated", row.GeneratedAt, "interpret_report", strconv.FormatUint(row.DomainID, 10))
		fact["report_id"] = row.DomainID
		fact["outcome_id"] = row.OutcomeID
		fact["assessment_id"] = row.AssessmentID
		fact["testee_id"] = row.TesteeID
		fact["answersheet_id"] = assessment.AnswerSheetID
		fact["questionnaire_code"] = assessment.QuestionnaireCode
		fact["questionnaire_version"] = assessment.QuestionnaireVersion
		applyFrozenAttribution(fact, attribution)
		if row.Model != nil {
			fact["model_kind"] = row.Model.Kind
			fact["model_code"] = row.Model.Code
			fact["model_version"] = row.Model.Version
		}
		candidates = append(candidates, factCandidate{SourceID: row.DomainID, FactType: "report_generated", Values: fact})
		if len(candidates) == collectorBatchSize {
			if err := flushCandidates(); err != nil {
				return err
			}
		}
	}
	if err := flushCandidates(); err != nil {
		return err
	}
	return cursor.Err()
}

func (c *AssessmentFactCollector) collectReportFailures(ctx context.Context, req statisticsDomain.CollectRequest, result *statisticsDomain.CollectResult, state *assessmentCollectionState) error {
	cursor, err := c.mongo.Collection("interpretation_runs").Find(ctx, bson.M{"org_id": req.OrgID, "status": "failed", "finished_at": bson.M{"$gte": req.Window.From, "$lt": req.Window.To}}, options.Find().SetSort(bson.D{{Key: "finished_at", Value: 1}, {Key: "domain_id", Value: 1}}).SetBatchSize(collectorBatchSize))
	if err != nil {
		return err
	}
	defer func() { _ = cursor.Close(ctx) }()
	candidates := make([]factCandidate, 0, collectorBatchSize)
	flushCandidates := func() error {
		err := writeFactCandidates(ctx, c.writer, "statistics_assessment_fact", candidates, req.Mode == statisticsDomain.CollectModeValidate, result)
		candidates = candidates[:0]
		return err
	}
	for cursor.Next(ctx) {
		var run struct {
			DomainID     uint64     `bson:"domain_id"`
			GenerationID uint64     `bson:"generation_id"`
			FinishedAt   *time.Time `bson:"finished_at"`
		}
		if err := cursor.Decode(&run); err != nil {
			if writeErr := flushCandidates(); writeErr != nil {
				return writeErr
			}
			return err
		}
		if run.FinishedAt == nil {
			continue
		}
		var generation struct {
			OutcomeID uint64 `bson:"outcome_id"`
		}
		if err := c.mongo.Collection("report_generations").FindOne(ctx, bson.M{"domain_id": run.GenerationID}, options.FindOne().SetProjection(bson.M{"outcome_id": 1})).Decode(&generation); err != nil {
			if writeErr := flushCandidates(); writeErr != nil {
				return writeErr
			}
			return err
		}
		var source struct {
			OrgID                                                                       int64
			AssessmentID, TesteeID, AnswerSheetID                                       uint64
			ModelKind, ModelCode, ModelVersion, QuestionnaireCode, QuestionnaireVersion string
		}
		if err := c.db.WithContext(ctx).Table("evaluation_outcome o").Select("o.org_id,o.assessment_id,o.testee_id,o.model_kind,o.model_code,o.model_version,a.answer_sheet_id,a.questionnaire_code,a.questionnaire_version").Joins("JOIN assessment a ON a.id=o.assessment_id AND a.org_id=o.org_id").Where("o.id=?", generation.OutcomeID).Take(&source).Error; err != nil {
			if writeErr := flushCandidates(); writeErr != nil {
				return writeErr
			}
			return err
		}
		if source.OrgID != req.OrgID {
			continue
		}
		state.rememberAssessment(source.AssessmentID, assessmentFactParent{
			AnswerSheetID: source.AnswerSheetID, QuestionnaireCode: source.QuestionnaireCode,
			QuestionnaireVersion: source.QuestionnaireVersion,
		})
		attribution, err := state.loadAttribution(ctx, source.AnswerSheetID)
		if err != nil {
			if writeErr := flushCandidates(); writeErr != nil {
				return writeErr
			}
			return err
		}
		fact := baseFact(req.OrgID, fmt.Sprintf("interpretation_run:%d:failed", run.DomainID), "report_failed", *run.FinishedAt, "interpretation_run", strconv.FormatUint(run.DomainID, 10))
		fact["outcome_id"], fact["assessment_id"], fact["testee_id"], fact["answersheet_id"] = generation.OutcomeID, source.AssessmentID, source.TesteeID, source.AnswerSheetID
		fact["model_kind"], fact["model_code"], fact["model_version"] = source.ModelKind, source.ModelCode, source.ModelVersion
		fact["questionnaire_code"], fact["questionnaire_version"] = source.QuestionnaireCode, source.QuestionnaireVersion
		applyFrozenAttribution(fact, attribution)
		candidates = append(candidates, factCandidate{SourceID: run.DomainID, FactType: "report_failed", Values: fact})
		if len(candidates) == collectorBatchSize {
			if err := flushCandidates(); err != nil {
				return err
			}
		}
	}
	if err := flushCandidates(); err != nil {
		return err
	}
	return cursor.Err()
}

func parseNullableID(raw string) any {
	if raw == "" {
		return nil
	}
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return nil
	}
	return id
}
