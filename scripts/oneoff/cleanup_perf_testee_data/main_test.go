package main

import (
	"errors"
	"testing"

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
