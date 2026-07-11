// Package evaluationroute exposes immutable runtime route values to consumers.
package evaluationroute

import evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"

type (
	RuntimeDescriptorKey = evalpipeline.RuntimeDescriptorKey
	ModelRoute           = evalpipeline.ModelRoute
)

var ExecutionRoutingFromRoute = evalpipeline.ExecutionRoutingFromRoute
