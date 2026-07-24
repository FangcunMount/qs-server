// Package serviceidentity owns the canonical identities used between qs-server workloads.
package serviceidentity

const (
	// CollectionServerServiceID is the transport-neutral collection-server workload identity.
	CollectionServerServiceID = "qs-collection-server"
	// CollectionServerCertificateCommonName is the canonical mTLS certificate identity and ACL key.
	CollectionServerCertificateCommonName = CollectionServerServiceID + ".svc"
)
