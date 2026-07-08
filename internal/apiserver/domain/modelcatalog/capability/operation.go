package capability

// CatalogOperation 是model-目录 API 操作 守卫ed 按 类型能力。
type CatalogOperation string

const (
	CatalogOpCreate            CatalogOperation = "create"
	CatalogOpList              CatalogOperation = "list"
	CatalogOpUpdateBasicInfo   CatalogOperation = "update_basic_info"
	CatalogOpDelete            CatalogOperation = "delete"
	CatalogOpPublish           CatalogOperation = "publish"
	CatalogOpUnpublish         CatalogOperation = "unpublish"
	CatalogOpArchive           CatalogOperation = "archive"
	CatalogOpBindQuestionnaire CatalogOperation = "bind_questionnaire"
	CatalogOpUpdateDefinition  CatalogOperation = "update_definition"
	CatalogOpPreview           CatalogOperation = "preview"
	CatalogOpQRCode            CatalogOperation = "qrcode"
)

// Allows reports whether the model-family capability permits an operation.
func (c ModelFamilyCapability) Allows(op CatalogOperation) bool {
	switch op {
	case CatalogOpCreate:
		return c.CreateSupported
	case CatalogOpList:
		return c.ListSupported
	case CatalogOpUpdateBasicInfo, CatalogOpDelete:
		return c.CreateSupported
	case CatalogOpPublish, CatalogOpUnpublish, CatalogOpArchive:
		return c.PublishSupported
	case CatalogOpBindQuestionnaire:
		return c.BindQuestionnaire
	case CatalogOpUpdateDefinition:
		return c.DefinitionUpdateSupported
	case CatalogOpPreview, CatalogOpQRCode:
		return false
	default:
		return false
	}
}
