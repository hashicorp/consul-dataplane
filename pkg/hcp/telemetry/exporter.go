package telemetry

import (
	"context"
	"time"

	"github.com/hashicorp/consul/proto-public/pbresource"
	"github.com/hashicorp/go-hclog"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

const exporterDefaultScrapeInterval = time.Minute

// Exporter is a telemetry exporter that is specific to HCP. If enabled via the hcp.v2.TelemetryState resource,
// this exporter periodically scrapes Envoy metrics from its admin endpoint and pushes the metrics up to HCP.
type Exporter struct {
	envoyProxyID   string
	logger         hclog.Logger
	scrapeInterval time.Duration
	scraper        scraper
	stateTracker   stateTracker
}

// NewHCPExporter creates a new HCP telemetry exporter.
func NewHCPExporter(resourceClient pbresource.ResourceServiceClient, logger hclog.Logger, envoyAdminHostPort, envoyProxyID string) *Exporter {
	return &Exporter{
		envoyProxyID:   envoyProxyID,
		logger:         logger,
		scrapeInterval: exporterDefaultScrapeInterval,
		scraper:        newScraper(envoyAdminHostPort, logger.Named("scraper")),
		stateTracker:   newStateTracker(resourceClient, logger.Named("state_tracker")),
	}
}

// Run starts the exporter's worker goroutine that periodically scrapes Envoy metrics and pushes them to HCP.
func (w *Exporter) Run(ctx context.Context) {
	go w.stateTracker.Run(ctx) // start syncing state from consul.
	ticker := time.NewTicker(w.scrapeInterval)

	for {
		select {
		case <-ticker.C:
			state, ok := w.stateTracker.GetState()
			if !ok || state.disabled {
				w.logger.Debug("metric exporting disabled")
				continue
			}

			labels := pcommon.NewMap()
			for k, v := range state.labels {
				labels.PutStr(k, v)
			}
			labels.PutStr("node.id", w.envoyProxyID)

			metrics, err := w.scraper.scrape(ctx, state.includeList, labels)
			if err != nil {
				w.logger.Error("failed to scrape envoy stats", "error", err)
				continue
			}

			if err := state.client.ExportMetrics(ctx, metrics); err != nil {
				w.logger.Error("failed to export metrics", "error", err)
			} else {
				w.logger.Debug("exported metrics to hcp", "metrics_count", metrics.MetricCount())
			}
		case <-ctx.Done():
			return
		}
	}
}
