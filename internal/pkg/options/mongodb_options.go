package options

import (
	"github.com/spf13/pflag"
)

// MongoDBOptions defines options for mongodb database.
type MongoDBOptions struct {
	URL                      string `json:"url,omitempty"                                mapstructure:"url"`
	UseSSL                   bool   `json:"use-ssl,omitempty"                            mapstructure:"use-ssl"`
	SSLInsecureSkipVerify    bool   `json:"ssl-insecure-skip-verify,omitempty"           mapstructure:"ssl-insecure-skip-verify"`
	SSLAllowInvalidHostnames bool   `json:"ssl-allow-invalid-hostnames,omitempty"        mapstructure:"ssl-allow-invalid-hostnames"`
	SSLCAFile                string `json:"ssl-ca-file,omitempty"                        mapstructure:"ssl-ca-file"`
	SSLPEMKeyfile            string `json:"ssl-pem-keyfile,omitempty"                    mapstructure:"ssl-pem-keyfile"`
}

// NewMongoDBOptions create a `zero` value instance.
func NewMongoDBOptions() *MongoDBOptions {
	return &MongoDBOptions{
		URL:                      "mongodb://127.0.0.1:27017",
		UseSSL:                   false,
		SSLInsecureSkipVerify:    false,
		SSLAllowInvalidHostnames: false,
		SSLCAFile:                "",
		SSLPEMKeyfile:            "",
	}
}

// Validate verifies flags passed to MongoDBOptions.
func (o *MongoDBOptions) Validate() []error {
	errs := []error{}

	return errs
}

// AddFlags adds flags related to mongodb storage for a specific APIServer to the specified FlagSet.
func (o *MongoDBOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.URL, "mongodb.url", o.URL, ""+
		"MongoDB connection URL. If left blank, the following related mongodb options will be ignored.")

	fs.BoolVar(&o.UseSSL, "mongodb.use-ssl", o.UseSSL, ""+
		"Enable SSL for mongodb connection.")

	fs.BoolVar(&o.SSLInsecureSkipVerify, "mongodb.ssl-insecure-skip-verify", o.SSLInsecureSkipVerify, ""+
		"Skip SSL certificate verification for mongodb.")

	fs.BoolVar(&o.SSLAllowInvalidHostnames, "mongodb.ssl-allow-invalid-hostnames", o.SSLAllowInvalidHostnames, ""+
		"Allow invalid hostnames in SSL certificates for mongodb.")

	fs.StringVar(&o.SSLCAFile, "mongodb.ssl-ca-file", o.SSLCAFile, ""+
		"Path to SSL CA certificate file for mongodb.")

	fs.StringVar(&o.SSLPEMKeyfile, "mongodb.ssl-pem-keyfile", o.SSLPEMKeyfile, ""+
		"Path to SSL PEM key file for mongodb.")
}
