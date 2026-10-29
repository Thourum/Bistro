package handler

import (
	"context"
)

const githubProvider = "github"

type currentUserReader interface {
	Get(context.Context, string) (string, bool, error)
}
type broadcaster interface{ Broadcast(interface{}) }
type pushMessage struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}
