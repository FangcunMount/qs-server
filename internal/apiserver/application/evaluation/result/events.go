package result

import (
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
)

func NewEventAssemblerRegistry(assemblers ...EventAssembler) (EventAssemblerRegistry, error) {
	return interpretationreporting.NewEventAssemblerRegistry(assemblers...)
}

type GenericEventAssembler = interpretationreporting.GenericEventAssembler

type ScaleEventAssembler = interpretationreporting.ScaleEventAssembler
