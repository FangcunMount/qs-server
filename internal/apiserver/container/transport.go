package container

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"strconv"
	"time"

	baseerrors "github.com/FangcunMount/component-base/pkg/errors"
	auth "github.com/FangcunMount/iam/v2/pkg/sdk/auth/verifier"
	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	operatorApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operator"
	cachegovernance "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	evaluationOperator "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/operator"
	evaluationScheduler "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/scheduler"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	interpretationAutomation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation"
	interpretationcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/catalogreconcile"
	interpretationReadmission "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/readmission"
	interpretationReportTemplate "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporttemplate"
	reportqueryjourney "github.com/FangcunMount/qs-server/internal/apiserver/application/journey/reportquery"
	reportwaitjourney "github.com/FangcunMount/qs-server/internal/apiserver/application/journey/reportwait"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	systemgovApp "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	workbenchApp "github.com/FangcunMount/qs-server/internal/apiserver/application/workbench"
	platformmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/platform"
	surveymod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/admission"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	domainreporttemplate "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/reporttemplate"
	interpretationrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	iaminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	grpctransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc"
	resttransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	grpcpkg "github.com/FangcunMount/qs-server/internal/pkg/grpc"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

func (c *Container) BuildRESTDeps(rateCfg *options.RateLimitOptions) resttransport.Deps {
	deps := resttransport.Deps{RateLimit: rateCfg}
	if c == nil {
		return deps
	}

	platformDeps := platformmod.ExportRESTIntegrationDeps(platformmod.RESTIntegrationDeps{
		CodesService:            c.CodesService,
		QRCodeObjectStore:       c.QRCodeObjectStore,
		QRCodeObjectKeyPrefix:   c.QRCodeObjectKeyPrefix,
		GovernanceStatusService: c.CacheGovernanceStatusService(),
		EventStatusService:      c.buildRESTEventStatusService(),
		IAM:                     c.exportRESTIAMDeps(),
	})
	deps.CodesService = platformDeps.CodesService
	deps.QRCodeObjectStore = platformDeps.QRCodeObjectStore
	deps.QRCodeObjectKeyPrefix = platformDeps.QRCodeObjectKeyPrefix
	deps.AssessmentAssetStore = c.AssessmentAssetStore
	deps.AssessmentAssetKeyPrefix = c.AssessmentAssetKeyPrefix
	deps.GovernanceStatusService = platformDeps.GovernanceStatusService
	deps.EventStatusService = platformDeps.EventStatusService
	deps.RateBudgets = c.resilience
	if c.resilience != nil {
		deps.ResilienceSnapshot = func() resilience.RuntimeSnapshot { return c.resilience.Snapshot(time.Now()) }
	}
	deps.IAM = platformDeps.IAM

	if c.SurveyModule != nil {
		deps.Survey = c.SurveyModule.ExportRESTDeps(surveymod.RESTExportOptions{
			QRCodeService: c.QRCodeService,
		})
	}
	if c.AssessmentModelModule != nil {
		exports := c.AssessmentModelModule.ExportRESTDeps(c.QRCodeService, c.CodesService, deps.Survey.QuestionnaireQueryService)
		deps.AssessmentModel = exports.AssessmentModel
		deps.AssessmentModel.Assets = c.OutcomeImageService
	}
	if c.ActorModule != nil {
		deps.Actor = c.ActorModule.ExportRESTDeps(c.QRCodeService)
	}
	if c.EvaluationModule != nil {
		deps.Evaluation = c.EvaluationModule.ExportRESTDeps()
		if c.ActorModule != nil {
			deps.Actor.TesteeScaleAnalysisService = c.EvaluationModule.ExportTesteeScaleAnalysisService()
		}
	}
	if c.EvaluationModule != nil && c.ReportModule != nil {
		reportQuery := reportqueryjourney.NewAdministrationService(c.ReportModule.ReportReader(), c.ReportModule.AdministrationService(), c.EvaluationModule.OperatorQuery)
		deps.Interpretation.ReportQueryJourney = reportQuery
		deps.Interpretation.ReportWaitJourney = reportwaitjourney.NewService(
			c.EvaluationModule.OperatorQuery,
			reportQuery,
		)
		deps.Interpretation.ClinicianService = c.ReportModule.ClinicianService()
		deps.Interpretation.OperationsService = c.ReportModule.OperationsService()
		deps.Interpretation.CatalogReconcile = c.ReportModule.CatalogReconcileService()
		deps.Interpretation.ReportTemplates = c.ReportModule.ReportTemplateService()
	}
	if c.PlanModule != nil {
		var testeeAccess actorAccessApp.TesteeAccessService
		if c.ActorModule != nil {
			testeeAccess = c.ActorModule.TesteeAccessService
		}
		deps.Plan = c.PlanModule.ExportRESTDeps(testeeAccess)
	}
	deps.Workbench = composeRESTWorkbenchDeps(c)
	if c.StatisticsModule != nil {
		deps.Statistics = c.StatisticsModule.ExportRESTDeps()
	}

	deps.SystemGovernanceFacade = c.buildRESTSystemGovernanceFacade()

	return deps
}

func (c *Container) buildRESTSystemGovernanceFacade() systemgovApp.Facade {
	if c == nil {
		return platformmod.BuildRESTSystemGovernanceFacade(platformmod.RESTSystemGovernanceInput{})
	}
	eventStatus := c.buildRESTEventStatusService()
	var outboxes []appEventing.NamedOutboxStatusReader
	if c.eventSubsystem != nil {
		outboxes = c.eventSubsystem.Outboxes()
	}
	cacheGovernance := cachegovernance.NewFacade(
		"apiserver",
		c.WarmupCoordinator(),
		c.CacheGovernanceStatusService(),
	)
	return platformmod.BuildRESTSystemGovernanceFacade(platformmod.RESTSystemGovernanceInput{
		Options:                 c.systemGovernanceOptions,
		EventStatusService:      eventStatus,
		EventOutboxes:           outboxes,
		CacheGovernance:         cacheGovernance,
		CachePolicyReloader:     c.CachePolicyReloader(),
		MySQLDB:                 c.mysqlDB,
		MongoDB:                 c.mongoDB,
		ResilienceGovernor:      c.resilience,
		LocalResilienceSnapshot: c.localResilienceSnapshot(),
		ActionAuditStore:        c.actionAuditStore,
		ActionHandlers:          c.retryGovernanceActionHandlers(),
		EventPublisher:          c.eventPublisher,
	})
}

type retryActionInput struct {
	ResourceID      string `json:"resource_id"`
	ExpectedAttempt int    `json:"expected_attempt"`
	Reason          string `json:"reason"`
}

type reportTemplateActionInput struct {
	TemplateID      string `json:"template_id"`
	TemplateVersion string `json:"template_version"`
	ExpectedStatus  string `json:"expected_status"`
	Reason          string `json:"reason"`
}

type readmissionActionInput struct {
	FailureFingerprint     string `json:"failure_fingerprint"`
	ExpectedReason         string `json:"expected_reason"`
	ExpectedOutcomeVersion string `json:"expected_outcome_version"`
	Reason                 string `json:"reason"`
}

type catalogRepairActionInput struct {
	DryRunID               string `json:"dry_run_id"`
	ExpectedCatalogVersion string `json:"expected_catalog_version"`
	ExpectedSource         string `json:"expected_source"`
	Reason                 string `json:"reason"`
}

func (c *Container) retryGovernanceActionHandlers() map[string]systemgovApp.ActionHandler {
	handlers := map[string]systemgovApp.ActionHandler{}
	if c != nil && c.EvaluationModule != nil && c.EvaluationModule.GovernedRetry != nil {
		for _, spec := range []struct {
			id     string
			origin retrygovernance.AttemptOrigin
		}{{"evaluation.retry", retrygovernance.AttemptOriginManual}, {"evaluation.force_retry", retrygovernance.AttemptOriginForce}} {
			spec := spec
			handlers[spec.id] = func(ctx context.Context, orgID int64, requestID string, input map[string]interface{}) (map[string]interface{}, error) {
				request, err := decodeRetryActionInput(input)
				if err != nil {
					return nil, err
				}
				assessmentID, err := strconv.ParseUint(request.ResourceID, 10, 64)
				if err != nil || assessmentID == 0 {
					return nil, fmt.Errorf("invalid evaluation resource_id")
				}
				run, err := c.EvaluationModule.GovernedRetry.Authorize(ctx, evaluationOperator.Actor{OrgID: orgID, OperatorUserID: int64(actorctx.GrantingUserID(ctx))}, evaluationOperator.GovernedRetryCommand{
					AssessmentID: assessmentID, ExpectedAttempt: request.ExpectedAttempt, Origin: spec.origin, RequestID: requestID, Reason: request.Reason,
				})
				if err != nil {
					return nil, normalizeGovernedRetryError(err)
				}
				return map[string]interface{}{"assessment_id": assessmentID, "authorized_attempt": run.Attempt().Number, "origin": spec.origin}, nil
			}
		}
	}
	if c != nil && c.ReportModule != nil && c.ReportModule.GovernedRetryService() != nil {
		for _, spec := range []struct {
			id     string
			origin retrygovernance.AttemptOrigin
		}{{"interpretation.retry", retrygovernance.AttemptOriginManual}, {"interpretation.force_retry", retrygovernance.AttemptOriginForce}} {
			spec := spec
			handlers[spec.id] = func(ctx context.Context, orgID int64, requestID string, input map[string]interface{}) (map[string]interface{}, error) {
				request, err := decodeRetryActionInput(input)
				if err != nil {
					return nil, err
				}
				generationID, err := meta.ParseID(request.ResourceID)
				if err != nil || generationID.IsZero() {
					return nil, fmt.Errorf("invalid interpretation resource_id")
				}
				run, err := c.ReportModule.GovernedRetryService().Authorize(ctx, interpretationAutomation.GovernedRetryCommand{
					OrgID: orgID, GenerationID: generationID, ExpectedAttempt: request.ExpectedAttempt, Origin: spec.origin, RequestID: requestID, Reason: request.Reason,
				})
				if err != nil {
					return nil, normalizeGovernedRetryError(err)
				}
				return map[string]interface{}{"generation_id": generationID.String(), "authorized_attempt": run.Attempt(), "origin": spec.origin}, nil
			}
		}
	}
	if c != nil && c.ReportModule != nil && c.ReportModule.ReportTemplateService() != nil {
		service := c.ReportModule.ReportTemplateService()
		handlers["interpretation.report_template_publish"] = reportTemplateGovernanceHandler(service, true)
		handlers["interpretation.report_template_disable"] = reportTemplateGovernanceHandler(service, false)
	}
	if c != nil && c.ReportModule != nil && c.ReportModule.ReadmissionService() != nil {
		handlers["interpretation.readmit_outcome"] = func(ctx context.Context, orgID int64, requestID string, input map[string]interface{}) (map[string]interface{}, error) {
			var request readmissionActionInput
			payload, err := json.Marshal(input)
			if err != nil {
				return nil, err
			}
			if err := json.Unmarshal(payload, &request); err != nil {
				return nil, err
			}
			result, err := c.ReportModule.ReadmissionService().Readmit(ctx, interpretationReadmission.Command{
				OrgID: orgID, OperatorUserID: int64(actorctx.GrantingUserID(ctx)), RequestID: requestID,
				FailureFingerprint:     request.FailureFingerprint,
				ExpectedReason:         admission.Kind(request.ExpectedReason),
				ExpectedOutcomeVersion: request.ExpectedOutcomeVersion, Reason: request.Reason,
			})
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"outcome_id": result.OutcomeID, "generation_id": result.GenerationID,
				"run_id": result.RunID, "report_id": result.ReportID, "status": result.Status,
			}, nil
		}
	}
	if c != nil && c.ReportModule != nil && c.ReportModule.CatalogReconcileService() != nil {
		handlers["interpretation.catalog_repair"] = func(ctx context.Context, orgID int64, _ string, input map[string]interface{}) (map[string]interface{}, error) {
			var request catalogRepairActionInput
			payload, err := json.Marshal(input)
			if err != nil {
				return nil, err
			}
			if err := json.Unmarshal(payload, &request); err != nil {
				return nil, err
			}
			if request.Reason == "" {
				return nil, fmt.Errorf("catalog repair reason is required")
			}
			result, err := c.ReportModule.CatalogReconcileService().Repair(ctx, interpretationcatalog.RepairCommand{
				OrgID: orgID, DryRunID: request.DryRunID,
				ExpectedCatalogVersion: request.ExpectedCatalogVersion, ExpectedSource: request.ExpectedSource,
			})
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{"status": result.Status, "item": result.Item}, nil
		}
	}
	return handlers
}

func reportTemplateGovernanceHandler(service interpretationReportTemplate.Service, publish bool) systemgovApp.ActionHandler {
	return func(ctx context.Context, _ int64, _ string, input map[string]interface{}) (map[string]interface{}, error) {
		var request reportTemplateActionInput
		payload, err := json.Marshal(input)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(payload, &request); err != nil {
			return nil, err
		}
		if request.TemplateID == "" || request.TemplateVersion == "" || request.ExpectedStatus == "" || request.Reason == "" {
			return nil, fmt.Errorf("template_id, template_version, expected_status and reason are required")
		}
		version := policy.TemplateVersion(request.TemplateVersion)
		current, err := service.Get(ctx, request.TemplateID, version)
		if err != nil {
			return nil, err
		}
		if string(current.Status()) != request.ExpectedStatus {
			return nil, baseerrors.WithCode(code.ErrConflict, "report template status changed")
		}
		actor := interpretationReportTemplate.Actor{OperatorUserID: int64(actorctx.GrantingUserID(ctx))}
		var updated *domainreporttemplate.ReportTemplate
		if publish {
			updated, err = service.Publish(ctx, interpretationReportTemplate.PublishCommand{
				Actor: actor, TemplateID: request.TemplateID, TemplateVersion: version,
			})
		} else {
			updated, err = service.Disable(ctx, interpretationReportTemplate.DisableCommand{
				Actor: actor, TemplateID: request.TemplateID, TemplateVersion: version,
			})
		}
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"template_id": updated.TemplateID(), "template_version": updated.TemplateVersion().String(),
			"status": updated.Status(),
		}, nil
	}
}

func normalizeGovernedRetryError(err error) error {
	if err == nil {
		return nil
	}
	if stderrors.Is(err, evaluationrun.ErrClaimLost) || stderrors.Is(err, evalrun.ErrInvalidRetrySchedule) ||
		stderrors.Is(err, generation.ErrVersionConflict) || stderrors.Is(err, interpretationrun.ErrInvalidRetrySchedule) {
		return baseerrors.WithCode(code.ErrConflict, "%s", err.Error())
	}
	return err
}

func decodeRetryActionInput(input map[string]interface{}) (retryActionInput, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return retryActionInput{}, err
	}
	var request retryActionInput
	if err := json.Unmarshal(payload, &request); err != nil {
		return retryActionInput{}, err
	}
	if request.ResourceID == "" || request.ExpectedAttempt < 1 || request.Reason == "" {
		return retryActionInput{}, fmt.Errorf("resource_id, expected_attempt and reason are required")
	}
	return request, nil
}

func (c *Container) localResilienceSnapshot() func() resilience.RuntimeSnapshot {
	if c != nil && c.resilience != nil {
		return func() resilience.RuntimeSnapshot { return c.resilience.Snapshot(time.Now()) }
	}
	return func() resilience.RuntimeSnapshot { return resilience.RuntimeSnapshot{Component: "apiserver"} }
}

func (c *Container) buildRESTEventStatusService() appEventing.StatusService {
	if c == nil || c.eventSubsystem == nil {
		return nil
	}
	return c.eventSubsystem.StatusService()
}

func (c *Container) exportRESTIAMDeps() platformmod.RESTIAMDeps {
	deps := platformmod.RESTIAMDeps{}
	if c == nil || c.IAMModule == nil {
		return deps
	}
	deps.Enabled = c.IAMModule.IsEnabled()
	deps.TokenVerifier = c.IAMModule.SDKTokenVerifier()
	deps.SnapshotLoader = c.IAMModule.AuthzSnapshotLoader()
	if client := c.IAMModule.Client(); client != nil && client.Config() != nil && client.Config().JWT != nil {
		deps.ForceRemoteVerification = client.Config().JWT.ForceRemoteVerification
	}
	return deps
}

func (c *Container) BuildGRPCDeps(server *grpcpkg.Server) grpctransport.Deps {
	deps := grpctransport.Deps{Server: server}
	if c == nil {
		return deps
	}

	platformDeps := platformmod.ExportGRPCIntegrationDeps(platformmod.GRPCIntegrationDeps{
		WarmupCoordinator:                  c.WarmupCoordinator(),
		QRCodeService:                      c.QRCodeService,
		MiniProgramTaskNotificationService: c.MiniProgramTaskNotificationService,
		AuthzSnapshotLoader:                c.exportGRPCAuthzSnapshotLoader(),
		PublishedModelCatalog:              c.exportGRPCPublishedModelCatalog(),
	})
	deps.WarmupCoordinator = platformDeps.WarmupCoordinator
	deps.QRCodeService = platformDeps.QRCodeService
	deps.MiniProgramTaskNotificationService = platformDeps.MiniProgramTaskNotificationService
	deps.IAM = platformDeps.IAM
	deps.PublishedModelCatalog = platformDeps.PublishedModelCatalog

	if c.SurveyModule != nil {
		deps.Survey = c.SurveyModule.ExportGRPCDeps()
	}
	if c.ActorModule != nil {
		deps.Actor = c.ActorModule.ExportGRPCDeps()
	}
	if c.EvaluationModule != nil {
		deps.Evaluation = c.EvaluationModule.ExportGRPCDeps()
	}
	if c.ReportModule != nil {
		deps.Interpretation = c.ReportModule.ExportGRPCDeps()
	}
	if c.AssessmentModelModule != nil {
		exports := c.AssessmentModelModule.ExportGRPCDeps()
		deps.AssessmentModelCatalog = exports.AssessmentModelCatalog
	}
	if c.PlanModule != nil {
		deps.Plan = c.PlanModule.ExportGRPCDeps()
	}
	return deps
}

func (c *Container) exportGRPCAuthzSnapshotLoader() *iaminfra.AuthzSnapshotLoader {
	if c == nil || c.IAMModule == nil {
		return nil
	}
	return c.IAMModule.AuthzSnapshotLoader()
}

func (c *Container) exportGRPCPublishedModelCatalog() rulesetport.Catalog {
	return c.PublishedModelCatalog()
}

func composeRESTWorkbenchDeps(c *Container) resttransport.WorkbenchDeps {
	deps := resttransport.WorkbenchDeps{}
	if c == nil || c.ActorModule == nil || c.EvaluationModule == nil || c.PlanModule == nil {
		return deps
	}
	if c.ActorModule.OperatorQueryService == nil ||
		c.ActorModule.ClinicianQueryService == nil ||
		c.ActorModule.ClinicianRelationshipService == nil ||
		c.ActorModule.ReadModel == nil ||
		c.workbenchLatestRiskReader == nil ||
		c.PlanModule.FollowUpQueueReader == nil {
		return deps
	}
	deps.WorkbenchService = workbenchApp.NewService(
		c.ActorModule.OperatorQueryService,
		c.ActorModule.ClinicianQueryService,
		c.ActorModule.ClinicianRelationshipService,
		c.ActorModule.ReadModel,
		c.ActorModule.ReadModel,
		c.workbenchLatestRiskReader,
		c.PlanModule.FollowUpQueueReader,
	)
	return deps
}

// ServerGRPCBootstrapDeps describes the narrow container-owned dependencies
// needed to build the process gRPC server.
type ServerGRPCBootstrapDeps struct {
	AuthzSnapshotLoader           *iaminfra.AuthzSnapshotLoader
	OperatorRoleProjectionUpdater operatorApp.OperatorRoleProjectionUpdater
	ActiveOperatorChecker         operatorApp.ActiveOperatorChecker
	TokenVerifier                 *auth.TokenVerifier
}

// ServerRuntimeDeps describes the narrow container-owned dependencies needed by
// background runtimes started from the apiserver process.
type ServerRuntimeDeps struct {
	LockBuilder                           *keyspace.Builder
	LockManager                           locklease.Manager
	WarmupCoordinator                     cachegovernance.WarmupCoordinator
	PlanCommandService                    planApp.PlanCommandService
	StatisticsCoordinator                 *statisticsApp.Coordinator
	EvaluationConsistencyReconcileService evaluationScheduler.Service
}

func (c *Container) BuildServerGRPCBootstrapDeps() ServerGRPCBootstrapDeps {
	var deps ServerGRPCBootstrapDeps
	if c == nil {
		return deps
	}
	if c.IAMModule != nil {
		deps.AuthzSnapshotLoader = c.IAMModule.AuthzSnapshotLoader()
		deps.TokenVerifier = c.IAMModule.SDKTokenVerifier()
	}
	if c.ActorModule != nil {
		deps.OperatorRoleProjectionUpdater = c.ActorModule.OperatorRoleProjectionUpdater
		deps.ActiveOperatorChecker = c.ActorModule.ActiveOperatorChecker
	}
	return deps
}

func (c *Container) BuildServerRuntimeDeps() ServerRuntimeDeps {
	var deps ServerRuntimeDeps
	if c == nil {
		return deps
	}

	if c.locks != nil {
		deps.LockBuilder = c.locks.Builder()
	}
	deps.LockManager = c.LockManager()
	deps.WarmupCoordinator = c.WarmupCoordinator()

	if c.PlanModule != nil {
		deps.PlanCommandService = c.PlanModule.CommandService
	}
	if c.StatisticsModule != nil {
		deps.StatisticsCoordinator = c.StatisticsModule.Coordinator
	}
	if c.EvaluationModule != nil {
		recoverers := []evaluationScheduler.LeaseRecoverer{}
		auditors := []evaluationScheduler.ConsistencyAuditor{}
		leaseRecoveryEnabled := c.systemGovernanceOptions == nil || c.systemGovernanceOptions.Retry == nil || c.systemGovernanceOptions.Retry.LeaseReconcileEnabled
		if leaseRecoveryEnabled && c.EvaluationModule.LeaseRecoverer != nil {
			recoverers = append(recoverers, c.EvaluationModule.LeaseRecoverer)
		}
		if leaseRecoveryEnabled && c.ReportModule != nil && c.ReportModule.LeaseRecoverer() != nil {
			recoverers = append(recoverers, c.ReportModule.LeaseRecoverer())
		}
		if c.ReportModule != nil && c.ReportModule.CatalogReconcileAuditor() != nil {
			auditors = append(auditors, c.ReportModule.CatalogReconcileAuditor())
		}
		deps.EvaluationConsistencyReconcileService = evaluationScheduler.NewGovernedServiceWithAuditors(
			c.EvaluationModule.SchedulerService,
			auditors,
			recoverers...,
		)
	}

	return deps
}
