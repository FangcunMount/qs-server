package platform

import (
	notificationApp "github.com/FangcunMount/qs-server/internal/apiserver/application/notification"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	iaminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	grpctransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc"
)

// GRPCIntegrationDeps are platform/integration surfaces wired into gRPC transport.
type GRPCIntegrationDeps struct {
	WarmupCoordinator                  statisticsApp.WarmupCoordinator
	QRCodeService                      grpctransport.SurveyScaleQRCodeGenerator
	MiniProgramTaskNotificationService notificationApp.MiniProgramTaskNotificationService
	AuthzSnapshotLoader                *iaminfra.AuthzSnapshotLoader
	PublishedModelCatalog              rulesetport.Catalog
}

// ExportGRPCIntegrationDeps maps platform integration ports to gRPC transport fields.
func ExportGRPCIntegrationDeps(in GRPCIntegrationDeps) grpctransport.Deps {
	return grpctransport.Deps{
		WarmupCoordinator:                  in.WarmupCoordinator,
		QRCodeService:                      in.QRCodeService,
		MiniProgramTaskNotificationService: in.MiniProgramTaskNotificationService,
		IAM: grpctransport.IAMDeps{
			AuthzSnapshotLoader: in.AuthzSnapshotLoader,
		},
		PublishedModelCatalog: in.PublishedModelCatalog,
	}
}
