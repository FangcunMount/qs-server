package platform

import (
	auth "github.com/FangcunMount/iam/v2/pkg/sdk/auth/verifier"
	cachegovernance "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	codesapp "github.com/FangcunMount/qs-server/internal/apiserver/application/codes"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	iaminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	objectstorageport "github.com/FangcunMount/qs-server/internal/apiserver/infra/objectstorage/port"
	resttransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest"
)

// RESTIntegrationDeps are platform/integration surfaces wired into REST transport.
type RESTIntegrationDeps struct {
	CodesService            codesapp.CodesService
	QRCodeObjectStore       objectstorageport.PublicObjectStore
	QRCodeObjectKeyPrefix   string
	GovernanceStatusService cachegovernance.StatusReader
	EventStatusService      appEventing.StatusService
	IAM                     RESTIAMDeps
}

// RESTIAMDeps are IAM integration surfaces for REST transport.
type RESTIAMDeps struct {
	Enabled                 bool
	TokenVerifier           *auth.TokenVerifier
	ForceRemoteVerification bool
	SnapshotLoader          *iaminfra.AuthzSnapshotLoader
}

// ExportRESTIntegrationDeps maps platform integration ports to REST transport fields.
func ExportRESTIntegrationDeps(in RESTIntegrationDeps) resttransport.Deps {
	return resttransport.Deps{
		CodesService:            in.CodesService,
		QRCodeObjectStore:       in.QRCodeObjectStore,
		QRCodeObjectKeyPrefix:   in.QRCodeObjectKeyPrefix,
		GovernanceStatusService: in.GovernanceStatusService,
		EventStatusService:      in.EventStatusService,
		IAM: resttransport.IAMDeps{
			Enabled:                 in.IAM.Enabled,
			TokenVerifier:           in.IAM.TokenVerifier,
			ForceRemoteVerification: in.IAM.ForceRemoteVerification,
			SnapshotLoader:          in.IAM.SnapshotLoader,
		},
	}
}
