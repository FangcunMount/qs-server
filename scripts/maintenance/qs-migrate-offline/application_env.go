package main

import (
	"errors"
	"strconv"

	"github.com/FangcunMount/component-base/pkg/database"
	mysql "github.com/go-sql-driver/mysql"
)

// Connection selection is explicit; the application mode never falls back to
// another owner or the standalone environment. It does not change action/approval.
func connectionInputs(a arguments, getenv func(string) string) (string, string, error) {
	if !a.applicationEnvironment {
		return getenv("QS_OFFLINE_MYSQL_DSN"), getenv("QS_OFFLINE_MONGO_URI"), nil
	}
	if getenv("QS_OFFLINE_MYSQL_DSN") != "" || getenv("QS_OFFLINE_MONGO_URI") != "" {
		return "", "", errors.New("ambiguous offline connection source")
	}
	get := func(k string) string { return getenv("QS_APISERVER_" + k) }
	if get("MYSQL_DATABASE") != a.mysqlDatabase || get("MONGODB_DATABASE") != a.mongoDatabase || get("MYSQL_HOST") == "" || get("MYSQL_USERNAME") == "" || get("MYSQL_TLS_MODE") != "VERIFY_IDENTITY" || get("MYSQL_TLS_CA_FILE") == "" {
		return "", "", errors.New("explicit application database/TLS binding required")
	}
	for _, key := range []string{"MODE", "CA_FILE"} {
		if value := getenv("MYSQL_TLS_" + key); value != "" && value != get("MYSQL_TLS_"+key) {
			return "", "", errors.New("conflicting application TLS selection")
		}
	}
	sql := mysql.NewConfig()
	sql.Net, sql.Addr, sql.User, sql.Passwd, sql.DBName = "tcp", get("MYSQL_HOST"), get("MYSQL_USERNAME"), get("MYSQL_PASSWORD"), a.mysqlDatabase
	sql.ParseTime, sql.MultiStatements = true, true
	mongo := database.MongoConfig{URL: get("MONGODB_URL"), Host: get("MONGODB_HOST"), Username: get("MONGODB_USERNAME"), Password: get("MONGODB_PASSWORD"), Database: a.mongoDatabase, ReplicaSet: get("MONGODB_REPLICA_SET")}
	if mongo.URL == "" && (mongo.Host == "" || mongo.Username == "") {
		return "", "", errors.New("explicit application Mongo binding required")
	}
	for key, dst := range map[string]*bool{"DIRECT_CONNECTION": &mongo.DirectConnection, "USE_SSL": &mongo.UseSSL, "SSL_INSECURE_SKIP_VERIFY": &mongo.SSLInsecureSkipVerify, "SSL_ALLOW_INVALID_HOSTNAMES": &mongo.SSLAllowInvalidHostnames} {
		if value := get("MONGODB_" + key); value != "" {
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return "", "", errors.New("invalid application Mongo transport flag")
			}
			*dst = parsed
		}
	}
	mongo.SSLCAFile, mongo.SSLPEMKeyfile = get("MONGODB_SSL_CA_FILE"), get("MONGODB_SSL_PEM_KEYFILE")
	return sql.FormatDSN(), mongo.BuildURL(), nil
}
