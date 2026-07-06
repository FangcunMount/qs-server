package modelcatalog

import "context"

func (s *service) resolveModelKind(ctx context.Context, modelCode string) (string, bool) {
	if s.personality.cmd != nil {
		if _, err := s.personality.cmd.Get(ctx, modelCode); err == nil {
			return KindPersonality, true
		}
	}
	if s.behavior.cmd != nil {
		if _, err := s.behavior.cmd.Get(ctx, modelCode); err == nil {
			return KindBehaviorAbility, true
		}
	}
	return "", false
}
