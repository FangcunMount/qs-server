package interpretationmodel

import (
	"go.mongodb.org/mongo-driver/mongo"

	evaluationinputInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
	mongoInterpretationmodel "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretationmodel"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationmodel"
)

// NewDefaultStaticCatalog 从内置 SBTI/MBTI seed 构建静态规则目录。
func NewDefaultStaticCatalog() (port.ModelCatalog, error) {
	sbtiCatalog, err := evaluationinputInfra.NewDefaultSBTIModelCatalog()
	if err != nil {
		return nil, err
	}
	mbtiCatalog, err := evaluationinputInfra.NewDefaultMBTIModelCatalog()
	if err != nil {
		return nil, err
	}
	return NewStaticCompositeCatalog(sbtiCatalog, mbtiCatalog), nil
}

// NewCatalog 优先读 Mongo 已发布规则，未命中时回退到内置 seed。
func NewCatalog(db *mongo.Database, opts ...mongoBase.BaseRepositoryOptions) (port.ModelCatalog, error) {
	static, err := NewDefaultStaticCatalog()
	if err != nil {
		return nil, err
	}
	if db == nil {
		return static, nil
	}
	store := mongoInterpretationmodel.NewRepository(db, opts...)
	return NewLayeredCatalog(store, static), nil
}
