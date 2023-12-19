package telemetry

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/hashicorp/consul/proto-public/pbresource"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/hcp-sdk-go/config"
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/hashicorp/consul-dataplane/pkg/hcp/telemetry/otlphttp"
	"github.com/hashicorp/consul-dataplane/pkg/version"
)

const defaultScrapeIntervalSecs = 60

type Worker struct {
	envoyAdminHostPort string
	logger             hclog.Logger
	resourceClient     pbresource.ResourceServiceClient
	scrapeInterval     time.Duration

	exporter *otlphttp.Client
}

func New(resourceClient pbresource.ResourceServiceClient, logger hclog.Logger, envoyAdminHostPort string) *Worker {
	return &Worker{
		envoyAdminHostPort: envoyAdminHostPort,
		logger:             logger,
		resourceClient:     resourceClient,
		scrapeInterval:     time.Duration(defaultScrapeIntervalSecs) * time.Second,
	}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.scrapeInterval)
	scraper := &scraper{
		logger:             w.logger.Named("metrics_scrape"),
		envoyAdminHostPort: w.envoyAdminHostPort,
		client:             http.DefaultTransport,
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

			metrics, err := scraper.scrape(ctx, state.Metrics.IncludeList, labels)
			if err != nil {
				w.logger.Error("failed to scrape envoy stats", "error", err)
				continue
			}

			if err := w.exporter.ExportMetrics(ctx, metrics); err != nil {
				w.logger.Error("failed to export metrics", "error", err)
			}
		case <-notifyCh:
			//telemetrystate changed, labels and filters will be picked up on next scrape interval
			if state, ok := stateTracker.State(); ok {
				if err := w.updateExporter(state); err != nil {
					w.logger.Error("failed to update exporter", "error", err)
				}
			} else {
				w.exporter = nil
			}

		}
	}
}

func (w *Worker) updateExporter(state State) error {
	hcpConfig, err := config.NewHCPConfig(
		config.FromEnv(),
		config.WithClientCredentials(state.ClientID, state.ClientSecret),
	)
	if err != nil {
		return fmt.Errorf("failed to build hcp config: %w", err)
	}

	cfg := &otlphttp.Config{
		MetricsEndpoint: state.MetricsEndpoint(),
		TLSConfig:       hcpConfig.APITLSConfig(),
		TokenSource:     hcpConfig,
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
