package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type fileRawSettingsSource struct {
	file      string
	envPrefix string
	flags     *pflag.FlagSet
}

func newRawSettingsSource(file, envPrefix string, flags *pflag.FlagSet) RawSettingsSource {
	return &fileRawSettingsSource{file: file, envPrefix: envPrefix, flags: flags}
}

func (s *fileRawSettingsSource) Read(ctx context.Context) (RawSettings, error) {
	if err := ctx.Err(); err != nil {
		return RawSettings{}, err
	}
	if s == nil || strings.TrimSpace(s.file) == "" {
		return RawSettings{}, fmt.Errorf("startup config file is unavailable")
	}
	v := viper.New()
	v.SetConfigFile(s.file)
	v.AutomaticEnv()
	v.SetEnvPrefix(s.envPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	if err := v.ReadInConfig(); err != nil {
		return RawSettings{}, err
	}
	// Only explicitly supplied flags override file/env values. Binding the whole
	// FlagSet would also apply flag defaults and silently change reload policy.
	if s.flags != nil {
		s.flags.Visit(func(flag *pflag.Flag) {
			v.Set(flag.Name, flag.Value.String())
		})
	}
	return RawSettings{Values: v.AllSettings(), Source: v.ConfigFileUsed()}, nil
}
