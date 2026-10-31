package main

import (
	"net/url"
	"testing"

	mysql "github.com/go-sql-driver/mysql"
)

func TestApplicationEnvironmentKeepsCredentialsInExplicitOwnerAndReadonlyDefault(t *testing.T) {
	env := map[string]string{"QS_APISERVER_MYSQL_HOST": "fixture:3306", "QS_APISERVER_MYSQL_USERNAME": "private-user", "QS_APISERVER_MYSQL_PASSWORD": "private:@/pass", "QS_APISERVER_MYSQL_DATABASE": "qs", "QS_APISERVER_MYSQL_TLS_MODE": "VERIFY_IDENTITY", "QS_APISERVER_MYSQL_TLS_CA_FILE": "/original/ca.pem", "QS_APISERVER_MONGODB_HOST": "mongo:27017", "QS_APISERVER_MONGODB_USERNAME": "private-user", "QS_APISERVER_MONGODB_PASSWORD": "private:@/pass", "QS_APISERVER_MONGODB_DATABASE": "qs", "QS_APISERVER_MONGODB_REPLICA_SET": "rs0"}
	a, err := parseArguments([]string{"--mysql-database=qs", "--mongo-database=qs", "--application-env", "--timeout=30s"})
	if err != nil || a.apply {
		t.Fatal("application environment changed action", err)
	}
	get := func(k string) string { return env[k] }
	dsn, uri, err := connectionInputs(a, get)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil || cfg.Passwd != env["QS_APISERVER_MYSQL_PASSWORD"] || cfg.DBName != "qs" {
		t.Fatal("MySQL owner or credential was rewritten", err)
	}
	u, err := url.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	password, _ := u.User.Password()
	if password != env["QS_APISERVER_MONGODB_PASSWORD"] || u.Path != "/qs" || u.Query().Get("replicaSet") != "rs0" {
		t.Fatal("Mongo owner or credential was rewritten")
	}
	for key, value := range map[string]string{"QS_OFFLINE_MYSQL_DSN": "foreign", "QS_OFFLINE_MONGO_URI": "foreign", "QS_APISERVER_MYSQL_DATABASE": "foreign", "QS_APISERVER_MONGODB_DATABASE": "foreign", "QS_APISERVER_MYSQL_TLS_MODE": "DISABLED", "MYSQL_TLS_MODE": "DISABLED", "QS_APISERVER_MONGODB_DIRECT_CONNECTION": "invalid"} {
		old, present := env[key]
		env[key] = value
		_, _, err := connectionInputs(a, get)
		if present {
			env[key] = old
		} else {
			delete(env, key)
		}
		if err == nil {
			t.Fatal("ambiguous/unverified connection accepted", key)
		}
	}
}
