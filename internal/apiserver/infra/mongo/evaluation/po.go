package evaluation

import (
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/FangcunMount/component-base/pkg/util/idutil"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
)

// ==================== InterpretReport 持久化对象 ====================

// InterpretReportPO 解读报告MongoDB持久化对象
type InterpretReportPO struct {
	base.BaseDocument `bson:",inline"`

	// 量表信息
	ScaleName string `bson:"scale_name" json:"scale_name"`
	ScaleCode string `bson:"scale_code" json:"scale_code"`

	// 受试者ID（冗余，用于查询）
	TesteeID uint64 `bson:"testee_id" json:"testee_id"`

	// 评估结果汇总
	TotalScore float64 `bson:"total_score" json:"total_score"`
	RiskLevel  string  `bson:"risk_level" json:"risk_level"`
	Conclusion string  `bson:"conclusion" json:"conclusion"`

	// 维度解读列表
	Dimensions []DimensionInterpretPO `bson:"dimensions" json:"dimensions"`

	// 建议列表
	Suggestions []string `bson:"suggestions" json:"suggestions"`
}

// DimensionInterpretPO 维度解读持久化对象
type DimensionInterpretPO struct {
	FactorCode  string   `bson:"factor_code" json:"factor_code"`
	FactorName  string   `bson:"factor_name" json:"factor_name"`
	RawScore    float64  `bson:"raw_score" json:"raw_score"`
	MaxScore    *float64 `bson:"max_score,omitempty" json:"max_score,omitempty"`
	RiskLevel   string   `bson:"risk_level" json:"risk_level"`
	Description string   `bson:"description" json:"description"`
}

// CollectionName 集合名称
func (InterpretReportPO) CollectionName() string {
	return "interpret_reports"
}

// BeforeInsert 插入前设置字段
func (p *InterpretReportPO) BeforeInsert() {
	if p.ID.IsZero() {
		p.ID = primitive.NewObjectID()
	}
	if p.DomainID.IsZero() {
		p.DomainID = report.ID(idutil.GetIntID())
	}
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	p.DeletedAt = nil
}

// BeforeUpdate 更新前设置字段
func (p *InterpretReportPO) BeforeUpdate() {
	p.UpdatedAt = time.Now()
}

// ToBsonM 转换为 BSON.M
func (p *InterpretReportPO) ToBsonM() (bson.M, error) {
	data, err := bson.Marshal(p)
	if err != nil {
		return nil, err
	}

	var result bson.M
	err = bson.Unmarshal(data, &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}
