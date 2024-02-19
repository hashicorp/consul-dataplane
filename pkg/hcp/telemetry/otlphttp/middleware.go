// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package otlphttp

import (
	"net/http"
)

// MiddlewareOption is a function that modifies an HTTP request.
type MiddlewareOption = func(req *http.Request)

// roundTripperWithMiddleware takes a plain Roundtripper and an array of MiddlewareOptions to apply to the Roundtripper's request.
type roundTripperWithMiddleware struct {
	OriginalRoundTripper http.RoundTripper
	MiddlewareOptions    []MiddlewareOption
}

func WithRequestHeaders(headers map[string]string) MiddlewareOption {
	return func(req *http.Request) {
		for key, value := range headers {
			req.Header.Set(key, value)
		}
	}
}

// RoundTrip attaches MiddlewareOption modifications to the request before sending along.
func (rt *roundTripperWithMiddleware) RoundTrip(req *http.Request) (*http.Response, error) {
	for _, mw := range rt.MiddlewareOptions {
		mw(req)
	}

	return rt.OriginalRoundTripper.RoundTrip(req)
}
