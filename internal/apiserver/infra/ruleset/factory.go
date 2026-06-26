package ruleset

import (
	"context"

	"go.mongodb.org/mongo-driver/mongo"

	aminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/assessmentmodel"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	mongoassessmentmodel "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/assessmentmodel"
	mongoruleset "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/ruleset"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/assessmentmodel"
)

// NewDefaultStaticCatalog 从内置 SBTI/MBTI 与可选量表 repo 回退构建静态规则目录。
func NewDefaultStaticCatalog(scaleSource ScaleBindingSource) (port.RuleSetCatalog, error) {
	ruleSets, err := DefaultEmbeddedRuleSets(context.Background())
	if err != nil {
		return nil, err
	}
	return NewStaticCompositeCatalog(ruleSets, scaleSource), nil
}

// NewCatalog 优先读 published_assessment_models，未命中时回退 evaluation_rule_sets / 静态 seed。
func NewCatalog(db *mongo.Database, scaleSource ScaleBindingSource, opts ...mongoBase.BaseRepositoryOptions) (port.RuleSetCatalog, error) {
	static, err := NewDefaultStaticCatalog(scaleSource)
	if err != nil {
		return nil, err
	}
	if db == nil {
		return static, nil
	}
	v2 := mongoassessmentmodel.NewRepository(db, opts...)
	legacy := mongoruleset.NewRepository(db, opts...)
	store := aminfra.NewDualStore(v2, legacy)
	return NewLayeredCatalog(store, static), nil
}
