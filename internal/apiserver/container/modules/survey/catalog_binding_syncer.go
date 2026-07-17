package survey

import (
	"context"
	"sync"

	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
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
		return nil
	}
	s.mu.RLock()
	service := s.management
	s.mu.RUnlock()
	if service == nil {
		return nil
	}
	return service.SynchronizeQuestionnaireVersion(ctx, modelcatalog.ActorContext{
		Principal: securityplane.Principal{Kind: securityplane.PrincipalKindService, Source: securityplane.PrincipalSourceServiceAuth},
	}, questionnaireCode, version)
}

func (s *catalogBindingSyncer) IsQuestionnaireBound(ctx context.Context, questionnaireCode string) (bool, error) {
	if s == nil {
		return false, nil
	}
	s.mu.RLock()
	service := s.management
	s.mu.RUnlock()
	reader, ok := service.(interface {
		IsQuestionnaireBound(context.Context, string) (bool, error)
	})
	if !ok {
		return false, nil
	}
	return reader.IsQuestionnaireBound(ctx, questionnaireCode)
}
