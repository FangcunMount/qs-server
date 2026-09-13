package aiexplanation

// Client is the current participant workflow transport, without legacy generation operations.
type Client interface {
	WorkflowClient
	WorkflowReader
	WorkflowSourceReader
}
