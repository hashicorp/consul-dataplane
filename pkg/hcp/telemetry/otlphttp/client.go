package otlphttp

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/hashicorp/go-hclog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"
	"golang.org/x/oauth2"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	maxHTTPResponseReadBytes = 64 * 1024

	protobufContentType = "application/x-protobuf"
)

type Config struct {
	MetricsEndpoint string
	TLSConfig       *tls.Config
	Middleware      []MiddlewareOption
	TokenSource     oauth2.TokenSource
	UserAgent       string
	Logger          hclog.Logger
}

type Client struct {
	metricsEndpointURL *url.URL
	userAgent          string
	client             *http.Client
	logger             hclog.Logger
}

func New(cfg *Config) (*Client, error) {
	client := &Client{
		userAgent: cfg.UserAgent,
		logger:    cfg.Logger,
	}
	if err := client.SetMetricsEndpoint(cfg.MetricsEndpoint); err != nil {
		return nil, err
	}

	tlsTransport := cleanhttp.DefaultPooledTransport()
	tlsTransport.TLSClientConfig = cfg.TLSConfig

	var transport http.RoundTripper = &oauth2.Transport{
		Base:   tlsTransport,
		Source: cfg.TokenSource,
	}

	transport = &roundTripperWithMiddleware{
		OriginalRoundTripper: transport,
		MiddlewareOptions:    cfg.Middleware,
	}
	client.client = &http.Client{Transport: transport}

	return client, nil
}

func (c *Client) SetMetricsEndpoint(endpoint string) error {
	metricsEndpointURL, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("bad metrics endpoint url %q: %w", endpoint, err)
	}
	c.metricsEndpointURL = metricsEndpointURL
	return nil
}

func (c *Client) ExportMetrics(ctx context.Context, m pmetric.Metrics) error {
	if c.metricsEndpointURL == nil {
		return fmt.Errorf("missing metrics endpoint url")
	}

	er := pmetricotlp.NewExportRequestFromMetrics(m)
	body, err := er.MarshalProto()
	if err != nil {
		return fmt.Errorf("failed to marshal export request: %w", err)
	}

	return c.export(ctx, c.metricsEndpointURL.String(), body)
}

func (c *Client) export(ctx context.Context, url string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to build http request: %w", err)
	}
	req.Header.Set("Content-Type", protobufContentType)
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make an HTTP request: %w", err)
	}

	defer func() {
		// Discard any remaining response body when we are done reading.
		io.CopyN(io.Discard, resp.Body, maxHTTPResponseReadBytes) // nolint:errcheck
		resp.Body.Close()
	}()

	respStatus := readResponseStatus(resp)

	// Format the error message. Use the status if it is present in the response.
	var formattedErr error
	if respStatus != nil {
		formattedErr = fmt.Errorf(
			"error exporting items, request to %s responded with HTTP Status Code %d, Message=%s, Details=%v",
			url, resp.StatusCode, respStatus.Message, respStatus.Details)
	} else {
		formattedErr = fmt.Errorf(
			"error exporting items, request to %s responded with HTTP Status Code %d",
			url, resp.StatusCode)
	}

	return formattedErr
}

func readResponseBody(resp *http.Response) ([]byte, error) {
	if resp.ContentLength == 0 {
		return nil, nil
	}

	maxRead := resp.ContentLength

	// if maxRead == -1, the ContentLength header has not been sent, so read up to
	// the maximum permitted body size. If it is larger than the permitted body
	// size, still try to read from the body in case the value is an error. If the
	// body is larger than the maximum size, proto unmarshaling will likely fail.
	if maxRead == -1 || maxRead > maxHTTPResponseReadBytes {
		maxRead = maxHTTPResponseReadBytes
	}
	protoBytes := make([]byte, maxRead)
	n, err := io.ReadFull(resp.Body, protoBytes)

	// No bytes read and an EOF error indicates there is no body to read.
	if n == 0 && (err == nil || errors.Is(err, io.EOF)) {
		return nil, nil
	}

	// io.ReadFull will return io.ErrorUnexpectedEOF if the Content-Length header
	// wasn't set, since we will try to read past the length of the body. If this
	// is the case, the body will still have the full message in it, so we want to
	// ignore the error and parse the message.
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}

	return protoBytes[:n], nil
}

// Read the response and decode the status.Status from the body.
// Returns nil if the response is empty or cannot be decoded.
func readResponseStatus(resp *http.Response) *status.Status {
	var respStatus *status.Status
	if resp.StatusCode >= 400 && resp.StatusCode <= 599 {
		// Request failed. Read the body. OTLP spec says:
		// "Response body for all HTTP 4xx and HTTP 5xx responses MUST be a
		// Protobuf-encoded Status message that describes the problem."
		respBytes, err := readResponseBody(resp)
		if err != nil {
			return nil
		}

		// Decode it as Status struct. See https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/otlp.md#failures
		respStatus = &status.Status{}
		err = proto.Unmarshal(respBytes, respStatus)
		if err != nil {
			return nil
		}
	}

	return respStatus
}
