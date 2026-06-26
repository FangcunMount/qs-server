package assessmentmodel

import (
	qrcodeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	scaleApp "github.com/FangcunMount/qs-server/internal/apiserver/application/scale"
	resttransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest"
)

// ExportRESTDeps exposes scale capabilities to REST transport.
func (m *Module) ExportRESTDeps(qrCodeService qrcodeApp.QRCodeService) resttransport.ScaleDeps {
	if m == nil || m.Scale == nil {
		return resttransport.ScaleDeps{}
	}
	return m.Scale.ExportRESTDeps(qrCodeService)
}

// ExportRESTDeps exposes scale capabilities to REST transport.
func (s *Scale) ExportRESTDeps(qrCodeService qrcodeApp.QRCodeService) resttransport.ScaleDeps {
	deps := resttransport.ScaleDeps{}
	if s == nil {
		return deps
	}
	deps.LifecycleService = s.LifecycleService
	deps.FactorService = s.FactorService
	deps.QueryService = s.QueryService
	deps.CategoryService = s.CategoryService
	deps.QRCodeService = scaleApp.NewQRCodeQueryService(qrCodeService)
	return deps
}
