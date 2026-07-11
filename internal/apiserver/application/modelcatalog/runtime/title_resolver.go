package runtime

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// publishedModelTitleResolver 已发布模型标题解析器
type publishedModelTitleResolver struct {
	lister modelcatalogport.PublishedModelLister // 已发布模型列表器
}

// NewTitleResolver 创建已发布模型标题解析器
func NewTitleResolver(lister modelcatalogport.PublishedModelLister) modelcatalog.PublishedModelTitleResolver {
	return &publishedModelTitleResolver{lister: lister}
}

// ResolvePublishedTitle 解析已发布模型标题
func (r *publishedModelTitleResolver) ResolvePublishedTitle(ctx context.Context, kind domain.Kind, codeValue string) (string, error) {
	if r == nil || r.lister == nil {
		return "", errors.WithCode(code.ErrInternalServerError, "published model title resolver is not configured")
	}
	if kind == "" || codeValue == "" {
		return "", errors.WithCode(code.ErrInvalidArgument, "published model kind and code are required")
	}
	model, err := r.lister.FindPublishedModelByCode(ctx, kind, codeValue)
	if err != nil {
		return "", err
	}
	if _, err := requireRuntimeDefinition(model); err != nil {
		return "", err
	}
	return model.Title, nil
}

// 验证接口实现
var _ modelcatalog.PublishedModelTitleResolver = (*publishedModelTitleResolver)(nil)
