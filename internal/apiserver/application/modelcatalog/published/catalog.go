package published

import port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"

// Catalog 是评估模型快照的读取端
type Catalog interface {
	port.PublishedModelReader // 已发布模型读取器
	port.PublishedModelLister // 已发布模型列表器
}

// Snapshot 是评估模型快照
type Snapshot = port.AssessmentSnapshot
