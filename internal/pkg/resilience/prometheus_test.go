package resilience

import (
	"math"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
)

func TestCollectionAdmissionHistogramsKeepPublicContract(t *testing.T) {
	ObserveHTTPGateWait(time.Millisecond)
	ObserveGRPCInflightWait(2 * time.Millisecond)

	for name, histogram := range map[string]interface {
		Write(*dto.Metric) error
	}{
		"collection_http_gate_wait_seconds":     collectionHTTPGateWaitSeconds,
		"collection_grpc_inflight_wait_seconds": collectionGRPCInflightWaitSeconds,
	} {
		metric := &dto.Metric{}
		if err := histogram.Write(metric); err != nil {
			t.Fatalf("%s Write() error = %v", name, err)
		}
		buckets := metric.GetHistogram().GetBucket()
		if len(buckets) != 14 {
			t.Fatalf("%s bucket count = %d, want 14", name, len(buckets))
		}
		for index, bucket := range buckets {
			want := 0.001 * math.Pow(2, float64(index))
			if math.Abs(bucket.GetUpperBound()-want) > 1e-12 {
				t.Fatalf("%s bucket[%d] = %v, want %v", name, index, bucket.GetUpperBound(), want)
			}
		}
	}
}
