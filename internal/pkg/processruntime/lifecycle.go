package processruntime

// ShutdownHook 是一个带名字的关闭钩子。
type ShutdownHook struct {
	name string
	run  func() error
}

// Name 返回钩子名称。
func (h ShutdownHook) Name() string {
	return h.name
}

// Run 执行关闭钩子。
func (h ShutdownHook) Run() error {
	if h.run == nil {
		return nil
	}
	return h.run()
}

// Lifecycle 负责收集并顺序执行关闭钩子。
type Lifecycle struct {
	hooks []ShutdownHook
}

// AddShutdownHook 添加关闭钩子。
func (l *Lifecycle) AddShutdownHook(name string, run func() error) {
	if l == nil || run == nil {
		return
	}
	l.hooks = append(l.hooks, ShutdownHook{name: name, run: run})
}

// Len 返回当前已注册的关闭钩子数量。
func (l Lifecycle) Len() int {
	return len(l.hooks)
}

// Run 顺序执行所有关闭钩子；如果某个钩子失败，会通过 onError 回调汇报，但不会中断后续执行。
func (l Lifecycle) Run(onError func(name string, err error)) {
	for _, hook := range l.hooks {
		if err := hook.Run(); err != nil && onError != nil {
			onError(hook.Name(), err)
		}
	}
}
