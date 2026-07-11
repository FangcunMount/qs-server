package registry

// NewReportBuilderRegistry 创建注册表 从 given builders。
func NewReportBuilderRegistry(builders ...ReportBuilder) (*mutableReportBuilderRegistry, error) {
	r := &mutableReportBuilderRegistry{
		mechanismItems: make(map[MechanismReportBuilderKey]ReportBuilder),
	}
	for _, builder := range builders {
		if err := r.Register(builder); err != nil {
			return nil, err
		}
	}
	return r, nil
}
