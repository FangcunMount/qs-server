package ruleset

import (
	"context"

	"go.mongodb.org/mongo-driver/mongo"

	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	mongoRuleset "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/ruleset"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/ruleset"
)

// NewDefaultStaticCatalog 从内置 SBTI/MBTI RuleSet 与可选量表 repo 回退构建静态规则目录。
func NewDefaultStaticCatalog(scaleSource ScaleBindingSource) (port.RuleSetCatalog, error) {
	ruleSets, err := DefaultEmbeddedRuleSets(context.Background())
	if err != nil {
		return nil, err
	}
	return NewStaticCompositeCatalog(ruleSets, scaleSource), nil
}

// NewCatalog 优先读 Mongo 已发布规则，未命中时回退到内置 RuleSet / 量表 repo。
func NewCatalog(db *mongo.Database, scaleSource ScaleBindingSource, opts ...mongoBase.BaseRepositoryOptions) (port.RuleSetCatalog, error) {
	static, err := NewDefaultStaticCatalog(scaleSource)
	if err != nil {
		return nil, err
	}
	if db == nil {
		return static, nil
	}
	store := mongoRuleset.NewRepository(db, opts...)
	return NewLayeredCatalog(store, static), nil
}
