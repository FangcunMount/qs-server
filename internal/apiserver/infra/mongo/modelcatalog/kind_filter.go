package modelcatalog

import (
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"go.mongodb.org/mongo-driver/bson"
)

func kindBSONFilter(kind domain.Kind) any {
	values := domain.KindQueryValues(kind)
	if len(values) == 0 {
		return ""
	}
	if len(values) == 1 {
		return values[0]
	}
	return bson.M{"$in": values}
}
