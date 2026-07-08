// Package definition is the mechanism-oriented home for scale scoring definitions.
package definition

import scaledef "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/definition"

type (
	ApplicableAge = scaledef.ApplicableAge
	BaseInfo = scaledef.BaseInfo
	Category = scaledef.Category
	ChangeAction = scaledef.ChangeAction
	DomainError = scaledef.DomainError
	ErrorKind = scaledef.ErrorKind
	Factor = scaledef.Factor
	FactorCode = scaledef.FactorCode
	FactorOption = scaledef.FactorOption
	FactorSnapshot = scaledef.FactorSnapshot
	FactorType = scaledef.FactorType
	HotScaleSummary = scaledef.HotScaleSummary
	InterpretationRule = scaledef.InterpretationRule
	InterpretationRules = scaledef.InterpretationRules
	Lifecycle = scaledef.Lifecycle
	MedicalScale = scaledef.MedicalScale
	MedicalScaleOption = scaledef.MedicalScaleOption
	RecordRole = scaledef.RecordRole
	Reporter = scaledef.Reporter
	Repository = scaledef.Repository
	RiskLevel = scaledef.RiskLevel
	ScaleChangedData = scaledef.ScaleChangedData
	ScaleChangedEvent = scaledef.ScaleChangedEvent
	ScaleVersion = scaledef.ScaleVersion
	ScoreRange = scaledef.ScoreRange
	ScoringParams = scaledef.ScoringParams
	ScoringSpec = scaledef.ScoringSpec
	ScoringStrategyCode = scaledef.ScoringStrategyCode
	Stage = scaledef.Stage
	Status = scaledef.Status
	Tag = scaledef.Tag
	ValidationError = scaledef.ValidationError
	Validator = scaledef.Validator
	Versioning = scaledef.Versioning
)

const (
	AggregateType = scaledef.AggregateType
	ApplicableAgeAdolescent = scaledef.ApplicableAgeAdolescent
	ApplicableAgeAdult = scaledef.ApplicableAgeAdult
	ApplicableAgeInfant = scaledef.ApplicableAgeInfant
	ApplicableAgePreschool = scaledef.ApplicableAgePreschool
	ApplicableAgeSchoolChild = scaledef.ApplicableAgeSchoolChild
	CategoryADHD = scaledef.CategoryADHD
	CategoryASD = scaledef.CategoryASD
	CategoryEmotion = scaledef.CategoryEmotion
	CategoryExecutiveFunction = scaledef.CategoryExecutiveFunction
	CategoryPersonality = scaledef.CategoryPersonality
	CategoryPressure = scaledef.CategoryPressure
	CategorySensoryIntegration = scaledef.CategorySensoryIntegration
	CategorySleep = scaledef.CategorySleep
	CategoryTicDisorder = scaledef.CategoryTicDisorder
	ChangeActionArchived = scaledef.ChangeActionArchived
	ChangeActionPublished = scaledef.ChangeActionPublished
	ChangeActionUnpublished = scaledef.ChangeActionUnpublished
	ChangeActionUpdated = scaledef.ChangeActionUpdated
	DefaultScaleVersion = scaledef.DefaultScaleVersion
	ErrorKindInvalidArgument = scaledef.ErrorKindInvalidArgument
	ErrorKindRuleFrozen = scaledef.ErrorKindRuleFrozen
	EventTypeChanged = scaledef.EventTypeChanged
	FactorTypeMultilevel = scaledef.FactorTypeMultilevel
	FactorTypePrimary = scaledef.FactorTypePrimary
	RecordRoleHead = scaledef.RecordRoleHead
	RecordRolePublishedSnapshot = scaledef.RecordRolePublishedSnapshot
	ReporterClinical = scaledef.ReporterClinical
	ReporterParent = scaledef.ReporterParent
	ReporterSelf = scaledef.ReporterSelf
	ReporterTeacher = scaledef.ReporterTeacher
	RiskLevelHigh = scaledef.RiskLevelHigh
	RiskLevelLow = scaledef.RiskLevelLow
	RiskLevelMedium = scaledef.RiskLevelMedium
	RiskLevelNone = scaledef.RiskLevelNone
	RiskLevelSevere = scaledef.RiskLevelSevere
	ScoringStrategyAvg = scaledef.ScoringStrategyAvg
	ScoringStrategyCnt = scaledef.ScoringStrategyCnt
	ScoringStrategySum = scaledef.ScoringStrategySum
	StageDeepAssessment = scaledef.StageDeepAssessment
	StageFollowUp = scaledef.StageFollowUp
	StageOutcome = scaledef.StageOutcome
	StatusArchived = scaledef.StatusArchived
	StatusDraft = scaledef.StatusDraft
	StatusPublished = scaledef.StatusPublished
)

var (
	AllCategories = scaledef.AllCategories
	ErrNotFound = scaledef.ErrNotFound
)

var (
	ErrorKindOf = scaledef.ErrorKindOf
	IsNotFound = scaledef.IsNotFound
	MustInterpretationRules = scaledef.MustInterpretationRules
	NewApplicableAge = scaledef.NewApplicableAge
	NewCategory = scaledef.NewCategory
	NewFactor = scaledef.NewFactor
	NewFactorCode = scaledef.NewFactorCode
	NewInterpretationRule = scaledef.NewInterpretationRule
	NewInterpretationRules = scaledef.NewInterpretationRules
	NewLifecycle = scaledef.NewLifecycle
	NewMedicalScale = scaledef.NewMedicalScale
	NewReporter = scaledef.NewReporter
	NewScaleChangedEvent = scaledef.NewScaleChangedEvent
	NewScaleVersion = scaledef.NewScaleVersion
	NewScoreRange = scaledef.NewScoreRange
	NewScoringParams = scaledef.NewScoringParams
	NewScoringSpec = scaledef.NewScoringSpec
	NewStage = scaledef.NewStage
	NewTag = scaledef.NewTag
	NormalizeRecordRole = scaledef.NormalizeRecordRole
	ParseFactorType = scaledef.ParseFactorType
	ParseStatus = scaledef.ParseStatus
	ToError = scaledef.ToError
	ToErrors = scaledef.ToErrors
	ValidateFactor = scaledef.ValidateFactor
	WithActivePublished = scaledef.WithActivePublished
	WithApplicableAges = scaledef.WithApplicableAges
	WithCategory = scaledef.WithCategory
	WithCreatedAt = scaledef.WithCreatedAt
	WithCreatedBy = scaledef.WithCreatedBy
	WithDescription = scaledef.WithDescription
	WithFactorType = scaledef.WithFactorType
	WithFactors = scaledef.WithFactors
	WithID = scaledef.WithID
	WithInterpretRules = scaledef.WithInterpretRules
	WithIsShow = scaledef.WithIsShow
	WithIsTotalScore = scaledef.WithIsTotalScore
	WithMaxScore = scaledef.WithMaxScore
	WithQuestionCodes = scaledef.WithQuestionCodes
	WithQuestionnaire = scaledef.WithQuestionnaire
	WithRecordRole = scaledef.WithRecordRole
	WithReporters = scaledef.WithReporters
	WithScaleVersion = scaledef.WithScaleVersion
	WithScoringParams = scaledef.WithScoringParams
	WithScoringSpec = scaledef.WithScoringSpec
	WithScoringStrategy = scaledef.WithScoringStrategy
	WithStages = scaledef.WithStages
	WithStatus = scaledef.WithStatus
	WithTags = scaledef.WithTags
	WithUpdatedAt = scaledef.WithUpdatedAt
	WithUpdatedBy = scaledef.WithUpdatedBy
)

