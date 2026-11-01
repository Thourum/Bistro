package github

import (
	"errors"
	"net/http"

	gh "github.com/google/go-github/v84/github"
	"github.com/itterno/bistro/internal/provider"
)

// mapAPIError adds provider classification without changing the client's message or cause.
func mapAPIError(err error) error {
	if err == nil {
		return nil
	}
	var pe *provider.ProviderError
	if errors.As(err, &pe) {
		return err
	}

	var rate *gh.RateLimitError
	var abuse *gh.AbuseRateLimitError
	var response *gh.ErrorResponse
	switch {
	case errors.As(err, &rate):
		pe = provider.NewRateLimitError(err.Error())
		if rate.Response != nil {
			pe.Status = rate.Response.StatusCode
		}
	case errors.As(err, &abuse):
		pe = provider.NewRateLimitError(err.Error())
		if abuse.Response != nil {
			pe.Status = abuse.Response.StatusCode
		}
	case errors.As(err, &response) && response.Response != nil:
		status := response.Response.StatusCode
		if status != http.StatusUnauthorized && status != http.StatusNotFound && status != http.StatusTooManyRequests {
			return err
		}
		pe = provider.WrapHTTPError(status, "%s", err.Error()).(*provider.ProviderError)
	default:
		return err
	}
	pe.Cause = err
	return pe
}
