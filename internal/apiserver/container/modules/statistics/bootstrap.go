package statistics

// Bootstrap is the stable composition seam for the statistics module.
func Bootstrap(in Deps) (*Module, error) { return New(in) }
