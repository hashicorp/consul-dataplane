package otlphttp

import (
	"net/http"
)

// MiddlewareOption is a function that modifies an HTTP request.
type MiddlewareOption = func(req *http.Request) error

// roundTripperWithMiddleware takes a plain Roundtripper and an array of MiddlewareOptions to apply to the Roundtripper's request.
type roundTripperWithMiddleware struct {
	OriginalRoundTripper http.RoundTripper
	MiddlewareOptions    []MiddlewareOption
}

func WithRequestHeaders(headers map[string]string) MiddlewareOption {
	return func(req *http.Request) error {
		for key, value := range headers {
			req.Header.Set(key, value)
		}
		return nil
	}
}

// RoundTrip attaches MiddlewareOption modifications to the request before sending along.
func (rt *roundTripperWithMiddleware) RoundTrip(req *http.Request) (*http.Response, error) {

	for _, mw := range rt.MiddlewareOptions {
		if err := mw(req); err != nil {
			// Failure to apply middleware should not fail the request
			continue
		}
	}

	return rt.OriginalRoundTripper.RoundTrip(req)
}
