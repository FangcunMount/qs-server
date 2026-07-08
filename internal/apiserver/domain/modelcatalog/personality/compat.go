// Package personality keeps typology publish helpers until typology drops modelcatalog import.
package personality

import modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"

type (
	Payload     = modeltypology.Payload
	RuntimeSpec = modeltypology.RuntimeSpec
)
