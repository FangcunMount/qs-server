package transaction

import (
	"testing"

	"go.mongodb.org/mongo-driver/mongo/readpref"
)

func TestMongoTransactionOptionsFreezePublishDurability(t *testing.T) {
	t.Parallel()

	opts := mongoTransactionOptions()
	if opts.ReadPreference == nil || opts.ReadPreference.Mode() != readpref.PrimaryMode {
		t.Fatalf("read preference = %#v, want primary", opts.ReadPreference)
	}
	if opts.ReadConcern == nil || opts.ReadConcern.Level != "snapshot" {
		t.Fatalf("read concern = %#v, want snapshot", opts.ReadConcern)
	}
	if opts.WriteConcern == nil || opts.WriteConcern.W != "majority" {
		t.Fatalf("write concern = %#v, want majority", opts.WriteConcern)
	}
}
