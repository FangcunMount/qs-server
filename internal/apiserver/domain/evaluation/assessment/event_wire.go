package assessment

import "github.com/FangcunMount/qs-server/internal/pkg/eventoutcome"

// EventModelIdentity is the wire projection of model identity on assessment events.
type EventModelIdentity = eventoutcome.ModelIdentity

// EventScoreValue is the wire projection of primary score on assessment events.
type EventScoreValue = eventoutcome.ScoreValue

// EventResultLevel is the wire projection of outcome level on assessment events.
type EventResultLevel = eventoutcome.ResultLevel
