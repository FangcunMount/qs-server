package app

import "github.com/spf13/viper"

// DecodeOptions lets a host preserve literal identifiers in configuration maps.
// Other CLI options retain Viper's normal decoding and flag/env precedence.
func DecodeOptions(reader *viper.Viper, target any) error {
	if decoder, ok := target.(interface{ DecodeConfig(*viper.Viper) error }); ok {
		return decoder.DecodeConfig(reader)
	}
	return reader.Unmarshal(target)
}
