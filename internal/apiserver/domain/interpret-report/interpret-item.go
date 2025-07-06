package interpretationreport

import "time"

// InterpretItem 解读项
type InterpretItem struct {
	factorCode string
	title      string
	score      int
	content    string
	createdAt  time.Time
	updatedAt  time.Time
}

// InterpretItemOption 解读项选项
type InterpretItemOption func(*InterpretItem)

// NewInterpretItem 创建解读项
func NewInterpretItem(factorCode, title string, score int, content string, opts ...InterpretItemOption) InterpretItem {
	item := InterpretItem{
		factorCode: factorCode,
		title:      title,
		score:      score,
		content:    content,
		createdAt:  time.Now(),
		updatedAt:  time.Now(),
	}

	for _, opt := range opts {
		opt(&item)
	}

	return item
}

// WithCreatedAt 设置创建时间
func WithItemCreatedAt(createdAt time.Time) InterpretItemOption {
	return func(i *InterpretItem) {
		i.createdAt = createdAt
	}
}

// WithUpdatedAt 设置更新时间
func WithItemUpdatedAt(updatedAt time.Time) InterpretItemOption {
	return func(i *InterpretItem) {
		i.updatedAt = updatedAt
	}
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
func (i *InterpretItem) GetScore() int {
	return i.score
}

// GetContent 获取内容
func (i *InterpretItem) GetContent() string {
	return i.content
}

// GetCreatedAt 获取创建时间
func (i *InterpretItem) GetCreatedAt() time.Time {
	return i.createdAt
}

// GetUpdatedAt 获取更新时间
func (i *InterpretItem) GetUpdatedAt() time.Time {
	return i.updatedAt
}

// 业务方法

// UpdateTitle 更新标题
func (i *InterpretItem) UpdateTitle(title string) {
	i.title = title
	i.updatedAt = time.Now()
}

// UpdateScore 更新分数
func (i *InterpretItem) UpdateScore(score int) {
	i.score = score
	i.updatedAt = time.Now()
}

// UpdateContent 更新内容
func (i *InterpretItem) UpdateContent(content string) {
	i.content = content
	i.updatedAt = time.Now()
}

// IsValidScore 判断分数是否有效
func (i *InterpretItem) IsValidScore() bool {
	return i.score >= 0
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
		createdAt:  i.createdAt,
		updatedAt:  time.Now(),
	}
}
