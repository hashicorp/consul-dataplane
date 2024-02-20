// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package otlphttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/hcp-sdk-go/config"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"golang.org/x/oauth2"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
)

type fakeTokenSource struct {
	accessToken string
	err         error
}

func (f *fakeTokenSource) Token() (*oauth2.Token, error) {
	if f.err != nil {
		return nil, f.err
	}

	return &oauth2.Token{
		AccessToken: f.accessToken,
	}, nil
}

func Test_Client(t *testing.T) {
	t.Parallel()

	tokenErr := errors.New("boom: invalid token")

	for name, tc := range map[string]struct {
		modTokenSource     func(*fakeTokenSource)
		modConfig          func(*Config)
		httpResponseWriter func(w http.ResponseWriter)

		expectNewErr           string
		exportExportMetricsErr error
	}{
		"success": {},
		"invalid metrics endpoint": {
			modConfig: func(c *Config) {
				c.MetricsEndpoint = "http://example .com"
			},
			expectNewErr: "bad metrics endpoint url",
		},
		"invalid token source": {
			modTokenSource: func(c *fakeTokenSource) {
				c.err = tokenErr
			},
			exportExportMetricsErr: tokenErr,
		},
		"failed metrics export body": {
			httpResponseWriter: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "internal server error")
			},
			exportExportMetricsErr: errClientFailedToParseResponse,
		},
		"failed metrics export failed parse": {
			httpResponseWriter: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "internal server error")
			},
			exportExportMetricsErr: errClientFailedToParseResponse,
		},
		"failed metrics export stats response": {
			httpResponseWriter: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusInternalServerError)

				st := &status.Status{
					Code:    int32(codes.Internal),
					Message: "failed to parse response",
				}
				body, err := proto.Marshal(st)
				if err != nil {
					panic(err)
				}

				fmt.Fprint(w, string(body))
			},
			exportExportMetricsErr: errClientFailedToExportMetrics,
		},
	} {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := require.New(t)

			// Create a test server to POST metrics to.
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				r.Equal("/v1/metrics", req.URL.Path)
				r.Equal("test", req.Header.Get("X-Test"))
				r.Equal("gzip", req.Header.Get("Content-Encoding"))
				r.Equal("application/x-protobuf", req.Header.Get("Content-Type"))

				if tc.httpResponseWriter != nil {
					tc.httpResponseWriter(w) // for testing errors in response to exports.
				} else {
					fmt.Fprint(w, "ok")
				}
			}))
			defer ts.Close()

			// Create test config.
			hcpConfig, err := config.NewHCPConfig(
				config.WithoutBrowserLogin(),
				config.WithClientCredentials("client_id", "client_secret"),
			)
			r.NoError(err)

			tokenSource := &fakeTokenSource{accessToken: "token"}
			if tc.modTokenSource != nil {
				tc.modTokenSource(tokenSource)
			}

			cfg := &Config{
				MetricsEndpoint: ts.URL + "/v1/metrics",
				UserAgent:       "test",
				Middleware: []MiddlewareOption{
					WithRequestHeaders(map[string]string{"X-Test": "test"}),
				},
				TokenSource: tokenSource,
				TLSConfig:   hcpConfig.APITLSConfig(),
				Logger:      hclog.NewNullLogger(),
			}
			if tc.modConfig != nil {
				tc.modConfig(cfg)
			}

			// Create the client.
			c, err := New(cfg)
			if tc.expectNewErr != "" {
				r.Error(err)
				r.Contains(err.Error(), tc.expectNewErr)
				return
			}
			r.NoError(err)
			r.NotNil(c)

			// Export metrics.
			metrics := pmetric.NewMetrics()
			metrics.ResourceMetrics().AppendEmpty()
			err = c.ExportMetrics(context.Background(), metrics)
			if tc.exportExportMetricsErr != nil {
				r.Error(err)
				r.ErrorIs(err, tc.exportExportMetricsErr)
				return
			}
			r.NoError(err)
		})
	}
}
