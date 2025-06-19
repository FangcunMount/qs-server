package app

// App 应用
type App struct {
	basename string
	name     string
}

// Option 应用选项
type Option func(*App)

// NewApp 创建应用
func NewApp(name string, basename string, opts ...Option) *App {
	// 创建 App
	a := &App{
		name:     name,
		basename: basename,
	}
	// 设置应用选项
	for _, opt := range opts {
		opt(a)
	}

	// 返回 app
	return a
}
