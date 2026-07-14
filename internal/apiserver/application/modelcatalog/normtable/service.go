// Package normtable owns immutable norm-table administration use cases.
package normtable

import (
	"context"
	stderrors "errors"

	"github.com/FangcunMount/component-base/pkg/errors"
	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
	modelnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type Service struct {
	Repository port.NormRepository
	Authorizer modelcatalog.Authorizer
}

func (s Service) Import(ctx context.Context, actor modelcatalog.ActorContext, table *domain.Norm) (*modelcatalog.NormTableDetail, error) {
	if err := s.authorize(ctx, actor, modelcatalog.ActionManageNormTables, modelcatalog.Resource{}); err != nil {
		return nil, err
	}
	if err := modelnorm.ValidateImport(table); err != nil {
		return nil, errors.WithCode(code.ErrInvalidArgument, "%v", err)
	}
	if s.Repository == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "norm repository is not configured")
	}
	if err := s.Repository.UpsertNorm(ctx, table); err != nil {
		if stderrors.Is(err, domain.ErrNormVersionConflict) {
			return nil, errors.WithCode(code.ErrConflict, "%v", err)
		}
		return nil, err
	}
	return modelcatalog.NormTableDetailFromDomain(table), nil
}

func (s Service) Get(ctx context.Context, actor modelcatalog.ActorContext, tableVersion string) (*modelcatalog.NormTableDetail, error) {
	if err := s.authorize(ctx, actor, modelcatalog.ActionReadNormTables, modelcatalog.Resource{Code: tableVersion}); err != nil {
		return nil, err
	}
	if tableVersion == "" {
		return nil, errors.WithCode(code.ErrInvalidArgument, "norm table version is required")
	}
	if s.Repository == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "norm repository is not configured")
	}
	table, err := s.Repository.FindNorm(ctx, tableVersion)
	if stderrors.Is(err, domain.ErrNotFound) {
		return nil, errors.WithCode(code.ErrPageNotFound, "norm table %s was not found", tableVersion)
	}
	if err != nil {
		return nil, err
	}
	return modelcatalog.NormTableDetailFromDomain(table), nil
}

func (s Service) List(ctx context.Context, actor modelcatalog.ActorContext, input modelcatalog.ListNormTablesDTO) (*modelcatalog.NormTableListResult, error) {
	if err := s.authorize(ctx, actor, modelcatalog.ActionReadNormTables, modelcatalog.Resource{}); err != nil {
		return nil, err
	}
	if s.Repository == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "norm repository is not configured")
	}
	filter, err := listFilter(input)
	if err != nil {
		return nil, err
	}
	items, total, err := s.Repository.ListNorms(ctx, filter)
	if err != nil {
		return nil, err
	}
	result := &modelcatalog.NormTableListResult{Items: make([]modelcatalog.NormTableSummary, 0, len(items)), Total: total, Page: filter.Page, PageSize: filter.PageSize}
	for _, table := range items {
		detail := modelcatalog.NormTableDetailFromDomain(table)
		result.Items = append(result.Items, detail.NormTableSummary)
	}
	return result, nil
}

func (s Service) authorize(ctx context.Context, actor modelcatalog.ActorContext, action modelcatalog.Action, resource modelcatalog.Resource) error {
	if s.Authorizer == nil {
		return errors.WithCode(code.ErrInternalServerError, "norm authorizer is not configured")
	}
	return s.Authorizer.Authorize(ctx, actor, action, resource)
}

func listFilter(input modelcatalog.ListNormTablesDTO) (port.NormListFilter, error) {
	page, pageSize := input.Page, input.PageSize
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	kind := identity.Kind(input.Kind)
	if kind != "" && !kind.IsValid() {
		return port.NormListFilter{}, errors.WithCode(code.ErrInvalidArgument, "norm kind is invalid")
	}
	algorithm := identity.Algorithm(input.Algorithm)
	if algorithm != "" && !knownAlgorithm(algorithm) {
		return port.NormListFilter{}, errors.WithCode(code.ErrInvalidArgument, "norm algorithm is invalid")
	}
	return port.NormListFilter{Kind: kind, Algorithm: algorithm, FormVariant: input.FormVariant, Page: page, PageSize: pageSize}, nil
}

func knownAlgorithm(value identity.Algorithm) bool {
	switch value {
	case identity.AlgorithmBrief2, identity.AlgorithmSPMSensory, identity.AlgorithmBehavioralRatingDefault:
		return true
	default:
		return false
	}
}
