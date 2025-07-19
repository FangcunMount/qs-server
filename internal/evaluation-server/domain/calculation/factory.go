package calculation

import (
	errCode "github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
)

// 静态计算器映射表，类似Java的static final Map
var calculaters = make(map[CalculaterType]Calculater)

// init函数在包被导入时自动执行，类似Java的static代码块
func init() {
	// 初始化所有计算器实例
	calculaters[CalculaterTheOption] = TheOptionCalculater{}
	calculaters[CalculaterTypeSum] = SumCalculater{}
	calculaters[CalculaterTypeAverage] = AverageCalculater{}
	calculaters[CalculaterTypeMax] = MaxCalculater{}
	calculaters[CalculaterTypeMin] = MinCalculater{}
}

// GetCalculater 获取计算器实例（类似Java的静态方法）
func GetCalculater(calculaterType CalculaterType) (Calculater, error) {
	if calculaterType == "" {
		return nil, errors.WithCode(errCode.ErrInvalidCalculaterType, "invalid calculater type")
	}

	calculater, exists := calculaters[calculaterType]
	if !exists {
		return nil, errors.WithCode(errCode.ErrCalculaterNotFound, "calculater not found")
	}

	return calculater, nil
}

// MustGetCalculater 获取计算器实例，如果不存在则panic（类似Java的get方法）
func MustGetCalculater(calculaterType CalculaterType) Calculater {
	calculater, err := GetCalculater(calculaterType)
	if err != nil {
		panic(err)
	}
	return calculater
}
