package subsystem

import (
	"context"
	"fmt"
	"time"

	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
)

// StandardProfile is a complete replacement for one legacy outbox profile.
// Run is a supervised blocking call; Drain runs only after every profile Run
// has returned. The caller retains ownership of the database and NSQ producer.
type StandardProfile struct {
	Binding      appEventing.ProfileBinding
	Supervisor   *standardoutbox.RelaySupervisor
	Drain        func(context.Context) error
	DrainTimeout time.Duration
	Status       appEventing.NamedOutboxStatusReader
}

// NewWithStandardProfiles selects each profile before any consumer or relay
// starts. A selected profile has no legacy immediate dispatcher, relay or
// Redis reconciler, so the same business flow has one writer and one runner.
// Every available host database requires a complete standard binding; a
// missing binding is a configuration error and never selects a legacy writer.
func NewWithStandardProfiles(opts Options, replacements map[eventcatalog.OutboxProfile]StandardProfile) (*Subsystem, error) {
	if len(replacements) == 0 {
		return nil, fmt.Errorf("at least one standard profile is required")
	}
	s, err := newBase(opts)
	if err != nil {
		return nil, err
	}
	if !s.publisher.IsMQBacked() {
		return nil, fmt.Errorf("standard outbox profile requires MQ-backed publisher mode")
	}
	prepared := make(map[eventcatalog.OutboxProfile]*profileRuntime, len(replacements))
	for profile, replacement := range replacements {
		name, available := standardProfileName(profile, opts)
		if !available || replacement.Binding.Stager == nil || replacement.Binding.PostCommit == nil ||
			replacement.Supervisor == nil || replacement.Drain == nil || replacement.DrainTimeout <= 0 ||
			replacement.Status.Name != name || replacement.Status.Reader == nil {
			return nil, fmt.Errorf("standard profile %q is incomplete or unknown", profile)
		}
		events := s.registry.EventsByProfile(profile)
		eventTypes := make([]string, 0, len(events))
		for _, evt := range events {
			eventTypes = append(eventTypes, evt.Type)
		}
		prepared[profile] = &profileRuntime{
			name: name, binding: replacement.Binding, run: replacement.Supervisor.Run,
			drain: replacement.Drain, drainTimeout: replacement.DrainTimeout,
			statusReporter: appEventing.NewOutboxStatusReporterWithEventTypes(name,
				replacement.Status.Reader, s.observer, eventTypes),
			runtimeStatus: func() appEventing.ProfileRuntimeStatus {
				snapshot := replacement.Supervisor.Snapshot()
				healthy := snapshot.ScanHealthy
				return appEventing.ProfileRuntimeStatus{
					Running: snapshot.Running, ScanHealthy: &healthy, LastFailureKind: snapshot.LastFailureKind,
				}
			},
			status: replacement.Status,
		}
	}
	for _, profile := range profileStartOrder {
		_, available := standardProfileName(profile, opts)
		if !available {
			continue
		}
		standard, ok := prepared[profile]
		if !ok {
			return nil, fmt.Errorf("standard profile %q is required for its host database", profile)
		}
		s.profiles[profile] = standard
	}
	s.buildConsumers(opts.Consumers)
	return s, nil
}

func standardProfileName(profile eventcatalog.OutboxProfile, opts Options) (string, bool) {
	switch profile {
	case eventcatalog.OutboxProfileMongoDomain:
		return "mongo-domain-events", opts.MongoDB != nil
	case eventcatalog.OutboxProfileAssessmentMySQL:
		return "assessment-mysql-outbox", opts.MySQLDB != nil
	default:
		return "", false
	}
}
