package response

// DefinitionV2Wire is the explicit REST wire contract for a model DefinitionV2.
// Its PascalCase member names deliberately match Definition.MarshalJSON.
type DefinitionV2Wire struct {
	Measure     DefinitionMeasureWire      `json:"Measure"`
	Calibration DefinitionCalibrationWire  `json:"Calibration"`
	Conclusions []DefinitionConclusionWire `json:"Conclusions"`
	Outcomes    []DefinitionOutcomeWire    `json:"Outcomes"`
	ReportMap   DefinitionReportMapWire    `json:"ReportMap"`
}

type DefinitionMeasureWire struct {
	Factors     []DefinitionFactorWire    `json:"Factors"`
	FactorGraph DefinitionFactorGraphWire `json:"FactorGraph"`
	Scoring     []DefinitionScoringWire   `json:"Scoring"`
}

type DefinitionFactorWire struct {
	Code  string `json:"Code"`
	Title string `json:"Title"`
	Role  string `json:"Role" enums:"dimension,total,index,validity,subtest,task_set,report_group,ability_domain"`
}

type DefinitionFactorGraphWire struct {
	Roots      []string                   `json:"Roots"`
	Edges      []DefinitionFactorEdgeWire `json:"Edges"`
	SortOrders map[string]int             `json:"SortOrders"`
}

type DefinitionFactorEdgeWire struct {
	ParentCode string `json:"ParentCode"`
	ChildCode  string `json:"ChildCode"`
}

type DefinitionScoringWire struct {
	FactorCode    string                        `json:"FactorCode"`
	Sources       []DefinitionScoringSourceWire `json:"Sources"`
	Strategy      string                        `json:"Strategy" enums:"sum,avg,weighted_sum,weighted_avg,max,min,cnt"`
	Params        *DefinitionScoringParamsWire  `json:"Params,omitempty"`
	MaxScore      *float64                      `json:"MaxScore,omitempty"`
	Weights       map[string]float64            `json:"Weights,omitempty"`
	Constant      float64                       `json:"Constant,omitempty"`
	OptionScoring string                        `json:"OptionScoring,omitempty" enums:"strict,compat"`
}

type DefinitionScoringSourceWire struct {
	Kind         string             `json:"Kind" enums:"question,factor"`
	Code         string             `json:"Code"`
	Sign         float64            `json:"Sign,omitempty"`
	OptionScores map[string]float64 `json:"OptionScores,omitempty"`
}

type DefinitionScoringParamsWire struct {
	CntOptionContents []string `json:"CntOptionContents,omitempty"`
}

type DefinitionCalibrationWire struct {
	NormRefs []DefinitionNormRefWire `json:"NormRefs"`
}

type DefinitionNormRefWire struct {
	FactorCode       string `json:"FactorCode"`
	NormTableVersion string `json:"NormTableVersion"`
}

// DefinitionConclusionWire is a tagged union. Kind selects the fields used by
// risk, norm, ability, or type conclusions.
type DefinitionConclusionWire struct {
	Kind           string                           `json:"Kind" enums:"risk,norm,ability,type"`
	FactorCode     string                           `json:"FactorCode,omitempty"`
	FactorCodes    []string                         `json:"FactorCodes,omitempty"`
	ScoreBasis     string                           `json:"ScoreBasis,omitempty" enums:"raw_score,t_score,percentile"`
	Primary        bool                             `json:"Primary,omitempty"`
	Rules          []DefinitionScoreRangeWire       `json:"Rules,omitempty"`
	Outcomes       []DefinitionOutcomeWire          `json:"Outcomes,omitempty"`
	Decision       DefinitionTypeDecisionWire       `json:"Decision,omitempty"`
	SpecialRules   []DefinitionTypeSpecialRuleWire  `json:"SpecialRules,omitempty"`
	OutcomeMapping DefinitionTypeOutcomeMappingWire `json:"OutcomeMapping,omitempty"`
	Profiles       []DefinitionTypeProfileWire      `json:"Profiles,omitempty"`
}

type DefinitionScoreRangeWire struct {
	MinScore    float64 `json:"MinScore"`
	MaxScore    float64 `json:"MaxScore"`
	Level       string  `json:"Level,omitempty"`
	OutcomeCode string  `json:"OutcomeCode,omitempty"`
	Title       string  `json:"Title,omitempty"`
	Summary     string  `json:"Summary,omitempty"`
	Description string  `json:"Description,omitempty"`
}

type DefinitionOutcomeWire struct {
	Code        string `json:"Code"`
	Title       string `json:"Title"`
	Summary     string `json:"Summary,omitempty"`
	Description string `json:"Description,omitempty"`
}

type DefinitionTypeDecisionWire struct {
	Kind                        string                       `json:"Kind,omitempty"`
	FallbackSimilarityThreshold float64                      `json:"FallbackSimilarityThreshold,omitempty"`
	FallbackCode                string                       `json:"FallbackCode,omitempty"`
	LevelRule                   *DefinitionTypeLevelRuleWire `json:"LevelRule,omitempty"`
	Poles                       []DefinitionTypePoleWire     `json:"Poles,omitempty"`
	TopK                        int                          `json:"TopK,omitempty"`
}

type DefinitionTypeLevelRuleWire struct {
	LowMax  float64 `json:"LowMax,omitempty"`
	HighMin float64 `json:"HighMin,omitempty"`
}

type DefinitionTypePoleWire struct {
	FactorCode string  `json:"FactorCode"`
	LeftPole   string  `json:"LeftPole"`
	RightPole  string  `json:"RightPole"`
	Threshold  float64 `json:"Threshold,omitempty"`
	Model      string  `json:"Model,omitempty"`
}

type DefinitionTypeSpecialRuleWire struct {
	Code          string   `json:"Code"`
	Kind          string   `json:"Kind"`
	Phase         string   `json:"Phase"`
	Trigger       string   `json:"Trigger,omitempty"`
	OutcomeCode   string   `json:"OutcomeCode,omitempty"`
	QuestionCodes []string `json:"QuestionCodes,omitempty"`
	OptionValues  []string `json:"OptionValues,omitempty"`
}

type DefinitionTypeOutcomeMappingWire struct {
	DetailKind       string `json:"DetailKind,omitempty"`
	DetailAdapterKey string `json:"DetailAdapterKey,omitempty"`
	Algorithm        string `json:"Algorithm,omitempty"`
}

type DefinitionTypeProfileWire struct {
	OutcomeCode string   `json:"OutcomeCode"`
	Pattern     string   `json:"Pattern,omitempty"`
	Traits      []string `json:"Traits,omitempty"`
	Strengths   []string `json:"Strengths,omitempty"`
	Weaknesses  []string `json:"Weaknesses,omitempty"`
	Suggestions []string `json:"Suggestions,omitempty"`
	ImageURL    string   `json:"ImageURL,omitempty"`
	Image       string   `json:"Image,omitempty"`
	IsSpecial   bool     `json:"IsSpecial,omitempty"`
	Trigger     string   `json:"Trigger,omitempty"`
	Commentary  string   `json:"Commentary,omitempty"`
}

type DefinitionReportMapWire struct {
	Sections []DefinitionReportSectionWire `json:"Sections"`
}

type DefinitionReportSectionWire struct {
	Code          string   `json:"Code"`
	Title         string   `json:"Title"`
	SourceRefs    []string `json:"SourceRefs,omitempty"`
	Kind          string   `json:"Kind"`
	AdapterKey    string   `json:"AdapterKey,omitempty"`
	TemplateID    string   `json:"TemplateID,omitempty"`
	CategoryLabel string   `json:"CategoryLabel,omitempty"`
}

// PreviewReportRequestWire describes the currently supported typology preview
// input. Answers can also be sent as the top-level array for compatibility.
type PreviewReportRequestWire struct {
	Answers  []PreviewAnswerWire `json:"answers"`
	SampleID string              `json:"sample_id,omitempty"`
}

type PreviewAnswerWire struct {
	QuestionCode string   `json:"question_code"`
	Value        string   `json:"value,omitempty"`
	Score        *float64 `json:"score,omitempty"`
}

type PreviewReportWire struct {
	Outcome        PreviewOutcomeWire           `json:"outcome"`
	ScoreDetail    map[string]float64           `json:"score_detail,omitempty"`
	ReportSections []PreviewReportSectionWire   `json:"report_sections"`
	Issues         []PreviewValidationIssueWire `json:"issues,omitempty"`
	RawReport      map[string]interface{}       `json:"raw_report,omitempty"`
}

type PreviewOutcomeWire struct {
	Code  string `json:"code,omitempty"`
	Title string `json:"title,omitempty"`
}

type PreviewReportSectionWire struct {
	Title   string `json:"title"`
	Content string `json:"content,omitempty"`
	Kind    string `json:"kind,omitempty"`
}

type PreviewValidationIssueWire struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
	Level   string `json:"level,omitempty"`
}

// Interpretation wire types document the internal lifecycle diagnostics.
type InterpretationFailureWire struct {
	Kind        string `json:"Kind"`
	Code        string `json:"Code"`
	SafeMessage string `json:"SafeMessage"`
	Retryable   bool   `json:"Retryable"`
}

type InterpretationRunWire struct {
	ID             uint64                     `json:"ID"`
	GenerationID   uint64                     `json:"GenerationID"`
	Attempt        int                        `json:"Attempt"`
	Status         string                     `json:"Status"`
	TraceID        string                     `json:"TraceID,omitempty"`
	Failure        *InterpretationFailureWire `json:"Failure,omitempty"`
	StartedAt      *string                    `json:"StartedAt,omitempty"`
	LeaseExpiresAt *string                    `json:"LeaseExpiresAt,omitempty"`
	FinishedAt     *string                    `json:"FinishedAt,omitempty"`
}

type InterpretationGenerationWire struct {
	ID              uint64                    `json:"ID"`
	OutcomeID       uint64                    `json:"OutcomeID"`
	LatestRunID     uint64                    `json:"LatestRunID,omitempty"`
	ReportID        uint64                    `json:"ReportID,omitempty"`
	ReportType      string                    `json:"ReportType"`
	TemplateVersion string                    `json:"TemplateVersion"`
	Status          string                    `json:"Status"`
	Version         uint64                    `json:"Version"`
	CreatedAt       string                    `json:"CreatedAt"`
	UpdatedAt       string                    `json:"UpdatedAt"`
	LatestRun       *InterpretationRunWire    `json:"LatestRun,omitempty"`
	Report          *InterpretationReportWire `json:"Report,omitempty"`
}

type InterpretationReportWire struct {
	ID              uint64 `json:"ID"`
	GenerationID    uint64 `json:"GenerationID"`
	OutcomeID       uint64 `json:"OutcomeID"`
	RunID           uint64 `json:"RunID"`
	AssessmentID    uint64 `json:"AssessmentID"`
	ReportType      string `json:"ReportType"`
	TemplateVersion string `json:"TemplateVersion"`
	GeneratedAt     string `json:"GeneratedAt"`
}
