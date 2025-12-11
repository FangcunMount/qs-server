package collection

import (
	"net/http"
	_ "net/http/pprof"
	"os"
	"strconv"
	"strings"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/collection-server/config"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	"github.com/FangcunMount/qs-server/pkg/app"
	"runtime/debug"
)

// commandDesc 命令描述
const commandDesc = `The Questionnaire Collection Server provides REST API for questionnaire collection system (mini-program).
It validates questionnaire submissions and communicates with apiserver via gRPC for data operations.`

// NewApp 创建 App
func NewApp(basename string) *app.App {
	opts := options.NewOptions()
	application := app.NewApp("Questionnaire Collection Server",
		basename,
		app.WithDescription(commandDesc),
		app.WithDefaultValidArgs(),
		app.WithOptions(opts),
		app.WithRunFunc(run(opts)),
	)

	return application
}

func run(opts *options.Options) app.RunFunc {
	return func(basename string) error {
		// 初始化日志
		log.Init(opts.Log)
		defer log.Flush()

		applyRuntimeTuning(opts.Runtime)

		// 启动 pprof 以便线上诊断（仅内网端口）
		go func() {
			_ = http.ListenAndServe(":6060", nil)
		}()

		log.Infof("Starting collection-server ... (mode=%s, healthz=%v)", opts.GenericServerRunOptions.Mode, opts.GenericServerRunOptions.Healthz)
		log.Infof("HTTP bind: %s:%d, HTTPS bind: %s:%d, gRPC endpoint: %s",
			opts.InsecureServing.BindAddress, opts.InsecureServing.BindPort,
			opts.SecureServing.BindAddress, opts.SecureServing.BindPort,
			opts.GRPCClient.Endpoint)

		// 打印安全与并发配置
		log.Infof("TLS cert: %s, key: %s", opts.SecureServing.TLS.CertFile, opts.SecureServing.TLS.KeyFile)
		log.Infof("Concurrency: max=%d, JWT expiry(h): %d", opts.Concurrency.MaxConcurrency, opts.JWT.TokenDuration)

		// 根据 options 创建 app 配置
		cfg, err := config.CreateConfigFromOptions(opts)
		if err != nil {
			return err
		}

		// 运行 app
		return Run(cfg)
	}
}

// applyRuntimeTuning 应用 GC/内存调优
func applyRuntimeTuning(rt *options.RuntimeOptions) {
	if rt == nil {
		return
	}

	if rt.GoMemLimit != "" {
		if bytes, err := parseBytes(rt.GoMemLimit); err != nil {
			log.Warnf("invalid runtime.go-mem-limit: %v", err)
		} else {
			debug.SetMemoryLimit(bytes)
			_ = os.Setenv("GOMEMLIMIT", rt.GoMemLimit)
			log.Infof("runtime: set GOMEMLIMIT to %s (%d bytes)", rt.GoMemLimit, bytes)
		}
	}

	if rt.GoGC > 0 {
		debug.SetGCPercent(rt.GoGC)
		_ = os.Setenv("GOGC", strconv.Itoa(rt.GoGC))
		log.Infof("runtime: set GOGC to %d", rt.GoGC)
	}
}

// parseBytes 解析带单位的内存字符串为字节数，支持 k/m/g 或 kib/mib/gib
func parseBytes(s string) (int64, error) {
	str := strings.TrimSpace(strings.ToLower(s))
	if str == "" {
		return 0, nil
	}

	multipliers := map[string]float64{
		"k":   1 << 10,
		"kb":  1 << 10,
		"ki":  1 << 10,
		"kib": 1 << 10,
		"m":   1 << 20,
		"mb":  1 << 20,
		"mi":  1 << 20,
		"mib": 1 << 20,
		"g":   1 << 30,
		"gb":  1 << 30,
		"gi":  1 << 30,
		"gib": 1 << 30,
	}

	for unit, mul := range multipliers {
		if strings.HasSuffix(str, unit) {
			value := strings.TrimSuffix(str, unit)
			v, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			if err != nil {
				return 0, err
			}
			return int64(v * mul), nil
		}
	}

	// 没有单位时按字节解析
	v, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		return 0, err
	}
	return v, nil
}
