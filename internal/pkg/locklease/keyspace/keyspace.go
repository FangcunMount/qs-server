package keyspace

import basekeyspace "github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"

// Keyspace builds lock lease keys.
type Keyspace struct {
	base basekeyspace.LockKeyspace
}

func New(ns string) Keyspace {
	return Keyspace{base: basekeyspace.NewLockKeyspace(ns)}
}

func FromBuilder(builder *basekeyspace.Builder) Keyspace {
	if builder == nil {
		return New("")
	}
	return New(builder.Namespace())
}

func (k Keyspace) AnswerSheetProcessing(answerSheetID uint64) string {
	return k.base.AnswerSheetProcessing(answerSheetID)
}

func (k Keyspace) Lock(raw string) string {
	return k.base.Lock(raw)
}
