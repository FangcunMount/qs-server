package code

// calculation errors.
const (
	// ErrOperandsEmpty - 400: Operands is empty.
	ErrOperandsEmpty int = iota + 110001
	// ErrOperandsOverside - 400: Operands is overside.
	ErrOperandsOverside
	// ErrInvalidCalculaterType - 400: Invalid calculater type.
	ErrInvalidCalculaterType
	// ErrCalculaterNotFound - 400: Calculater not found.
	ErrCalculaterNotFound
)
