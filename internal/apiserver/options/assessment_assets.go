package options

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/pflag"
)

const defaultAssessmentImageUploadBytes int64 = 5 * 1024 * 1024

// AssessmentAssetsOptions configures the private OSS-backed image proxy used
// by assessment-model definition editors.
type AssessmentAssetsOptions struct {
	Enabled         bool   `json:"enabled" mapstructure:"enabled"`
	ObjectKeyPrefix string `json:"object_key_prefix" mapstructure:"object-key-prefix"`
	PublicURLPrefix string `json:"public_url_prefix" mapstructure:"public-url-prefix"`
	MaxUploadBytes  int64  `json:"max_upload_bytes" mapstructure:"max-upload-bytes"`
}

func NewAssessmentAssetsOptions() *AssessmentAssetsOptions {
	return &AssessmentAssetsOptions{
		Enabled:         false,
		ObjectKeyPrefix: "assessment-assets/typology",
		PublicURLPrefix: "https://qs.fangcunmount.cn/api/v1/assessment-assets/typology",
		MaxUploadBytes:  defaultAssessmentImageUploadBytes,
	}
}

func (o *AssessmentAssetsOptions) Validate() []error {
	if o == nil || !o.Enabled {
		return nil
	}
	var errs []error
	if strings.Trim(o.ObjectKeyPrefix, "/ ") == "" {
		errs = append(errs, fmt.Errorf("assessment_assets.object_key_prefix is required when enabled"))
	}
	if parsed, err := url.Parse(o.PublicURLPrefix); err != nil || parsed.Scheme == "" || parsed.Host == "" {
		errs = append(errs, fmt.Errorf("assessment_assets.public_url_prefix must be an absolute URL when enabled"))
	}
	if o.MaxUploadBytes <= 0 {
		errs = append(errs, fmt.Errorf("assessment_assets.max_upload_bytes must be greater than 0"))
	}
	return errs
}

func (o *AssessmentAssetsOptions) AddFlags(fs *pflag.FlagSet) {
	if o == nil {
		return
	}
	fs.BoolVar(&o.Enabled, "assessment-assets.enabled", o.Enabled, "Enable private OSS-backed assessment image assets.")
	fs.StringVar(&o.ObjectKeyPrefix, "assessment-assets.object-key-prefix", o.ObjectKeyPrefix, "OSS object-key prefix for assessment image assets.")
	fs.StringVar(&o.PublicURLPrefix, "assessment-assets.public-url-prefix", o.PublicURLPrefix, "Stable public qs-server URL prefix for assessment image assets.")
	fs.Int64Var(&o.MaxUploadBytes, "assessment-assets.max-upload-bytes", o.MaxUploadBytes, "Maximum assessment image upload size in bytes.")
}
