package mongodb

import (
	"context"
	"fmt"
	"sync"

	"github.com/vinllen/mgo"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/document"
	genericoptions "github.com/yshujie/questionnaire-scale/internal/pkg/options"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

type datastore struct {
	session    *mgo.Session
	database   string
	collection string
}

// DocumentStorage returns the document storage interface.
func (ds *datastore) DocumentStorage() document.DocumentStorage {
	return newDocumentStorage(ds)
}

// Close closes the MongoDB connection.
func (ds *datastore) Close() error {
	if ds.session != nil {
		ds.session.Close()
	}
	return nil
}

var (
	mongoFactory document.Factory
	once         sync.Once
)

// GetMongoFactoryOr creates a MongoDB factory with the given config.
func GetMongoFactoryOr(opts *genericoptions.MongoDBOptions) (document.Factory, error) {
	if opts == nil && mongoFactory == nil {
		return nil, fmt.Errorf("failed to get mongodb store factory")
	}

	var err error
	once.Do(func() {
		var session *mgo.Session

		// Parse MongoDB URL to get database name
		dialInfo, parseErr := mgo.ParseURL(opts.URL)
		if parseErr != nil {
			err = fmt.Errorf("failed to parse MongoDB URL: %w", parseErr)
			return
		}

		// Create session
		session, err = mgo.DialWithInfo(dialInfo)
		if err != nil {
			err = fmt.Errorf("failed to connect to MongoDB: %w", err)
			return
		}

		// Set session mode
		session.SetMode(mgo.Monotonic, true)

		// Test connection
		if err = session.Ping(); err != nil {
			err = fmt.Errorf("failed to ping MongoDB: %w", err)
			return
		}

		mongoFactory = &datastore{
			session:  session,
			database: dialInfo.Database,
		}

		log.Infof("MongoDB connected successfully to %s", opts.URL)
	})

	if mongoFactory == nil || err != nil {
		return nil, fmt.Errorf("failed to get mongodb store factory, mongoFactory: %+v, error: %w", mongoFactory, err)
	}

	return mongoFactory, nil
}

// documentStorage implements the DocumentStorage interface.
type documentStorage struct {
	*datastore
}

// newDocumentStorage creates a new document storage instance.
func newDocumentStorage(ds *datastore) document.DocumentStorage {
	return &documentStorage{ds}
}

// Insert inserts a document into the collection.
func (ds *documentStorage) Insert(ctx context.Context, collection string, doc interface{}) error {
	session := ds.session.Copy()
	defer session.Close()

	c := session.DB(ds.database).C(collection)
	return c.Insert(doc)
}

// InsertMany inserts multiple documents into the collection.
func (ds *documentStorage) InsertMany(ctx context.Context, collection string, docs []interface{}) error {
	session := ds.session.Copy()
	defer session.Close()

	c := session.DB(ds.database).C(collection)
	return c.Insert(docs...)
}

// FindOne finds a single document by filter.
func (ds *documentStorage) FindOne(ctx context.Context, collection string, filter interface{}, result interface{}) error {
	session := ds.session.Copy()
	defer session.Close()

	c := session.DB(ds.database).C(collection)
	return c.Find(filter).One(result)
}

// Find finds multiple documents by filter.
func (ds *documentStorage) Find(ctx context.Context, collection string, filter interface{}) ([]interface{}, error) {
	session := ds.session.Copy()
	defer session.Close()

	c := session.DB(ds.database).C(collection)

	var results []interface{}
	err := c.Find(filter).All(&results)
	return results, err
}

// Update updates a document by filter.
func (ds *documentStorage) Update(ctx context.Context, collection string, filter interface{}, update interface{}) error {
	session := ds.session.Copy()
	defer session.Close()

	c := session.DB(ds.database).C(collection)
	return c.Update(filter, update)
}

// Delete deletes documents by filter.
func (ds *documentStorage) Delete(ctx context.Context, collection string, filter interface{}) error {
	session := ds.session.Copy()
	defer session.Close()

	c := session.DB(ds.database).C(collection)
	return c.Remove(filter)
}

// Count counts documents by filter.
func (ds *documentStorage) Count(ctx context.Context, collection string, filter interface{}) (int64, error) {
	session := ds.session.Copy()
	defer session.Close()

	c := session.DB(ds.database).C(collection)
	count, err := c.Find(filter).Count()
	return int64(count), err
}

// CreateIndex creates an index on the collection.
func (ds *documentStorage) CreateIndex(ctx context.Context, collection string, index interface{}) error {
	session := ds.session.Copy()
	defer session.Close()

	c := session.DB(ds.database).C(collection)

	// Convert index to mgo.Index if it's a map
	if indexMap, ok := index.(map[string]interface{}); ok {
		mgoIndex := mgo.Index{}
		if key, exists := indexMap["key"]; exists {
			if keySlice, ok := key.([]string); ok {
				mgoIndex.Key = keySlice
			}
		}
		if unique, exists := indexMap["unique"]; exists {
			if uniqueBool, ok := unique.(bool); ok {
				mgoIndex.Unique = uniqueBool
			}
		}
		if sparse, exists := indexMap["sparse"]; exists {
			if sparseBool, ok := sparse.(bool); ok {
				mgoIndex.Sparse = sparseBool
			}
		}
		return c.EnsureIndex(mgoIndex)
	}

	// If index is already mgo.Index
	if mgoIndex, ok := index.(mgo.Index); ok {
		return c.EnsureIndex(mgoIndex)
	}

	return fmt.Errorf("unsupported index type")
}

// Close closes the document storage connection.
func (ds *documentStorage) Close() error {
	return ds.datastore.Close()
}
