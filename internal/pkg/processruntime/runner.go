package processruntime

// Stage 表示一个可以按顺序执行的进程启动阶段。
type Stage[S any] interface {
	Name() string
	Run(*S) error
}

// Runner 负责按顺序执行一组阶段，并在成功后构建 prepared output。
type Runner[S any, P any] struct {
	State         *S
	Stages        []Stage[S]
	BuildPrepared func(*S) P
}

// Run 按顺序执行阶段，并返回 prepared output、失败阶段名和错误。
func (r Runner[S, P]) Run() (P, string, error) {
	var zero P

	state := r.State
	if state == nil {
		state = new(S)
	}

	for _, stage := range r.Stages {
		if stage == nil {
			continue
		}
		if err := stage.Run(state); err != nil {
			return zero, stage.Name(), err
		}
	}

	if r.BuildPrepared == nil {
		return zero, "", nil
	}
	return r.BuildPrepared(state), "", nil
}
