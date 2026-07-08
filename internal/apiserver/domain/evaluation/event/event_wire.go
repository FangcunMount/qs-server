package event

import "github.com/FangcunMount/qs-server/internal/pkg/eventoutcome"

// ModelIdentity 是线缆投影 of 模型身份 on 测评事件。
type ModelIdentity = eventoutcome.ModelIdentity

// ScoreValue 是线缆投影 of 主 score on 测评事件。
type ScoreValue = eventoutcome.ScoreValue

// ResultLevel 是线缆投影 of 结果等级 on 测评事件。
type ResultLevel = eventoutcome.ResultLevel
