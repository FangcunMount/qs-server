package options

import (
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestProductionConfigSelectsOnlyMongoStandardOutbox(t *testing.T) {
	config := viper.New()
	config.SetConfigFile(filepath.Join("..", "..", "..", "configs", "apiserver.prod.yaml"))
	if err := config.ReadInConfig(); err != nil {
		t.Fatal(err)
	}

	loaded := NewOptions()
	if err := config.Unmarshal(loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.Eventing == nil || loaded.Eventing.StandardOutbox == nil {
		t.Fatal("production eventing selection was not loaded")
	}
	if !loaded.Eventing.StandardOutbox.Mongo || loaded.Eventing.StandardOutbox.Assessment {
		t.Fatalf("first cutover batch must select Mongo only: %+v", loaded.Eventing.StandardOutbox)
	}
}
