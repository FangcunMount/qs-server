package cache

import sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"

type catalogSpec struct {
	Owner         string
	Layer         sharedcache.Layer
	MetricLabel   string
	TopologyGroup string
	TopologyOrder int
	ReadModel     string
}

var collectionCatalog = map[sharedcache.Capability]catalogSpec{
	"catalog.questionnaire": {
		Owner: "catalog", Layer: sharedcache.LayerL1, MetricLabel: "questionnaire",
		TopologyGroup: "questionnaire", TopologyOrder: 10, ReadModel: "questionnaire published Mongo read model",
	},
	"catalog.published_model": {
		Owner: "catalog", Layer: sharedcache.LayerL1, MetricLabel: "published_model",
		TopologyGroup: "published-model", TopologyOrder: 10, ReadModel: "published-model Mongo snapshot",
	},
	"catalog.typology": {
		Owner: "catalog", Layer: sharedcache.LayerL1, MetricLabel: "typology",
	},
	"evaluation.assessment_detail": {
		Owner: "evaluation", Layer: sharedcache.LayerL1, MetricLabel: "assessment_detail",
		TopologyGroup: "assessment-detail", TopologyOrder: 10, ReadModel: "MySQL evaluation assessment read model",
	},
	"evaluation.assessment_access": {
		Owner: "evaluation", Layer: sharedcache.LayerL1, MetricLabel: "assessment_access",
		TopologyGroup: "assessment-access", TopologyOrder: 10, ReadModel: "MySQL assessment ownership lookup",
	},
}

func lookupCatalogSpec(capability sharedcache.Capability) (catalogSpec, bool) {
	spec, ok := collectionCatalog[capability]
	return spec, ok
}
