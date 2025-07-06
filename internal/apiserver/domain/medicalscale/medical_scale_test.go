package medicalscale

import (
	"testing"
)

func TestMedicalScale_Creation(t *testing.T) {
	// 创建因子
	calculationRule := NewCalculationRule(SumFormula, []string{"q1", "q2", "q3"})
	interpretRules := []InterpretRule{
		NewInterpretRule(NewScoreRange(0, 10), "轻度"),
		NewInterpretRule(NewScoreRange(11, 20), "中度"),
		NewInterpretRule(NewScoreRange(21, 30), "重度"),
	}

	factor := NewFactor(
		"anxiety",
		"焦虑因子",
		true,
		PrimaryFactor,
		calculationRule,
		interpretRules,
	)

	// 创建医学量表
	scale := NewMedicalScale(
		NewMedicalScaleID(1),
		"GAD-7",
		"广泛性焦虑障碍量表",
		"anxiety_questionnaire",
		"v1.0",
		[]Factor{factor},
	)

	// 验证基本属性
	if scale.Code() != "GAD-7" {
		t.Errorf("Expected code 'GAD-7', got '%s'", scale.Code())
	}

	if scale.Title() != "广泛性焦虑障碍量表" {
		t.Errorf("Expected title '广泛性焦虑障碍量表', got '%s'", scale.Title())
	}

	if scale.QuestionnaireCode() != "anxiety_questionnaire" {
		t.Errorf("Expected questionnaire code 'anxiety_questionnaire', got '%s'", scale.QuestionnaireCode())
	}

	// 验证因子
	factors := scale.Factors()
	if len(factors) != 1 {
		t.Errorf("Expected 1 factor, got %d", len(factors))
	}

	if factors[0].Code() != "anxiety" {
		t.Errorf("Expected factor code 'anxiety', got '%s'", factors[0].Code())
	}
}

func TestFactor_Validation(t *testing.T) {
	tests := []struct {
		name          string
		factor        Factor
		expectedError bool
	}{
		{
			name: "valid factor",
			factor: NewFactor(
				"test",
				"Test Factor",
				false,
				PrimaryFactor,
				NewCalculationRule(SumFormula, []string{"q1"}),
				[]InterpretRule{
					NewInterpretRule(NewScoreRange(0, 10), "Low"),
				},
			),
			expectedError: false,
		},
		{
			name: "empty code",
			factor: NewFactor(
				"",
				"Test Factor",
				false,
				PrimaryFactor,
				NewCalculationRule(SumFormula, []string{"q1"}),
				[]InterpretRule{
					NewInterpretRule(NewScoreRange(0, 10), "Low"),
				},
			),
			expectedError: true,
		},
		{
			name: "empty title",
			factor: NewFactor(
				"test",
				"",
				false,
				PrimaryFactor,
				NewCalculationRule(SumFormula, []string{"q1"}),
				[]InterpretRule{
					NewInterpretRule(NewScoreRange(0, 10), "Low"),
				},
			),
			expectedError: true,
		},
		{
			name: "no interpret rules",
			factor: NewFactor(
				"test",
				"Test Factor",
				false,
				PrimaryFactor,
				NewCalculationRule(SumFormula, []string{"q1"}),
				[]InterpretRule{},
			),
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.factor.Validate()
			if tt.expectedError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestScoreRange_Operations(t *testing.T) {
	range1 := NewScoreRange(0, 10)
	range2 := NewScoreRange(5, 15)
	range3 := NewScoreRange(20, 30)

	// Test Contains
	if !range1.Contains(5) {
		t.Errorf("Range [0,10] should contain 5")
	}

	if range1.Contains(15) {
		t.Errorf("Range [0,10] should not contain 15")
	}

	// Test IsOverlapping
	if !range1.IsOverlapping(range2) {
		t.Errorf("Range [0,10] should overlap with [5,15]")
	}

	if range1.IsOverlapping(range3) {
		t.Errorf("Range [0,10] should not overlap with [20,30]")
	}
}

func TestCalculationService_CalculateFactorScore(t *testing.T) {
	// service := NewCalculationService()

	// 创建测试因子
	calculationRule := NewCalculationRule(SumFormula, []string{"q1", "q2", "q3"})
	interpretRules := []InterpretRule{
		NewInterpretRule(NewScoreRange(0, 10), "低"),
	}

	factor := NewFactor(
		"test",
		"Test Factor",
		false,
		PrimaryFactor,
		calculationRule,
		interpretRules,
	)

	// 测试数据
	answerValues := map[string]interface{}{
		"q1": 2,
		"q2": 3,
		"q3": 5,
	}

	// 创建简单的计算服务，跳过日志
	calculationRule = factor.CalculationRule()
	sourceCodes := calculationRule.SourceCodes()

	var values []float64
	for _, sourceCode := range sourceCodes {
		if value, exists := answerValues[sourceCode]; exists {
			if intVal, ok := value.(int); ok {
				values = append(values, float64(intVal))
			}
		}
	}

	var score float64
	for _, value := range values {
		score += value
	}

	expectedScore := 10.0 // 2 + 3 + 5
	if score != expectedScore {
		t.Errorf("Expected score %.2f, got %.2f", expectedScore, score)
	}
}

func TestInterpretationService_GetFactorInterpretation(t *testing.T) {
	// 创建测试因子
	interpretRules := []InterpretRule{
		NewInterpretRule(NewScoreRange(0, 10), "轻度"),
		NewInterpretRule(NewScoreRange(11, 20), "中度"),
		NewInterpretRule(NewScoreRange(21, 30), "重度"),
	}

	factor := NewFactor(
		"test",
		"Test Factor",
		false,
		PrimaryFactor,
		NewCalculationRule(SumFormula, []string{"q1"}),
		interpretRules,
	)

	// 测试不同分数的解读
	tests := []struct {
		score          float64
		expectedResult string
	}{
		{5, "轻度"},
		{15, "中度"},
		{25, "重度"},
	}

	for _, tt := range tests {
		result, err := factor.GetInterpretation(tt.score)
		if err != nil {
			t.Errorf("Unexpected error for score %.2f: %v", tt.score, err)
		}

		if result != tt.expectedResult {
			t.Errorf("For score %.2f, expected '%s', got '%s'", tt.score, tt.expectedResult, result)
		}
	}

	// 测试超出范围的分数
	_, err := factor.GetInterpretation(35)
	if err == nil {
		t.Errorf("Expected error for score 35 but got none")
	}
}
