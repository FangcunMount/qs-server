package calculation

import (
	errCode "github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
)

// CalculatorFactory 计算器工厂接口
type CalculatorFactory interface {
	GetCalculator(calculatorType CalculaterType) (Calculater, error)
	RegisterCalculator(calculatorType CalculaterType, calculator Calculater)
}

// calculatorFactory 计算器工厂实现
type calculatorFactory struct {
	calculators map[CalculaterType]Calculater
}

// NewCalculatorFactory 创建计算器工厂
func NewCalculatorFactory() CalculatorFactory {
	factory := &calculatorFactory{
		calculators: make(map[CalculaterType]Calculater),
	}

	// 注册默认计算器
	factory.RegisterDefaultCalculators()

	return factory
}

// RegisterDefaultCalculators 注册默认计算器
func (f *calculatorFactory) RegisterDefaultCalculators() {
	f.RegisterCalculator(CalculaterTheOption, &TheOptionCalculater{})
	f.RegisterCalculator(CalculaterTypeScore, &TheOptionCalculater{}) // score类型使用the_option计算器
	f.RegisterCalculator(CalculaterTypeSum, &SumCalculater{})
	f.RegisterCalculator(CalculaterTypeAverage, &AverageCalculater{})
	f.RegisterCalculator(CalculaterTypeMax, &MaxCalculater{})
	f.RegisterCalculator(CalculaterTypeMin, &MinCalculater{})
}

// GetCalculator 获取计算器实例
func (f *calculatorFactory) GetCalculator(calculatorType CalculaterType) (Calculater, error) {
	if calculatorType == "" {
		return nil, errors.WithCode(errCode.ErrInvalidCalculaterType, "invalid calculator type")
	}

	calculator, exists := f.calculators[calculatorType]
	if !exists {
		return nil, errors.WithCode(errCode.ErrCalculaterNotFound, "calculator not found: %s", calculatorType)
	}

	return calculator, nil
}

// RegisterCalculator 注册计算器
func (f *calculatorFactory) RegisterCalculator(calculatorType CalculaterType, calculator Calculater) {
	f.calculators[calculatorType] = calculator
}

// 保持向后兼容性的全局函数
var globalFactory = NewCalculatorFactory()

// GetCalculater 获取计算器实例（向后兼容）
func GetCalculater(calculaterType CalculaterType) (Calculater, error) {
	return globalFactory.GetCalculator(calculaterType)
}

// MustGetCalculater 获取计算器实例，如果不存在则panic（向后兼容）
func MustGetCalculater(calculaterType CalculaterType) Calculater {
	calculater, err := globalFactory.GetCalculator(calculaterType)
	if err != nil {
		panic(err)
	}
	return calculater
}
