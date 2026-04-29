package redisops

import rediskit "github.com/FangcunMount/component-base/pkg/redis"

type opsKeyspace struct {
	keyspace rediskit.Keyspace
}

func newOpsKeyspace(ns string) opsKeyspace {
	return opsKeyspace{keyspace: rediskit.NewKeyspace(ns)}
}

func (k opsKeyspace) IdempotencyInflight(key string) string {
	return k.keyspace.Prefix(submitInflightKey(key))
}

func (k opsKeyspace) IdempotencyDone(key string) string {
	return k.keyspace.Prefix(submitDoneKey(key))
}
