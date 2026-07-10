package option

// CatalogOperation identifies an application-level model-catalog operation.
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
