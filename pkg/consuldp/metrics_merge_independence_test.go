// Copyright IBM Corp. 2022, 2026
// SPDX-License-Identifier: MPL-2.0

package consuldp

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

// The merged metrics endpoint scrapes several sources in sequence. A source
// that is unreachable must not suppress the sources listed after it, otherwise
// the merged output silently depends on the order the sources happen to be
// configured in. That is not a theoretical concern: a service exposing metrics
// on two ports lists one URL per port, so a single dead port would drop every
// port after it while still returning a plausible-looking response.

// newTestMetricsConfig returns a metricsConfig wired to scrape the given urls.
func newTestMetricsConfig(urls ...string) *metricsConfig {
	m := &metricsConfig{
		logger: hclog.NewNullLogger(),
		client: &http.Client{Timeout: 5 * time.Second},
	}
	for _, u := range urls {
		m.urls = append(m.urls, staticUrlFn(u))
	}
	return m
}

// serve starts a test server returning body, and returns its URL.
func serve(t *testing.T, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(rw, body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// serveStatus starts a test server returning the given status code.
func serveStatus(t *testing.T, code int) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(code)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// deadURL returns a URL that is guaranteed to refuse connections.
func deadURL(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()
	return url
}

func scrapeMerged(t *testing.T, m *metricsConfig) *httptest.ResponseRecorder {
	t.Helper()
	rw := httptest.NewRecorder()
	m.mergedMetricsHandler(rw, httptest.NewRequest(http.MethodGet, "/stats/prometheus", nil))
	return rw
}

func TestMergedMetricsHandler_FailingSourceDoesNotSuppressOthers(t *testing.T) {
	first := serve(t, "first_metric 1\n")
	last := serve(t, "last_metric 2\n")

	t.Run("dead source in the middle", func(t *testing.T) {
		m := newTestMetricsConfig(first, deadURL(t), last)
		rw := scrapeMerged(t, m)

		require.Equal(t, http.StatusOK, rw.Code)
		require.Contains(t, rw.Body.String(), "first_metric 1")
		require.Contains(t, rw.Body.String(), "last_metric 2",
			"a source after an unreachable one must still be scraped")
	})

	t.Run("dead source first", func(t *testing.T) {
		// The regression: the only failing source is the first one, so an
		// early return would drop every real source while still returning 200.
		m := newTestMetricsConfig(deadURL(t), first, last)
		rw := scrapeMerged(t, m)

		require.Equal(t, http.StatusOK, rw.Code)
		require.Contains(t, rw.Body.String(), "first_metric 1")
		require.Contains(t, rw.Body.String(), "last_metric 2")
	})

	t.Run("non-2xx source in the middle", func(t *testing.T) {
		m := newTestMetricsConfig(first, serveStatus(t, http.StatusInternalServerError), last)
		rw := scrapeMerged(t, m)

		require.Equal(t, http.StatusOK, rw.Code)
		require.Contains(t, rw.Body.String(), "first_metric 1")
		require.Contains(t, rw.Body.String(), "last_metric 2")
	})

	t.Run("body is not polluted by the error message", func(t *testing.T) {
		m := newTestMetricsConfig(first, deadURL(t), last)
		rw := scrapeMerged(t, m)

		require.NotContains(t, rw.Body.String(), "failed to scrape metrics",
			"an error message must never be appended into the metrics body")
	})
}

func TestMergedMetricsHandler_AllSourcesFailing(t *testing.T) {
	m := newTestMetricsConfig(deadURL(t), deadURL(t))
	rw := scrapeMerged(t, m)

	require.Equal(t, http.StatusInternalServerError, rw.Code,
		"with nothing scraped the request should fail rather than return an empty 200")
}

func TestMergedMetricsHandler_AllSourcesHealthy(t *testing.T) {
	m := newTestMetricsConfig(serve(t, "a 1\n"), serve(t, "b 2\n"), serve(t, "c 3\n"))
	rw := scrapeMerged(t, m)

	require.Equal(t, http.StatusOK, rw.Code)
	require.Equal(t, "a 1\nb 2\nc 3\n", rw.Body.String())
}
