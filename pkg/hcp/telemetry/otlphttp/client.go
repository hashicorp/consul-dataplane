package otlphttp

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"golang.org/x/net/http/httpproxy"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/hashicorp/go-hclog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"
	"golang.org/x/oauth2"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/proto"
)

var (
	errClientFailedToParseResponse    = errors.New("failed to parse response")
	errClientFailedToReadResponseBody = errors.New("failed to read response body")
	errClientFailedToExportMetrics    = errors.New("status error")
)

const (
	// maxRespBodySizeBytes is the maximum number of bytes to read from an HTTP response.
	// This is to prevent reading an unexpectedly large response body into memory.
	maxRespBodySizeBytes = 1024
)

// Client is an interface for sending metrics to the HCP Consul metrics telemetry endpoint.
type Client interface {
	// ExportMetrics sends metrics to the HCP Consul metrics telemetry endpoint configured on the client.
	// This returns an error if the request fails or the response status code is not in the 200-299 range.
	ExportMetrics(ctx context.Context, m pmetric.Metrics) error
}

type Config struct {
	Logger hclog.Logger

	MetricsEndpoint string
	Middleware      []MiddlewareOption
	TLSConfig       *tls.Config
	TokenSource     oauth2.TokenSource
	UserAgent       string

	// Proxy settings.
	HTTPSProxy string
	HTTPProxy  string
	NoProxy    string
}

type client struct {
	client             *http.Client
	logger             hclog.Logger
	metricsEndpointURL *url.URL
	userAgent          string
}

func New(cfg *Config) (Client, error) {
	c := &client{
		userAgent: cfg.UserAgent,
		logger:    cfg.Logger,
	}

	// Set endpoint.
	metricsEndpointURL, err := url.Parse(cfg.MetricsEndpoint)
	if err != nil {
		return nil, fmt.Errorf("bad metrics endpoint url %q: %w", cfg.MetricsEndpoint, err)
	}
	c.metricsEndpointURL = metricsEndpointURL

	// Support proxy if set.
	proxyConfig := httpproxy.Config{
		HTTPSProxy: cfg.HTTPSProxy,
		HTTPProxy:  cfg.HTTPProxy,
		NoProxy:    cfg.NoProxy,
	}
	proxyFunc := proxyConfig.ProxyFunc()

	// Set transport with oauth and proxy settings.
	tlsTransport := cleanhttp.DefaultPooledTransport()
	tlsTransport.TLSClientConfig = cfg.TLSConfig
	tlsTransport.Proxy = func(r *http.Request) (*url.URL, error) {
		return proxyFunc(r.URL)
	}

	transport := &roundTripperWithMiddleware{
		OriginalRoundTripper: &oauth2.Transport{
			Base:   tlsTransport,
			Source: cfg.TokenSource,
		},
		MiddlewareOptions: cfg.Middleware,
	}

	c.client = &http.Client{Transport: transport}
	return c, nil
}

func (c *client) ExportMetrics(ctx context.Context, m pmetric.Metrics) error {
	req := pmetricotlp.NewExportRequestFromMetrics(m)

	body, err := req.MarshalProto()
	if err != nil {
		return fmt.Errorf("failed to marshal export request: %w", err)
	}

	if err := c.export(ctx, c.metricsEndpointURL.String(), body); err != nil {
		return err
	}

	return nil
}

func (c *client) export(ctx context.Context, url string, body []byte) error {
	// Compress the payload with gzip.
	buf := bytes.NewBuffer([]byte{})
	writer := gzip.NewWriter(buf)
	if _, err := writer.Write(body); err != nil {
		return fmt.Errorf("failed to gzip metrics: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close gzip metrics writer: %w", err)
	}

	// Create the request.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, buf)
	if err != nil {
		return fmt.Errorf("failed to build http request: %w", err)
	}
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("User-Agent", c.userAgent)

	// Issue the request.
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make an HTTP request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.Error("failed to close response body", "error", err)
		}
	}()

	// Check the response status code.
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	// Read the response body (up to maxRespBodySizeBytes) to get the error message.
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxRespBodySizeBytes))
	if err != nil {
		return fmt.Errorf("failed to export metrics (status: %d): %w: %w", resp.StatusCode, errClientFailedToReadResponseBody, err)
	}

	st := &status.Status{}
	if err := proto.Unmarshal(respBody, st); err != nil {
		return fmt.Errorf("failed to export metrics (status: %d): %w: %w", resp.StatusCode, errClientFailedToParseResponse, err)
	}

	return fmt.Errorf("failed to export metrics (status: %d): %w: %s", resp.StatusCode, errClientFailedToExportMetrics, st.GetMessage())
}
