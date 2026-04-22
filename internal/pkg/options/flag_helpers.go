package options

import (
	"time"

	"github.com/spf13/pflag"
)

func addStringFlag(fs *pflag.FlagSet, target *string, name string, value string, usage string) {
	fs.StringVar(target, name, value, usage)
}

func addStringSliceFlag(fs *pflag.FlagSet, target *[]string, name string, value []string, usage string) {
	fs.StringSliceVar(target, name, value, usage)
}

func addBoolFlag(fs *pflag.FlagSet, target *bool, name string, value bool, usage string) {
	fs.BoolVar(target, name, value, usage)
}

func addIntFlag(fs *pflag.FlagSet, target *int, name string, value int, usage string) {
	fs.IntVar(target, name, value, usage)
}

func addDurationFlag(fs *pflag.FlagSet, target *time.Duration, name string, value time.Duration, usage string) {
	fs.DurationVar(target, name, value, usage)
}

type stringFlagSpec struct {
	target *string
	name   string
	value  string
	usage  string
}

type stringSliceFlagSpec struct {
	target *[]string
	name   string
	value  []string
	usage  string
}

type boolFlagSpec struct {
	target *bool
	name   string
	value  bool
	usage  string
}

type intFlagSpec struct {
	target *int
	name   string
	value  int
	usage  string
}

func addStringFlags(fs *pflag.FlagSet, specs []stringFlagSpec) {
	for _, spec := range specs {
		addStringFlag(fs, spec.target, spec.name, spec.value, spec.usage)
	}
}

func addStringSliceFlags(fs *pflag.FlagSet, specs []stringSliceFlagSpec) {
	for _, spec := range specs {
		addStringSliceFlag(fs, spec.target, spec.name, spec.value, spec.usage)
	}
}

func addBoolFlags(fs *pflag.FlagSet, specs []boolFlagSpec) {
	for _, spec := range specs {
		addBoolFlag(fs, spec.target, spec.name, spec.value, spec.usage)
	}
}

func addIntFlags(fs *pflag.FlagSet, specs []intFlagSpec) {
	for _, spec := range specs {
		addIntFlag(fs, spec.target, spec.name, spec.value, spec.usage)
	}
}
