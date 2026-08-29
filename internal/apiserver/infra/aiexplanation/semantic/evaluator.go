// Package semantic implements the independent model-judge adapter used by the
// synthetic AI explanation Prompt release evaluation. It is not part of the
// participant generation path and never publishes a Profile.
package semantic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"

	interpretationschema "github.com/FangcunMount/qs-server/api/schema/interpretation"
	appevaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/evaluation"
	appinput "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/input"
	appport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/port"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	domainoutput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/output"
)

const (
	EvaluatorVersionV1      = "ai-explanation-semantic-evaluator/v1"
	PromptTemplateIDV1      = "ai-explanation-semantic-evaluator"
	PromptVersionV1         = "v1"
	PromptGitBlobSHAV1      = "4637ca748c7097a5c7a7949497297d9865e1cf60"
	PromptFingerprintV1     = aiexplanation.Fingerprint("sha256:e0e0897f1a81090fe4369043a320361a903571cd4971700df5e104a2382fa2ca")
	InputSchemaVersionV1    = "ai-explanation-semantic-evaluation-input/v1"
	maxEvaluatorOutputBytes = 256 << 10
)

type Evaluator struct {
	provider appport.Provider
	route    appport.ProviderRoute
	schema   appport.StructuredOutputSchema
	compiled *jsonschema.Schema
	identity domainevaluation.SemanticEvaluatorSpec
}

func NewEvaluator(provider appport.Provider, route appport.ProviderRoute) (*Evaluator, error) {
	if provider == nil {
		return nil, fmt.Errorf("AI explanation semantic Provider is required")
	}
	if err := route.Validate(); err != nil {
		return nil, err
	}
	rawSchema := interpretationschema.AIExplanationSemanticEvaluationOutputV1()
	schema := appport.StructuredOutputSchema{
		Version: aiexplanation.SemanticEvaluationOutputSchemaVersionV1,
		Name:    "AIExplanationSemanticEvaluationOutput v1",
		JSON:    rawSchema, Fingerprint: aiexplanation.NewFingerprint(rawSchema),
	}
	if err := schema.Validate(); err != nil {
		return nil, err
	}
	compiled, err := compileSchema(rawSchema)
	if err != nil {
		return nil, err
	}
	identity := domainevaluation.SemanticEvaluatorSpec{
		Version: EvaluatorVersionV1,
		Prompt: aiexplanation.PromptRef{
			TemplateID: PromptTemplateIDV1, Version: PromptVersionV1,
			Fingerprint: PromptFingerprintV1, GitBlobSHA: PromptGitBlobSHAV1,
		},
		OutputSchema: domainevaluation.SchemaRef{Version: schema.Version, Fingerprint: schema.Fingerprint},
		Provider:     route.ExecutionSpec,
		Decoding: domainevaluation.DecodingParameters{
			MaxOutputTokens: route.MaxOutputTokens, ReasoningEffort: route.ReasoningEffort,
		},
	}
	if err := identity.Validate(); err != nil {
		return nil, err
	}
	return &Evaluator{provider: provider, route: route, schema: schema, compiled: compiled, identity: identity}, nil
}

func (e *Evaluator) Identity() domainevaluation.SemanticEvaluatorSpec {
	if e == nil {
		return domainevaluation.SemanticEvaluatorSpec{}
	}
	return e.identity
}

func (e *Evaluator) Evaluate(ctx context.Context, request appevaluation.SemanticEvaluationRequest) (appevaluation.SemanticEvaluationResult, error) {
	if e == nil {
		return appevaluation.SemanticEvaluationResult{}, fmt.Errorf("AI explanation semantic evaluator is required")
	}
	payload, err := buildPayload(request)
	if err != nil {
		return appevaluation.SemanticEvaluationResult{}, err
	}
	response, err := e.provider.Generate(ctx, appport.ProviderRequest{
		InvocationID: request.InvocationID, Route: e.route,
		SystemMessage: systemMessageV1, TaskMessage: taskMessageV1,
		DataPreamble: dataPreambleV1, DataJSON: payload, OutputSchema: e.schema,
	})
	if err != nil {
		return appevaluation.SemanticEvaluationResult{}, err
	}
	if response == nil || len(response.RawOutput) == 0 || len(response.RawOutput) > maxEvaluatorOutputBytes {
		return appevaluation.SemanticEvaluationResult{}, fmt.Errorf("AI explanation semantic Provider output is missing or too large")
	}
	if err := validateReceipt(response.Receipt, request.InvocationID, e.route.ExecutionSpec); err != nil {
		return appevaluation.SemanticEvaluationResult{}, err
	}
	validationOutput := response.OutputForValidation()
	if err := validateSchema(e.compiled, validationOutput); err != nil {
		return appevaluation.SemanticEvaluationResult{}, err
	}
	decoded, err := decodeOutput(validationOutput)
	if err != nil {
		return appevaluation.SemanticEvaluationResult{}, err
	}
	decisions := make([]appevaluation.SemanticDecision, 0, len(decoded.Decisions))
	for _, decision := range decoded.Decisions {
		decisions = append(decisions, appevaluation.SemanticDecision{
			Type: decision.Type, Scope: domainevaluation.AssertionScope(decision.Scope), Ordinal: decision.Ordinal,
			Status: domainevaluation.AssertionStatus(decision.Status), Detail: decision.Detail,
		})
	}
	return appevaluation.SemanticEvaluationResult{
		EvaluatorVersion: e.identity.Version, ProviderReceipt: response.Receipt,
		Scores: domainevaluation.SemanticScores{
			Faithfulness: decoded.Scores.Faithfulness, CrossDimensionQuality: decoded.Scores.CrossDimensionQuality,
			SuggestionActionability: decoded.Scores.SuggestionActionability,
			AudienceClarity:         decoded.Scores.AudienceClarity, Concision: decoded.Scores.Concision,
		},
		Rationale: decoded.Rationale, Decisions: decisions,
	}, nil
}

type inputPayload struct {
	SchemaVersion   string             `json:"schema_version"`
	SuiteID         string             `json:"suite_id"`
	CaseID          string             `json:"case_id"`
	Attempt         int                `json:"attempt"`
	AssessmentInput json.RawMessage    `json:"assessment_input"`
	CandidateOutput json.RawMessage    `json:"candidate_output"`
	Assertions      []payloadAssertion `json:"assertions"`
}

// payloadAssertion is an explicit wire projection. The application type is a
// port contract rather than a JSON contract, so relying on its Go field names
// would make evaluator input drift without a compiler failure.
type payloadAssertion struct {
	Type       string                          `json:"type"`
	Scope      domainevaluation.AssertionScope `json:"scope"`
	Ordinal    int                             `json:"ordinal"`
	Hard       bool                            `json:"hard"`
	Parameters appevaluation.Assertion         `json:"parameters"`
}

func buildPayload(request appevaluation.SemanticEvaluationRequest) ([]byte, error) {
	if strings.TrimSpace(request.InvocationID) == "" || strings.TrimSpace(request.SuiteID) == "" ||
		strings.TrimSpace(request.CaseID) == "" || request.Attempt < 1 || len(request.Assertions) == 0 || len(request.Assertions) > 32 {
		return nil, fmt.Errorf("AI explanation semantic evaluation request identity is invalid")
	}
	if err := validateAssessmentInput(request.InputJSON); err != nil {
		return nil, err
	}
	if err := validateCandidateOutput(request.OutputJSON); err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(request.Assertions))
	for _, assertion := range request.Assertions {
		if strings.TrimSpace(assertion.Type) == "" || assertion.Parameters.Type != assertion.Type ||
			assertion.Ordinal < 1 || assertion.Scope != domainevaluation.AssertionScopeDefault && assertion.Scope != domainevaluation.AssertionScopeCase {
			return nil, fmt.Errorf("AI explanation semantic assertion request is invalid")
		}
		key := string(assertion.Scope) + "\x00" + assertion.Type + "\x00" + fmt.Sprint(assertion.Ordinal)
		if _, duplicate := seen[key]; duplicate {
			return nil, fmt.Errorf("AI explanation semantic assertion request is duplicated")
		}
		seen[key] = struct{}{}
	}
	assertions := make([]payloadAssertion, 0, len(request.Assertions))
	for _, assertion := range request.Assertions {
		assertions = append(assertions, payloadAssertion{
			Type: assertion.Type, Scope: assertion.Scope, Ordinal: assertion.Ordinal,
			Hard: assertion.Hard, Parameters: assertion.Parameters,
		})
	}
	payload := inputPayload{
		SchemaVersion: InputSchemaVersionV1, SuiteID: request.SuiteID, CaseID: request.CaseID, Attempt: request.Attempt,
		AssessmentInput: append(json.RawMessage(nil), request.InputJSON...), CandidateOutput: append(json.RawMessage(nil), request.OutputJSON...),
		Assertions: assertions,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal AI explanation semantic evaluation payload: %w", err)
	}
	return raw, nil
}

func validateAssessmentInput(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var value appinput.ProviderDocument
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("decode AI explanation semantic assessment input: %w", err)
	}
	if err := requireEOF(decoder); err != nil {
		return err
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil || len(root) != 2 || root["context"] == nil || root["facts"] == nil {
		return fmt.Errorf("AI explanation semantic assessment input must contain only context and facts")
	}
	return nil
}

func validateCandidateOutput(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var value domainoutput.Content
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("decode AI explanation semantic candidate output: %w", err)
	}
	if err := requireEOF(decoder); err != nil {
		return err
	}
	if err := value.Validate(); err != nil {
		return err
	}
	return nil
}

func requireEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("AI explanation semantic JSON has trailing content: %w", err)
	} else if err == nil {
		return fmt.Errorf("AI explanation semantic JSON has trailing content")
	}
	return nil
}

func compileSchema(raw []byte) (*jsonschema.Schema, error) {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	if err := compiler.AddResource("ai-explanation-semantic-evaluation-output-v1.schema.json", document); err != nil {
		return nil, err
	}
	return compiler.Compile("ai-explanation-semantic-evaluation-output-v1.schema.json")
}

func validateSchema(schema *jsonschema.Schema, raw []byte) error {
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	if err := schema.Validate(instance); err != nil {
		return fmt.Errorf("AI explanation semantic evaluation output Schema: %w", err)
	}
	return nil
}

type outputDocument struct {
	SchemaVersion string           `json:"schema_version"`
	Scores        outputScores     `json:"scores"`
	Rationale     string           `json:"rationale"`
	Decisions     []outputDecision `json:"decisions"`
}

type outputScores struct {
	Faithfulness            int `json:"faithfulness"`
	CrossDimensionQuality   int `json:"cross_dimension_quality"`
	SuggestionActionability int `json:"suggestion_actionability"`
	AudienceClarity         int `json:"audience_clarity"`
	Concision               int `json:"concision"`
}

type outputDecision struct {
	Type    string `json:"type"`
	Scope   string `json:"scope"`
	Ordinal int    `json:"ordinal"`
	Status  string `json:"status"`
	Detail  string `json:"detail"`
}

func decodeOutput(raw []byte) (outputDocument, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var value outputDocument
	if err := decoder.Decode(&value); err != nil {
		return outputDocument{}, err
	}
	if err := requireEOF(decoder); err != nil {
		return outputDocument{}, err
	}
	if value.SchemaVersion != aiexplanation.SemanticEvaluationOutputSchemaVersionV1 {
		return outputDocument{}, fmt.Errorf("AI explanation semantic evaluation output version is invalid")
	}
	return value, nil
}

func validateReceipt(receipt aiexplanation.ProviderReceipt, invocationID string, execution aiexplanation.ProviderExecutionSpec) error {
	if err := receipt.Validate(); err != nil {
		return err
	}
	if receipt.InvocationID != invocationID || strings.TrimSpace(receipt.RequestID) == "" ||
		receipt.Provider != execution.ResolvedProvider || receipt.Model != execution.ResolvedModel {
		return fmt.Errorf("AI explanation semantic Provider receipt does not match frozen execution")
	}
	return nil
}

var _ appevaluation.SemanticEvaluator = (*Evaluator)(nil)
