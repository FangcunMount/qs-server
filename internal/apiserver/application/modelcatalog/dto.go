package modelcatalog

import (
	"encoding/json"

	report "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

const (
	KindScale                          = string(domain.KindScale)
	KindTypology                       = string(domain.KindTypology)
	KindBehavioralRating               = string(domain.KindBehavioralRating)
	KindCognitive                      = string(domain.KindCognitive)
	SubKindTypology                    = "typology"
	SubKindScale                       = "scale"
	StatusDraft                        = "draft"
	StatusPublished                    = "published"
	StatusArchived                     = "archived"
	PayloadFormatScaleV1               = domain.PayloadFormatAssessmentScaleV1
	PayloadFormatPersonalityTypologyV1 = "assessmentmodel.personality.typology.v1"
)

type Option struct {
	Label    string `json:"label"`
	Value    string `json:"value"`
	Disabled bool   `json:"disabled,omitempty"`
}

type ListModelsDTO struct {
	Kind                 string
	SubKind              string
	Status               string
	Keyword              string
	Category             string
	Algorithm            string
	ProductChannel       string
	QuestionnaireCode    string
	QuestionnaireVersion string
	Page                 int
	PageSize             int
}

type CreateModelDTO struct {
	Code                 string
	Kind                 string
	SubKind              string
	Algorithm            string
	ProductChannel       string
	Title                string
	Description          string
	Category             string
	Tags                 []string
	Stages               []string
	ApplicableAges       []string
	Reporters            []string
	QuestionnaireCode    string
	QuestionnaireVersion string
}

type UpdateBasicInfoDTO struct {
	Code           string
	Title          string
	Description    string
	SubKind        string
	Algorithm      string
	ProductChannel string
	Category       string
	Tags           []string
	Stages         []string
	ApplicableAges []string
	Reporters      []string
}

type BindQuestionnaireDTO struct {
	Code                 string
	QuestionnaireCode    string
	QuestionnaireVersion string
}

type DefinitionDTO struct {
	Kind            string          `json:"kind" example:"typology"`
	SubKind         string          `json:"sub_kind,omitempty" example:"typology"`
	Algorithm       string          `json:"algorithm,omitempty"`
	ProductChannel  string          `json:"product_channel,omitempty"`
	AlgorithmFamily string          `json:"algorithm_family,omitempty"`
	PayloadFormat   string          `json:"payload_format"`
	Payload         json.RawMessage `json:"payload"`
}

type ApplyCodesDTO struct {
	Code   string
	Target string
	Count  int
}

type ModelSummary struct {
	Code                 string       `json:"code"`
	Kind                 string       `json:"kind" example:"typology"`
	SubKind              string       `json:"sub_kind,omitempty" example:"typology"`
	Algorithm            string       `json:"algorithm,omitempty"`
	ProductChannel       string       `json:"product_channel,omitempty" example:"typology"`
	AlgorithmFamily      string       `json:"algorithm_family,omitempty"`
	Title                string       `json:"title"`
	Description          string       `json:"description,omitempty"`
	Status               string       `json:"status"`
	Category             string       `json:"category,omitempty"`
	Stages               []string     `json:"stages,omitempty"`
	ApplicableAges       []string     `json:"applicable_ages,omitempty"`
	Reporters            []string     `json:"reporters,omitempty"`
	Tags                 []string     `json:"tags,omitempty"`
	QuestionnaireCode    string       `json:"questionnaire_code,omitempty"`
	QuestionnaireVersion string       `json:"questionnaire_version,omitempty"`
	CreatedAt            string       `json:"created_at,omitempty"`
	UpdatedAt            string       `json:"updated_at,omitempty"`
	ReleaseState         ReleaseState `json:"release_state"`
}

type ReleaseState struct {
	WorkingStatus         string `json:"working_status"`
	WorkingVersion        string `json:"working_version"`
	OnlineStatus          string `json:"online_status"`
	ActiveVersion         string `json:"active_version,omitempty"`
	HasUnpublishedChanges bool   `json:"has_unpublished_changes"`
}

type AssessmentReleaseVersion struct {
	ModelVersion         string `json:"model_version"`
	QuestionnaireCode    string `json:"questionnaire_code"`
	QuestionnaireVersion string `json:"questionnaire_version"`
	ReleaseStatus        string `json:"release_status"`
	PublishedAt          string `json:"published_at,omitempty"`
	ArchivedAt           string `json:"archived_at,omitempty"`
	Current              bool   `json:"current"`
}

// PublishedModelDetail is the transport-neutral published catalogue view.
// Definition is canonical DefinitionV2 and is never reconstructed from a
// legacy payload.
type PublishedModelDetail struct {
	ModelSummary
	Version       string             `json:"version"`
	DecisionKind  string             `json:"decision_kind,omitempty"`
	PayloadFormat string             `json:"payload_format,omitempty"`
	Definition    *domain.Definition `json:"definition"`
}

type PublishedModelListResult struct {
	Items    []PublishedModelDetail `json:"items"`
	Total    int64                  `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
}

type HotModelSummary struct {
	ModelSummary
	Rank            int   `json:"rank"`
	SubmissionCount int64 `json:"submission_count"`
	HeatScore       int64 `json:"heat_score"`
}

type HotModelListResult struct {
	Items      []HotModelSummary `json:"items"`
	Total      int64             `json:"total"`
	Limit      int               `json:"limit"`
	WindowDays int               `json:"window_days"`
}

type ModelListResult struct {
	Items    []ModelSummary `json:"items"`
	Total    int64          `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
}

type QuestionnaireBindingResult struct {
	QuestionnaireCode    string `json:"questionnaire_code"`
	QuestionnaireVersion string `json:"questionnaire_version"`
	Title                string `json:"title,omitempty"`
	QuestionCount        int    `json:"question_count"`
}

// AssessmentRelease is the single public lifecycle view for a questionnaire
// and its assessment model. QuestionnaireVersion is the release version; the
// model's persisted revision is intentionally not exposed as a business
// version.
type AssessmentRelease struct {
	ModelCode            string `json:"model_code"`
	ModelStatus          string `json:"model_status"`
	QuestionnaireCode    string `json:"questionnaire_code"`
	QuestionnaireVersion string `json:"questionnaire_version"`
	QuestionnaireStatus  string `json:"questionnaire_status"`
	PublishedAt          string `json:"published_at,omitempty"`
	ArchivedAt           string `json:"archived_at,omitempty"`
}

type OptionsResult struct {
	Kinds             []Option `json:"kinds"`
	ModelFamilies     []Option `json:"model_families,omitempty"`
	ProductChannels   []Option `json:"product_channels,omitempty"`
	AlgorithmFamilies []Option `json:"algorithm_families,omitempty"`
	Categories        []Option `json:"categories"`
	Algorithms        []Option `json:"algorithms"`
	SubKinds          []Option `json:"sub_kinds"`
	Tags              []Option `json:"tags,omitempty"`
	Stages            []Option `json:"stages,omitempty"`
	ApplicableAges    []Option `json:"applicable_ages,omitempty"`
	Reporters         []Option `json:"reporters,omitempty"`
}

type ValidationIssue struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
	Level   string `json:"level,omitempty"`
}

type ValidationResult struct {
	Passed bool              `json:"passed"`
	Valid  bool              `json:"valid"` // Deprecated: mirror Passed 用于 向后兼容。
	Issues []ValidationIssue `json:"issues"`
	Errors []string          `json:"errors"` // Deprecated: 派生 从 Issues 用于 向后兼容。
}

func NewValidationResult(issues []ValidationIssue) *ValidationResult {
	passed := true
	for _, issue := range issues {
		if issue.Level == "" || issue.Level == string(domain.ValidationLevelError) {
			passed = false
			break
		}
	}
	result := &ValidationResult{
		Passed: passed,
		Valid:  passed,
		Issues: issues,
	}
	if !passed {
		result.Errors = make([]string, 0, len(issues))
		for _, issue := range issues {
			if issue.Level == "" || issue.Level == string(domain.ValidationLevelError) {
				result.Errors = append(result.Errors, issue.Message)
			}
		}
	}
	return result
}

type PreviewOutcome struct {
	Code  string `json:"code,omitempty"`
	Title string `json:"title,omitempty"`
}

type PreviewReportSection struct {
	Title   string `json:"title"`
	Content string `json:"content,omitempty"`
	Kind    string `json:"kind,omitempty"`
}

type PreviewReportResult struct {
	Outcome        PreviewOutcome         `json:"outcome"`
	ScoreDetail    map[string]float64     `json:"score_detail,omitempty"`
	ReportSections []PreviewReportSection `json:"report_sections"`
	Issues         []ValidationIssue      `json:"issues,omitempty"`
	RawReport      *report.Draft          `json:"raw_report,omitempty"`
}

type ListNormTablesDTO struct {
	Kind        string
	Algorithm   string
	FormVariant string
	Page        int
	PageSize    int
}

type NormTableSummary struct {
	TableVersion string `json:"table_version"`
	FormVariant  string `json:"form_variant,omitempty"`
	Kind         string `json:"kind"`
	Algorithm    string `json:"algorithm"`
	FactorCount  int    `json:"factor_count"`
}

type NormTableListResult struct {
	Items    []NormTableSummary `json:"items"`
	Total    int64              `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
}

type NormTableDetail struct {
	NormTableSummary
	Factors []NormFactorTable `json:"factors"`
}

type NormFactorTable struct {
	FactorCode string            `json:"factor_code"`
	Bands      []NormBand        `json:"bands,omitempty"`
	Lookup     []NormLookupEntry `json:"lookup,omitempty"`
}

type NormBand struct {
	MinAgeMonths int      `json:"min_age_months,omitempty"`
	MaxAgeMonths int      `json:"max_age_months,omitempty"`
	Gender       string   `json:"gender,omitempty"`
	Mean         *float64 `json:"mean,omitempty"`
	StdDev       *float64 `json:"std_dev,omitempty"`
}

type NormLookupEntry struct {
	RawScoreMin   float64  `json:"raw_score_min"`
	RawScoreMax   float64  `json:"raw_score_max"`
	MinAgeMonths  int      `json:"min_age_months,omitempty"`
	MaxAgeMonths  int      `json:"max_age_months,omitempty"`
	Gender        string   `json:"gender,omitempty"`
	TScore        float64  `json:"t_score"`
	Percentile    float64  `json:"percentile"`
	StandardScore *float64 `json:"standard_score,omitempty"`
}
