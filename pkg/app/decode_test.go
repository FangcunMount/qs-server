package app

import (
	"errors"
	"testing"

	"github.com/spf13/viper"
)

type keyedConfig struct {
	called bool
	err    error
}

func (o *keyedConfig) DecodeConfig(*viper.Viper) error { o.called = true; return o.err }
func TestDecodeOptionsDispatchAndOrdinaryFallback(t *testing.T) {
	r := viper.New()
	sentinel := errors.New("decode denied")
	keyed := &keyedConfig{err: sentinel}
	if err := DecodeOptions(r, keyed); !errors.Is(err, sentinel) || !keyed.called {
		t.Fatal("host decoder or failure was ignored")
	}
	r.Set("name", "retained")
	plain := &struct{ Name string }{}
	if err := DecodeOptions(r, plain); err != nil || plain.Name != "retained" {
		t.Fatal("ordinary options decoding changed")
	}
}
