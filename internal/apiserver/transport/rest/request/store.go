package request

type CreateStoreRequest struct {
	Code    string `json:"code" binding:"required"`
	Name    string `json:"name" binding:"required"`
	Address string `json:"address"`
}
type UpdateStoreRequest struct {
	Name            string `json:"name" binding:"required"`
	Address         string `json:"address"`
	ExpectedVersion uint32 `json:"expected_version" binding:"required"`
}
type StoreStatusRequest struct {
	ExpectedVersion uint32 `json:"expected_version" binding:"required"`
}
type AssignClinicianStoreRequest struct {
	StoreID         uint64 `swaggertype:"string" json:"store_id,string" binding:"required"`
	ExpectedVersion uint32 `json:"expected_version" binding:"required"`
	Reason          string `json:"reason" binding:"required"`
	RequestID       string `json:"request_id" binding:"required"`
}

type AssignTesteeStoreRequest struct {
	StoreID         uint64 `swaggertype:"string" json:"store_id,string" binding:"required"`
	ExpectedVersion uint32 `json:"expected_version" binding:"required"`
	Reason          string `json:"reason" binding:"required"`
	RequestID       string `json:"request_id" binding:"required"`
}
