package options

import "github.com/spf13/viper"

// DecodeConfig preserves literal transaction owners in the standard CLI path.
func (o *Options) DecodeConfig(reader *viper.Viper) error {
	if err := reader.Unmarshal(o); err != nil {
		return err
	}
	if o.Messaging != nil {
		return o.Messaging.RocketMQ.DecodeTransactions(reader, "messaging.rocketmq.transactions")
	}
	return nil
}
