package calculation

import (
	"testing"
)

func TestStaticFactory(t *testing.T) {
	// 测试数据
	operands := []Operand{1.0, 2.0, 3.0, 4.0, 5.0}

	// 测试所有计算器类型
	testCases := []struct {
		name     string
		calcType CalculaterType
		expected float64
	}{
		{"Sum", CalculaterTypeSum, 15.0},
		{"Average", CalculaterTypeAverage, 3.0},
		{"Max", CalculaterTypeMax, 5.0},
		{"Min", CalculaterTypeMin, 1.0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 使用静态工厂获取计算器
			calculater, err := GetCalculater(tc.calcType)
			if err != nil {
				t.Fatalf("获取计算器失败: %v", err)
			}

			// 执行计算
			result, err := calculater.Calculate(operands)
			if err != nil {
				t.Fatalf("计算失败: %v", err)
			}

			// 验证结果
			if result.Value() != tc.expected {
				t.Errorf("期望结果 %f, 实际结果 %f", tc.expected, result.Value())
			}
		})
	}
}

func TestMustGetCalculater(t *testing.T) {
	// 测试正常情况
	calculater := MustGetCalculater(CalculaterTypeSum)
	if calculater == nil {
		t.Fatal("MustGetCalculater 返回了 nil")
	}

	// 测试 panic 情况
	defer func() {
		if r := recover(); r == nil {
			t.Error("期望 MustGetCalculater 在无效类型时 panic")
		}
	}()

	MustGetCalculater("invalid_type")
}

func TestErrorHandling(t *testing.T) {
	// 测试空类型
	_, err := GetCalculater("")
	if err == nil {
		t.Error("期望返回错误，但返回了 nil")
	}

	// 测试不存在的类型
	_, err = GetCalculater("non_existent")
	if err == nil {
		t.Error("期望返回错误，但返回了 nil")
	}
}

// 性能测试：静态工厂的性能
func BenchmarkStaticFactory(b *testing.B) {
	for i := 0; i < b.N; i++ {
		calculater, _ := GetCalculater(CalculaterTypeSum)
		operands := []Operand{1.0, 2.0, 3.0, 4.0, 5.0}
		calculater.Calculate(operands)
	}
}
