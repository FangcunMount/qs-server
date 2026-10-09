package compatibilityretirementdbcensus

import (
	"context"
	"encoding/json"
	"errors"

	"go.mongodb.org/mongo-driver/bson"
)

const dbCensusUserBatch = 32

// All-user usersInfo forbids the two expansion flags. First read the full
// bounded list, then expand only those exact user/db identities in bounded
// batches. Mongo <5.2 rejects showCustomData:false; do not retry without it.
func dbCensusMongoUsers(ctx context.Context, run dbCensusMongoCommand) ([]bson.M, []bson.M, error) {
	reply, err := run(ctx, "admin", bson.D{{Key: "usersInfo", Value: bson.D{{Key: "forAllDBs", Value: true}}}, {Key: "showCredentials", Value: false}, {Key: "showCustomData", Value: false}})
	if err != nil {
		return nil, nil, err
	}
	identities, err := dbCensusArray(reply["users"])
	if err != nil {
		return nil, nil, err
	}
	identities, err = dbCensusUserRows(identities, false)
	if err != nil {
		return nil, nil, err
	}
	expected := map[string]bson.M{}
	for _, row := range identities {
		key := dbCensusUserKey(row)
		if _, duplicate := expected[key]; duplicate {
			return nil, nil, errors.New("db_census_user_duplicate_identity")
		}
		expected[key] = row
	}
	all := make([]bson.M, 0, len(identities))
	bytes := 0
	for start := 0; start < len(identities); start += dbCensusUserBatch {
		if ctx.Err() != nil {
			return identities, nil, ctx.Err()
		}
		end := start + dbCensusUserBatch
		if end > len(identities) {
			end = len(identities)
		}
		requested := bson.A{}
		batch := map[string]bool{}
		for _, row := range identities[start:end] {
			requested = append(requested, bson.D{{Key: "user", Value: row["user"]}, {Key: "db", Value: row["db"]}})
			batch[dbCensusUserKey(row)] = true
		}
		reply, err = run(ctx, "admin", bson.D{{Key: "usersInfo", Value: requested}, {Key: "showCredentials", Value: false}, {Key: "showCustomData", Value: false}, {Key: "showPrivileges", Value: true}, {Key: "showAuthenticationRestrictions", Value: true}})
		if err != nil {
			return identities, nil, err
		}
		rows, readErr := dbCensusArray(reply["users"])
		if readErr == nil {
			rows, readErr = dbCensusUserRows(rows, true)
		}
		if readErr != nil {
			return identities, nil, readErr
		}
		if len(rows) != len(batch) {
			return identities, nil, errors.New("db_census_user_expansion_membership_changed")
		}
		for _, row := range rows {
			key := dbCensusUserKey(row)
			if !batch[key] {
				return identities, nil, errors.New("db_census_user_expansion_membership_changed")
			}
			identity, projectErr := dbCensusProject([]bson.M{row}, dbCensusUserIdentityFields, []string{"user", "db", "roles"})
			if projectErr != nil || checksum(identity[0]) != checksum(expected[key]) {
				return identities, nil, errors.New("db_census_user_expansion_identity_changed")
			}
			delete(batch, key)
		}
		raw, marshalErr := bson.Marshal(bson.M{"users": rows})
		bytes += len(raw)
		if marshalErr != nil || bytes > dbCensusByteLimit {
			return identities, nil, errors.New("db_census_user_expansion_byte_budget_exceeded")
		}
		all = append(all, rows...)
	}
	if ctx.Err() != nil {
		return identities, nil, ctx.Err()
	}
	all, err = dbCensusProject(all, dbCensusUserExpandedFields, nil)
	return identities, all, err
}

var dbCensusUserIdentityFields = []string{"_id", "userId", "user", "db", "roles", "mechanisms"}
var dbCensusUserExpandedFields = append(append([]string{}, dbCensusUserIdentityFields...), "inheritedRoles", "inheritedPrivileges", "authenticationRestrictions", "inheritedAuthenticationRestrictions")

func dbCensusUserKey(row bson.M) string {
	key, _ := json.Marshal([2]any{row["user"], row["db"]})
	return string(key)
}

func dbCensusUserRows(rows []bson.M, expanded bool) ([]bson.M, error) {
	for _, row := range rows {
		for _, forbidden := range []string{"credentials", "customData"} {
			if _, found := row[forbidden]; found {
				return nil, errors.New("db_census_user_private_body_returned")
			}
		}
		for _, name := range []string{"user", "db"} {
			if value, ok := row[name].(string); !ok || value == "" {
				return nil, errors.New("db_census_user_identity_schema_rejected")
			}
		}
		roles, err := dbCensusArray(row["roles"])
		if err != nil {
			return nil, err
		}
		for _, role := range roles {
			for _, name := range []string{"role", "db"} {
				if value, ok := role[name].(string); !ok || value == "" {
					return nil, errors.New("db_census_user_role_schema_rejected")
				}
			}
		}
		if expanded {
			for _, name := range []string{"inheritedRoles", "inheritedPrivileges", "authenticationRestrictions", "inheritedAuthenticationRestrictions"} {
				if _, ok := row[name].(bson.A); !ok {
					if _, ok := row[name].([]any); !ok {
						return nil, errors.New("db_census_user_expansion_schema_rejected")
					}
				}
			}
		}
	}
	fields := dbCensusUserIdentityFields
	if expanded {
		fields = dbCensusUserExpandedFields
	}
	return dbCensusProject(rows, fields, []string{"user", "db", "roles"})
}
