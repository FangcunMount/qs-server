package main

import (
	"fmt"
	"log"

	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/spf13/viper"
)

func main() {
	// 设置配置文件
	viper.SetConfigFile("configs/apiserver.yaml")
	viper.SetConfigType("yaml")

	// 读取配置文件
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}

	// 创建选项并反序列化
	opts := options.NewOptions()
	if err := viper.Unmarshal(opts); err != nil {
		log.Fatalf("Error unmarshaling config: %v", err)
	}

	fmt.Printf("Options after unmarshal:\n")
	fmt.Printf("  InsecureServing.BindAddress: %s\n", opts.InsecureServing.BindAddress)
	fmt.Printf("  InsecureServing.BindPort: %d\n", opts.InsecureServing.BindPort)
	fmt.Printf("  SecureServing.BindAddress: %s\n", opts.SecureServing.BindAddress)
	fmt.Printf("  SecureServing.BindPort: %d\n", opts.SecureServing.BindPort)

	// 创建配置
	cfg, err := config.CreateConfigFromOptions(opts)
	if err != nil {
		log.Fatalf("Error creating config: %v", err)
	}

	fmt.Printf("\nConfig after creation:\n")
	fmt.Printf("  InsecureServing.BindAddress: %s\n", cfg.InsecureServing.BindAddress)
	fmt.Printf("  InsecureServing.BindPort: %d\n", cfg.InsecureServing.BindPort)
	fmt.Printf("  SecureServing.BindAddress: %s\n", cfg.SecureServing.BindAddress)
	fmt.Printf("  SecureServing.BindPort: %d\n", cfg.SecureServing.BindPort)

	// 测试 ApplyTo 方法
	fmt.Printf("\nTesting ApplyTo method:\n")

	// 创建服务器配置
	serverConfig := &struct {
		InsecureServing *struct {
			Address string
		}
		SecureServing *struct {
			Address string
		}
	}{}

	// 模拟 ApplyTo 方法
	if cfg.InsecureServing != nil {
		serverConfig.InsecureServing = &struct {
			Address string
		}{
			Address: fmt.Sprintf("%s:%d", cfg.InsecureServing.BindAddress, cfg.InsecureServing.BindPort),
		}
	}

	if cfg.SecureServing != nil {
		serverConfig.SecureServing = &struct {
			Address string
		}{
			Address: fmt.Sprintf("%s:%d", cfg.SecureServing.BindAddress, cfg.SecureServing.BindPort),
		}
	}

	fmt.Printf("  InsecureServing.Address: %s\n", serverConfig.InsecureServing.Address)
	fmt.Printf("  SecureServing.Address: %s\n", serverConfig.SecureServing.Address)
}
