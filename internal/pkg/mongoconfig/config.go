package mongoconfig

import (
	"fmt"
	"net/url"
	"strconv"

	componentdatabase "github.com/FangcunMount/component-base/pkg/database"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
)

// Build converts shared MongoDB options into the component-base connection
// configuration and applies typed pool settings to the effective URI.
func Build(opts *genericoptions.MongoDBOptions) (*componentdatabase.MongoConfig, error) {
	if opts == nil {
		return nil, fmt.Errorf("mongodb options are required")
	}
	if errs := opts.Validate(); len(errs) > 0 {
		return nil, errs[0]
	}

	config := &componentdatabase.MongoConfig{
		URL:                      opts.URL,
		Host:                     opts.Host,
		Username:                 opts.Username,
		Password:                 opts.Password,
		Database:                 opts.Database,
		ReplicaSet:               opts.ReplicaSet,
		DirectConnection:         opts.DirectConnection,
		UseSSL:                   opts.UseSSL,
		SSLInsecureSkipVerify:    opts.SSLInsecureSkipVerify,
		SSLAllowInvalidHostnames: opts.SSLAllowInvalidHostnames,
		SSLCAFile:                opts.SSLCAFile,
		SSLPEMKeyfile:            opts.SSLPEMKeyfile,
		EnableLogger:             opts.EnableLogger,
		SlowThreshold:            opts.SlowThreshold,
		LogCommandDetail:         opts.LogCommandDetail,
		LogReplyDetail:           opts.LogReplyDetail,
		LogStarted:               opts.LogStarted,
	}

	effectiveURL, err := applyPoolOptions(config.BuildURL(), opts)
	if err != nil {
		return nil, err
	}
	config.URL = effectiveURL
	return config, nil
}

func applyPoolOptions(rawURL string, opts *genericoptions.MongoDBOptions) (string, error) {
	if opts == nil || (opts.MinPoolSize == 0 && opts.MaxPoolSize == 0 && opts.MaxConnecting == 0 && opts.MaxConnIdleTime == 0) {
		return rawURL, nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse mongodb url: %w", err)
	}
	if parsed.Scheme != "mongodb" && parsed.Scheme != "mongodb+srv" {
		return "", fmt.Errorf("mongodb url must use mongodb or mongodb+srv scheme")
	}

	query := parsed.Query()
	if opts.MinPoolSize > 0 {
		query.Set("minPoolSize", strconv.FormatUint(opts.MinPoolSize, 10))
	}
	if opts.MaxPoolSize > 0 {
		query.Set("maxPoolSize", strconv.FormatUint(opts.MaxPoolSize, 10))
	}
	if opts.MaxConnecting > 0 {
		query.Set("maxConnecting", strconv.FormatUint(opts.MaxConnecting, 10))
	}
	if opts.MaxConnIdleTime > 0 {
		query.Set("maxIdleTimeMS", strconv.FormatInt(opts.MaxConnIdleTime.Milliseconds(), 10))
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
