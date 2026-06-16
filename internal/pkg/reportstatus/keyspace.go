package reportstatus

import rediskit "github.com/FangcunMount/component-base/pkg/redis"

type keyspace struct {
	ks rediskit.Keyspace
}

func newKeyspace(namespace string) keyspace {
	return keyspace{ks: rediskit.NewKeyspace(namespace)}
}

func (k keyspace) ReportStatus(assessmentID string) string {
	return k.ks.Prefix("report_status:" + assessmentID)
}
