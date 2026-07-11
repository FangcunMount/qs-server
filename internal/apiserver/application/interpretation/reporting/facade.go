package reporting

import (
	interpretationprojection "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting/projection"
	interpretationregistry "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting/registry"
	interpretationwriter "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting/writer"
)

type (
	ReportBuilder                     = interpretationregistry.ReportBuilder
	Generation                        = interpretationwriter.Generation
	Generator                         = interpretationwriter.Generator
	MechanismReportBuilderKey         = interpretationregistry.MechanismReportBuilderKey
	MechanismKeyedReportBuilder       = interpretationregistry.MechanismKeyedReportBuilder
	MultiMechanismKeyedReportBuilder  = interpretationregistry.MultiMechanismKeyedReportBuilder
	ReportBuilderRegistry             = interpretationregistry.ReportBuilderRegistry
	ReportRoutingContext              = interpretationregistry.ReportRoutingContext
	ReportDurableSaver                = interpretationwriter.ReportDurableSaver
	ReportDurableWriter               = interpretationwriter.ReportDurableWriter
	ReportEventStager                 = interpretationwriter.ReportEventStager
	EventAssembler                    = interpretationprojection.EventAssembler
	EventAssemblerRegistry            = interpretationprojection.EventAssemblerRegistry
	MechanismKeyedEventAssembler      = interpretationprojection.MechanismKeyedEventAssembler
	MultiMechanismKeyedEventAssembler = interpretationprojection.MultiMechanismKeyedEventAssembler
	MechanismCanonicalEventAssembler  = interpretationprojection.MechanismCanonicalEventAssembler
	TypologyMechanismEventAssembler   = interpretationprojection.TypologyMechanismEventAssembler
	GenericEventAssembler             = interpretationprojection.GenericEventAssembler
	ScaleEventAssembler               = interpretationprojection.ScaleEventAssembler
)

var (
	NewReportBuilderRegistry                          = interpretationregistry.NewReportBuilderRegistry
	MechanismReportBuilderKeyFromRuntimeDescriptorKey = interpretationregistry.MechanismReportBuilderKeyFromRuntimeDescriptorKey
	MechanismReportBuilderKeyFromExecutionIdentity    = interpretationregistry.MechanismReportBuilderKeyFromExecutionIdentity
	MechanismReportBuilderKeyFromOutcome              = interpretationregistry.MechanismReportBuilderKeyFromOutcome
	ReportRoutingContextFromOutcome                   = interpretationregistry.ReportRoutingContextFromOutcome
	OutcomeReportType                                 = interpretationregistry.OutcomeReportType
	NewTransactionalReportDurableSaver                = interpretationwriter.NewTransactionalReportDurableSaver
	NewGenerator                                      = interpretationwriter.NewGenerator
	ExecutionPathForMechanismFamily                   = interpretationwriter.ExecutionPathForMechanismFamily
	ExecutionPathForReportBuilder                     = interpretationwriter.ExecutionPathForReportBuilder
	ResolveOutcomeKey                                 = interpretationwriter.ResolveOutcomeKey
	NewEventAssemblerRegistry                         = interpretationprojection.NewEventAssemblerRegistry
	NewMechanismCanonicalEventAssembler               = interpretationprojection.NewMechanismCanonicalEventAssembler
	DefaultMechanismEventAssemblers                   = interpretationprojection.DefaultMechanismEventAssemblers
	AttachReportOutcomeSummary                        = interpretationprojection.AttachReportOutcomeSummary
	BuildReportFailedEvent                            = interpretationprojection.BuildReportFailedEvent
)
