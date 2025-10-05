package assembler

import (
	"go.mongodb.org/mongo-driver/mongo"

	interpretreportapp "github.com/fangcun-mount/qs-server/internal/apiserver/application/interpret-report"
	interpretreportport "github.com/fangcun-mount/qs-server/internal/apiserver/domain/interpret-report/port"
	interpretreportmongo "github.com/fangcun-mount/qs-server/internal/apiserver/infra/mongo/interpret-report"
)

// InterpretReportModule 解读报告模块
type InterpretReportModule struct {
	IRCreator interpretreportport.InterpretReportCreator
	IREditor  interpretreportport.InterpretReportEditor
	IRQueryer interpretreportport.InterpretReportQueryer
}

// NewInterpretReportModule 创建解读报告模块
func NewInterpretReportModule(mongoDB *mongo.Database) *InterpretReportModule {
	// 创建仓储
	repo := interpretreportmongo.NewRepository(mongoDB)

	// 创建应用服务
	creator := interpretreportapp.NewCreator(repo)
	editor := interpretreportapp.NewEditor(repo)
	queryer := interpretreportapp.NewQueryer(repo)

	return &InterpretReportModule{
		IRCreator: creator,
		IREditor:  editor,
		IRQueryer: queryer,
	}
}

// GetCreator 获取创建器
func (m *InterpretReportModule) GetCreator() interpretreportport.InterpretReportCreator {
	return m.IRCreator
}

// GetEditor 获取编辑器
func (m *InterpretReportModule) GetEditor() interpretreportport.InterpretReportEditor {
	return m.IREditor
}

// GetQueryer 获取查询器
func (m *InterpretReportModule) GetQueryer() interpretreportport.InterpretReportQueryer {
	return m.IRQueryer
}

// Initialize 初始化模块
func (m *InterpretReportModule) Initialize(params ...interface{}) error {
	// 此模块在构造函数中已经初始化，这里不需要做额外的初始化
	return nil
}

// CheckHealth 检查模块健康状态
func (m *InterpretReportModule) CheckHealth() error {
	return nil
}

// Cleanup 清理模块资源
func (m *InterpretReportModule) Cleanup() error {
	return nil
}

// ModuleInfo 返回模块信息
func (m *InterpretReportModule) ModuleInfo() ModuleInfo {
	return ModuleInfo{
		Name:        "interpretreport",
		Version:     "1.0.0",
		Description: "解读报告管理模块",
	}
}
