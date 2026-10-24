package options

import (
	"errors"
	"github.com/spf13/viper"
)

// DecodeTransactions reads the original map so dotted owner IDs stay literal.
// Always decode a fresh map: merging would retain Viper's spurious "qs" key.
func (o *RocketMQOptions) DecodeTransactions(reader *viper.Viper, path string) error {
	if !reader.IsSet(path) {
		return nil
	}
	if o == nil {
		return errors.New("transaction bindings require RocketMQ configuration")
	}
	var bindings map[string]TransactionOptions
	if err := reader.UnmarshalKey(path, &bindings); err != nil {
		return err
	}
	o.Transactions = bindings
	return nil
}
