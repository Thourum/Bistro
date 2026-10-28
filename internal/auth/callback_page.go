package auth

import (
	"bytes"
	_ "embed" // Embed the self-contained OAuth callback page.
	"html/template"
	"net/http"
	"runtime"
	"time"

	"github.com/itterno/bistro/internal/logging"
)

//go:embed callback.html
var callbackHTML string

var callbackTemplate = template.Must(template.New("callback").Parse(callbackHTML))

type callbackPage struct {
	Success                                bool
	Code, Sentence, Error, Description, At string
	ExchangeStep, Store                    string
}

func keyringName() string {
	names := map[string]string{
		"darwin":  "macOS Keychain",
		"windows": "Windows Credential Manager",
		"linux":   "the Secret Service keyring on Linux",
	}
	if name, ok := names[runtime.GOOS]; ok {
		return name
	}
	return "your operating system's keyring"
}

func successPage(cfg ProviderConfig) callbackPage {
	step := "Bistro's sign-in endpoint swapped it for a token and passed it straight back. It stored nothing."
	if cfg.ClientSecret != "" {
		step = "Bistro swapped it for a token on this machine."
	}
	return callbackPage{Success: true, ExchangeStep: step, Store: keyringName()}
}

func failurePage(page callbackPage) callbackPage {
	return callbackPage{Code: page.Code, Sentence: page.Sentence, Error: page.Error,
		Description: page.Description, At: time.Now().UTC().Format(time.RFC3339)}
}

func writeCallbackPage(w http.ResponseWriter, status int, page callbackPage) {
	var body bytes.Buffer
	if err := callbackTemplate.Execute(&body, page); err != nil {
		logging.FromContext(nil).Error("callback page render failed", "error", err)
		http.Error(w, "Unable to display sign-in result. Return to Bistro.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if _, err := w.Write(body.Bytes()); err != nil {
		logging.FromContext(nil).Warn("callback page write failed", "error", err)
	}
}
