// Package definition is a compatibility seam; canonical home is scoring/definition.
package definition

import scoringdef "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"

type (
	ApplicableAge = scoringdef.ApplicableAge
	BaseInfo = scoringdef.BaseInfo
	Category = scoringdef.Category
	ChangeAction = scoringdef.ChangeAction
	DomainError = scoringdef.DomainError
	ErrorKind = scoringdef.ErrorKind
	Factor = scoringdef.Factor
	FactorCode = scoringdef.FactorCode
	FactorOption = scoringdef.FactorOption
	FactorSnapshot = scoringdef.FactorSnapshot
	FactorType = scoringdef.FactorType
	HotScaleSummary = scoringdef.HotScaleSummary
	InterpretationRule = scoringdef.InterpretationRule
	InterpretationRules = scoringdef.InterpretationRules
	Lifecycle = scoringdef.Lifecycle
	MedicalScale = scoringdef.MedicalScale
	MedicalScaleOption = scoringdef.MedicalScaleOption
	RecordRole = scoringdef.RecordRole
	Reporter = scoringdef.Reporter
	Repository = scoringdef.Repository
	RiskLevel = scoringdef.RiskLevel
	ScaleChangedData = scoringdef.ScaleChangedData
	ScaleChangedEvent = scoringdef.ScaleChangedEvent
	ScaleVersion = scoringdef.ScaleVersion
	ScoreRange = scoringdef.ScoreRange
	ScoringParams = scoringdef.ScoringParams
	ScoringSpec = scoringdef.ScoringSpec
	ScoringStrategyCode = scoringdef.ScoringStrategyCode
	Stage = scoringdef.Stage
	Status = scoringdef.Status
	Tag = scoringdef.Tag
	ValidationError = scoringdef.ValidationError
	Validator = scoringdef.Validator
	Versioning = scoringdef.Versioning
)

const (
	AggregateType = scoringdef.AggregateType
	ApplicableAgeAdolescent = scoringdef.ApplicableAgeAdolescent
	ApplicableAgeAdult = scoringdef.ApplicableAgeAdult
	ApplicableAgeInfant = scoringdef.ApplicableAgeInfant
	ApplicableAgePreschool = scoringdef.ApplicableAgePreschool
	ApplicableAgeSchoolChild = scoringdef.ApplicableAgeSchoolChild
	CategoryADHD = scoringdef.CategoryADHD
	CategoryASD = scoringdef.CategoryASD
	CategoryEmotion = scoringdef.CategoryEmotion
	CategoryExecutiveFunction = scoringdef.CategoryExecutiveFunction
	CategoryPersonality = scoringdef.CategoryPersonality
	CategoryPressure = scoringdef.CategoryPressure
	CategorySensoryIntegration = scoringdef.CategorySensoryIntegration
	CategorySleep = scoringdef.CategorySleep
	CategoryTicDisorder = scoringdef.CategoryTicDisorder
	ChangeActionArchived = scoringdef.ChangeActionArchived
	ChangeActionPublished = scoringdef.ChangeActionPublished
	ChangeActionUnpublished = scoringdef.ChangeActionUnpublished
	ChangeActionUpdated = scoringdef.ChangeActionUpdated
	DefaultScaleVersion = scoringdef.DefaultScaleVersion
	ErrorKindInvalidArgument = scoringdef.ErrorKindInvalidArgument
	ErrorKindRuleFrozen = scoringdef.ErrorKindRuleFrozen
	EventTypeChanged = scoringdef.EventTypeChanged
	FactorTypeMultilevel = scoringdef.FactorTypeMultilevel
	FactorTypePrimary = scoringdef.FactorTypePrimary
	RecordRoleHead = scoringdef.RecordRoleHead
	RecordRolePublishedSnapshot = scoringdef.RecordRolePublishedSnapshot
	ReporterClinical = scoringdef.ReporterClinical
	ReporterParent = scoringdef.ReporterParent
	ReporterSelf = scoringdef.ReporterSelf
	ReporterTeacher = scoringdef.ReporterTeacher
	RiskLevelHigh = scoringdef.RiskLevelHigh
	RiskLevelLow = scoringdef.RiskLevelLow
	RiskLevelMedium = scoringdef.RiskLevelMedium
	RiskLevelNone = scoringdef.RiskLevelNone
	RiskLevelSevere = scoringdef.RiskLevelSevere
	ScoringStrategyAvg = scoringdef.ScoringStrategyAvg
	ScoringStrategyCnt = scoringdef.ScoringStrategyCnt
	ScoringStrategySum = scoringdef.ScoringStrategySum
	StageDeepAssessment = scoringdef.StageDeepAssessment
	StageFollowUp = scoringdef.StageFollowUp
	StageOutcome = scoringdef.StageOutcome
	StatusArchived = scoringdef.StatusArchived
	StatusDraft = scoringdef.StatusDraft
	StatusPublished = scoringdef.StatusPublished
)

var (
	AllCategories = scoringdef.AllCategories
	ErrNotFound = scoringdef.ErrNotFound
)

var (
	ErrorKindOf = scoringdef.ErrorKindOf
	IsNotFound = scoringdef.IsNotFound
	MustInterpretationRules = scoringdef.MustInterpretationRules
	NewApplicableAge = scoringdef.NewApplicableAge
	NewCategory = scoringdef.NewCategory
	NewFactor = scoringdef.NewFactor
	NewFactorCode = scoringdef.NewFactorCode
	NewInterpretationRule = scoringdef.NewInterpretationRule
	NewInterpretationRules = scoringdef.NewInterpretationRules
	NewLifecycle = scoringdef.NewLifecycle
	NewMedicalScale = scoringdef.NewMedicalScale
	NewReporter = scoringdef.NewReporter
	NewScaleChangedEvent = scoringdef.NewScaleChangedEvent
	NewScaleVersion = scoringdef.NewScaleVersion
	NewScoreRange = scoringdef.NewScoreRange
	NewScoringParams = scoringdef.NewScoringParams
	NewScoringSpec = scoringdef.NewScoringSpec
	NewStage = scoringdef.NewStage
	NewTag = scoringdef.NewTag
	NormalizeRecordRole = scoringdef.NormalizeRecordRole
	ParseFactorType = scoringdef.ParseFactorType
	ParseStatus = scoringdef.ParseStatus
	ToError = scoringdef.ToError
	ToErrors = scoringdef.ToErrors
	ValidateFactor = scoringdef.ValidateFactor
	WithActivePublished = scoringdef.WithActivePublished
	WithApplicableAges = scoringdef.WithApplicableAges
	WithCategory = scoringdef.WithCategory
	WithCreatedAt = scoringdef.WithCreatedAt
	WithCreatedBy = scoringdef.WithCreatedBy
	WithDescription = scoringdef.WithDescription
	WithFactorType = scoringdef.WithFactorType
	WithFactors = scoringdef.WithFactors
	WithID = scoringdef.WithID
	WithInterpretRules = scoringdef.WithInterpretRules
	WithIsShow = scoringdef.WithIsShow
	WithIsTotalScore = scoringdef.WithIsTotalScore
	WithMaxScore = scoringdef.WithMaxScore
	WithQuestionCodes = scoringdef.WithQuestionCodes
	WithQuestionnaire = scoringdef.WithQuestionnaire
	WithRecordRole = scoringdef.WithRecordRole
	WithReporters = scoringdef.WithReporters
	WithScaleVersion = scoringdef.WithScaleVersion
	WithScoringParams = scoringdef.WithScoringParams
	WithScoringSpec = scoringdef.WithScoringSpec
	WithScoringStrategy = scoringdef.WithScoringStrategy
	WithStages = scoringdef.WithStages
	WithStatus = scoringdef.WithStatus
	WithTags = scoringdef.WithTags
	WithUpdatedAt = scoringdef.WithUpdatedAt
	WithUpdatedBy = scoringdef.WithUpdatedBy
)

