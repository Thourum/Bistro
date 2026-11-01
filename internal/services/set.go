package services

// Set holds one instance of every service; main.go registers each field with Wails.
type Set struct {
	Shell          *Shell
	Expertise      *Expertise
	Classification *Classification
	Editor         *Editor
	RepoLocation   *RepoLocation
	DevFolderScan  *DevFolderScan
	Comments       *Comments
	Dismiss        *Dismiss
	Threads        *Threads
	Drafts         *Drafts
	Diff           *Diff
	Patchsets      *Patchsets
	Triage         *Triage
	Interdiff      *Interdiff
	Viewed         *Viewed
	ReadState      *ReadState
	Auth           *Auth
	PR             *PR
	Sync           *Sync
	Repos          *Repos
	ReviewQueue    *ReviewQueue
}
