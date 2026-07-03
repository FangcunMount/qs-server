package personalitymodel

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
	"github.com/FangcunMount/qs-server/internal/pkg/cancelerr"
	"golang.org/x/sync/singleflight"
)

type personalityModelClient interface {
	GetPersonalityModel(ctx context.Context, code string) (*grpcclient.PersonalityModelOutput, error)
	ListPersonalityModels(ctx context.Context, page, pageSize int32, algorithm string) (*grpcclient.ListPersonalityModelsOutput, error)
	GetPersonalityModelCategories(ctx context.Context) (*grpcclient.PersonalityModelCategoriesOutput, error)
}

// QueryService is the BFF layer for personality model catalog reads.
type QueryService struct {
	client            personalityModelClient
	cache             CatalogCache
	singleflightGroup singleflight.Group
	useSingleflight   bool
}

func NewQueryService(client personalityModelClient, cache CatalogCache, useSingleflight bool) *QueryService {
	return &QueryService{
		client:          client,
		cache:           cache,
		useSingleflight: useSingleflight,
	}
}

// HasCachedDetail 进程内 L1 是否已有人格模型详情。
func (s *QueryService) HasCachedDetail(code string) bool {
	if s == nil || s.cache == nil || code == "" {
		return false
	}
	_, ok := s.cache.GetDetail(code)
	return ok
}

// HasCachedList 进程内 L1 是否已有人格模型列表。
func (s *QueryService) HasCachedList(req *ListPersonalityModelsRequest) bool {
	if s == nil || s.cache == nil {
		return false
	}
	if req == nil {
		req = &ListPersonalityModelsRequest{}
	}
	s.normalizeListRequest(req)
	_, ok := s.cache.GetListByRequest(req)
	return ok
}

// HasCachedCategories 进程内 L1 是否已有人格模型分类。
func (s *QueryService) HasCachedCategories() bool {
	if s == nil || s.cache == nil {
		return false
	}
	_, ok := s.cache.GetCategories()
	return ok
}

func (s *QueryService) Get(ctx context.Context, code string) (*PersonalityModelResponse, error) {
	if s.cache != nil {
		if cached, ok := s.cache.GetDetail(code); ok {
			return cached, nil
		}
	}

	load := func() (*PersonalityModelResponse, error) {
		log.Infof("Getting personality model: code=%s", code)
		result, err := s.client.GetPersonalityModel(ctx, code)
		if err != nil {
			logPersonalityGRPCError("Failed to get personality model via gRPC", err)
			return nil, err
		}
		if result == nil {
			return nil, nil
		}
		return convertDetail(result), nil
	}

	if s.cache != nil && s.useSingleflight {
		key := detailCacheKey(code)
		value, err, _ := s.singleflightGroup.Do(key, func() (interface{}, error) {
			if cached, ok := s.cache.GetDetail(code); ok {
				return cached, nil
			}
			resp, loadErr := load()
			if loadErr != nil || resp == nil {
				return resp, loadErr
			}
			s.cache.SetDetail(code, resp)
			return clonePersonalityModelResponse(resp), nil
		})
		if err != nil {
			return nil, err
		}
		if value == nil {
			return nil, nil
		}
		return value.(*PersonalityModelResponse), nil
	}

	resp, err := load()
	if err != nil || resp == nil || s.cache == nil {
		return resp, err
	}
	s.cache.SetDetail(code, resp)
	return clonePersonalityModelResponse(resp), nil
}

func (s *QueryService) List(ctx context.Context, req *ListPersonalityModelsRequest) (*ListPersonalityModelsResponse, error) {
	if req == nil {
		req = &ListPersonalityModelsRequest{}
	}
	s.normalizeListRequest(req)

	if s.cache != nil {
		if cached, ok := s.cache.GetListByRequest(req); ok {
			return cached, nil
		}
	}

	load := func() (*ListPersonalityModelsResponse, error) {
		result, err := s.client.ListPersonalityModels(ctx, req.Page, req.PageSize, req.Algorithm)
		if err != nil {
			logPersonalityGRPCError("Failed to list personality models via gRPC", err)
			return nil, err
		}
		models := make([]PersonalityModelSummaryResponse, 0, len(result.Models))
		for _, model := range result.Models {
			models = append(models, convertSummary(model))
		}
		return &ListPersonalityModelsResponse{
			Models:     models,
			Total:      result.Total,
			Page:       result.Page,
			PageSize:   result.PageSize,
			TotalPages: result.TotalPages,
		}, nil
	}

	listKey := listCacheKey(req)
	if s.cache != nil && s.useSingleflight {
		value, err, _ := s.singleflightGroup.Do(listKey, func() (interface{}, error) {
			if cached, hit := s.cache.GetListByRequest(req); hit {
				return cached, nil
			}
			resp, loadErr := load()
			if loadErr != nil || resp == nil {
				return resp, loadErr
			}
			s.cache.SetListByRequest(req, resp)
			return cloneListPersonalityModelsResponse(resp), nil
		})
		if err != nil {
			return nil, err
		}
		if value == nil {
			return nil, nil
		}
		return value.(*ListPersonalityModelsResponse), nil
	}

	resp, err := load()
	if err != nil || resp == nil || s.cache == nil {
		return resp, err
	}
	s.cache.SetListByRequest(req, resp)
	return cloneListPersonalityModelsResponse(resp), nil
}

func (s *QueryService) GetCategories(ctx context.Context) (*PersonalityModelCategoriesResponse, error) {
	if s.cache != nil {
		if cached, ok := s.cache.GetCategories(); ok {
			return cached, nil
		}
	}

	load := func() (*PersonalityModelCategoriesResponse, error) {
		result, err := s.client.GetPersonalityModelCategories(ctx)
		if err != nil {
			logPersonalityGRPCError("Failed to get personality model categories via gRPC", err)
			return nil, err
		}
		categories := make([]CategoryResponse, 0, len(result.Categories))
		for _, item := range result.Categories {
			categories = append(categories, CategoryResponse{Value: item.Value, Label: item.Label})
		}
		return &PersonalityModelCategoriesResponse{Categories: categories}, nil
	}

	if s.cache != nil && s.useSingleflight {
		value, err, _ := s.singleflightGroup.Do(cacheKeyCategories, func() (interface{}, error) {
			if cached, ok := s.cache.GetCategories(); ok {
				return cached, nil
			}
			resp, loadErr := load()
			if loadErr != nil || resp == nil {
				return resp, loadErr
			}
			s.cache.SetCategories(resp)
			return clonePersonalityModelCategoriesResponse(resp), nil
		})
		if err != nil {
			return nil, err
		}
		if value == nil {
			return nil, nil
		}
		return value.(*PersonalityModelCategoriesResponse), nil
	}

	resp, err := load()
	if err != nil || resp == nil || s.cache == nil {
		return resp, err
	}
	s.cache.SetCategories(resp)
	return clonePersonalityModelCategoriesResponse(resp), nil
}

func (s *QueryService) normalizeListRequest(req *ListPersonalityModelsRequest) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}
	if req.PageSize > 100 {
		req.PageSize = 100
	}
}

func convertSummary(model grpcclient.PersonalityModelSummaryOutput) PersonalityModelSummaryResponse {
	return PersonalityModelSummaryResponse{
		Code:                 model.Code,
		Version:              model.Version,
		Title:                model.Title,
		Algorithm:            model.Algorithm,
		Description:          model.Description,
		QuestionnaireCode:    model.QuestionnaireCode,
		QuestionnaireVersion: model.QuestionnaireVersion,
		Status:               model.Status,
		QuestionCount:        model.QuestionCount,
	}
}

func convertDetail(model *grpcclient.PersonalityModelOutput) *PersonalityModelResponse {
	dimensions := make([]PersonalityDimensionResponse, 0, len(model.Dimensions))
	for _, dim := range model.Dimensions {
		dimensions = append(dimensions, PersonalityDimensionResponse{
			Code:      dim.Code,
			Name:      dim.Name,
			LeftPole:  dim.LeftPole,
			RightPole: dim.RightPole,
		})
	}
	outcomes := make([]PersonalityOutcomeResponse, 0, len(model.Outcomes))
	for _, outcome := range model.Outcomes {
		outcomes = append(outcomes, PersonalityOutcomeResponse{
			Code:     outcome.Code,
			Name:     outcome.Name,
			OneLiner: outcome.OneLiner,
			ImageURL: outcome.ImageURL,
		})
	}
	summary := convertSummary(model.Summary)
	return &PersonalityModelResponse{
		Code:                 summary.Code,
		Version:              summary.Version,
		Title:                summary.Title,
		Algorithm:            summary.Algorithm,
		Description:          summary.Description,
		QuestionnaireCode:    summary.QuestionnaireCode,
		QuestionnaireVersion: summary.QuestionnaireVersion,
		Status:               summary.Status,
		QuestionCount:        summary.QuestionCount,
		DimensionOrder:       append([]string(nil), model.DimensionOrder...),
		Dimensions:           dimensions,
		Outcomes:             outcomes,
	}
}

func logPersonalityGRPCError(message string, err error) {
	if cancelerr.Is(err) {
		log.Debugf("%s: %v", message, err)
		return
	}
	log.Errorf("%s: %v", message, err)
}
