// Package registry is the public wiring surface for evaluation runtime mechanisms.
//
// Production assembly (container, compose, preview) must depend on this package.
// Mechanism implementations live in registry/mechanisms and are imported only by registry and runtime.
package registry
