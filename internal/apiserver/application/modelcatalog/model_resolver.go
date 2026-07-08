package modelcatalog

import "context"

func (s *service) resolveModelKind(ctx context.Context, modelCode string) (string, bool) {
	if s.typologyKind.cmd != nil {
		if _, err := s.typologyKind.cmd.Get(ctx, modelCode); err == nil {
			return KindPersonality, true
		}
	}
	if s.normingKind.cmd != nil {
		if _, err := s.normingKind.cmd.Get(ctx, modelCode); err == nil {
			return KindBehavioralRating, true
		}
	}
	if s.taskPerformanceKind.cmd != nil {
		if _, err := s.taskPerformanceKind.cmd.Get(ctx, modelCode); err == nil {
			return KindCognitive, true
		}
	}
	return "", false
}
