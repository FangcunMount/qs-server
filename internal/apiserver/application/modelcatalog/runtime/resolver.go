package runtime

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	codepkg "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/securityplane"
)

// Resolver 是已发布模型的应用服务，仅用于受信任的运行时消费者。它从不访问 ModelRepository 或负载解码器。
type Resolver struct {
	Reader       modelcatalogport.PublishedModelReader       // 已发布模型读取器
	ActiveReader modelcatalogport.ActivePublishedModelReader // 当前线上模型读取器
	Lister       modelcatalogport.PublishedModelLister       // 已发布模型列表器
	Authorizer   modelcatalog.Authorizer                     // 授权器
}

// ResolveByRef 解析已发布模型引用
func (s Resolver) ResolveByRef(ctx context.Context, actor modelcatalog.ActorContext, ref modelcatalogport.Ref) (*modelcatalogport.PublishedModel, error) {
	if ref.Code == "" || ref.Version == "" {
		return nil, errors.WithCode(codepkg.ErrInvalidArgument, "published model code and version are required")
	}
	if s.Reader == nil || s.Authorizer == nil {
		return nil, errors.WithCode(codepkg.ErrInternalServerError, "published model resolver is not configured")
	}
	if err := s.Authorizer.Authorize(ctx, actor, modelcatalog.ActionResolvePublished, modelcatalog.Resource{Code: ref.Code, Kind: ref.Kind}); err != nil {
		return nil, err
	}
	model, err := s.Reader.GetPublishedModelByRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	return requireRuntimeDefinition(model)
}

// ResolveActiveByRef resolves an exact identity for admission and rejects
// archived releases. Execution and retries use ResolveByRef instead.
func (s Resolver) ResolveActiveByRef(ctx context.Context, actor modelcatalog.ActorContext, ref modelcatalogport.Ref) (*modelcatalogport.PublishedModel, error) {
	if ref.Code == "" || ref.Version == "" {
		return nil, errors.WithCode(codepkg.ErrInvalidArgument, "published model code and version are required")
	}
	if s.ActiveReader == nil || s.Authorizer == nil {
		return nil, errors.WithCode(codepkg.ErrInternalServerError, "active published model resolver is not configured")
	}
	if err := s.Authorizer.Authorize(ctx, actor, modelcatalog.ActionResolvePublished, modelcatalog.Resource{Code: ref.Code, Kind: ref.Kind}); err != nil {
		return nil, err
	}
	model, err := s.ActiveReader.GetActivePublishedModelByRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	return requireRuntimeDefinition(model)
}

// ResolveByQuestionnaire 解析已发布模型问卷
func (s Resolver) ResolveByQuestionnaire(ctx context.Context, actor modelcatalog.ActorContext, questionnaireCode, questionnaireVersion string) (*modelcatalogport.PublishedModel, error) {
	if questionnaireCode == "" {
		return nil, errors.WithCode(codepkg.ErrInvalidArgument, "questionnaire code is required")
	}
	if s.Reader == nil || s.Authorizer == nil {
		return nil, errors.WithCode(codepkg.ErrInternalServerError, "published model resolver is not configured")
	}
	if err := s.Authorizer.Authorize(ctx, actor, modelcatalog.ActionResolvePublished, modelcatalog.Resource{}); err != nil {
		return nil, err
	}
	model, err := s.Reader.FindPublishedModelByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
	if err != nil {
		return nil, err
	}
	return requireRuntimeDefinition(model)
}

// ResolveLatestByCode 解析最新已发布模型代码
func (s Resolver) ResolveLatestByCode(ctx context.Context, actor modelcatalog.ActorContext, kind domain.Kind, code string) (*modelcatalogport.PublishedModel, error) {
	if kind == "" || code == "" {
		return nil, errors.WithCode(codepkg.ErrInvalidArgument, "published model kind and code are required")
	}
	if s.Lister == nil || s.Authorizer == nil {
		return nil, errors.WithCode(codepkg.ErrInternalServerError, "published model resolver is not configured")
	}
	if err := s.Authorizer.Authorize(ctx, actor, modelcatalog.ActionResolvePublished, modelcatalog.Resource{Code: code, Kind: kind}); err != nil {
		return nil, err
	}
	model, err := s.Lister.FindPublishedModelByCode(ctx, kind, code)
	if err != nil {
		return nil, err
	}
	return requireRuntimeDefinition(model)
}

// ListPublished 列出已发布模型
func (s Resolver) ListPublished(ctx context.Context, actor modelcatalog.ActorContext, filter modelcatalogport.ListPublishedFilter) ([]*modelcatalogport.PublishedModel, int64, error) {
	if s.Lister == nil || s.Authorizer == nil {
		return nil, 0, errors.WithCode(codepkg.ErrInternalServerError, "published model resolver is not configured")
	}
	if err := s.Authorizer.Authorize(ctx, actor, modelcatalog.ActionResolvePublished, modelcatalog.Resource{Kind: filter.Kind}); err != nil {
		return nil, 0, err
	}
	models, total, err := s.Lister.ListPublishedModels(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	for _, model := range models {
		if _, err := requireRuntimeDefinition(model); err != nil {
			return nil, 0, err
		}
	}
	return models, total, nil
}

// requireRuntimeDefinition 要求运行时定义不为空
func requireRuntimeDefinition(model *modelcatalogport.PublishedModel) (*modelcatalogport.PublishedModel, error) {
	if model == nil {
		return nil, domain.ErrNotFound
	}
	if model.DefinitionV2 == nil {
		return nil, errors.WithCode(codepkg.ErrInvalidArgument, "published model definition_v2 is required for runtime: %s", model.Code)
	}
	return model, nil
}

// TrustedRuntimeResolver 适配已验证的服务主体到应用解析器，以便基础设施消费者不构造授权上下文。
type TrustedRuntimeResolver struct {
	Resolver modelcatalog.PublishedModelResolver // 已发布模型解析器
	Actor    modelcatalog.ActorContext           // 授权上下文
}

// GetPublishedModelByRef 获取已发布模型引用
func (r TrustedRuntimeResolver) GetPublishedModelByRef(ctx context.Context, ref modelcatalogport.Ref) (*modelcatalogport.PublishedModel, error) {
	if r.Resolver == nil {
		return nil, errors.WithCode(codepkg.ErrInternalServerError, "trusted published model resolver is not configured")
	}
	return r.Resolver.ResolveByRef(ctx, r.Actor, ref)
}

func (r TrustedRuntimeResolver) GetActivePublishedModelByRef(ctx context.Context, ref modelcatalogport.Ref) (*modelcatalogport.PublishedModel, error) {
	if r.Resolver == nil {
		return nil, errors.WithCode(codepkg.ErrInternalServerError, "trusted published model resolver is not configured")
	}
	return r.Resolver.ResolveActiveByRef(ctx, r.Actor, ref)
}

// FindPublishedModelByQuestionnaire 查找已发布模型问卷
func (r TrustedRuntimeResolver) FindPublishedModelByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*modelcatalogport.PublishedModel, error) {
	if r.Resolver == nil {
		return nil, errors.WithCode(codepkg.ErrInternalServerError, "trusted published model resolver is not configured")
	}
	return r.Resolver.ResolveByQuestionnaire(ctx, r.Actor, questionnaireCode, questionnaireVersion)
}

// FindPublishedModelByCode 查找已发布模型代码
func (r TrustedRuntimeResolver) FindPublishedModelByCode(ctx context.Context, kind domain.Kind, code string) (*modelcatalogport.PublishedModel, error) {
	if r.Resolver == nil {
		return nil, errors.WithCode(codepkg.ErrInternalServerError, "trusted published model resolver is not configured")
	}
	return r.Resolver.ResolveLatestByCode(ctx, r.Actor, kind, code)
}

// ListPublishedModels 列出已发布模型
func (r TrustedRuntimeResolver) ListPublishedModels(ctx context.Context, filter modelcatalogport.ListPublishedFilter) ([]*modelcatalogport.PublishedModel, int64, error) {
	if r.Resolver == nil {
		return nil, 0, errors.WithCode(codepkg.ErrInternalServerError, "trusted published model resolver is not configured")
	}
	return r.Resolver.ListPublished(ctx, r.Actor, filter)
}

// TrustedRuntimeCatalog 是唯一传递给受信任运行时的目录适配器。所有点查找都遍历 PublishedModelResolver；列表读取限于不可变的已发布记录，并拒绝没有 DefinitionV2 的记录。
type TrustedRuntimeCatalog struct {
	Resolver TrustedRuntimeResolver // 已发布模型解析器
}

// NewTrustedRuntimeCatalog 创建受信任运行时目录适配器
func NewTrustedRuntimeCatalog(reader modelcatalogport.PublishedModelReader, lister modelcatalogport.PublishedModelLister) *TrustedRuntimeCatalog {
	activeReader, _ := reader.(modelcatalogport.ActivePublishedModelReader)
	resolver := Resolver{
		Reader:       reader,
		ActiveReader: activeReader,
		Lister:       lister,
		Authorizer:   trustedRuntimeAuthorizer{},
	}
	return &TrustedRuntimeCatalog{
		Resolver: TrustedRuntimeResolver{
			Resolver: resolver,
			Actor: modelcatalog.ActorContext{Principal: securityplane.Principal{
				Kind:   securityplane.PrincipalKindService,
				Source: securityplane.PrincipalSourceMTLS,
			}},
		},
	}
}

// GetPublishedModelByRef 获取已发布模型引用
func (c *TrustedRuntimeCatalog) GetPublishedModelByRef(ctx context.Context, ref modelcatalogport.Ref) (*modelcatalogport.PublishedModel, error) {
	if c == nil {
		return nil, domain.ErrNotFound
	}
	return c.Resolver.GetPublishedModelByRef(ctx, ref)
}

func (c *TrustedRuntimeCatalog) GetActivePublishedModelByRef(ctx context.Context, ref modelcatalogport.Ref) (*modelcatalogport.PublishedModel, error) {
	if c == nil {
		return nil, domain.ErrNotFound
	}
	return c.Resolver.GetActivePublishedModelByRef(ctx, ref)
}

// FindPublishedModelByQuestionnaire 查找已发布模型问卷
func (c *TrustedRuntimeCatalog) FindPublishedModelByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*modelcatalogport.PublishedModel, error) {
	if c == nil {
		return nil, domain.ErrNotFound
	}
	return c.Resolver.FindPublishedModelByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
}

// ResolveByQuestionnaire 解析已发布模型问卷
func (c *TrustedRuntimeCatalog) ResolveByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (modelcatalogport.Ref, bool, error) {
	model, err := c.FindPublishedModelByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
	if err != nil {
		if domain.IsNotFound(err) {
			return modelcatalogport.Ref{}, false, nil
		}
		return modelcatalogport.Ref{}, false, err
	}
	return modelcatalogport.RefFromPublished(model), true, nil
}

// FindPublishedModelByCode 查找已发布模型代码
func (c *TrustedRuntimeCatalog) FindPublishedModelByCode(ctx context.Context, kind domain.Kind, code string) (*modelcatalogport.PublishedModel, error) {
	if c == nil {
		return nil, domain.ErrNotFound
	}
	return c.Resolver.FindPublishedModelByCode(ctx, kind, code)
}

// ListPublishedModels 列出已发布模型
func (c *TrustedRuntimeCatalog) ListPublishedModels(ctx context.Context, filter modelcatalogport.ListPublishedFilter) ([]*modelcatalogport.PublishedModel, int64, error) {
	if c == nil {
		return nil, 0, domain.ErrNotFound
	}
	return c.Resolver.ListPublishedModels(ctx, filter)
}

// trustedRuntimeAuthorizer 受信任运行时授权器
type trustedRuntimeAuthorizer struct{}

// Authorize 授权
func (trustedRuntimeAuthorizer) Authorize(_ context.Context, actor modelcatalog.ActorContext, action modelcatalog.Action, _ modelcatalog.Resource) error {
	if action != modelcatalog.ActionResolvePublished || !modelcatalog.IsTrustedServiceActor(actor) {
		return errors.WithCode(codepkg.ErrPermissionDenied, "trusted runtime actor is required")
	}
	return nil
}

// 验证接口实现
var _ modelcatalog.PublishedModelResolver = Resolver{}
var _ modelcatalogport.PublishedModelReader = TrustedRuntimeResolver{}
var _ modelcatalogport.ActivePublishedModelReader = TrustedRuntimeResolver{}
var _ modelcatalogport.PublishedModelLister = TrustedRuntimeResolver{}
var _ modelcatalogport.Catalog = (*TrustedRuntimeCatalog)(nil)
var _ modelcatalogport.PublishedModelLister = (*TrustedRuntimeCatalog)(nil)
var _ modelcatalogport.ActivePublishedModelReader = (*TrustedRuntimeCatalog)(nil)
