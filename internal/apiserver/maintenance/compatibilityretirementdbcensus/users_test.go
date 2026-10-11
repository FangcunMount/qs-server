package compatibilityretirementdbcensus

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func censusUserFixture(count int, mutate func(int, bson.A) (bson.A, error), calls *int) dbCensusMongoCommand {
	identities := bson.A{}
	for i := 0; i < count; i++ {
		identities = append(identities, bson.M{"user": fmt.Sprintf("user-%d", i), "db": "actual-db", "roles": bson.A{}, "userId": primitive.Binary{Subtype: 4, Data: []byte{byte(i)}}, "mechanisms": bson.A{"SCRAM-SHA-256"}})
	}
	return func(ctx context.Context, db string, command bson.D) (bson.M, error) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		flags := map[string]any{}
		for _, item := range command {
			flags[item.Key] = item.Value
		}
		if db != "admin" || flags["showCredentials"] != false || flags["showCustomData"] != false {
			return nil, errors.New("unsafe flags")
		}
		if all, ok := command[0].Value.(bson.D); ok {
			if len(all) != 1 || all[0].Key != "forAllDBs" || all[0].Value != true || len(command) != 3 {
				return nil, errors.New("invalid full enumeration")
			}
			if mutate != nil {
				rows, err := mutate(-1, identities)
				return bson.M{"users": rows}, err
			}
			return bson.M{"users": identities}, nil
		}
		requested, ok := command[0].Value.(bson.A)
		if !ok || len(requested) == 0 || len(requested) > dbCensusUserBatch || flags["showPrivileges"] != true || flags["showAuthenticationRestrictions"] != true || len(command) != 5 {
			return nil, errors.New("invalid expansion")
		}
		index := *calls
		*calls++
		result := bson.A{}
		for _, spec := range requested {
			spec := spec.(bson.D)
			found := false
			for _, value := range identities {
				identity := value.(bson.M)
				if spec[0].Key != "user" || spec[1].Key != "db" {
					return nil, errors.New("identity specification rejected")
				}
				if identity["user"] == spec[0].Value && identity["db"] == spec[1].Value {
					row := bson.M{}
					for key, value := range identity {
						row[key] = value
					}
					row["inheritedRoles"] = bson.A{}
					row["inheritedPrivileges"] = bson.A{}
					row["authenticationRestrictions"] = bson.A{}
					row["inheritedAuthenticationRestrictions"] = bson.A{}
					result = append(result, row)
					found = true
				}
			}
			if !found {
				return nil, errors.New("guessed identity")
			}
		}
		if mutate != nil {
			result, err := mutate(index, result)
			return bson.M{"users": result}, err
		}
		return bson.M{"users": result}, nil
	}
}

func TestDBWriterCensusUsersLegalEnumerationAndBoundedExpansion(t *testing.T) {
	for _, count := range []int{0, 1, 32, 33, 65} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			calls := 0
			identities, expanded, err := dbCensusMongoUsers(context.Background(), censusUserFixture(count, nil, &calls))
			if err != nil || len(identities) != count || len(expanded) != count || calls != (count+31)/32 {
				t.Fatalf("enumeration/expansion rejected: count=%d calls=%d error=%v", count, calls, err)
			}
		})
	}
}

func TestDBWriterCensusUsersRoleAndPrivilegeOrderDoesNotChangeIdentityOrSnapshot(t *testing.T) {
	var first []bson.M
	for round := 0; round < 2; round++ {
		calls := 0
		_, expanded, err := dbCensusMongoUsers(context.Background(), censusUserFixture(1, func(index int, rows bson.A) (bson.A, error) {
			row := rows[0].(bson.M)
			read := bson.M{"role": "read", "db": "actual-db", "preserved": "value"}
			write := bson.M{"role": "readWrite", "db": "another-db"}
			row["roles"] = bson.A{read, write}
			if (index < 0) == (round == 0) {
				row["roles"] = bson.A{write, read}
			}
			if index >= 0 {
				row["inheritedRoles"] = bson.A{read, write}
				find := bson.M{"resource": bson.M{"db": "actual-db", "collection": "one"}, "actions": bson.A{"find", "insert"}}
				remove := bson.M{"resource": bson.M{"db": "actual-db", "collection": "two"}, "actions": bson.A{"remove", "update"}}
				row["inheritedPrivileges"] = bson.A{find, remove}
				if round == 1 {
					row["inheritedRoles"] = bson.A{write, read}
					find["actions"] = bson.A{"insert", "find"}
					remove["actions"] = bson.A{"update", "remove"}
					row["inheritedPrivileges"] = bson.A{remove, find}
				}
			}
			return rows, nil
		}, &calls))
		if err != nil || len(expanded) != 1 || calls != 1 {
			t.Fatalf("equivalent role order rejected: round=%d calls=%d error=%v", round, calls, err)
		}
		if round == 0 {
			first = expanded
		} else if checksum(first) != checksum(expanded) {
			t.Fatal("equivalent inherited role/action order changed the snapshot")
		}
	}
}

func TestDBWriterCensusUsersRoleNormalizationPreservesIdentityChanges(t *testing.T) {
	for _, change := range []string{"role", "role-db", "role-extra-field", "role-duplicate", "_id", "userId", "mechanisms"} {
		t.Run(change, func(t *testing.T) {
			calls := 0
			_, expanded, err := dbCensusMongoUsers(context.Background(), censusUserFixture(1, func(index int, rows bson.A) (bson.A, error) {
				row := rows[0].(bson.M)
				row["_id"] = "actual-db.user-0"
				role := bson.M{"role": "read", "db": "actual-db"}
				row["roles"] = bson.A{role}
				if index >= 0 {
					switch change {
					case "role":
						role["role"] = "readWrite"
					case "role-db":
						role["db"] = "another-db"
					case "role-extra-field":
						role["unexpected"] = true
					case "role-duplicate":
						row["roles"] = bson.A{role, role}
					case "_id":
						row["_id"] = "different-user-id"
					case "userId":
						row["userId"] = primitive.Binary{Subtype: 4, Data: []byte{255}}
					case "mechanisms":
						row["mechanisms"] = bson.A{"SCRAM-SHA-1"}
					}
				}
				return rows, nil
			}, &calls))
			if err == nil || err.Error() != "db_census_user_expansion_identity_changed" || expanded != nil {
				t.Fatalf("identity change accepted: %s error=%v", change, err)
			}
		})
	}
}

func TestDBWriterCensusUsersExpansionFailureNeverCompletes(t *testing.T) {
	for _, failure := range []string{"denied", "missing", "extra", "duplicate", "cross-db", "user-recreated", "roles-changed", "restriction-missing", "custom-data", "credentials", "byte-budget", "duplicate-enumeration", "unsupported-version"} {
		t.Run(failure, func(t *testing.T) {
			calls := 0
			_, rows, err := dbCensusMongoUsers(context.Background(), censusUserFixture(2, func(index int, rows bson.A) (bson.A, error) {
				if index < 0 {
					if failure == "unsupported-version" {
						return nil, errors.New("unknown showCustomData field")
					}
					if failure == "duplicate-enumeration" {
						return append(rows, rows[0]), nil
					}
					return rows, nil
				}
				row := rows[0].(bson.M)
				switch failure {
				case "denied":
					return nil, errors.New("viewUser denied")
				case "missing":
					rows = rows[:1]
				case "extra":
					rows = append(rows, row)
				case "duplicate":
					rows[1] = row
				case "cross-db":
					row["db"] = "another-db"
				case "user-recreated":
					row["userId"] = primitive.Binary{Subtype: 4, Data: []byte{255}}
				case "roles-changed":
					row["roles"] = bson.A{bson.M{"role": "changed", "db": "actual-db"}}
				case "restriction-missing":
					delete(row, "authenticationRestrictions")
				case "custom-data":
					row["customData"] = bson.M{"unexpected": "body"}
				case "credentials":
					row["credentials"] = bson.M{}
				case "byte-budget":
					row["inheritedPrivileges"] = bson.A{strings.Repeat("x", dbCensusByteLimit)}
				}
				return rows, nil
			}, &calls))
			if err == nil || rows != nil {
				t.Fatal("partial/ambiguous expansion claimed complete")
			}
		})
	}
}

func TestDBWriterCensusUsersSecondRoundMembershipAndCancellation(t *testing.T) {
	calls := 0
	_, first, err := dbCensusMongoUsers(context.Background(), censusUserFixture(1, nil, &calls))
	if err != nil {
		t.Fatal(err)
	}
	_, second, err := dbCensusMongoUsers(context.Background(), censusUserFixture(2, nil, &calls))
	if err != nil {
		t.Fatal(err)
	}
	rechecked := dbCensusFinishSections([]Section{{Name: "mongodb_users", EnumerationComplete: true, Items: len(first), SHA256: checksum(first)}}, []Section{{Name: "mongodb_users", EnumerationComplete: true, Items: len(second), SHA256: checksum(second)}})
	if rechecked[0].RecheckEqual {
		t.Fatal("changed user directory claimed equal")
	}
	calls = 0
	ctx, cancel := context.WithCancel(context.Background())
	_, rows, err := dbCensusMongoUsers(ctx, censusUserFixture(33, func(index int, rows bson.A) (bson.A, error) {
		if index == 0 {
			cancel()
		}
		return rows, nil
	}, &calls))
	if err == nil || rows != nil {
		t.Fatal("cancelled expansion completed")
	}
}
