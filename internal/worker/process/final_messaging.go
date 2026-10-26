package process

import "errors"

// Workers keep their existing shared consumption and deferred-business budgets;
// they never acquire a transaction owner or a republishing client.
func (s *server) validateFinalMessaging() error {
	if s == nil || s.config == nil || s.config.Options == nil || s.config.Messaging == nil || !s.consumeOnlyRocketMQ() || len(s.config.Messaging.RocketMQ.Transactions) != 0 {
		return errors.New("final consume-only RocketMQ worker required; NSQ and worker publishing are retired")
	}
	return nil
}
