package assembler

import (
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/FangcunMount/component-base/pkg/errors"
	scaleApp "github.com/FangcunMount/qs-server/internal/apiserver/application/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	scaleInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/handler"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// ScaleModule Scale 模块（量表子域）
// 按照 DDD 限界上下文组织
type ScaleModule struct {
	// repository 层
	Repo scale.Repository

	// handler 层
	Handler *handler.ScaleHandler

	// service 层 - 按行为者组织
	LifecycleService scaleApp.ScaleLifecycleService
	FactorService    scaleApp.ScaleFactorService
	QueryService     scaleApp.ScaleQueryService
}

// NewScaleModule 创建 Scale 模块
func NewScaleModule() *ScaleModule {
	return &ScaleModule{}
}

// Initialize 初始化 Scale 模块
func (m *ScaleModule) Initialize(params ...interface{}) error {
	mongoDB := params[0].(*mongo.Database)
	if mongoDB == nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
	}

	// 初始化 repository 层
	m.Repo = scaleInfra.NewRepository(mongoDB)

	// 初始化 service 层（依赖 repository）
	m.LifecycleService = scaleApp.NewLifecycleService(m.Repo)
	m.FactorService = scaleApp.NewFactorService(m.Repo)
	m.QueryService = scaleApp.NewQueryService(m.Repo)

	// 初始化 handler 层
	m.Handler = handler.NewScaleHandler(
		m.LifecycleService,
		m.FactorService,
		m.QueryService,
	)

	return nil
}

// Cleanup 清理模块资源
func (m *ScaleModule) Cleanup() error {
	return nil
}

// CheckHealth 检查模块健康状态
func (m *ScaleModule) CheckHealth() error {
	return nil
}

// ModuleInfo 返回模块信息
func (m *ScaleModule) ModuleInfo() ModuleInfo {
	return ModuleInfo{
		Name:        "scale",
		Version:     "2.0.0",
		Description: "量表管理模块（重构版）",
	}
}
