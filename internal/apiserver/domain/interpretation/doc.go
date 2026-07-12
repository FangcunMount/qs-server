// Package interpretation owns the durable terminal events emitted by report
// generation. The Generation, Run and immutable Report models live in their
// explicit subpackages; rendering consumes Evaluation outcomes as read-only
// facts and never advances Evaluation state.
package interpretation
