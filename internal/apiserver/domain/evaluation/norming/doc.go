// Package norming is the mechanism-oriented domain entry for norm-profile execution.
// AlgorithmFamily enum: factor_norm. See modelcatalog.AlgorithmFamilyFactorNorm.
// Norm projection uses domain/calculation/norm and modelcatalog/norming metadata.
package norming

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"

// AlgorithmFamily is the mechanism family for this package.
const AlgorithmFamily = modelcatalog.AlgorithmFamilyFactorNorm
