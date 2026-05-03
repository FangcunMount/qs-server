package plan

import "context"

// ScaleCatalog 定义了量表目录的接口，提供了根据量表编码检查量表是否存在以及解析量表标题的方法。实现该接口的组件负责管理和查询量表信息，以支持应用程序在处理与量表相关的功能时能够获取必要的量表数据和元信息。
type ScaleCatalog interface {
	ExistsByCode(ctx context.Context, code string) (bool, error)
	ResolveTitle(ctx context.Context, code string) string
	ResolveTitles(ctx context.Context, codes []string) map[string]string
}
