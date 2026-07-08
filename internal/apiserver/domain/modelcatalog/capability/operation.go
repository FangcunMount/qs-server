package capability

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"

type CatalogOperation = binding.CatalogOperation

const (
	CatalogOpCreate            = binding.CatalogOpCreate
	CatalogOpList              = binding.CatalogOpList
	CatalogOpUpdateBasicInfo   = binding.CatalogOpUpdateBasicInfo
	CatalogOpDelete            = binding.CatalogOpDelete
	CatalogOpPublish           = binding.CatalogOpPublish
	CatalogOpUnpublish         = binding.CatalogOpUnpublish
	CatalogOpArchive           = binding.CatalogOpArchive
	CatalogOpBindQuestionnaire = binding.CatalogOpBindQuestionnaire
	CatalogOpUpdateDefinition  = binding.CatalogOpUpdateDefinition
	CatalogOpPreview           = binding.CatalogOpPreview
	CatalogOpQRCode            = binding.CatalogOpQRCode
)
