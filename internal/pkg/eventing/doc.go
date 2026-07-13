// Package eventing is the shared qs-server event engineering entry point.
//
// Its subpackages separate the product event catalog, transport runtime,
// bounded observability labels, and cross-process wire contracts. Process-level
// lifecycle and infrastructure ownership remain in apiserver and worker
// EventSubsystems rather than this shared package.
package eventing
