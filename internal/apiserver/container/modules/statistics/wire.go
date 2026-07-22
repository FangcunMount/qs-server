package statistics

// Wire builds the canonical statistics module.
func Wire(in Deps) (*Module, error) { return Bootstrap(in) }
