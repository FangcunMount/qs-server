package platform

import (
	auth "github.com/FangcunMount/iam/v2/pkg/sdk/auth/verifier"
	codesapp "github.com/FangcunMount/qs-server/internal/apiserver/application/codes"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/cache/governance"
	iaminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	objectstorageport "github.com/FangcunMount/qs-server/internal/apiserver/infra/objectstorage/port"
	resttransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
)

// RESTIntegrationDeps are platform/integration surfaces wired into REST transport.
type RESTIntegrationDeps struct {
	CodesService            codesapp.CodesService
	QRCodeObjectStore       objectstorageport.PublicObjectStore
	QRCodeObjectKeyPrefix   string
	GovernanceStatusService cachegov.StatusService
	EventStatusService      appEventing.StatusService
	Backpressure            []resilienceplane.BackpressureSnapshot
	IAM                     RESTIAMDeps
}

// RESTIAMDeps are IAM integration surfaces for REST transport.
type RESTIAMDeps struct {
	Enabled                 bool
	TokenVerifier           *auth.TokenVerifier
	ForceRemoteVerification bool
	SnapshotLoader          *iaminfra.AuthzSnapshotLoader
}

// RESTEventStatusInput collects outbox status readers for event status export.
type RESTEventStatusInput struct {
	Catalog                    *eventcatalog.Catalog
	SurveyAnswerSheetOutbox    appEventing.NamedOutboxStatusReader
	EvaluationAssessmentOutbox appEventing.NamedOutboxStatusReader
}

// ExportRESTIntegrationDeps maps platform integration ports to REST transport fields.
func ExportRESTIntegrationDeps(in RESTIntegrationDeps) resttransport.Deps {
	return resttransport.Deps{
		CodesService:            in.CodesService,
		QRCodeObjectStore:       in.QRCodeObjectStore,
		QRCodeObjectKeyPrefix:   in.QRCodeObjectKeyPrefix,
		GovernanceStatusService: in.GovernanceStatusService,
		EventStatusService:      in.EventStatusService,
		Backpressure:            in.Backpressure,
		IAM: resttransport.IAMDeps{
			Enabled:                 in.IAM.Enabled,
			TokenVerifier:           in.IAM.TokenVerifier,
			ForceRemoteVerification: in.IAM.ForceRemoteVerification,
			SnapshotLoader:          in.IAM.SnapshotLoader,
		},
	}
}

// BuildRESTEventStatusService assembles the read-only event status service.
func BuildRESTEventStatusService(in RESTEventStatusInput) appEventing.StatusService {
	outboxes := make([]appEventing.NamedOutboxStatusReader, 0, 2)
	if in.SurveyAnswerSheetOutbox.Reader != nil {
		outboxes = append(outboxes, in.SurveyAnswerSheetOutbox)
	}
	if in.EvaluationAssessmentOutbox.Reader != nil {
		outboxes = append(outboxes, in.EvaluationAssessmentOutbox)
	}
	return appEventing.NewStatusService(appEventing.StatusServiceOptions{
		Catalog:  in.Catalog,
		Outboxes: outboxes,
	})
}
