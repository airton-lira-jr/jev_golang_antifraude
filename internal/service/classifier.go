package service

import (
	"context"

	"fraud-detector-jev/internal/domain"
)

// Classifier manda um state ao modelo e devolve a decisão. O jev.Client
// implementa esta interface.
type Classifier interface {
	Classify(ctx context.Context, state string) (domain.Decision, error)
}
