package options

import (
	"fmt"
	"net/url"
	"time"

	"github.com/spf13/pflag"
)

// OSSOptions defines options for QR code object storage backed by OSS.
type OSSOptions struct {
	Enabled             bool          `json:"enabled" mapstructure:"enabled"`
	Region              string        `json:"region,omitempty" mapstructure:"region"`
	Endpoint            string        `json:"endpoint,omitempty" mapstructure:"endpoint"`
	Bucket              string        `json:"bucket,omitempty" mapstructure:"bucket"`
	PublicBaseURL       string        `json:"public_base_url,omitempty" mapstructure:"public-base-url"`
	ObjectKeyPrefix     string        `json:"object_key_prefix,omitempty" mapstructure:"object-key-prefix"`
	CacheControl        string        `json:"cache_control,omitempty" mapstructure:"cache-control"`
	AccessKeyID         string        `json:"access_key_id,omitempty" mapstructure:"access-key-id"`
	AccessKeySecret     string        `json:"-" mapstructure:"access-key-secret"`
	SessionToken        string        `json:"-" mapstructure:"session-token"`
	UseInternalEndpoint bool          `json:"use_internal_endpoint,omitempty" mapstructure:"use-internal-endpoint"`
	UseCName            bool          `json:"use_cname,omitempty" mapstructure:"use-cname"`
	ConnectTimeout      time.Duration `json:"connect_timeout,omitempty" mapstructure:"connect-timeout"`
	ReadWriteTimeout    time.Duration `json:"read_write_timeout,omitempty" mapstructure:"read-write-timeout"`
	RetryMaxAttempts    int           `json:"retry_max_attempts,omitempty" mapstructure:"retry-max-attempts"`
}

// NewOSSOptions creates a default options instance.
func NewOSSOptions() *OSSOptions {
	return &OSSOptions{
		Enabled:             false,
		Region:              "",
		Endpoint:            "",
		Bucket:              "",
		PublicBaseURL:       "",
		ObjectKeyPrefix:     "qrcode",
		CacheControl:        "public, max-age=604800",
		AccessKeyID:         "",
		AccessKeySecret:     "",
		SessionToken:        "",
		UseInternalEndpoint: false,
		UseCName:            false,
		ConnectTimeout:      3 * time.Second,
		ReadWriteTimeout:    10 * time.Second,
		RetryMaxAttempts:    2,
	}
}

// Validate verifies flags passed to OSSOptions.
func (o *OSSOptions) Validate() []error {
	if o == nil || !o.Enabled {
		return nil
	}

	var errs []error
	if o.Bucket == "" {
		errs = append(errs, fmt.Errorf("oss.bucket is required when oss.enabled is true"))
	}
	if o.PublicBaseURL == "" {
		errs = append(errs, fmt.Errorf("oss.public-base-url is required when oss.enabled is true"))
	} else if parsed, err := url.Parse(o.PublicBaseURL); err != nil {
		errs = append(errs, fmt.Errorf("oss.public-base-url must be a valid URL: %w", err))
	} else if parsed.Scheme == "" || parsed.Host == "" {
		errs = append(errs, fmt.Errorf("oss.public-base-url must include scheme and host"))
	}
	if o.Region == "" && o.Endpoint == "" {
		errs = append(errs, fmt.Errorf("oss.region or oss.endpoint must be set when oss.enabled is true"))
	}
	if (o.AccessKeyID == "") != (o.AccessKeySecret == "") {
		errs = append(errs, fmt.Errorf("oss.access-key-id and oss.access-key-secret must be set together"))
	}
	if o.ConnectTimeout < 0 {
		errs = append(errs, fmt.Errorf("oss.connect-timeout cannot be negative"))
	}
	if o.ReadWriteTimeout < 0 {
		errs = append(errs, fmt.Errorf("oss.read-write-timeout cannot be negative"))
	}
	if o.RetryMaxAttempts < 0 {
		errs = append(errs, fmt.Errorf("oss.retry-max-attempts cannot be negative"))
	}

	return errs
}

// AddFlags adds flags related to object storage to the specified FlagSet.
func (o *OSSOptions) AddFlags(fs *pflag.FlagSet) {
	addBoolFlags(fs, []boolFlagSpec{
		{target: &o.Enabled, name: "oss.enabled", value: o.Enabled, usage: "Enable OSS-backed object storage for QR code uploads."},
		{target: &o.UseInternalEndpoint, name: "oss.use-internal-endpoint", value: o.UseInternalEndpoint, usage: "Use OSS internal endpoint for uploads."},
		{target: &o.UseCName, name: "oss.use-cname", value: o.UseCName, usage: "Treat the configured endpoint as a CNAME endpoint."},
	})

	addStringFlags(fs, []stringFlagSpec{
		{target: &o.Region, name: "oss.region", value: o.Region, usage: "Alibaba Cloud OSS region, for example cn-shanghai."},
		{target: &o.Endpoint, name: "oss.endpoint", value: o.Endpoint, usage: "Alibaba Cloud OSS endpoint override, for example oss-cn-shanghai-internal.aliyuncs.com."},
		{target: &o.Bucket, name: "oss.bucket", value: o.Bucket, usage: "Alibaba Cloud OSS bucket name."},
		{target: &o.PublicBaseURL, name: "oss.public-base-url", value: o.PublicBaseURL, usage: "QR code URL prefix returned to clients, for example https://qs.example.com/api/v1/qrcodes."},
		{target: &o.ObjectKeyPrefix, name: "oss.object-key-prefix", value: o.ObjectKeyPrefix, usage: "Object key prefix used for uploaded QR codes."},
		{target: &o.CacheControl, name: "oss.cache-control", value: o.CacheControl, usage: "Cache-Control header applied to uploaded QR code objects."},
		{target: &o.AccessKeyID, name: "oss.access-key-id", value: o.AccessKeyID, usage: "OSS access key ID. If empty, the SDK falls back to environment variables."},
		{target: &o.AccessKeySecret, name: "oss.access-key-secret", value: o.AccessKeySecret, usage: "OSS access key secret. If empty, the SDK falls back to environment variables."},
		{target: &o.SessionToken, name: "oss.session-token", value: o.SessionToken, usage: "Optional OSS STS session token."},
	})

	addDurationFlag(fs, &o.ConnectTimeout, "oss.connect-timeout", o.ConnectTimeout, "OSS client connect timeout.")
	addDurationFlag(fs, &o.ReadWriteTimeout, "oss.read-write-timeout", o.ReadWriteTimeout, "OSS client read/write timeout.")
	addIntFlag(fs, &o.RetryMaxAttempts, "oss.retry-max-attempts", o.RetryMaxAttempts, "Maximum OSS client retry attempts.")
}
