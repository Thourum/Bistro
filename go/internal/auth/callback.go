package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/itterno/bistro/internal/logging"
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

// sendResult delivers a result without blocking if the channel is already full.
func (cs *CallbackServer) sendResult(result CallbackResult) {
	select {
	case cs.resultCh <- result:
	default:
	}
}

func (cs *CallbackServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	log := logging.FromContext(r.Context()).With(
		"remote_addr", r.RemoteAddr,
		"auth_callback", true,
	)

	incomingState := r.URL.Query().Get("state")
	if incomingState != cs.state {
		log.Warn("oauth callback state mismatch",
			"incoming_state_len", len(incomingState),
			"expected_state_len", len(cs.state),
		)
		http.Error(w, "Invalid state parameter", http.StatusForbidden)
		cs.sendResult(CallbackResult{Err: errors.New("state mismatch")})
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		log.Warn("oauth callback missing code",
			"query_params", r.URL.Query().Encode(),
			"error", r.URL.Query().Get("error"),
		)
		http.Error(w, "Missing code parameter", http.StatusBadRequest)
		cs.sendResult(CallbackResult{Err: errors.New("missing code")})
		return
	}

	log.Debug("oauth callback: exchanging code for token")
	token, err := ExchangeToken(r.Context(), cs.cfg, code, cs.redirectURI)
	if err != nil {
		log.Error("oauth token exchange failed", "error", err)
		http.Error(w, "Token exchange failed", http.StatusInternalServerError)
		cs.sendResult(CallbackResult{Err: fmt.Errorf("token exchange: %w", err)})
		return
	}

	log.Info("oauth callback success")

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, `<!DOCTYPE html><html><body style="font-family:system-ui;text-align:center;padding:60px">
		<h2>Authorization successful!</h2><p>You can close this tab and return to Bistro.</p>
	</body></html>`)

	cs.sendResult(CallbackResult{Token: token})
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

// exchangeRequestBody is the JSON body sent to the get-bistro.com exchange
// endpoint when no client secret is configured.
type exchangeRequestBody struct {
	Code        string `json:"code"`
	RedirectURI string `json:"redirect_uri"`
}

// buildTokenRequest builds the token exchange request. With a client secret
// (dev builds) it posts GitHub's form encoding directly to cfg.TokenURL. Without
// one (packaged builds) it posts JSON {code, redirect_uri} to cfg.TokenURL, which
// is the get-bistro.com endpoint that holds the secret. The JSON path sends no
// client_id or secret.
func buildTokenRequest(ctx context.Context, cfg ProviderConfig, code, redirectURI string) (*http.Request, error) {
	if cfg.ClientSecret == "" {
		body, err := json.Marshal(exchangeRequestBody{Code: code, RedirectURI: redirectURI})
		if err != nil {
			return nil, fmt.Errorf("encoding token exchange body: %w", err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenURL, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		return req, nil
	}

	params := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	return req, nil
}

// ExchangeToken exchanges an authorization code for an access token.
// It validates that the token_type is "bearer" and that the returned scopes
// contain all the requested scopes. See buildTokenRequest for the two request
// shapes: the direct GitHub form post when a client secret is set, and the JSON
// post to the get-bistro.com exchange when it is not.
func ExchangeToken(ctx context.Context, cfg ProviderConfig, code, redirectURI string) (string, error) {
	log := logging.FromContext(ctx)

	req, err := buildTokenRequest(ctx, cfg, code, redirectURI)
	if err != nil {
		return "", fmt.Errorf("creating token request: %w", err)
	}

	log.Debug("oauth token exchange request", "token_url", cfg.TokenURL, "via_exchange", cfg.ClientSecret == "")
	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	dur := time.Since(start).Milliseconds()
	if err != nil {
		return "", fmt.Errorf("token request to %s failed after %dms: %w", cfg.TokenURL, dur, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		log.Warn("oauth token endpoint non-2xx",
			"status", resp.StatusCode,
			"duration_ms", dur,
		)
	} else {
		log.Debug("oauth token endpoint ok",
			"status", resp.StatusCode,
			"duration_ms", dur,
		)
	}

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", fmt.Errorf("decoding token response: %w", err)
	}

	if tr.Error != "" {
		log.Warn("oauth token response contained error",
			"error", tr.Error,
			"description", tr.ErrorDesc,
		)
		return "", fmt.Errorf("%s: %s", tr.Error, tr.ErrorDesc)
	}

	if strings.ToLower(tr.TokenType) != "bearer" {
		log.Warn("oauth token_type not bearer", "token_type", tr.TokenType)
		return "", fmt.Errorf("unexpected token_type: %s", tr.TokenType)
	}

	if err := validateScopes(cfg.Scopes, tr.Scope); err != nil {
		return "", fmt.Errorf("validate oauth scopes: %w", err)
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
