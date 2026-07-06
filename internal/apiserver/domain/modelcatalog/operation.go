package modelcatalog

// CatalogOperation is a model-catalog API operation guarded by KindCapability.
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

// Allows reports whether the capability matrix permits an operation for a model family.
func (c KindCapability) Allows(op CatalogOperation) bool {
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
	case CatalogOpPreview:
		return c.PreviewSupported
	case CatalogOpQRCode:
		return c.QRCodeSupported
	default:
		return false
	}
}
