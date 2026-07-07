package assessment

import "github.com/FangcunMount/qs-server/internal/pkg/eventoutcome"

// EventModelIdentity 是线缆投影 of 模型身份 on 测评事件。
type EventModelIdentity = eventoutcome.ModelIdentity

// EventScoreValue 是线缆投影 of 主 score on 测评事件。
type EventScoreValue = eventoutcome.ScoreValue

// EventResultLevel 是线缆投影 of 结果 等级 on 测评事件。
type EventResultLevel = eventoutcome.ResultLevel
