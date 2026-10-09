//go:build integration

package compatibilityretirementdbcensus

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// This narrowly authorized native regression uses only the existing owned
// loopback fixture. Its one-time role/user belong to a random test database;
// creation response/identity uncertainty retains that namespace for inspection.
func TestDBWriterCensusUsersExpandedNative(t *testing.T) {
	uri := strings.TrimSpace(os.Getenv("QS_SERVER_TEST_MONGO_URI"))
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != "mongodb" || parsed.Host != "127.0.0.1:33317" || parsed.User != nil || parsed.Path != "/" || parsed.Query().Get("replicaSet") != "qscompat" || parsed.Query().Get("directConnection") != "true" {
		t.Fatal("owned_loopback_census_fixture_required")
	}
	var random [24]byte
	if _, err = rand.Read(random[:]); err != nil {
		t.Fatal("census_fixture_random_failed")
	}
	name := "qs_census_test_" + hex.EncodeToString(random[:12])
	password := hex.EncodeToString(random[12:])
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal("census_fixture_connect_failed")
	}
	db := client.Database(name)
	roleName, userName := "owned_census_role", "owned_census_user"
	var userIdentity, roleIdentity string
	unknown := false
	roleCreated, userCreated := false, false
	t.Cleanup(func() {
		cleanup, done := context.WithTimeout(context.Background(), 15*time.Second)
		defer done()
		defer func() {
			if client.Disconnect(cleanup) != nil {
				t.Error("census_fixture_disconnect_failed")
			}
		}()
		if unknown {
			t.Error("census_fixture_unknown_namespace_retained")
			return
		}
		// Before a confirmed owned creation, an identity/precheck failure has
		// no cleanup write authority. The deferred Disconnect is the only action.
		if !roleCreated && !userCreated {
			return
		}
		if userCreated {
			actual, e := dbCensusMongoRun(cleanup, client, "admin", bson.D{{Key: "usersInfo", Value: bson.D{{Key: "user", Value: userName}, {Key: "db", Value: name}}}, {Key: "showCredentials", Value: false}, {Key: "showCustomData", Value: false}})
			rows, parseErr := dbCensusArray(actual["users"])
			if e != nil || parseErr != nil || len(rows) != 1 || checksum(rows[0]) != userIdentity {
				t.Error("census_fixture_user_cleanup_identity_unknown")
				return
			}
			if db.RunCommand(cleanup, bson.D{{Key: "dropUser", Value: userName}}).Err() != nil {
				t.Error("census_fixture_drop_user_unknown")
				return
			}
		}
		if roleCreated {
			actual, e := dbCensusMongoRun(cleanup, client, name, bson.D{{Key: "rolesInfo", Value: roleName}})
			rows, parseErr := dbCensusArray(actual["roles"])
			if e != nil || parseErr != nil || len(rows) != 1 || checksum(rows[0]) != roleIdentity {
				t.Error("census_fixture_role_cleanup_identity_unknown")
				return
			}
			if db.RunCommand(cleanup, bson.D{{Key: "dropRole", Value: roleName}}).Err() != nil {
				t.Error("census_fixture_drop_role_unknown")
				return
			}
		}
		for _, command := range []bson.D{{{Key: "usersInfo", Value: bson.D{{Key: "user", Value: userName}, {Key: "db", Value: name}}}, {Key: "showCredentials", Value: false}, {Key: "showCustomData", Value: false}}, {{Key: "rolesInfo", Value: bson.D{{Key: "role", Value: roleName}, {Key: "db", Value: name}}}}} {
			reply, e := dbCensusMongoRun(cleanup, client, "admin", command)
			key := "users"
			if command[0].Key == "rolesInfo" {
				key = "roles"
			}
			rows, parseErr := dbCensusArray(reply[key])
			if e != nil || parseErr != nil || len(rows) != 0 {
				t.Error("census_fixture_cleanup_absence_unproven")
				return
			}
		}
		// Only after actual user/role absence is proven may this owned namespace drop.
		if db.Drop(cleanup) != nil {
			t.Error("census_fixture_drop_namespace_unknown")
			return
		}
		remaining, e := client.ListDatabaseNames(cleanup, bson.D{{Key: "name", Value: name}})
		if e != nil || len(remaining) != 0 {
			t.Error("census_fixture_namespace_absence_unproven")
			return
		}
		t.Log("native_census_cleanup users=0 roles=0 owned_namespace_dropped=true")
	})
	hello, err := dbCensusMongoRun(ctx, client, "admin", bson.D{{Key: "hello", Value: 1}})
	if err != nil || hello["setName"] != "qscompat" {
		t.Fatal("census_fixture_replica_identity_rejected")
	}
	// No existing namespace may be adopted, even on this approved local fixture.
	databases, err := client.ListDatabaseNames(ctx, bson.D{{Key: "name", Value: name}})
	if err != nil || len(databases) != 0 {
		unknown = true
		t.Fatal("census_fixture_namespace_collision_or_unknown")
	}
	for _, command := range []bson.D{{{Key: "usersInfo", Value: bson.D{{Key: "user", Value: userName}, {Key: "db", Value: name}}}, {Key: "showCredentials", Value: false}, {Key: "showCustomData", Value: false}}, {{Key: "rolesInfo", Value: bson.D{{Key: "role", Value: roleName}, {Key: "db", Value: name}}}}} {
		reply, e := dbCensusMongoRun(ctx, client, "admin", command)
		key := "users"
		if command[0].Key == "rolesInfo" {
			key = "roles"
		}
		rows, parseErr := dbCensusArray(reply[key])
		if e != nil || parseErr != nil || len(rows) != 0 {
			unknown = true
			t.Fatal("census_fixture_auth_namespace_collision_or_unknown")
		}
	}
	unknown = true
	if db.RunCommand(ctx, bson.D{{Key: "createRole", Value: roleName}, {Key: "privileges", Value: bson.A{bson.D{{Key: "resource", Value: bson.D{{Key: "db", Value: name}, {Key: "collection", Value: ""}}}, {Key: "actions", Value: bson.A{"find"}}}}}, {Key: "roles", Value: bson.A{}}}).Err() != nil {
		t.Fatal("census_fixture_create_role_unknown")
	}
	role, err := dbCensusMongoRun(ctx, client, name, bson.D{{Key: "rolesInfo", Value: roleName}})
	roleRows, parseErr := dbCensusArray(role["roles"])
	if err != nil || parseErr != nil || len(roleRows) != 1 {
		t.Fatal("census_fixture_created_role_readback_unknown")
	}
	roleIdentity = checksum(roleRows[0])
	roleCreated = true
	unknown = false
	unknown = true
	if db.RunCommand(ctx, bson.D{{Key: "createUser", Value: userName}, {Key: "pwd", Value: password}, {Key: "roles", Value: bson.A{bson.D{{Key: "role", Value: roleName}, {Key: "db", Value: name}}}}, {Key: "customData", Value: bson.D{{Key: "fixture_body", Value: "must_not_return"}}}}).Err() != nil {
		t.Fatal("census_fixture_create_user_unknown")
	}
	user, err := dbCensusMongoRun(ctx, client, "admin", bson.D{{Key: "usersInfo", Value: bson.D{{Key: "user", Value: userName}, {Key: "db", Value: name}}}, {Key: "showCredentials", Value: false}, {Key: "showCustomData", Value: false}})
	userRows, parseErr := dbCensusArray(user["users"])
	if err != nil || parseErr != nil || len(userRows) != 1 {
		t.Fatal("census_fixture_created_user_readback_unknown")
	}
	userIdentity = checksum(userRows[0])
	userCreated = true
	unknown = false
	borrowed := func(ctx context.Context, db string, command bson.D) (bson.M, error) {
		return dbCensusMongoRun(ctx, client, db, command)
	}
	var first string
	for pass := 0; pass < 2; pass++ {
		_, expanded, e := dbCensusMongoUsers(ctx, borrowed)
		if e != nil {
			t.Fatal("native_census_user_expansion_failed")
		}
		found := false
		for _, row := range expanded {
			if row["user"] == userName && row["db"] == name {
				privileges, parseErr := dbCensusArray(row["inheritedPrivileges"])
				if parseErr != nil || len(privileges) == 0 || checksum(row["authenticationRestrictions"]) != checksum(bson.A{}) {
					t.Fatal("native_census_nonempty_expansion_unproven")
				}
				found = true
			}
		}
		if !found {
			t.Fatal("native_census_owned_user_missing")
		}
		actual := checksum(expanded)
		if pass == 1 && actual != first {
			t.Fatal("native_census_two_round_membership_changed")
		}
		first = actual
	}
	t.Log("native_census_nonempty_users expanded=true two_rounds_equal=true credentials_returned=false custom_data_returned=false")
}
