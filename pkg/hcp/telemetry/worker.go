package telemetry

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/hashicorp/consul/proto-public/pbresource"
	"github.com/hashicorp/go-hclog"
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/hashicorp/consul-dataplane/pkg/hcp/auth"
	"github.com/hashicorp/consul-dataplane/pkg/hcp/telemetry/otlphttp"
	"github.com/hashicorp/consul-dataplane/pkg/version"
)

const defaultScrapeIntervalSecs = 10

type Worker struct {
	resourceClient pbresource.ResourceServiceClient
	scrapeInterval time.Duration
	logger         hclog.Logger

	exporter *otlphttp.Client
}

func New(resourceClient pbresource.ResourceServiceClient, logger hclog.Logger) *Worker {
	return &Worker{
		resourceClient: resourceClient,
		scrapeInterval: time.Duration(defaultScrapeIntervalSecs) * time.Second,
		logger:         logger,
	}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.scrapeInterval)
	scraper := &Scraper{
		Logger:             w.logger.Named("metrics_scrape"),
		EnvoyAdminHostPort: "TODO",
		Client:             http.DefaultTransport,
	}

	stateTracker := NewStateTracker(w.resourceClient, w.logger.Named("state_tracker"))
	notifyCh := stateTracker.Run(ctx)

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			if w.exporter == nil {
				// exporter hasn't been configured which happens when state change notifications are handled
				continue
			}

			state, ok := stateTracker.State()
			if !ok || state.Metrics.Disabled {
				continue
			}

			labels := pcommon.NewMap()
			for k, v := range state.Labels {
				labels.PutStr(k, v)
			}

			metrics, err := scraper.Scrape(ctx, state.Metrics.IncludeList, labels)
			if err != nil {
				w.logger.Error("failed to scrape envoy stats", "error", err)
				continue
			}

			if err := w.exporter.ExportMetrics(ctx, metrics); err != nil {
				if retryErr, ok := err.(otlphttp.RetryableError); ok && retryErr.After.After(time.Now()) {
					//TODO schedule future retry
				}
				//TODO backoff? buffer?
			}

		case <-notifyCh:
			//telemetrystate changed, labels and filters will be picked up on next scrape interval
			if state, ok := stateTracker.State(); ok {
				w.updateExporter(state)
			} else {
				w.exporter = nil
			}

		}
	}
}

func (w *Worker) updateExporter(state State) error {
	cfg := &otlphttp.Config{
		MetricsEndpoint: state.MetricsEndpoint(),
		TLSConfig:       &tls.Config{},
		TokenSource:     auth.TokenSource(state.ClientID, state.ClientSecret),
		Middleware: []otlphttp.MiddlewareOption{otlphttp.WithRequestHeaders(map[string]string{
			"X-HCP-Resource-ID":    state.ResourceID,
			"X-HCP-Source-Channel": fmt.Sprintf("consul-dataplane %s", version.GetHumanVersion()),
		})},
		UserAgent: fmt.Sprintf("consul-dataplane/%s (%s/%s)",
			version.GetHumanVersion(), runtime.GOOS, runtime.GOARCH),
		Logger: w.logger.Named("otlphttp"),
	}

	exporter, err := otlphttp.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to build otlphttp client: %w", err)
	}

	w.exporter = exporter
	return nil
}
