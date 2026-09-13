package aiexplanation

import (
	"errors"

	aiport "github.com/FangcunMount/qs-server/internal/collection-server/port/aiexplanation"
)

var (
	ErrInvalidRequest = errors.New("invalid AI explanation request")
	ErrUnavailable    = errors.New("AI explanation service unavailable")
)

type Service struct{ client aiport.Client }

func NewService(client aiport.Client) *Service { return &Service{client: client} }
