// Package securityplane defines read-only security control-plane vocabulary.
//
// The package intentionally owns only transport-agnostic model types. It does
// not verify JWTs, load IAM snapshots, evaluate business permissions, or depend
// on Gin/gRPC/IAM SDK packages. Runtime adapters keep their existing behavior
// and can project their state into these models for tests, docs, and future
// seam work.
package securityplane
