package migration

import (
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/mongo"
)

func TestIgnorableMongoIndexCleanupError(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", want: true},
		{name: "namespace not found", err: mongo.CommandError{Code: 26}, want: true},
		{name: "index not found", err: mongo.CommandError{Code: 27}, want: true},
		{name: "other command error", err: mongo.CommandError{Code: 13}, want: false},
		{name: "other error", err: errors.New("network failure"), want: false},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := isIgnorableMongoIndexCleanupError(test.err); got != test.want {
				t.Fatalf("isIgnorableMongoIndexCleanupError(%v) = %v, want %v", test.err, got, test.want)
			}
		})
	}
}
