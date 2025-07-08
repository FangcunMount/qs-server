package interpretationreport

import (
	"math"
)

// InterpretItem 解读项
type InterpretItem struct {
	factorCode string
	title      string
	score      float64
	content    string
}

// InterpretItemOption 解读项选项
type InterpretItemOption func(*InterpretItem)

// NewInterpretItem 创建解读项
func NewInterpretItem(factorCode, title string, score float64, content string, opts ...InterpretItemOption) InterpretItem {
	item := InterpretItem{
		factorCode: factorCode,
		title:      title,
		score:      score,
		content:    content,
	}

	for _, opt := range opts {
		opt(&item)
	}

	return item
}

// Getter 方法

// GetFactorCode 获取因子代码
func (i *InterpretItem) GetFactorCode() string {
	return i.factorCode
}

// GetTitle 获取标题
func (i *InterpretItem) GetTitle() string {
	return i.title
}

// GetScore 获取分数
func (i *InterpretItem) GetScore() float64 {
	return i.score
}

// GetContent 获取内容
func (i *InterpretItem) GetContent() string {
	return i.content
}

// 业务方法

// SetID 设置ID
func (i *InterpretItem) SetID(id string) {
	i.factorCode = id
}

// UpdateTitle 更新标题
func (i *InterpretItem) UpdateTitle(title string) {
	i.title = title
}

// UpdateScore 更新分数
func (i *InterpretItem) UpdateScore(score float64) {
	i.score = score
}

// UpdateContent 更新内容
func (i *InterpretItem) UpdateContent(content string) {
	i.content = content
}

// IsValidScore 判断分数是否有效
func (i *InterpretItem) IsValidScore() bool {
	return !math.IsNaN(i.score) && !math.IsInf(i.score, 0)
}

// HasContent 判断是否有内容
func (i *InterpretItem) HasContent() bool {
	return len(i.content) > 0
}

// IsComplete 判断解读项是否完整（有标题、分数和内容）
func (i *InterpretItem) IsComplete() bool {
	return len(i.title) > 0 && i.IsValidScore() && i.HasContent()
}

// Clone 克隆解读项
func (i *InterpretItem) Clone() InterpretItem {
	return InterpretItem{
		factorCode: i.factorCode,
		title:      i.title,
		score:      i.score,
		content:    i.content,
	}
}
