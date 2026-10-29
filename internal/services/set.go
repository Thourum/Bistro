package services

// Set holds one instance of every service; main.go registers each field with Wails.
type Set struct {
	Auth        *Auth
	PR          *PR
	Sync        *Sync
	Repos       *Repos
	ReviewQueue *ReviewQueue
}
