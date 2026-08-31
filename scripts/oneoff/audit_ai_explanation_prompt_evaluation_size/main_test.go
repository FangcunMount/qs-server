package main

import (
	"testing"
	"time"
)

func TestDistributionUsesNearestRankAndTracksMissing(t *testing.T) {
	got := distribution([]int64{100, 20, 60, 40, 80}, 2)
	if got.Observed != 5 || got.Missing != 2 || got.P50 != 60 || got.P95 != 100 || got.Max != 100 {
		t.Fatalf("distribution = %#v", got)
	}
}

func TestBuildReportProjectsBoundedSlotPayloadFromObservedSizes(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	got := buildReport(now, "runs", 2, measurements{
		runBSON: []int64{1000, 2000}, runBSONWithoutOutputs: []int64{500, 700},
		generationRaw: []int64{10, 20}, generationNormalized: []int64{30, 40},
		semanticRaw: []int64{50, 60}, semanticNormalized: []int64{70, 80},
		generationExecutions: 2, semanticExecutions: 2,
		largestRuns: []runSize{{RunID: "2", BSONBytes: 2000}, {RunID: "1", BSONBytes: 1000}},
	})
	if got.Truncated || !got.V2ObservedOutputProjection.Available {
		t.Fatalf("report projection = %#v", got.V2ObservedOutputProjection)
	}
	wantP95 := int64(700 + 70*(20+40) + 70*(60+80))
	if got.V2ObservedOutputProjection.P95ProjectedBytes != wantP95 {
		t.Fatalf("p95 projected = %d, want %d", got.V2ObservedOutputProjection.P95ProjectedBytes, wantP95)
	}
	if len(got.LargestRuns) != 2 || got.LargestRuns[0].RunID != "2" {
		t.Fatalf("largest runs = %#v", got.LargestRuns)
	}
}

func TestProjectionFailsClosedForTruncatedOrMissingSemanticSamples(t *testing.T) {
	base := report{
		RunBSONWithoutStoredOutputs: distribution([]int64{500}, 0),
		GenerationRawOutput:         distribution([]int64{10}, 0), GenerationNormalizedOutput: distribution([]int64{20}, 0),
		SemanticRawOutput: distribution([]int64{30}, 0), SemanticNormalizedOutput: distribution([]int64{40}, 0),
	}
	base.Truncated = true
	if got := projectV2(base); got.Available || got.Reason == "" {
		t.Fatalf("truncated projection = %#v", got)
	}
	base.Truncated = false
	base.SemanticRawOutput = sizeDistribution{}
	if got := projectV2(base); got.Available || got.Reason == "" {
		t.Fatalf("missing semantic projection = %#v", got)
	}
}

func TestBuildReportKeepsOnlyTenLargestRuns(t *testing.T) {
	values := measurements{}
	for index := 1; index <= 12; index++ {
		values.runBSON = append(values.runBSON, int64(index))
		values.runBSONWithoutOutputs = append(values.runBSONWithoutOutputs, int64(index))
		values.largestRuns = append(values.largestRuns, runSize{RunID: string(rune('a' + index - 1)), BSONBytes: int64(index)})
	}
	got := buildReport(time.Now(), "runs", 12, values)
	if len(got.LargestRuns) != 10 || got.LargestRuns[0].BSONBytes != 12 || got.LargestRuns[9].BSONBytes != 3 {
		t.Fatalf("largest runs = %#v", got.LargestRuns)
	}
}
