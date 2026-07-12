package operator

import (
	"context"
	"time"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	evaluationoutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelbinding "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	"github.com/FangcunMount/qs-server/internal/pkg/outboxpolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type AccessScope struct {
	IsAdmin     bool
	ClinicianID *uint64
}

// AccessChecker is the narrow authorization port owned by the Operator use cases.
type AccessChecker interface {
	ResolveAccessScope(context.Context, int64, int64) (*AccessScope, error)
	ValidateTesteeAccess(context.Context, int64, int64, uint64) error
	ListAccessibleTesteeIDs(context.Context, int64, int64) ([]uint64, error)
}

type EventStager interface {
	Stage(context.Context, ...event.DomainEvent) error
}

type ListQuery struct {
	Page, PageSize        int
	TesteeID              *uint64
	Status                string
	AccessibleTesteeIDs   []uint64
	RestrictToAccessScope bool
}
type TesteeListScope struct {
	TesteeID            uint64
	AccessibleTesteeIDs []uint64
	Restricted          bool
}

type TrendQuery struct {
	TesteeID   uint64
	FactorCode string
	Limit      int
}

type ModelIdentity struct {
	Kind, SubKind, Algorithm, Code, Version, Title, ProductChannel, AlgorithmFamily string
}
type ScoreValue struct {
	Kind  string
	Value float64
	Label string
	Max   *float64
}
type ResultLevel struct{ Code, Label, Severity string }

type Assessment struct {
	ID, OrgID, TesteeID, AnswerSheetID      uint64
	QuestionnaireCode, QuestionnaireVersion string
	ModelKind, ModelSubKind, ModelAlgorithm *string
	ModelCode, ModelVersion, ModelTitle     *string
	OriginType                              string
	OriginID                                *string
	Status                                  string
	TotalScore                              *float64
	RiskLevel                               *string
	SubmittedAt, EvaluatedAt, FailedAt      *time.Time
	FailureReason                           *string
}
type AssessmentList struct {
	Items                             []*Assessment
	Total, Page, PageSize, TotalPages int
}
type OutcomeAssessment struct {
	ID, OrgID, TesteeID, AnswerSheetID      uint64
	QuestionnaireCode, QuestionnaireVersion string
	Model                                   ModelIdentity
	PrimaryScore                            *ScoreValue
	Level                                   *ResultLevel
	OriginType                              string
	OriginID                                *string
	Status                                  string
	SubmittedAt, FailedAt                   *time.Time
	FailureReason                           *string
}
type OutcomeAssessmentList struct {
	Items                             []*OutcomeAssessment
	Total, Page, PageSize, TotalPages int
}
type FactorScore struct {
	FactorCode, FactorName string
	RawScore               float64
	MaxScore               *float64
	RiskLevel              string
	IsTotalScore           bool
}
type Score struct {
	AssessmentID uint64
	TotalScore   float64
	RiskLevel    string
	FactorScores []FactorScore
}
type TrendPoint struct {
	AssessmentID uint64
	RawScore     float64
	RiskLevel    string
}
type FactorTrend struct {
	TesteeID               uint64
	FactorCode, FactorName string
	DataPoints             []TrendPoint
}
type HighRiskFactors struct {
	AssessmentID    uint64
	HasHighRisk     bool
	HighRiskFactors []FactorScore
	NeedsUrgentCare bool
}
type Run struct {
	RunID            string
	AssessmentID     uint64
	AttemptNo        int
	Status           string
	Retryable        bool
	ErrorCode        string
	ErrorMessage     string
	StartedAt        time.Time
	FinishedAt       *time.Time
	TraceID          string
	InputSnapshotRef string
}
type RunList struct{ Items []*Run }
type RetryableFailedRun struct {
	Run
	OrgID int64
}
type RetryableFailedRunList struct {
	Items      []*RetryableFailedRun
	NextCursor uint64
}

type QueryService interface {
	ValidateTesteeAccess(context.Context, Actor, uint64) error
	ScopeTesteeList(context.Context, Actor, uint64) (TesteeListScope, error)
	GetAssessment(context.Context, Actor, uint64) (*Assessment, error)
	ListAssessments(context.Context, Actor, ListQuery) (*AssessmentList, error)
	GetAssessmentOutcome(context.Context, Actor, uint64) (*OutcomeAssessment, error)
	ListAssessmentsOutcome(context.Context, Actor, ListQuery) (*OutcomeAssessmentList, error)
	GetScores(context.Context, Actor, uint64) (*Score, error)
	GetHighRiskFactors(context.Context, Actor, uint64) (*HighRiskFactors, error)
	GetFactorTrend(context.Context, Actor, TrendQuery) (*FactorTrend, error)
	ListAssessmentRuns(context.Context, Actor, uint64, int) (*RunList, error)
	GetLatestAssessmentRun(context.Context, Actor, uint64) (*Run, error)
	ListRetryableFailedRuns(context.Context, Actor, int, uint64) (*RetryableFailedRunList, error)
}

func (s *queryService) ScopeTesteeList(ctx context.Context, actor Actor, testeeID uint64) (TesteeListScope, error) {
	result := TesteeListScope{TesteeID: testeeID}
	if testeeID != 0 {
		return result, s.ValidateTesteeAccess(ctx, actor, testeeID)
	}
	if s.access == nil {
		return result, evalerrors.ModuleNotConfigured("testee access checker is not configured")
	}
	scope, err := s.access.ResolveAccessScope(ctx, actor.OrgID, actor.OperatorUserID)
	if err != nil {
		return result, err
	}
	if scope != nil && scope.IsAdmin {
		return result, nil
	}
	result.AccessibleTesteeIDs, err = s.access.ListAccessibleTesteeIDs(ctx, actor.OrgID, actor.OperatorUserID)
	result.Restricted = true
	return result, err
}

type queryService struct {
	assessments domainassessment.Repository
	reader      evaluationreadmodel.AssessmentReader
	access      AccessChecker
	scores      evaluationoutcome.ScoreFactReader
	runs        evaluationrun.Repository
}

func NewQueryService(assessments domainassessment.Repository, reader evaluationreadmodel.AssessmentReader, access AccessChecker, scores evaluationoutcome.ScoreFactReader, runs evaluationrun.Repository) QueryService {
	return &queryService{assessments: assessments, reader: reader, access: access, scores: scores, runs: runs}
}

func (s *queryService) ValidateTesteeAccess(ctx context.Context, actor Actor, testeeID uint64) error {
	if actor.OrgID == 0 || actor.OperatorUserID == 0 {
		return evalerrors.InvalidArgument("操作者范围不能为空")
	}
	if s.access == nil {
		return evalerrors.ModuleNotConfigured("testee access checker is not configured")
	}
	return s.access.ValidateTesteeAccess(ctx, actor.OrgID, actor.OperatorUserID, testeeID)
}

func (s *queryService) loadAccessible(ctx context.Context, actor Actor, id uint64) (*domainassessment.Assessment, error) {
	return (authorizer{assessments: s.assessments, access: s.access}).loadAssessment(ctx, actor, id)
}

func (s *queryService) GetAssessment(ctx context.Context, actor Actor, id uint64) (*Assessment, error) {
	a, err := s.loadAccessible(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	return assessmentFromDomain(a)
}

func (s *queryService) scopedList(ctx context.Context, actor Actor, q ListQuery) (ListQuery, error) {
	if actor.OrgID == 0 || actor.OperatorUserID == 0 {
		return q, evalerrors.InvalidArgument("操作者范围不能为空")
	}
	if s.access == nil {
		return q, evalerrors.ModuleNotConfigured("testee access checker is not configured")
	}
	if q.TesteeID != nil {
		if err := s.access.ValidateTesteeAccess(ctx, actor.OrgID, actor.OperatorUserID, *q.TesteeID); err != nil {
			return q, err
		}
		return q, nil
	}
	scope, err := s.access.ResolveAccessScope(ctx, actor.OrgID, actor.OperatorUserID)
	if err != nil {
		return q, err
	}
	if scope != nil && scope.IsAdmin {
		return q, nil
	}
	q.AccessibleTesteeIDs, err = s.access.ListAccessibleTesteeIDs(ctx, actor.OrgID, actor.OperatorUserID)
	q.RestrictToAccessScope = true
	return q, err
}

func (s *queryService) listRows(ctx context.Context, actor Actor, q ListQuery) ([]evaluationreadmodel.AssessmentRow, int64, int, int, error) {
	if s.reader == nil {
		return nil, 0, 0, 0, evalerrors.ModuleNotConfigured("assessment read model is not configured")
	}
	q, err := s.scopedList(ctx, actor, q)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	page, pageSize := normalizePagination(q.Page, q.PageSize)
	filter := evaluationreadmodel.AssessmentFilter{OrgID: actor.OrgID, TesteeID: q.TesteeID, AccessibleTesteeIDs: q.AccessibleTesteeIDs, RestrictToAccessScope: q.RestrictToAccessScope}
	if q.Status != "" {
		status := domainassessment.Status(q.Status)
		if !status.IsValid() {
			return []evaluationreadmodel.AssessmentRow{}, 0, page, pageSize, nil
		}
		filter.Statuses = []string{status.String()}
	}
	rows, total, err := s.reader.ListAssessments(ctx, filter, evaluationreadmodel.PageRequest{Page: page, PageSize: pageSize})
	if err != nil {
		return nil, 0, 0, 0, evalerrors.Database(err, "查询测评列表失败")
	}
	return rows, total, page, pageSize, nil
}

func (s *queryService) ListAssessments(ctx context.Context, actor Actor, q ListQuery) (*AssessmentList, error) {
	rows, total, page, pageSize, err := s.listRows(ctx, actor, q)
	if err != nil {
		return nil, err
	}
	items := make([]*Assessment, 0, len(rows))
	for _, row := range rows {
		item, mapErr := assessmentFromRow(row)
		if mapErr != nil {
			return nil, mapErr
		}
		items = append(items, item)
	}
	return assessmentList(items, total, page, pageSize)
}

func (s *queryService) GetAssessmentOutcome(ctx context.Context, actor Actor, id uint64) (*OutcomeAssessment, error) {
	if _, err := s.loadAccessible(ctx, actor, id); err != nil {
		return nil, err
	}
	if s.reader == nil {
		return nil, evalerrors.ModuleNotConfigured("assessment read model is not configured")
	}
	row, err := s.reader.GetAssessment(ctx, id)
	if err != nil {
		return nil, evalerrors.AssessmentNotFound(err, "测评不存在")
	}
	return outcomeFromRow(*row)
}

func (s *queryService) ListAssessmentsOutcome(ctx context.Context, actor Actor, q ListQuery) (*OutcomeAssessmentList, error) {
	rows, total, page, pageSize, err := s.listRows(ctx, actor, q)
	if err != nil {
		return nil, err
	}
	items := make([]*OutcomeAssessment, 0, len(rows))
	for _, row := range rows {
		item, mapErr := outcomeFromRow(row)
		if mapErr != nil {
			return nil, mapErr
		}
		items = append(items, item)
	}
	count, err := safeconv.Int64ToInt(total)
	if err != nil {
		return nil, evalerrors.DatabaseMessage("测评总数超出安全范围")
	}
	return &OutcomeAssessmentList{Items: items, Total: count, Page: page, PageSize: pageSize, TotalPages: pages(count, pageSize)}, nil
}

func (s *queryService) GetScores(ctx context.Context, actor Actor, id uint64) (*Score, error) {
	if _, err := s.loadAccessible(ctx, actor, id); err != nil {
		return nil, err
	}
	fact, err := s.scores.Get(ctx, id)
	if err != nil {
		return nil, evalerrors.AssessmentScoreNotFound(err, "得分不存在")
	}
	return scoreFromFact(fact), nil
}

func (s *queryService) GetHighRiskFactors(ctx context.Context, actor Actor, id uint64) (*HighRiskFactors, error) {
	if _, err := s.loadAccessible(ctx, actor, id); err != nil {
		return nil, err
	}
	fact, err := s.scores.Get(ctx, id)
	if err != nil {
		return &HighRiskFactors{AssessmentID: id, HighRiskFactors: []FactorScore{}}, nil
	}
	score := scoreFromFact(fact)
	result := &HighRiskFactors{AssessmentID: id, HighRiskFactors: []FactorScore{}}
	for _, factor := range score.FactorScores {
		if factor.RiskLevel == "high" || factor.RiskLevel == "severe" {
			result.HighRiskFactors = append(result.HighRiskFactors, factor)
		}
	}
	result.HasHighRisk = len(result.HighRiskFactors) > 0 || score.RiskLevel == "high" || score.RiskLevel == "severe"
	result.NeedsUrgentCare = score.RiskLevel == "severe"
	return result, nil
}

func (s *queryService) GetFactorTrend(ctx context.Context, actor Actor, q TrendQuery) (*FactorTrend, error) {
	if err := s.ValidateTesteeAccess(ctx, actor, q.TesteeID); err != nil {
		return nil, err
	}
	fact, err := s.scores.Trend(ctx, q.TesteeID, q.FactorCode, q.Limit)
	if err != nil {
		return nil, err
	}
	result := &FactorTrend{TesteeID: fact.TesteeID, FactorCode: fact.FactorCode, FactorName: fact.FactorName, DataPoints: make([]TrendPoint, 0, len(fact.DataPoints))}
	for _, point := range fact.DataPoints {
		result.DataPoints = append(result.DataPoints, TrendPoint{AssessmentID: point.AssessmentID, RawScore: point.RawScore, RiskLevel: point.RiskLevel})
	}
	return result, nil
}

func (s *queryService) ListAssessmentRuns(ctx context.Context, actor Actor, id uint64, limit int) (*RunList, error) {
	if _, err := s.loadAccessible(ctx, actor, id); err != nil {
		return nil, err
	}
	if s.runs == nil {
		return nil, evalerrors.ModuleNotConfigured("evaluation run repository is not configured")
	}
	limit = normalizeLimit(limit, 20, 100)
	runs, err := s.runs.ListByAssessmentID(ctx, id, limit)
	if err != nil {
		return nil, err
	}
	items := make([]*Run, 0, len(runs))
	for _, run := range runs {
		items = append(items, runFromDomain(run))
	}
	return &RunList{Items: items}, nil
}

func (s *queryService) GetLatestAssessmentRun(ctx context.Context, actor Actor, id uint64) (*Run, error) {
	if _, err := s.loadAccessible(ctx, actor, id); err != nil {
		return nil, err
	}
	if s.runs == nil {
		return nil, evalerrors.ModuleNotConfigured("evaluation run repository is not configured")
	}
	run, err := s.runs.FindLatestByAssessmentID(ctx, id)
	if err != nil || run == nil {
		return nil, err
	}
	return runFromDomain(*run), nil
}

func (s *queryService) ListRetryableFailedRuns(ctx context.Context, actor Actor, limit int, cursor uint64) (*RetryableFailedRunList, error) {
	if actor.OrgID <= 0 || actor.OperatorUserID <= 0 {
		return nil, evalerrors.InvalidArgument("操作者范围不能为空")
	}
	if s.runs == nil {
		return nil, evalerrors.ModuleNotConfigured("evaluation run repository is not configured")
	}
	page, err := s.runs.ListRetryableFailed(ctx, evaluationrun.ListRetryableFailedParams{OrgID: actor.OrgID, Limit: normalizeLimit(limit, 50, 200), Cursor: cursor})
	if err != nil {
		return nil, err
	}
	result := &RetryableFailedRunList{}
	if page == nil {
		return result, nil
	}
	result.NextCursor = page.NextCursor
	result.Items = make([]*RetryableFailedRun, 0, len(page.Items))
	for _, item := range page.Items {
		result.Items = append(result.Items, &RetryableFailedRun{Run: *runFromDomain(item.Run), OrgID: item.OrgID})
	}
	return result, nil
}

type RecoveryService interface {
	Retry(context.Context, Actor, uint64) (*Assessment, error)
}
type recoveryService struct {
	assessments domainassessment.Repository
	tx          apptransaction.Runner
	events      EventStager
	authorizer  authorizer
}

func NewRecoveryService(assessments domainassessment.Repository, tx apptransaction.Runner, events EventStager, access AccessChecker) RecoveryService {
	return &recoveryService{assessments: assessments, tx: tx, events: events, authorizer: authorizer{assessments: assessments, access: access}}
}

func (s *recoveryService) Retry(ctx context.Context, actor Actor, id uint64) (*Assessment, error) {
	if s.assessments == nil || s.tx == nil || s.events == nil {
		return nil, evalerrors.ModuleNotConfigured("assessment recovery transactional outbox is not configured")
	}
	a, err := s.authorizer.loadAssessment(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	if !a.Status().IsFailed() {
		return nil, evalerrors.AssessmentInvalidStatus("只能重试失败的测评")
	}
	if err := a.RetryFromFailed(); err != nil {
		return nil, evalerrors.WrapAssessmentInvalidStatus(err, "重置测评状态失败")
	}
	err = s.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := s.assessments.Save(txCtx, a); err != nil {
			return err
		}
		events := outboxpolicy.Filter(a.Events())
		if len(events) == 0 {
			return nil
		}
		return s.events.Stage(txCtx, events...)
	})
	if err != nil {
		return nil, evalerrors.Database(err, "保存测评失败")
	}
	a.ClearEvents()
	return assessmentFromDomain(a)
}

func assessmentFromDomain(a *domainassessment.Assessment) (*Assessment, error) {
	if a == nil {
		return nil, nil
	}
	orgID, err := safeconv.Int64ToUint64(a.OrgID())
	if err != nil {
		return nil, evalerrors.DatabaseMessage("测评机构ID超出安全范围")
	}
	result := &Assessment{ID: a.ID().Uint64(), OrgID: orgID, TesteeID: a.TesteeID().Uint64(), QuestionnaireCode: a.QuestionnaireRef().Code().String(), QuestionnaireVersion: a.QuestionnaireRef().Version(), AnswerSheetID: a.AnswerSheetRef().ID().Uint64(), OriginType: a.OriginType().String(), OriginID: a.OriginID(), Status: a.Status().String(), TotalScore: a.TotalScore(), SubmittedAt: a.SubmittedAt(), EvaluatedAt: a.EvaluatedAt(), FailedAt: a.FailedAt(), FailureReason: a.FailureReason()}
	if risk := a.RiskLevel(); risk != nil {
		value := string(*risk)
		result.RiskLevel = &value
	}
	if model := a.EvaluationModelRef(); model != nil && !model.IsEmpty() {
		kind, sub, algorithm, code, version, title := model.Kind().String(), string(model.SubKind()), string(model.Algorithm()), model.Code().String(), model.Version(), model.Title()
		result.ModelKind, result.ModelSubKind, result.ModelAlgorithm, result.ModelCode, result.ModelVersion, result.ModelTitle = &kind, &sub, &algorithm, &code, &version, &title
	}
	return result, nil
}

func assessmentFromRow(row evaluationreadmodel.AssessmentRow) (*Assessment, error) {
	orgID, err := safeconv.Int64ToUint64(row.OrgID)
	if err != nil {
		return nil, evalerrors.DatabaseMessage("测评机构ID超出安全范围")
	}
	return &Assessment{ID: row.ID, OrgID: orgID, TesteeID: row.TesteeID, QuestionnaireCode: row.QuestionnaireCode, QuestionnaireVersion: row.QuestionnaireVersion, AnswerSheetID: row.AnswerSheetID, ModelKind: row.EvaluationModelKind, ModelSubKind: row.EvaluationModelSubKind, ModelAlgorithm: row.EvaluationModelAlgorithm, ModelCode: row.EvaluationModelCode, ModelVersion: row.EvaluationModelVersion, ModelTitle: row.EvaluationModelTitle, OriginType: row.OriginType, OriginID: row.OriginID, Status: row.Status, TotalScore: row.TotalScore, RiskLevel: row.RiskLevel, SubmittedAt: row.SubmittedAt, EvaluatedAt: row.EvaluatedAt, FailedAt: row.FailedAt, FailureReason: row.FailureReason}, nil
}

func outcomeFromRow(row evaluationreadmodel.AssessmentRow) (*OutcomeAssessment, error) {
	base, err := assessmentFromRow(row)
	if err != nil {
		return nil, err
	}
	return &OutcomeAssessment{ID: base.ID, OrgID: base.OrgID, TesteeID: base.TesteeID, QuestionnaireCode: base.QuestionnaireCode, QuestionnaireVersion: base.QuestionnaireVersion, AnswerSheetID: base.AnswerSheetID, Model: modelFromRow(row), PrimaryScore: primaryScoreFromRow(row), Level: levelFromRow(row), OriginType: base.OriginType, OriginID: base.OriginID, Status: base.Status, SubmittedAt: base.SubmittedAt, FailedAt: base.FailedAt, FailureReason: base.FailureReason}, nil
}

func modelFromRow(row evaluationreadmodel.AssessmentRow) ModelIdentity {
	kind, sub, algorithm := deref(row.EvaluationModelKind), deref(row.EvaluationModelSubKind), deref(row.EvaluationModelAlgorithm)
	if algorithm == "" && kind != "" {
		if k, s, a, ok := modelcatalog.LegacyKindMapping(modelcatalog.Kind(kind)); ok {
			kind = string(k)
			if sub == "" {
				sub = string(s)
			}
			algorithm = string(a)
		}
	}
	result := ModelIdentity{Kind: kind, SubKind: sub, Algorithm: algorithm, Code: deref(row.EvaluationModelCode), Version: deref(row.EvaluationModelVersion), Title: deref(row.EvaluationModelTitle)}
	k := modelbinding.Kind(result.Kind)
	result.ProductChannel = modelbinding.ProductChannelForIdentity(k, "")
	result.AlgorithmFamily = modelbinding.AlgorithmFamilyStringFromIdentity(k, modelbinding.SubKind(result.SubKind), modelbinding.Algorithm(result.Algorithm))
	return result
}

func primaryScoreFromRow(row evaluationreadmodel.AssessmentRow) *ScoreValue {
	if row.PrimaryScoreKind != nil && row.PrimaryScoreValue != nil {
		return &ScoreValue{Kind: *row.PrimaryScoreKind, Value: *row.PrimaryScoreValue, Label: deref(row.PrimaryScoreLabel), Max: row.PrimaryScoreMax}
	}
	if row.TotalScore != nil {
		return &ScoreValue{Kind: string(domainoutcome.ScoreKindRawTotal), Value: *row.TotalScore}
	}
	return nil
}

func levelFromRow(row evaluationreadmodel.AssessmentRow) *ResultLevel {
	if row.LevelCode != nil {
		return &ResultLevel{Code: *row.LevelCode, Label: deref(row.LevelLabel), Severity: deref(row.Severity)}
	}
	if row.RiskLevel == nil || !domainassessment.IsRiskLevelCode(*row.RiskLevel) {
		return nil
	}
	severity := "none"
	switch domainassessment.RiskLevel(*row.RiskLevel) {
	case domainassessment.RiskLevelSevere, domainassessment.RiskLevelHigh:
		severity = "high"
	case domainassessment.RiskLevelMedium:
		severity = "medium"
	case domainassessment.RiskLevelLow:
		severity = "low"
	}
	return &ResultLevel{Code: *row.RiskLevel, Label: *row.RiskLevel, Severity: severity}
}

func scoreFromFact(fact *evaluationoutcome.ScoreFact) *Score {
	result := &Score{AssessmentID: fact.AssessmentID, TotalScore: fact.TotalScore, RiskLevel: fact.RiskLevel, FactorScores: make([]FactorScore, 0, len(fact.FactorScores))}
	for _, factor := range fact.FactorScores {
		result.FactorScores = append(result.FactorScores, FactorScore{FactorCode: factor.FactorCode, FactorName: factor.FactorName, RawScore: factor.RawScore, MaxScore: factor.MaxScore, RiskLevel: factor.RiskLevel, IsTotalScore: factor.IsTotalScore})
	}
	return result
}

func runFromDomain(run evalrun.EvaluationRun) *Run {
	attempt := run.Attempt()
	result := &Run{RunID: run.ID().String(), AssessmentID: run.AssessmentID(), AttemptNo: attempt.Number, Status: attempt.Status.String(), Retryable: run.Retryable(), StartedAt: run.StartedAt(), FinishedAt: run.FinishedAt(), TraceID: run.TraceID(), InputSnapshotRef: run.InputSnapshotRef()}
	if failure := run.Failure(); failure != nil {
		result.ErrorCode, result.ErrorMessage, result.Retryable = failure.Kind.String(), failure.Message, failure.Retryable
	}
	return result
}

func assessmentList(items []*Assessment, total int64, page, pageSize int) (*AssessmentList, error) {
	count, err := safeconv.Int64ToInt(total)
	if err != nil {
		return nil, evalerrors.DatabaseMessage("测评总数超出安全范围")
	}
	return &AssessmentList{Items: items, Total: count, Page: page, PageSize: pageSize, TotalPages: pages(count, pageSize)}, nil
}
func normalizePagination(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}
func normalizeLimit(limit, defaultValue, max int) int {
	if limit <= 0 {
		return defaultValue
	}
	if limit > max {
		return max
	}
	return limit
}
func pages(total, pageSize int) int {
	if total == 0 {
		return 0
	}
	return (total + pageSize - 1) / pageSize
}
func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
