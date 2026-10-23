package options

import "errors"

var ErrTransactionStartupMigration = errors.New("transaction runtime requires explicit migration.enabled=false; install both selected schemas and bindings offline")

func (o *Options) transactionStartupConfigured() bool {
	if o.MessagingOptions != nil && o.MessagingOptions.Provider == "rocketmq" && o.MessagingOptions.RocketMQ != nil && len(o.MessagingOptions.RocketMQ.Transactions) > 0 {
		return true
	}
	if o.AIWorkflow != nil {
		m := o.AIWorkflow.Messaging
		return m.Enabled && m.SelectedProvider() == "rocketmq" && m.RocketMQ != nil && len(m.RocketMQ.Transactions) > 0
	}
	return false
}

func (o *Options) ValidateTransactionStartupPolicy() error {
	if o != nil && o.transactionStartupConfigured() && (o.MigrationOptions == nil || o.MigrationOptions.Enabled) {
		return ErrTransactionStartupMigration
	}
	return nil
}
