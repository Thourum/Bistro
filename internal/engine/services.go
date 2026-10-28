package engine

import "github.com/itterno/bistro/internal/services"

// Services builds the service set from the engine's prepared dependencies.
// Valid only after the stores are prepared (Run today, Start from Task 9).
func (a *App) Services() *services.Set { return &services.Set{} }
