package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	CallbackPort    = 19284
	CallbackPath    = "/callback"
	CallbackTimeout = 5 * time.Minute
	RedirectURI     = "http://localhost:19284/callback"
)

// CallbackResult is sent on the result channel when the callback completes.
type CallbackResult struct {
	Token string
	Err   error
}

// CallbackServer is a temporary HTTP server that receives the OAuth callback.
type CallbackServer struct {
	cfg         ProviderConfig
	state       string
	redirectURI string
	resultCh    chan<- CallbackResult
	server      *http.Server
}

// NewCallbackServer creates a callback server. The state parameter is validated
// against the incoming request. Results are sent to resultCh.
func NewCallbackServer(cfg ProviderConfig, state, redirectURI string, resultCh chan<- CallbackResult) *CallbackServer {
	return &CallbackServer{
		cfg:         cfg,
		state:       state,
		redirectURI: redirectURI,
		resultCh:    resultCh,
	}
}

// handler returns the HTTP handler for the callback endpoint.
func (cs *CallbackServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(CallbackPath, cs.handleCallback)
	return mux
}

func (cs *CallbackServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	incomingState := r.URL.Query().Get("state")
	if incomingState != cs.state {
		http.Error(w, "Invalid state parameter", http.StatusForbidden)
		cs.resultCh <- CallbackResult{Err: fmt.Errorf("state mismatch")}
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Missing code parameter", http.StatusBadRequest)
		cs.resultCh <- CallbackResult{Err: fmt.Errorf("missing code")}
		return
	}

	token, err := ExchangeToken(r.Context(), cs.cfg, code, cs.redirectURI)
	if err != nil {
		http.Error(w, "Token exchange failed", http.StatusInternalServerError)
		cs.resultCh <- CallbackResult{Err: fmt.Errorf("token exchange: %w", err)}
		return
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, `<!DOCTYPE html><html><body style="font-family:system-ui;text-align:center;padding:60px">
		<h2>Authorization successful!</h2><p>You can close this tab and return to Bistro.</p>
	</body></html>`)

	cs.resultCh <- CallbackResult{Token: token}
}

// ListenAndServe starts the callback server. It blocks until the server is shut down.
func (cs *CallbackServer) ListenAndServe() error {
	addr := fmt.Sprintf("localhost:%d", CallbackPort)
	cs.server = &http.Server{
		Addr:    addr,
		Handler: cs.handler(),
	}
	return cs.server.ListenAndServe()
}

// Shutdown gracefully shuts down the callback server.
func (cs *CallbackServer) Shutdown(ctx context.Context) error {
	if cs.server == nil {
		return nil
	}
	return cs.server.Shutdown(ctx)
}

// tokenResponse represents the JSON body returned by the token endpoint.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
}

// ExchangeToken exchanges an authorization code for an access token.
// It validates that the token_type is "bearer" and that the returned scopes
// contain all the requested scopes.
func ExchangeToken(ctx context.Context, cfg ProviderConfig, code, redirectURI string) (string, error) {
	params := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenURL, strings.NewReader(params.Encode()))
	if err != nil {
		return "", fmt.Errorf("creating token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", fmt.Errorf("decoding token response: %w", err)
	}

	if tr.Error != "" {
		return "", fmt.Errorf("%s: %s", tr.Error, tr.ErrorDesc)
	}

	if strings.ToLower(tr.TokenType) != "bearer" {
		return "", fmt.Errorf("unexpected token_type: %s", tr.TokenType)
	}

	if err := validateScopes(cfg.Scopes, tr.Scope); err != nil {
		return "", err
	}

	return tr.AccessToken, nil
}

// validateScopes checks that all requested scopes are present in the returned
// scope string. GitHub returns scopes comma-separated.
func validateScopes(requested []string, returned string) error {
	// GitHub returns scopes as comma-separated (e.g. "repo,read:user")
	// Normalize: split on both comma and space.
	returnedSet := make(map[string]bool)
	for _, s := range strings.FieldsFunc(returned, func(r rune) bool {
		return r == ',' || r == ' '
	}) {
		returnedSet[strings.TrimSpace(s)] = true
	}

	for _, scope := range requested {
		if !returnedSet[scope] {
			return fmt.Errorf("scope_mismatch: requested %q but not in returned scopes %q", scope, returned)
		}
	}
	return nil
}
