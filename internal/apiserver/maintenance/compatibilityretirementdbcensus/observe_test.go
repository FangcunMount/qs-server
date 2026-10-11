package compatibilityretirementdbcensus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
)

func TestDBWriterCensusLocalSessionReadConcern(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock).CreateCollection(false).
		ClientOptions(options.Client().SetReadConcern(readconcern.Majority())))
	for _, item := range []struct {
		stage, db, collection, level string
	}{
		{"$currentOp", "admin", "", "local"},
		{"$listLocalSessions", "admin", "", "local"},
		{"$listSessions", "config", "system.sessions", "majority"},
	} {
		mt.Run(item.stage, func(mt *mtest.T) {
			mt.AddMockResponses(mtest.CreateCursorResponse(0, item.db+"."+item.collection, mtest.FirstBatch))
			rows, err := dbCensusMongoCursor(context.Background(), mt.Client, item.db, item.collection, dbCensusSessionPipeline(item.stage))
			if err != nil || len(rows) != 0 {
				mt.Fatalf("bounded session cursor failed: %v", err)
			}
			command := mt.GetStartedEvent()
			if command == nil || command.CommandName != "aggregate" || command.DatabaseName != item.db {
				mt.Fatal("session aggregation was not observed")
			}
			if actual := command.Command.Lookup("readConcern", "level").StringValue(); actual != item.level {
				mt.Fatalf("session read concern = %q, want %q", actual, item.level)
			}
			if mt.Client.Database(item.db).ReadConcern().GetLevel() != "majority" {
				mt.Fatal("borrowed client's read concern was changed")
			}
		})
	}
}

func censusPtr(v string) *string { return &v }
func censusSQLFixture(ctx context.Context, q string) ([][]*string, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	switch {
	case strings.HasPrefix(q, "SELECT User,Host,plugin,account_locked"):
		return [][]*string{{censusPtr("writer'o"), censusPtr("localhost"), censusPtr("caching_sha2_password"), censusPtr("N")}}, nil
	case q == "SHOW GRANTS FOR CURRENT_USER":
		return [][]*string{{censusPtr("GRANT PROCESS ON *.* TO 'observer'@'localhost'")}}, nil
	case strings.HasPrefix(q, "SHOW GRANTS FOR 'writer''o'@'localhost'"):
		return [][]*string{{censusPtr("GRANT SELECT, UPDATE ON `qs`.* TO 'writer''o'@'localhost'")}}, nil
	case strings.Contains(q, "PROCESSLIST"):
		return [][]*string{{censusPtr("17"), censusPtr("writer'o"), censusPtr("127.0.0.1:3012"), censusPtr("qs"), censusPtr("Sleep"), nil}}, nil
	default:
		return [][]*string{}, nil
	}
}
func TestDBWriterCensusSQLFullSleepAndPrivateGrants(t *testing.T) {
	queries := []string{}
	rows, sections, gaps := dbCensusSQLSnapshot(context.Background(), func(ctx context.Context, q string) ([][]*string, error) {
		queries = append(queries, q)
		return censusSQLFixture(ctx, q)
	})
	if len(rows["accounts"]) != 1 || len(rows["account_grants"]) != 1 || len(rows["connections"]) != 1 || *rows["connections"][0][4] != "Sleep" {
		t.Fatal("missing full account/grant/idle connection facts")
	}
	for _, q := range queries {
		lower := strings.ToLower(q)
		if strings.HasPrefix(lower, "grant ") || strings.HasPrefix(lower, "revoke ") {
			t.Fatal("mutating grant command")
		}
		for _, forbidden := range []string{"authentication_string", "password", "select *", "kill ", "alter "} {
			if strings.Contains(lower, forbidden) {
				t.Fatal("credential/writer query selected")
			}
		}
	}
	for _, s := range sections {
		if !s.EnumerationComplete {
			t.Fatal(s.Name)
		}
	}
	if len(gaps) != 1 || gaps[0] != "mysql_external_authentication_and_direct_writer_admission_not_fenced" {
		t.Fatal("external gap disappeared")
	}
}
func TestDBWriterCensusSQLPermissionFailureNotZero(t *testing.T) {
	for _, which := range []string{"account_grant", "process_privilege", "role_edges"} {
		t.Run(which, func(t *testing.T) {
			_, sections, _ := dbCensusSQLSnapshot(context.Background(), func(ctx context.Context, q string) ([][]*string, error) {
				if which == "account_grant" && strings.HasPrefix(q, "SHOW GRANTS FOR '") {
					return nil, errors.New("denied")
				}
				if which == "process_privilege" && q == "SHOW GRANTS FOR CURRENT_USER" {
					return [][]*string{{censusPtr("GRANT 'observer_role'@'%' TO 'observer'@'localhost'")}}, nil
				}
				if which == "role_edges" && strings.Contains(q, "mysql.role_edges") {
					return nil, errors.New("denied")
				}
				return censusSQLFixture(ctx, q)
			})
			name := map[string]string{"account_grant": "mysql_account_grants", "process_privilege": "mysql_connections", "role_edges": "mysql_role_edges"}[which]
			for _, s := range sections {
				if s.Name == name {
					if s.EnumerationComplete || s.ErrorCategory == "none" {
						t.Fatal("unknown permission became complete empty")
					}
					return
				}
			}
			t.Fatal("section omitted")
		})
	}
}
func TestDBWriterCensusAccountEscapingAndSecondEOF(t *testing.T) {
	if quoted, e := dbCensusLiteral("x'y"); e != nil || quoted != "'x''y'" {
		t.Fatal("quoted account changed")
	}
	for _, v := range []string{"a\\b", "a\x00b", "a\nb", strings.Repeat("x", 256)} {
		if _, e := dbCensusLiteral(v); e == nil {
			t.Fatal("unsafe account accepted")
		}
	}
	first := []Section{{Name: "a", EnumerationComplete: true, Items: 1, SHA256: strings.Repeat("a", 64), ErrorCategory: "none"}}
	later := append([]Section(nil), first...)
	later[0].EnumerationComplete = false
	later[0].ErrorCategory = "denied"
	result := dbCensusFinishSections(first, later)
	if result[0].EnumerationComplete || result[0].RecheckEqual || result[0].ErrorCategory != "denied" {
		t.Fatal("failed second read claimed complete")
	}
	result = dbCensusFinishSections(first, nil)
	if result[0].EnumerationComplete || result[0].ErrorCategory != "db_census_second_enumeration_missing" {
		t.Fatal("missing second read claimed complete")
	}
}
func censusMongoRunFixture(ctx context.Context, db string, q bson.D) (bson.M, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	switch q[0].Key {
	case "connectionStatus":
		return bson.M{"authInfo": bson.M{"authenticatedUserPrivileges": bson.A{bson.M{"resource": bson.M{"cluster": true}, "actions": bson.A{"inprog", "listSessions"}}}}}, nil
	case "usersInfo":
		flags := map[string]any{}
		for _, item := range q {
			flags[item.Key] = item.Value
		}
		if flags["showCredentials"] != false || flags["showCustomData"] != false {
			return nil, errors.New("unsafe usersInfo flags")
		}
		row := bson.M{"user": "private_user", "db": "authOnly", "roles": bson.A{bson.M{"role": "writerRole", "db": "roleOnly"}}}
		if _, all := q[0].Value.(bson.D); all {
			if _, exists := flags["showPrivileges"]; exists {
				return nil, errors.New("illegal all-user expansion")
			}
			if _, exists := flags["showAuthenticationRestrictions"]; exists {
				return nil, errors.New("illegal all-user expansion")
			}
		} else {
			if flags["showPrivileges"] != true || flags["showAuthenticationRestrictions"] != true {
				return nil, errors.New("missing expansion")
			}
			row["inheritedRoles"] = bson.A{bson.M{"role": "writerRole", "db": "roleOnly"}}
			row["inheritedPrivileges"] = bson.A{bson.M{"resource": bson.M{"db": "roleOnly", "collection": ""}, "actions": bson.A{"find"}}}
			row["authenticationRestrictions"] = bson.A{}
			row["inheritedAuthenticationRestrictions"] = bson.A{}
		}
		return bson.M{"users": bson.A{row}}, nil
	case "listDatabases":
		return bson.M{"databases": bson.A{bson.M{"name": "qs"}}}, nil
	case "rolesInfo":
		return bson.M{"roles": bson.A{bson.M{"role": "writerRole", "db": db, "roles": bson.A{}, "privileges": bson.A{}}}}, nil
	case "getParameter":
		return bson.M{"authenticationMechanisms": bson.A{"SCRAM-SHA-256"}, "enableLocalhostAuthBypass": false}, nil

	}
	return nil, errors.New("unexpected command")
}
func censusMongoAggregateFixture(ctx context.Context, db, col string, p mongo.Pipeline) ([]bson.M, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if col == "system.roles" {
		return []bson.M{{"role": "orphanRole", "db": "orphanDatabase", "roles": bson.A{}, "privileges": bson.A{}}}, nil
	}
	return []bson.M{{"connectionId": int32(42), "active": false, "type": "idleSession"}}, nil
}
func TestDBWriterCensusMongoNativeAccountScopeAndPrivacy(t *testing.T) {
	roleDBs := map[string]bool{}
	stages := map[string]bson.D{}
	rows, sections, gaps := dbCensusMongoSnapshot(context.Background(), func(ctx context.Context, db string, q bson.D) (bson.M, error) {
		if q[0].Key == "rolesInfo" {
			roleDBs[db] = true
		}
		if q[0].Key == "usersInfo" {
			if q[1].Key != "showCredentials" || q[1].Value != false {
				t.Fatal("credentials requested")
			}
		}
		return censusMongoRunFixture(ctx, db, q)
	}, func(ctx context.Context, db, col string, p mongo.Pipeline) ([]bson.M, error) {
		stages[p[0][0].Key] = p[0][0].Value.(bson.D)
		if col != "system.roles" {
			raw, _ := bson.Marshal(p[1])
			if strings.Contains(string(raw), "command") || strings.Contains(string(raw), "clientMetadata") {
				t.Fatal("command content selected")
			}
		}
		return censusMongoAggregateFixture(ctx, db, col, p)
	})
	if !roleDBs["authOnly"] || !roleDBs["roleOnly"] || !roleDBs["orphanDatabase"] || !roleDBs["qs"] {
		t.Fatal("roles in absent logical database lost")
	}
	for _, stage := range []string{"$currentOp", "$listLocalSessions", "$listSessions"} {
		v := stages[stage]
		if v == nil || v[0].Value != true {
			t.Fatal("allUsers missing")
		}
	}
	op := stages["$currentOp"]
	for _, key := range []string{"idleConnections", "idleSessions", "idleCursors"} {
		found := false
		for _, item := range op {
			if item.Key == key && item.Value == true {
				found = true
			}
		}
		if !found {
			t.Fatal("idle scope lost")
		}
	}
	encoded, _ := json.Marshal(rows)
	for _, secret := range []string{"never-persist", "private-key-source", "secret-command", "credentials", "keyFile", "argv"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatal("credential/config body persisted")
		}
	}
	for _, section := range sections {
		if section.Name == "mongodb_authentication_configuration" {
			if section.EnumerationComplete {
				t.Fatal("startup configuration guessed")
			}
			continue
		}
		if !section.EnumerationComplete {
			t.Fatal(section.Name)
		}
	}
	if len(gaps) != 4 {
		t.Fatal("unread other nodes/external admission disappeared")
	}
}
func TestDBWriterCensusMongoEmptyNoAllSessionPermission(t *testing.T) {
	_, sections, _ := dbCensusMongoSnapshot(context.Background(), func(ctx context.Context, db string, q bson.D) (bson.M, error) {
		if q[0].Key == "connectionStatus" {
			return bson.M{"authInfo": bson.M{"authenticatedUserPrivileges": bson.A{}}}, nil
		}
		return censusMongoRunFixture(ctx, db, q)
	}, func(context.Context, string, string, mongo.Pipeline) ([]bson.M, error) { return []bson.M{}, nil })
	for _, s := range sections {
		if strings.Contains(s.Name, "sessions") || s.Name == "mongodb_connections_and_idle_operations" {
			if s.EnumerationComplete || s.ErrorCategory == "none" {
				t.Fatal("empty filtered query became full coverage")
			}
		}
	}
}

func TestDBWriterCensusMongoFixedCommandErrorCategories(t *testing.T) {
	for _, code := range []int32{2, 9, 13, 18, 20, 26, 50, 59, 72, 115, 40324} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			command := mongo.CommandError{Code: code, Name: "private-error-name", Message: "private-error-body", Raw: bson.Raw("private-error-raw")}
			for _, err := range []error{command, &command, fmt.Errorf("private-wrapper: %w", command), fmt.Errorf("private-wrapper: %w", &command)} {
				want := fmt.Sprintf("db_census_mongo_command_code_%d", code)
				if got := dbCensusMongoErrorCategory(err); got != want {
					t.Fatalf("category = %q, want %q", got, want)
				}
			}
		})
	}
}

func TestDBWriterCensusMongoFixedInternalErrorCategories(t *testing.T) {
	for _, literal := range []string{
		"db_census_borrowed_mongo_missing",
		"db_census_command_byte_budget_exceeded",
		"db_census_row_budget_exceeded",
		"db_census_byte_budget_exceeded",
		"db_census_total_byte_budget_exceeded",
		"db_census_cursor_schema_rejected",
		"db_census_mongo_array_rejected",
		"db_census_mongo_record_rejected",
		"db_census_mongo_projection_rejected",
		"db_census_mongo_privilege_schema_rejected",
		"db_census_mongo_all_users_privilege_unproven",
		"db_census_user_duplicate_identity",
		"db_census_user_expansion_membership_changed",
		"db_census_user_expansion_identity_changed",
		"db_census_user_expansion_byte_budget_exceeded",
		"db_census_user_private_body_returned",
		"db_census_user_identity_schema_rejected",
		"db_census_user_role_schema_rejected",
		"db_census_user_expansion_schema_rejected",
	} {
		t.Run(literal, func(t *testing.T) {
			for _, err := range []error{errors.New(literal), fmt.Errorf("private-wrapper: %w", errors.New(literal))} {
				if got := dbCensusMongoErrorCategory(err); got != literal {
					t.Fatalf("category = %q, want %q", got, literal)
				}
			}
		})
	}
	var nilCommand *mongo.CommandError
	for _, err := range []error{
		nilCommand,
		fmt.Errorf("private-wrapper: %w", nilCommand),
		mongo.CommandError{Code: 99999, Name: "private-error-name", Message: "db_census_mongo_array_rejected", Wrapped: errors.New("db_census_mongo_array_rejected")},
		errors.New("private-error-body"),
		errors.New("db_census_mongo_array_rejected: private-error-body"),
		errors.New("db_census_startup_authentication_configuration_unread"),
		context.Canceled,
		context.DeadlineExceeded,
	} {
		if got := dbCensusMongoErrorCategory(err); got != "db_census_mongo_query_failed_or_bounded" {
			t.Fatalf("unknown/startup category = %q", got)
		}
	}
	if got := dbCensusMongoErrorCategory(nil); got != "none" {
		t.Fatalf("nil category = %q", got)
	}
}

func TestDBWriterCensusMongoFailureCategorySurvivesSecondRead(t *testing.T) {
	for _, tc := range []struct{ name, category string }{
		{"users", "db_census_mongo_command_code_9"},
		{"connections_and_idle_operations", "db_census_mongo_command_code_40324"},
		{"authentication_parameters", "db_census_mongo_projection_rejected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, first, _ := dbCensusMongoSnapshot(context.Background(), censusMongoRunFixture, censusMongoAggregateFixture)
			rows, second, gaps := dbCensusMongoSnapshot(context.Background(), func(ctx context.Context, db string, q bson.D) (bson.M, error) {
				if tc.name == "users" && q[0].Key == "usersInfo" {
					return nil, fmt.Errorf("private-wrapper: %w", mongo.CommandError{Code: 9, Message: "private-error-body"})
				}
				if tc.name == "authentication_parameters" && q[0].Key == "getParameter" {
					return bson.M{}, nil
				}
				return censusMongoRunFixture(ctx, db, q)
			}, func(ctx context.Context, db, col string, p mongo.Pipeline) ([]bson.M, error) {
				if tc.name == "connections_and_idle_operations" && p[0][0].Key == "$currentOp" {
					return nil, &mongo.CommandError{Code: 40324, Message: "private-error-body"}
				}
				return censusMongoAggregateFixture(ctx, db, col, p)
			})
			if _, ok := rows[tc.name]; ok {
				t.Fatal("failed section retained rows")
			}
			if !strings.Contains(strings.Join(gaps, ","), "mongodb_"+tc.name+"_unread") {
				t.Fatal("failed section lost unread gap")
			}
			found := false
			for _, section := range dbCensusFinishSections(first, second) {
				if section.Name == "mongodb_"+tc.name {
					found = true
					if section.EnumerationComplete || section.RecheckEqual || section.ErrorCategory != tc.category {
						t.Fatal("second read error lost its fixed category or claimed complete")
					}
				}
			}
			if !found {
				t.Fatal("failed section omitted")
			}
			encoded, err := json.Marshal(struct {
				Rows     map[string][]bson.M
				Sections []Section
				Gaps     []string
			}{rows, second, gaps})
			if err != nil || strings.Contains(string(encoded), "private-error") {
				t.Fatal("error body entered snapshot")
			}
		})
	}
}
