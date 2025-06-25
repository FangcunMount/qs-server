// Package document defines document storage interfaces for unstructured data.
package document

import (
	"context"
)

// DocumentStorage defines the document storage interface for MongoDB.
type DocumentStorage interface {
	// Insert inserts a document into the collection
	Insert(ctx context.Context, collection string, doc interface{}) error

	// InsertMany inserts multiple documents into the collection
	InsertMany(ctx context.Context, collection string, docs []interface{}) error

	// FindOne finds a single document by filter
	FindOne(ctx context.Context, collection string, filter interface{}, result interface{}) error

	// Find finds multiple documents by filter
	Find(ctx context.Context, collection string, filter interface{}) ([]interface{}, error)

	// Update updates a document by filter
	Update(ctx context.Context, collection string, filter interface{}, update interface{}) error

	// Delete deletes documents by filter
	Delete(ctx context.Context, collection string, filter interface{}) error

	// Count counts documents by filter
	Count(ctx context.Context, collection string, filter interface{}) (int64, error)

	// CreateIndex creates an index on the collection
	CreateIndex(ctx context.Context, collection string, index interface{}) error

	// Close closes the connection
	Close() error
}

// Factory defines the document storage factory interface.
type Factory interface {
	DocumentStorage() DocumentStorage
	Close() error
}

var client Factory

// Client returns the document storage client instance.
func Client() Factory {
	return client
}

// SetClient sets the document storage client.
func SetClient(factory Factory) {
	client = factory
}
