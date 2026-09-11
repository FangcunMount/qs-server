package response

import "time"

type StoreResponse struct {
	ID             uint64    `swaggertype:"string" json:"id,string"`
	OrgID          int64     `swaggertype:"string" json:"org_id,string"`
	Code           string    `json:"code"`
	Name           string    `json:"name"`
	Address        string    `json:"address"`
	IsActive       bool      `json:"is_active"`
	Version        uint32    `json:"version"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	CreatedBy      int64     `swaggertype:"string" json:"created_by,string"`
	UpdatedBy      int64     `swaggertype:"string" json:"updated_by,string"`
	ClinicianCount int64     `json:"clinician_count"`
}
type ClinicianStoreHistoryResponse struct {
	ID               uint64    `swaggertype:"string" json:"id,string"`
	OrgID            int64     `swaggertype:"string" json:"org_id,string"`
	ClinicianID      uint64    `swaggertype:"string" json:"clinician_id,string"`
	FromStoreID      *uint64   `swaggertype:"string" json:"from_store_id,string"`
	ToStoreID        uint64    `swaggertype:"string" json:"to_store_id,string"`
	Kind             string    `json:"kind"`
	ActorID          int64     `swaggertype:"string" json:"actor_id,string"`
	CreatedAt        time.Time `json:"created_at"`
	Reason           string    `json:"reason"`
	RequestID        string    `json:"request_id"`
	InvalidatedCount int64     `json:"invalidated_count"`
	Version          uint32    `json:"version"`
}

// StoreListResponse 门店分页响应。
type StoreListResponse struct {
	Items []*StoreResponse `json:"items"`
	Total int64            `json:"total"`
}
type StoreProgressResponse struct {
	Total              int64 `json:"total"`
	Configured         int64 `json:"configured"`
	Unconfigured       int64 `json:"unconfigured"`
	ActiveTotal        int64 `json:"active_total"`
	ActiveConfigured   int64 `json:"active_configured"`
	ActiveUnconfigured int64 `json:"active_unconfigured"`
}
type ClinicianStoreAssignmentResponse struct {
	Clinician        *ClinicianResponse            `json:"clinician"`
	Change           ClinicianStoreHistoryResponse `json:"change"`
	InvalidatedCount int64                         `json:"invalidated_count"`
}
type ClinicianStoreHistoryListResponse struct {
	Items []ClinicianStoreHistoryResponse `json:"items"`
}
