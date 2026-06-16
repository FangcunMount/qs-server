package main

import (
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestIsMongoUnauthorized(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "command error code",
			err:  mongo.CommandError{Code: 13, Name: "Unauthorized", Message: "requires authentication"},
			want: true,
		},
		{
			name: "wrapped text error",
			err:  errors.New("(Unauthorized) Command find requires authentication"),
			want: true,
		},
		{
			name: "ordinary error",
			err:  errors.New("server selection timeout"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMongoUnauthorized(tt.err); got != tt.want {
				t.Fatalf("isMongoUnauthorized() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMongoOutboxFiltersAreChunked(t *testing.T) {
	ids := scopeIDs{
		AnswerSheetIDs: makeUint64Range(1, mongoIDChunkSize+1),
		AssessmentIDs:  makeUint64Range(10_000, mongoIDChunkSize+1),
		ReportIDs:      makeUint64Range(20_000, 2),
		TesteeIDs:      []uint64{30_000},
	}

	filters := mongoOutboxFilters(ids)
	if len(filters) < 5 {
		t.Fatalf("filter count = %d, want chunked filters", len(filters))
	}
	for _, filter := range filters {
		idsFilter, ok := filter["aggregate_id"].(bson.M)
		if !ok {
			t.Fatalf("aggregate_id filter = %#v, want bson.M", filter["aggregate_id"])
		}
		values, ok := idsFilter["$in"].([]string)
		if !ok {
			t.Fatalf("$in = %#v, want []string", idsFilter["$in"])
		}
		if len(values) > mongoIDChunkSize {
			t.Fatalf("chunk size = %d, want <= %d", len(values), mongoIDChunkSize)
		}
	}
}

func makeUint64Range(start uint64, count int) []uint64 {
	out := make([]uint64, count)
	for i := range out {
		out[i] = start + uint64(i)
	}
	return out
}
