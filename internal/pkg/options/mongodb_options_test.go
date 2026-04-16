package options

import "testing"

func TestMongoDBOptionsBuildURIUsesExplicitURL(t *testing.T) {
	opts := &MongoDBOptions{
		URL:      "mongodb://mongo:27017/qs?replicaSet=rs0&directConnection=true",
		Host:     "127.0.0.1:27017",
		Database: "ignored",
	}

	if got := opts.BuildURI(); got != opts.URL {
		t.Fatalf("BuildURI() = %q, want explicit url %q", got, opts.URL)
	}
}

func TestMongoDBOptionsBuildURIFromFields(t *testing.T) {
	opts := &MongoDBOptions{
		Host:             "mongo:27017",
		Username:         "app_user",
		Password:         "s3cret",
		Database:         "qs",
		ReplicaSet:       "rs0",
		DirectConnection: true,
	}

	want := "mongodb://app_user:s3cret@mongo:27017/qs?directConnection=true&replicaSet=rs0"
	if got := opts.BuildURI(); got != want {
		t.Fatalf("BuildURI() = %q, want %q", got, want)
	}
}
