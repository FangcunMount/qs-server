package code

// calculation errors (113xxx).
const (
// ErrOperandsEmpty - 400: Operands is empty.
ErrOperandsEmpty int = iota + 113001

// ErrOperandsOverside - 400: Operands is overside.
ErrOperandsOverside

// ErrInvalidCalculaterType - 400: Invalid calculater type.
ErrInvalidCalculaterType

// ErrCalculaterNotFound - 400: Calculater not found.
ErrCalculaterNotFound
)

func init() {
	register(ErrOperandsEmpty, 400, "Operands is empty")
	register(ErrOperandsOverside, 400, "Operands is oversize")
	register(ErrInvalidCalculaterType, 400, "Invalid calculator type")
	register(ErrCalculaterNotFound, 404, "Calculator not found")
}
