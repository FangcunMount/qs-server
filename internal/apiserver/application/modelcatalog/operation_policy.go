package modelcatalog

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

func requireCatalogOperation(apiKind string, op domain.CatalogOperation) error {
	if !catalogRegistry.Allows(normalizeAPIKind(apiKind), op) {
		return invalidArgument("模型类型无效")
	}
	return nil
}

func (s *service) requireModelOperation(
	ctx context.Context,
	modelCode string,
	explicitKind string,
	op domain.CatalogOperation,
) (string, error) {
	return s.requireModelOperationWithNotFound(ctx, modelCode, explicitKind, op, nil)
}

func (s *service) requireModelOperationWithNotFound(
	ctx context.Context,
	modelCode string,
	explicitKind string,
	op domain.CatalogOperation,
	notFound error,
) (string, error) {
	kind := explicitKind
	if kind == "" {
		resolved, ok := s.resolveModelKind(ctx, modelCode)
		if !ok {
			if notFound != nil {
				return "", notFound
			}
			return "", invalidArgument("模型类型无效")
		}
		kind = resolved
	}
	if err := requireCatalogOperation(kind, op); err != nil {
		return "", err
	}
	return kind, nil
}

func modelNotFoundError() error {
	return errors.WithCode(code.ErrMedicalScaleNotFound, "测评模型不存在")
}
