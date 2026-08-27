// Package safety provides deterministic pre-publication safety gates. These
// rules are a required baseline, not a replacement for the semantic evaluator
// and human evidence required by the Prompt validation matrix.
package safety

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	appport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/port"
)

const DeterministicValidatorVersion = "ai-explanation-safety-deterministic-zh-en/v1"

type DeterministicGate struct{}

func NewDeterministicGate() *DeterministicGate { return &DeterministicGate{} }

func (*DeterministicGate) Evaluate(_ context.Context, request appport.SafetyRequest) (appport.SafetyResult, error) {
	if err := request.Content.Validate(); err != nil {
		return appport.SafetyResult{}, fmt.Errorf("validate AI explanation safety input: %w", err)
	}
	if len(request.Policy.ForbiddenClaims) != 7 || request.Policy.PolicyVersion == "" {
		return appport.SafetyResult{}, fmt.Errorf("AI explanation safety policy is incomplete")
	}
	raw, err := json.Marshal(request.Content)
	if err != nil {
		return appport.SafetyResult{}, fmt.Errorf("encode AI explanation safety content: %w", err)
	}
	normalized := normalize(string(raw))
	for _, rule := range forbiddenRules {
		for _, phrase := range rule.phrases {
			if strings.Contains(normalized, normalize(phrase)) {
				return rejected("forbidden_"+rule.claim, "AI 解读结果包含不允许的确定性或专业判断"), nil
			}
		}
	}
	limitations := normalize(strings.Join(request.Content.Limitations, " "))
	if !containsAny(limitations, []string{"本次测评", "本次结果", "current assessment", "this assessment"}) ||
		!containsAny(limitations, []string{"不构成诊断", "不能作为诊断", "not a diagnosis", "not diagnostic"}) ||
		!containsAny(limitations, []string{"确定性判断", "确定性结论", "definitive conclusion", "deterministic judgment"}) {
		return rejected("limitations_incomplete", "AI 解读结果缺少必要的使用边界"), nil
	}
	return appport.SafetyResult{Allowed: true, ValidatorVersion: DeterministicValidatorVersion}, nil
}

type forbiddenRule struct {
	claim   string
	phrases []string
}

var forbiddenRules = []forbiddenRule{
	{claim: "diagnosis", phrases: []string{"诊断为", "确诊", "证明患有", "diagnosed with", "you have a disorder"}},
	{claim: "causality", phrases: []string{"由此导致", "这导致了", "is caused by", "this causes"}},
	{claim: "medication", phrases: []string{"建议服用", "应当服药", "开始用药", "take medication", "start medication"}},
	{claim: "treatment_plan", phrases: []string{"治疗方案是", "需要接受心理治疗", "treatment plan is", "must undergo therapy"}},
	{claim: "risk_reclassification", phrases: []string{"重新分类为高风险", "属于危机状态", "reclassified as high risk", "in a crisis state"}},
	{claim: "identity_inference", phrases: []string{"说明你就是", "你本质上是", "这证明你是", "this means you are", "you are inherently"}},
	{claim: "deterministic_future_prediction", phrases: []string{"你一定会", "未来必然", "肯定会发生", "you will definitely", "will inevitably"}},
}

func rejected(code, message string) appport.SafetyResult {
	return appport.SafetyResult{Allowed: false, ValidatorVersion: DeterministicValidatorVersion, FailureCode: code, SafeMessage: message}
}

func containsAny(value string, candidates []string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, normalize(candidate)) {
			return true
		}
	}
	return false
}

func normalize(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, value)
}
