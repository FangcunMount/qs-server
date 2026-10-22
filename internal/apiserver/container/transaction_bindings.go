package container

import (
	"errors"

	mongoTx "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/eventtransaction"
	mysqlTx "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventtransaction"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
)

// EventTransactionBindings supplies one existing owner for each original host
// database. The process retains lifecycle and database ownership.
type EventTransactionBindings struct {
	MySQL  *mysqlTx.Committer
	Mongo  *mongoTx.Owner
	Routes eventcatalog.TopicResolver
}

func (c *Container) validateTransactionBindings() error {
	if c.transactions == nil {
		return nil
	}
	b := c.transactions
	if b.Routes == nil || (c.mysqlDB != nil && b.MySQL == nil) ||
		(c.mongoDB != nil && b.Mongo == nil) || (b.MySQL == nil && b.Mongo == nil) {
		return errors.New("complete transaction owners and routes required for host databases")
	}
	if b.MySQL != nil && !b.MySQL.UsesPrimary(c.mysqlDB) {
		return errors.New("transaction MySQL owner does not borrow the host primary")
	}
	if b.Mongo != nil && !b.Mongo.UsesDatabase(c.mongoDB) {
		return errors.New("transaction Mongo owner does not borrow the host database")
	}
	return nil
}

func (c *Container) EvaluationTransactionOwner() *mysqlTx.Committer {
	if c.transactions == nil {
		return nil
	}
	return c.transactions.MySQL
}

func (c *Container) PlanTransactionOwner() *mysqlTx.Committer { return c.EvaluationTransactionOwner() }
func (c *Container) AICommandTransactionOwner() *mysqlTx.Committer {
	if c.aiWorkflowOptions == nil || !c.aiWorkflowOptions.Enabled || !c.aiWorkflowOptions.Messaging.Enabled {
		return nil
	}
	return c.EvaluationTransactionOwner()
}

func (c *Container) EvaluationTransactionRoutes() eventcatalog.TopicResolver {
	if c.EvaluationTransactionOwner() == nil {
		return nil
	}
	return c.transactions.Routes
}

func (c *Container) PlanTransactionRoutes() eventcatalog.TopicResolver {
	return c.EvaluationTransactionRoutes()
}
func (c *Container) ReportTransactionRoutes() eventcatalog.TopicResolver {
	if c.ReportTransactionOwner() == nil {
		return nil
	}
	return c.transactions.Routes
}
func (c *Container) ReportTransactionOwner() *mongoTx.Owner {
	if c.transactions == nil {
		return nil
	}
	return c.transactions.Mongo
}
