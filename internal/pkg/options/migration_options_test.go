package options

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func TestMigrationBootstrapAuthorizationOptions(t *testing.T) {
	for _, test := range []struct {
		name, file, hash string
		valid            bool
	}{
		{"installed_defaults", "", "", true},
		{"explicit_pristine", "/private/approved/bootstrap.json", strings.Repeat("a", 64), true},
		{"missing_hash", "/private/approved/bootstrap.json", "", false},
		{"missing_file", "", strings.Repeat("a", 64), false},
		{"relative", "bootstrap.json", strings.Repeat("a", 64), false},
		{"unclean", "/private/approved/../bootstrap.json", strings.Repeat("a", 64), false},
		{"upper_hash", "/private/approved/bootstrap.json", strings.Repeat("A", 64), false},
		{"short_hash", "/private/approved/bootstrap.json", strings.Repeat("a", 63), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			o := NewMigrationOptions()
			o.BootstrapAuthorizationFile = test.file
			o.BootstrapAuthorizationSHA256 = test.hash
			if got := len(o.Validate()) == 0; got != test.valid {
				t.Fatalf("valid=%v expected=%v", got, test.valid)
			}
		})
	}
	var missing *MigrationOptions
	if len(missing.Validate()) == 0 {
		t.Fatal("nil migration options accepted")
	}
}

func TestMigrationBootstrapAuthorizationFlagsAndMapping(t *testing.T) {
	hash := strings.Repeat("a", 64)
	o := NewMigrationOptions()
	flags := pflag.NewFlagSet("migration", pflag.ContinueOnError)
	o.AddFlags(flags)
	if err := flags.Parse([]string{"--migration.retirement-bootstrap-authorization-file=/private/approved/bootstrap.json", "--migration.retirement-bootstrap-authorization-sha256=" + hash}); err != nil {
		t.Fatal(err)
	}
	if o.BootstrapAuthorizationFile != "/private/approved/bootstrap.json" || o.BootstrapAuthorizationSHA256 != hash {
		t.Fatal("declared flags did not bind options")
	}
	v := viper.New()
	v.Set("migration.retirement-bootstrap-authorization-file", o.BootstrapAuthorizationFile)
	v.Set("migration.retirement-bootstrap-authorization-sha256", hash)
	var mapped MigrationOptions
	if err := v.UnmarshalKey("migration", &mapped); err != nil {
		t.Fatal(err)
	}
	if mapped.BootstrapAuthorizationFile != o.BootstrapAuthorizationFile || mapped.BootstrapAuthorizationSHA256 != hash {
		t.Fatal("declared configuration keys did not map options")
	}
	raw, err := json.Marshal(mapped)
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]any
	if err = json.Unmarshal(raw, &keys); err != nil {
		t.Fatal(err)
	}
	if keys["retirement-bootstrap-authorization-file"] != o.BootstrapAuthorizationFile || keys["retirement-bootstrap-authorization-sha256"] != hash {
		t.Fatal("JSON declaration differs from configuration keys")
	}
}
