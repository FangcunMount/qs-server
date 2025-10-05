package assembler

import (
	"go.mongodb.org/mongo-driver/mongo"

	msApp "github.com/fangcun-mount/qs-server/internal/apiserver/application/medical-scale"
	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/medical-scale/port"
	msInfra "github.com/fangcun-mount/qs-server/internal/apiserver/infra/mongo/medical-scale"
	"github.com/fangcun-mount/qs-server/internal/apiserver/interface/restful/handler"
	"github.com/fangcun-mount/qs-server/internal/pkg/code"
	"github.com/fangcun-mount/qs-server/pkg/errors"
)

// MedicalScaleModule 医学量表模块
type MedicalScaleModule struct {
	// repository 层
	MSRepo port.MedicalScaleRepositoryMongo

	// handler 层
	MSHandler *handler.MedicalScaleHandler

	// service 层
	MSCreator port.MedicalScaleCreator
	MSEditor  port.MedicalScaleEditor
	MSQueryer port.MedicalScaleQueryer
}

// NewMedicalScaleModule 创建医学量表模块
func NewMedicalScaleModule() *MedicalScaleModule {
	return &MedicalScaleModule{}
}

// Initialize 初始化模块
func (m *MedicalScaleModule) Initialize(params ...interface{}) error {
	mongoDB := params[0].(*mongo.Database)
	if mongoDB == nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
	}

	// 初始化 repository 层
	m.MSRepo = msInfra.NewRepository(mongoDB)

	// 初始化 service 层
	m.MSCreator = msApp.NewCreator(m.MSRepo)
	m.MSEditor = msApp.NewEditor(m.MSRepo)
	m.MSQueryer = msApp.NewQueryer(m.MSRepo)

	// 初始化 handler 层
	m.MSHandler = handler.NewMedicalScaleHandler(
		m.MSCreator,
		m.MSQueryer,
		m.MSEditor,
	)

	return nil
}

// Cleanup 清理模块资源
func (m *MedicalScaleModule) Cleanup() error {
	// 如果有需要清理的资源，在这里进行清理
	return nil
}

// CheckHealth 检查模块健康状态
func (m *MedicalScaleModule) CheckHealth() error {
	return nil
}

// ModuleInfo 返回模块信息
func (m *MedicalScaleModule) ModuleInfo() ModuleInfo {
	return ModuleInfo{
		Name:        "medicalscale",
		Version:     "1.0.0",
		Description: "医学量表管理模块",
	}
}
