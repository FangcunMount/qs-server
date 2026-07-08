package platform

import (
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	notificationApp "github.com/FangcunMount/qs-server/internal/apiserver/application/notification"
	iaminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	grpctransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc"
)

// GRPCIntegrationDeps are platform/integration surfaces wired into gRPC transport.
type GRPCIntegrationDeps struct {
	WarmupCoordinator                  cachegov.Coordinator
	QRCodeService                      grpctransport.SurveyScaleQRCodeGenerator
	MiniProgramTaskNotificationService notificationApp.MiniProgramTaskNotificationService
	AuthzSnapshotLoader                *iaminfra.AuthzSnapshotLoader
	RuleSetCatalog                     rulesetport.Catalog
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
		RuleSet: grpctransport.RuleSetDeps{
			RuleSetCatalog: in.RuleSetCatalog,
		},
	}
}
