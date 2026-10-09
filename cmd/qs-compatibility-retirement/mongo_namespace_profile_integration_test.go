//go:build integration

package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	identitymeta "github.com/FangcunMount/qs-server/internal/pkg/databaseidentity"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// The existing permission fixture owns all Docker assets and verifies actual
// ownership. This new case touches only its own random DB/user. It is not a
// production privilege test or business evidence.
func TestMongoNamespaceNativeReadScopeWithoutReplicaAdmin(t *testing.T) {
	root := permissionNativeRoot(t)
	f := permissionOwnedMongo(t, root, true)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	// Initialize and authenticate only this owned fixture, using the same
	// localhost-exception path as the existing replica permission test.
	init := `const v=JSON.parse(require("fs").readFileSync(0,"utf8"));const r=db.getSiblingDB("admin").runCommand({replSetInitiate:{_id:v.replica,members:[{_id:0,host:"127.0.0.1:27017"}]}});if(r.ok!==1){quit(2);}`
	input, _ := json.Marshal(map[string]string{"replica": f.Replica})
	for attempts := 0; ; attempts++ {
		if _, err := permissionDocker(ctx, input, "exec", "-i", f.ID, "mongosh", "--quiet", "--norc", "--eval", init); err == nil {
			break
		}
		if attempts >= 30 {
			t.Fatal("owned_auth_replica_initiate_failed")
		}
		time.Sleep(200 * time.Millisecond)
	}
	initial := permissionConnect(t, f.Port, f.Replica, "", "", "")
	permissionHello(t, initial, true)
	adminUser := "admin_" + primitive.NewObjectID().Hex()
	adminSecret := make([]byte, 32)
	if _, err := rand.Read(adminSecret); err != nil {
		t.Fatal("owned_admin_credentials_failed")
	}
	adminPassword := base64.RawURLEncoding.EncodeToString(adminSecret)
	credentials := map[string]string{"user": adminUser, "password": adminPassword}
	if permissionPrivateJSON(filepath.Join(filepath.Dir(f.Manifest), "credentials.json"), credentials) != nil {
		t.Fatal("owned_credentials_registration_failed")
	}
	create := `const v=JSON.parse(require("fs").readFileSync(0,"utf8"));const r=db.getSiblingDB("admin").runCommand({createUser:v.user,pwd:v.password,roles:[{role:"root",db:"admin"}],mechanisms:["SCRAM-SHA-256"]});if(r.ok!==1){quit(2);}`
	input, _ = json.Marshal(credentials)
	if _, err := permissionDocker(ctx, input, "exec", "-i", f.ID, "mongosh", "--quiet", "--norc", "--eval", create); err != nil {
		t.Fatal("owned_admin_create_failed")
	}
	admin := permissionConnect(t, f.Port, f.Replica, adminUser, adminPassword, "admin")
	permissionHello(t, admin, true)
	name := "qs_namespace_profile_" + primitive.NewObjectID().Hex()
	db := admin.Database(name)
	user := "reader_" + primitive.NewObjectID().Hex()
	secret := make([]byte, 24)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal("fixture_secret_failed")
	}
	password := base64.RawURLEncoding.EncodeToString(secret)
	createdUser := false
	t.Cleanup(func() {
		q, done := context.WithTimeout(context.Background(), 15*time.Second)
		defer done()
		if createdUser {
			if db.RunCommand(q, bson.D{{Key: "dropUser", Value: user}}).Err() != nil {
				t.Error("owned_user_cleanup_failed")
				return
			}
			var result struct {
				Users []bson.Raw `bson:"users"`
			}
			if db.RunCommand(q, bson.D{{Key: "usersInfo", Value: user}}).Decode(&result) != nil || len(result.Users) != 0 {
				t.Error("owned_user_absence_unproven")
				return
			}
		}
		if db.Drop(q) != nil {
			t.Error("owned_database_cleanup_failed")
			return
		}
		t.Log("owned_namespace_profile_user_remaining=0")
	})
	if _, err := db.Collection("answersheets").InsertOne(ctx, bson.D{{Key: "_id", Value: 1}, {Key: "fixture_only", Value: true}}); err != nil {
		t.Fatal("owned_collection_failed")
	}
	if db.RunCommand(ctx, bson.D{{Key: "createUser", Value: user}, {Key: "pwd", Value: password}, {Key: "roles", Value: bson.A{bson.D{{Key: "role", Value: "read"}, {Key: "db", Value: name}}}}, {Key: "mechanisms", Value: bson.A{"SCRAM-SHA-256"}}}).Err() != nil {
		t.Fatal("owned_reader_create_failed")
	}
	createdUser = true
	reader := permissionConnect(t, f.Port, f.Replica, user, password, name)
	hello := permissionHello(t, reader, true)
	denied, err := mongoDatabaseAnchor(ctx, reader.Database(name), hello)
	if err == nil || err.Error() != "mongo_replica_anchor_not_authorized" || denied != "" {
		t.Fatal("legacy_replica_profile_permission_contract_changed")
	}
	port, err := strconv.Atoi(f.Port)
	if err != nil {
		t.Fatal("owned_port_invalid")
	}
	endpoint, err := identitymeta.MongoEndpointSHA256("127.0.0.1", port, name)
	if err != nil {
		t.Fatal("owned_endpoint_failed")
	}
	a, err := identitymeta.ObserveMongoNamespaceAnchor(ctx, reader.Database(name), endpoint)
	if err != nil || a.Validate() != nil || a.Kind != identitymeta.MongoNamespaceAnchorKind || !a.Collections[0].Present {
		t.Fatal("actual_read_scope_namespace_anchor_failed")
	}
	if err = db.Collection("answersheets").Drop(ctx); err != nil {
		t.Fatal("owned_replacement_failed")
	}
	if _, err = db.Collection("answersheets").InsertOne(ctx, bson.D{{Key: "_id", Value: 1}, {Key: "fixture_only", Value: true}}); err != nil {
		t.Fatal("owned_replacement_failed")
	}
	b, err := identitymeta.ObserveMongoNamespaceAnchor(ctx, reader.Database(name), endpoint)
	if err != nil || identitymeta.MatchMongoNamespaceAnchors(a, b) {
		t.Fatal("same_name_replacement_not_detected")
	}
	t.Log("namespace_read_scope_verified=true legacy_replica_denied_13=true actual_kept_uuid_replacement_rejected=true")
}
