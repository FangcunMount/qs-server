package statistics

import resttransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest"

func (m *Module) ExportRESTDeps() resttransport.StatisticsDeps {
	if m == nil {
		return resttransport.StatisticsDeps{}
	}
	return resttransport.StatisticsDeps{
		Enabled:     true,
		ReadService: m.ReadService,
		Coordinator: m.Coordinator,
		RunStore:    m.RunStore,
	}
}
