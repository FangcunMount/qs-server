package modelcatalog

import "context"

func (s *service) resolveModelKind(ctx context.Context, modelCode string) (string, bool) {
	if s.personality.cmd != nil {
		if _, err := s.personality.cmd.Get(ctx, modelCode); err == nil {
			return KindPersonality, true
		}
	}
	if s.behavioralRating.cmd != nil {
		if _, err := s.behavioralRating.cmd.Get(ctx, modelCode); err == nil {
			return KindBehavioralRating, true
		}
	}
	if s.cognitive.cmd != nil {
		if _, err := s.cognitive.cmd.Get(ctx, modelCode); err == nil {
			return KindCognitive, true
		}
	}
	return "", false
}
