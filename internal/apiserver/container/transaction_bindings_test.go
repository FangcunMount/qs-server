package container

import (
	"testing"

	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	mongoTx "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/eventtransaction"
	mysqlTx "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventtransaction"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"gorm.io/gorm"
)

func TestContainerBorrowsOneOwnerAcrossAllModulePorts(t *testing.T) {
	mysqlOwner, mongoOwner := &mysqlTx.Committer{}, &mongoTx.Owner{}
	routes := eventcatalog.NewCatalog(nil)
	input := EventTransactionBindings{MySQL: mysqlOwner, Mongo: mongoOwner, Routes: routes}
	c := NewContainerWithOptions(nil, nil, nil, ContainerOptions{Transactions: &input, Silent: true})
	input.MySQL, input.Mongo, input.Routes = nil, nil, nil
	if c.EvaluationTransactionOwner() != mysqlOwner || c.PlanTransactionOwner() != mysqlOwner || c.AICommandTransactionOwner() != mysqlOwner {
		t.Fatal("module ports did not retain the single borrowed MySQL owner")
	}
	if c.ReportTransactionOwner() != mongoOwner || c.ReportTransactionRoutes() != routes || c.PlanTransactionRoutes() != routes || c.EvaluationTransactionRoutes() != routes {
		t.Fatal("module ports lost the shared owner or configured host routes")
	}
	// These zero owner handles cannot perform I/O or lifecycle work. Merely
	// exposing module ports must not create, start or close a different owner.
}

func TestContainerRejectsPartialOrForeignBindingsBeforeModuleInitialization(t *testing.T) {
	routes := eventcatalog.NewCatalog(nil)
	for _, binding := range []EventTransactionBindings{
		{}, {Routes: routes}, {MySQL: &mysqlTx.Committer{}, Routes: routes},
		{Mongo: &mongoTx.Owner{}, Routes: routes},
	} {
		c := newBaseContainer(&gorm.DB{}, nil, nil)
		c.transactions = &binding
		c.eventSubsystem = &eventsubsystem.Subsystem{}
		if err := c.Initialize(); err == nil {
			t.Fatal("incomplete or foreign owner accepted")
		}
		if c.initialized || len(c.modules) != 0 {
			t.Fatal("invalid binding reached module initialization")
		}
	}
}

func TestContainerLegacyPortsRemainUnselected(t *testing.T) {
	c := newBaseContainer(nil, nil, nil)
	if err := c.validateTransactionBindings(); err != nil {
		t.Fatal(err)
	}
	if c.EvaluationTransactionOwner() != nil || c.PlanTransactionOwner() != nil || c.AICommandTransactionOwner() != nil || c.ReportTransactionOwner() != nil || c.EvaluationTransactionRoutes() != nil {
		t.Fatal("legacy container silently selected transaction owners")
	}
}

func TestContainerDoesNotExposeRoutesWithoutTheirOwner(t *testing.T) {
	c := newBaseContainer(nil, nil, nil)
	c.transactions = &EventTransactionBindings{MySQL: &mysqlTx.Committer{}, Routes: eventcatalog.NewCatalog(nil)}
	if c.ReportTransactionRoutes() != nil {
		t.Fatal("report routes selected without Mongo owner")
	}
	c.transactions.MySQL = nil
	c.transactions.Mongo = &mongoTx.Owner{}
	if c.EvaluationTransactionRoutes() != nil || c.PlanTransactionRoutes() != nil {
		t.Fatal("MySQL routes selected without MySQL owner")
	}
}
