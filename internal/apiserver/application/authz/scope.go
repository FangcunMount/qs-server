package authz

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"math"
	"sort"
)

// DataScope is the IAM range paired with a permission, not clinician/guardian relations.
type DataScope struct {
	OrgID    int64
	Kind     string
	StoreIDs []uint64
}

// StoreRange is applied inside an already company-filtered QS query.
// AllStores never removes the company predicate or includes unassigned objects.
type StoreRange struct {
	AllStores bool
	StoreIDs  []uint64
}

func (r StoreRange) Contains(storeID *uint64) bool {
	if storeID == nil || *storeID == 0 {
		return false
	}
	if r.AllStores {
		return true
	}
	i := sort.Search(len(r.StoreIDs), func(i int) bool { return r.StoreIDs[i] >= *storeID })
	return i < len(r.StoreIDs) && r.StoreIDs[i] == *storeID
}

// ResolveStoreRange matches the action first and unions only its paired ranges.
// It cannot replace QS object ownership checks or independent self-service rules.
func (s *Snapshot) ResolveStoreRange(orgID int64, resource, action string) (StoreRange, error) {
	denied := func() (StoreRange, error) {
		return StoreRange{}, errors.WithCode(code.ErrPermissionDenied, "valid company data scope required")
	}
	if s == nil || s.ScopeContractVersion != 1 || s.AuthzVersion <= 0 || orgID <= 0 || resource == "" || action == "" {
		return denied()
	}
	ids := map[uint64]bool{}
	all := false
	for _, permission := range s.Permissions {
		if permission.Mode != AuthorizationModeUnconditional || !resourceCovers(permission.Resource, resource) || !actionCovers(permission.Action, action) {
			continue
		}
		for _, value := range permission.Scopes {
			if value.OrgID <= 0 {
				return denied()
			}
			switch value.Kind {
			case "all_stores":
				if len(value.StoreIDs) != 0 {
					return denied()
				}
			case "stores":
				if len(value.StoreIDs) == 0 {
					return denied()
				}
				for _, id := range value.StoreIDs {
					if id == 0 || id > math.MaxInt64 {
						return denied()
					}
				}
			default:
				return denied()
			}
			if value.OrgID != orgID {
				continue
			}
			if value.Kind == "all_stores" {
				all = true
			}
			for _, id := range value.StoreIDs {
				ids[id] = true
			}
		}
	}
	if !all && len(ids) == 0 {
		return denied()
	}
	if all {
		return StoreRange{AllStores: true}, nil
	}
	result := StoreRange{StoreIDs: make([]uint64, 0, len(ids))}
	for id := range ids {
		result.StoreIDs = append(result.StoreIDs, id)
	}
	sort.Slice(result.StoreIDs, func(i, j int) bool { return result.StoreIDs[i] < result.StoreIDs[j] })
	return result, nil
}
