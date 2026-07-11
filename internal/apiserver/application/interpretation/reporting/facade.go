package reporting

import interpretationregistry "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting/registry"

type (
	ReportBuilder                    = interpretationregistry.ReportBuilder
	MechanismReportBuilderKey        = interpretationregistry.MechanismReportBuilderKey
	MechanismKeyedReportBuilder      = interpretationregistry.MechanismKeyedReportBuilder
	MultiMechanismKeyedReportBuilder = interpretationregistry.MultiMechanismKeyedReportBuilder
	ReportBuilderRegistry            = interpretationregistry.ReportBuilderRegistry
	ReportRoutingContext             = interpretationregistry.ReportRoutingContext
)

var (
	NewReportBuilderRegistry           = interpretationregistry.NewReportBuilderRegistry
	MechanismReportBuilderKeyFromInput = interpretationregistry.MechanismReportBuilderKeyFromInput
	ReportRoutingContextFromInput      = interpretationregistry.ReportRoutingContextFromInput
)
