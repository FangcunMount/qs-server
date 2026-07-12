// Package testee contains Evaluation queries performed by a participant.
package testee

import (
	"context"
	"time"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelbinding "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

type Actor struct{ TesteeID uint64 }
type ListQuery struct {
	Page, PageSize                                                       int
	Status, ScaleCode, RiskLevel, ModelKind, ModelCode, DateFrom, DateTo string
}
type TrendQuery struct {
	FactorCode string
	Limit      int
}

type ModelIdentity struct{ Kind, SubKind, Algorithm, Code, Version, Title, ProductChannel, AlgorithmFamily string }
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
	Model                                   ModelIdentity
	PrimaryScore                            *ScoreValue
	Level                                   *ResultLevel
	OriginType                              string
	OriginID                                *string
	Status                                  string
	SubmittedAt, FailedAt                   *time.Time
	FailureReason                           *string
}
type AssessmentList struct {
	Items                             []*Assessment
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

type Service interface {
	AuthorizeAssessment(context.Context, Actor, uint64) error
	GetAssessment(context.Context, Actor, uint64) (*Assessment, error)
	ListAssessments(context.Context, Actor, ListQuery) (*AssessmentList, error)
	GetScore(context.Context, Actor, uint64) (*Score, error)
	GetFactorTrend(context.Context, Actor, TrendQuery) (*FactorTrend, error)
	GetHighRiskFactors(context.Context, Actor, uint64) (*HighRiskFactors, error)
}

type service struct {
	assessments domainassessment.Repository
	reader      evaluationreadmodel.AssessmentReader
	scores      evaloutcome.ScoreFactReader
}

func NewService(assessments domainassessment.Repository, reader evaluationreadmodel.AssessmentReader, scores evaloutcome.ScoreFactReader) Service {
	return &service{assessments: assessments, reader: reader, scores: scores}
}

func (s *service) AuthorizeAssessment(ctx context.Context, actor Actor, id uint64) error {
	if actor.TesteeID == 0 || id == 0 {
		return evalerrors.InvalidArgument("受试者ID和测评ID不能为空")
	}
	if s.assessments == nil {
		return evalerrors.ModuleNotConfigured("assessment repository is not configured")
	}
	a, err := s.assessments.FindByID(ctx, meta.FromUint64(id))
	if err != nil {
		return evalerrors.AssessmentNotFound(err, "测评不存在")
	}
	if a.TesteeID().Uint64() != actor.TesteeID {
		return evalerrors.Forbidden("无权访问此测评")
	}
	return nil
}
func (s *service) GetAssessment(ctx context.Context, actor Actor, id uint64) (*Assessment, error) {
	if err := s.AuthorizeAssessment(ctx, actor, id); err != nil {
		return nil, err
	}
	if s.reader == nil {
		return nil, evalerrors.ModuleNotConfigured("assessment read model is not configured")
	}
	row, err := s.reader.GetAssessment(ctx, id)
	if err != nil {
		return nil, err
	}
	return assessmentFromRow(*row)
}
func (s *service) ListAssessments(ctx context.Context, actor Actor, q ListQuery) (*AssessmentList, error) {
	if actor.TesteeID == 0 {
		return nil, evalerrors.InvalidArgument("受试者ID不能为空")
	}
	if s.reader == nil {
		return nil, evalerrors.ModuleNotConfigured("assessment read model is not configured")
	}
	page, size := normalizePagination(q.Page, q.PageSize)
	from, err := parseDate(q.DateFrom, false)
	if err != nil {
		return nil, evalerrors.InvalidArgument("date_from 格式不正确")
	}
	to, err := parseDate(q.DateTo, true)
	if err != nil {
		return nil, evalerrors.InvalidArgument("date_to 格式不正确")
	}
	rows, total, err := s.reader.ListAssessments(ctx, evaluationreadmodel.AssessmentFilter{TesteeID: &actor.TesteeID, Statuses: normalizeStatuses(q.Status), ScaleCode: q.ScaleCode, RiskLevel: q.RiskLevel, ModelKind: q.ModelKind, ModelCode: q.ModelCode, DateFrom: from, DateTo: to}, evaluationreadmodel.PageRequest{Page: page, PageSize: size})
	if err != nil {
		return nil, evalerrors.Database(err, "查询测评列表失败")
	}
	items := make([]*Assessment, 0, len(rows))
	for _, row := range rows {
		item, mapErr := assessmentFromRow(row)
		if mapErr != nil {
			return nil, mapErr
		}
		items = append(items, item)
	}
	count, err := safeconv.Int64ToInt(total)
	if err != nil {
		return nil, evalerrors.DatabaseMessage("测评总数超出安全范围")
	}
	return &AssessmentList{Items: items, Total: count, Page: page, PageSize: size, TotalPages: (count + size - 1) / size}, nil
}
func (s *service) GetScore(ctx context.Context, actor Actor, id uint64) (*Score, error) {
	if err := s.AuthorizeAssessment(ctx, actor, id); err != nil {
		return nil, err
	}
	fact, err := s.scores.Get(ctx, id)
	if err != nil {
		return nil, evalerrors.AssessmentScoreNotFound(err, "得分不存在")
	}
	return scoreFromFact(fact), nil
}
func (s *service) GetFactorTrend(ctx context.Context, actor Actor, q TrendQuery) (*FactorTrend, error) {
	if actor.TesteeID == 0 {
		return nil, evalerrors.InvalidArgument("受试者ID不能为空")
	}
	fact, err := s.scores.Trend(ctx, actor.TesteeID, q.FactorCode, q.Limit)
	if err != nil {
		return nil, err
	}
	points := make([]TrendPoint, 0, len(fact.DataPoints))
	for _, p := range fact.DataPoints {
		points = append(points, TrendPoint{AssessmentID: p.AssessmentID, RawScore: p.RawScore, RiskLevel: p.RiskLevel})
	}
	return &FactorTrend{TesteeID: fact.TesteeID, FactorCode: fact.FactorCode, FactorName: fact.FactorName, DataPoints: points}, nil
}
func (s *service) GetHighRiskFactors(ctx context.Context, actor Actor, id uint64) (*HighRiskFactors, error) {
	if err := s.AuthorizeAssessment(ctx, actor, id); err != nil {
		return nil, err
	}
	fact, err := s.scores.Get(ctx, id)
	if err != nil {
		return &HighRiskFactors{AssessmentID: id}, nil
	}
	score := scoreFromFact(fact)
	result := &HighRiskFactors{AssessmentID: id, HighRiskFactors: []FactorScore{}}
	for _, f := range score.FactorScores {
		if f.RiskLevel == "high" || f.RiskLevel == "severe" {
			result.HighRiskFactors = append(result.HighRiskFactors, f)
		}
	}
	result.HasHighRisk = len(result.HighRiskFactors) > 0 || score.RiskLevel == "high" || score.RiskLevel == "severe"
	result.NeedsUrgentCare = score.RiskLevel == "severe"
	return result, nil
}

func assessmentFromRow(row evaluationreadmodel.AssessmentRow) (*Assessment, error) {
	org, err := safeconv.Int64ToUint64(row.OrgID)
	if err != nil {
		return nil, evalerrors.DatabaseMessage("机构ID超出 uint64 范围")
	}
	return &Assessment{ID: row.ID, OrgID: org, TesteeID: row.TesteeID, QuestionnaireCode: row.QuestionnaireCode, QuestionnaireVersion: row.QuestionnaireVersion, AnswerSheetID: row.AnswerSheetID, Model: modelFromRow(row), PrimaryScore: primaryScoreFromRow(row), Level: levelFromRow(row), OriginType: row.OriginType, OriginID: row.OriginID, Status: row.Status, SubmittedAt: row.SubmittedAt, FailedAt: row.FailedAt, FailureReason: row.FailureReason}, nil
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
	result.ProductChannel = modelbinding.ProductChannelForIdentity(k, result.ProductChannel)
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
func scoreFromFact(f *evaloutcome.ScoreFact) *Score {
	factors := make([]FactorScore, 0, len(f.FactorScores))
	for _, v := range f.FactorScores {
		factors = append(factors, FactorScore{FactorCode: v.FactorCode, FactorName: v.FactorName, RawScore: v.RawScore, MaxScore: v.MaxScore, RiskLevel: v.RiskLevel, IsTotalScore: v.IsTotalScore})
	}
	return &Score{AssessmentID: f.AssessmentID, TotalScore: f.TotalScore, RiskLevel: f.RiskLevel, FactorScores: factors}
}
func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
func normalizePagination(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}
	if size > 100 {
		size = 100
	}
	return page, size
}
func parseDate(raw string, end bool) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		v, err := time.Parse(layout, raw)
		if err == nil {
			if layout == "2006-01-02" && end {
				v = v.Add(24 * time.Hour)
			}
			return &v, nil
		}
	}
	return nil, evalerrors.InvalidArgument("日期格式不正确")
}
func normalizeStatuses(raw string) []string {
	switch raw {
	case "":
		return nil
	case "pending":
		return []string{"pending", "submitted"}
	case "done":
		return []string{"evaluated"}
	default:
		return []string{raw}
	}
}
