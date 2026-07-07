package spm

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	taskperf "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/task_performance"
)

// NormContext carries SPM norm/task metadata without embedding norm table bodies.
type NormContext struct {
	NormTableVersion string
	ItemSetCodes     []string
}

// ApplyNormMetadata annotates canonical factors with SPM task-set roles and norm references.
func ApplyNormMetadata(factors []factor.FactorSnapshot, ctx NormContext) []factor.FactorSnapshot {
	return taskperf.ApplyNormMetadata(factors, taskperf.MetadataContext{
		NormTableVersion: ctx.NormTableVersion,
		ItemSetCodes:     ctx.ItemSetCodes,
	})
}
