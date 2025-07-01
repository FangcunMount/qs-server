package assembler

// Module 模块接口
type Module interface {
	Initialize(params ...interface{}) error
	CheckHealth() error
	Cleanup() error
	ModuleInfo() ModuleInfo
}

// ModuleInfo 模块信息
type ModuleInfo struct {
	Name        string
	Version     string
	Description string
}

// RepoComponent 响应组件
type RepoComponent struct {
	Name        string
	Description string
	Repository  interface{}
}

// ServiceComponent 服务组件
type ServiceComponent struct {
	Name        string
	Description string
	Service     interface{}
}

// HandlerComponent 处理器组件
type HandlerComponent struct {
	Name        string
	Description string
	Handler     interface{}
}
