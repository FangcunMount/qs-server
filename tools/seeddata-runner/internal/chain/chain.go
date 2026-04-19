package chain

import (
	"context"
	"fmt"
	"strings"
)

type Decision struct {
	Continue   bool
	StopReason string
}

func Next() Decision {
	return Decision{Continue: true}
}

func Stop(reason string) Decision {
	return Decision{
		Continue:   false,
		StopReason: strings.TrimSpace(reason),
	}
}

type Handler[T any] interface {
	Name() string
	Handle(context.Context, *T) (Decision, error)
}

type FuncHandler[T any] struct {
	HandlerName string
	HandlerFunc func(context.Context, *T) (Decision, error)
}

func (h FuncHandler[T]) Name() string {
	return strings.TrimSpace(h.HandlerName)
}

func (h FuncHandler[T]) Handle(ctx context.Context, state *T) (Decision, error) {
	if h.HandlerFunc == nil {
		return Decision{}, fmt.Errorf("handler func is nil")
	}
	return h.HandlerFunc(ctx, state)
}

func Run[T any](ctx context.Context, label string, state *T, handlers ...Handler[T]) (Decision, error) {
	chainLabel := strings.TrimSpace(label)
	if chainLabel == "" {
		chainLabel = "chain"
	}

	decision := Next()
	for _, handler := range handlers {
		if ctx.Err() != nil {
			return Decision{}, ctx.Err()
		}
		if handler == nil {
			return Decision{}, fmt.Errorf("%s handler is nil", chainLabel)
		}
		handlerName := strings.TrimSpace(handler.Name())
		if handlerName == "" {
			handlerName = "unnamed_handler"
		}

		nextDecision, err := handler.Handle(ctx, state)
		if err != nil {
			return nextDecision, fmt.Errorf("%s handler %s: %w", chainLabel, handlerName, err)
		}
		decision = nextDecision
		if !decision.Continue {
			return decision, nil
		}
	}
	return decision, nil
}
