package modelcatalog

import domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"

func kindBSONFilter(kind domain.Kind) any {
	return string(kind)
}
