package options

import "github.com/spf13/viper"

// DecodeConfig preserves literal owner keys before Complete/Validate or startup.
func (o *Options) DecodeConfig(reader *viper.Viper) error {
	if err := reader.Unmarshal(o); err != nil {
		return err
	}
	if o.MessagingOptions != nil {
		if err := o.MessagingOptions.RocketMQ.DecodeTransactions(reader, "messaging.rocketmq.transactions"); err != nil {
			return err
		}
	}
	if o.AIWorkflow != nil {
		if err := o.AIWorkflow.Messaging.RocketMQ.DecodeTransactions(reader, "ai_workflow.messaging.rocketmq.transactions"); err != nil {
			return err
		}
	}
	return nil
}
