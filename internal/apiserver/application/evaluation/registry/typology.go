package registry

import factorclassification "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology"

type TypologyExecutor = factorclassification.Executor

func NewConfiguredTypologyExecutor() (*TypologyExecutor, error) {
	return factorclassification.NewConfiguredTypologyExecutor()
}
