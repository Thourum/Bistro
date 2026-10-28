package services

// Emitter publishes push events to the renderer; production wraps app.Event.Emit.
type Emitter interface{ Emit(name string, data any) }

// EmitterFunc adapts the production event function without coupling services to Wails.
type EmitterFunc func(name string, data any)

func (f EmitterFunc) Emit(name string, data any) { f(name, data) }

// NopEmitter drops events (engine default until main.go sets the real one).
type NopEmitter struct{}

func (NopEmitter) Emit(string, any) {}
