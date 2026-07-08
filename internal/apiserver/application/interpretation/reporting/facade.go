package reporting

import (
	interpretationprojection "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting/projection"
	interpretationregistry "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting/registry"
	interpretationwriter "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting/writer"
)

type (
	ReportBuilder                     = interpretationregistry.ReportBuilder
	Writer                            = interpretationwriter.Writer
	ScoreProjector                    = interpretationprojection.ScoreProjector
	ScoreProjectorRegistry            = interpretationprojection.ScoreProjectorRegistry
	CompletionNotifier                = interpretationwriter.CompletionNotifier
	MechanismReportBuilderKey         = interpretationregistry.MechanismReportBuilderKey
	MechanismKeyedReportBuilder       = interpretationregistry.MechanismKeyedReportBuilder
	MultiMechanismKeyedReportBuilder  = interpretationregistry.MultiMechanismKeyedReportBuilder
	MechanismKeyedScoreProjector      = interpretationprojection.MechanismKeyedScoreProjector
	MultiMechanismKeyedScoreProjector = interpretationprojection.MultiMechanismKeyedScoreProjector
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
	NewInterpretationWriter                           = interpretationwriter.NewInterpretationWriter
	NewWriter                                         = interpretationwriter.NewWriter
	NewWriterWithEventAssemblers                      = interpretationwriter.NewWriterWithEventAssemblers
	NewTransactionalReportDurableSaver                = interpretationwriter.NewTransactionalReportDurableSaver
	NewWaiterCompletionNotifier                       = interpretationwriter.NewWaiterCompletionNotifier
	ExecutionPathForMechanismFamily                   = interpretationwriter.ExecutionPathForMechanismFamily
	ExecutionPathForReportBuilder                     = interpretationwriter.ExecutionPathForReportBuilder
	ExecutionPathForScoreProjector                    = interpretationwriter.ExecutionPathForScoreProjector
	ResolveOutcomeKey                                 = interpretationwriter.ResolveOutcomeKey
	ErrWriterNotConfigured                            = interpretationwriter.ErrWriterNotConfigured
	NewScoreProjectorRegistry                         = interpretationprojection.NewScoreProjectorRegistry
	NewEventAssemblerRegistry                         = interpretationprojection.NewEventAssemblerRegistry
	NewMechanismCanonicalEventAssembler               = interpretationprojection.NewMechanismCanonicalEventAssembler
	DefaultMechanismEventAssemblers                   = interpretationprojection.DefaultMechanismEventAssemblers
	AttachReportOutcomeSummary                        = interpretationprojection.AttachReportOutcomeSummary
)
