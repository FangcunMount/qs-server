package survey

import (
	"context"
	"sync"

	"github.com/FangcunMount/component-base/pkg/errors"
	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/securityplane"
)

// catalogBindingSyncer adapts questionnaire publication to the unified
// catalog-management use case after both modules have been assembled.
type catalogBindingSyncer struct {
	mu         sync.RWMutex
	management modelcatalog.CatalogManagementService
}

func (s *catalogBindingSyncer) SetCatalogManagementService(service modelcatalog.CatalogManagementService) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.management = service
	s.mu.Unlock()
}

func (s *catalogBindingSyncer) SyncQuestionnaireVersion(ctx context.Context, questionnaireCode, version string) error {
	if s == nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "questionnaire catalog management is not configured")
	}
	s.mu.RLock()
	service := s.management
	s.mu.RUnlock()
	if service == nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "questionnaire catalog management is not configured")
	}
	return service.SynchronizeQuestionnaireVersion(ctx, modelcatalog.ActorContext{
		Principal: securityplane.Principal{Kind: securityplane.PrincipalKindService, Source: securityplane.PrincipalSourceInternal},
	}, questionnaireCode, version)
}

func (s *catalogBindingSyncer) IsQuestionnaireBound(ctx context.Context, questionnaireCode string) (bool, error) {
	if s == nil {
		return false, errors.WithCode(code.ErrModuleInitializationFailed, "questionnaire catalog binding reader is not configured")
	}
	s.mu.RLock()
	service := s.management
	s.mu.RUnlock()
	reader, ok := service.(interface {
		IsQuestionnaireBound(context.Context, string) (bool, error)
	})
	if !ok {
		return false, errors.WithCode(code.ErrModuleInitializationFailed, "questionnaire catalog binding reader is not configured")
	}
	return reader.IsQuestionnaireBound(ctx, questionnaireCode)
}

func (s *catalogBindingSyncer) validateReadiness() error {
	if s == nil {
		return missingApplicationService("questionnaire catalog management")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.management == nil {
		return missingApplicationService("questionnaire catalog management")
	}
	if _, ok := s.management.(interface {
		IsQuestionnaireBound(context.Context, string) (bool, error)
	}); !ok {
		return missingApplicationService("questionnaire catalog binding reader")
	}
	return nil
}
