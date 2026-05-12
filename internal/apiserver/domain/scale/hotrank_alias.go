package scale

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale/hotrank"
)

// 以下类型别名保持历史 API：热度榜契约定义在子包 hotrank，根包 re-export。

type ScaleHotRankSubmissionFact = hotrank.SubmissionFact
type ScaleHotRankQuery = hotrank.Query
type ScaleHotRankEntry = hotrank.Entry
type ScaleHotRankProjection = hotrank.Projection
type ScaleHotRankReadModel = hotrank.ReadModel
