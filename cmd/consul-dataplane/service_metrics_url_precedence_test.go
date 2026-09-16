// Copyright IBM Corp. 2022, 2026
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// The service metrics URL can arrive from a config file, from the environment,
// or from repeated -telemetry-prom-service-metrics-url flags, and the file may
// still use the deprecated single-URL field. buildDataplaneConfig documents that
// flags outrank the file, so the higher-precedence list must fully replace the
// deprecated value rather than being scraped in addition to it.
func TestServiceMetricsURLPrecedence(t *testing.T) {
	const (
		fileURL = "http://file:1111/metrics"
		flagURL = "http://flag:2222/metrics"
		envURL  = "http://env:3333/metrics"
	)

	cases := map[string]struct {
		fileJSON string
		// setFlags mutates the flag-sourced config the way flag parsing would.
		setFlags func(t *testing.T, p *PrometheusTelemetryFlags)
		expURL   string
		expURLs  []string
	}{
		"file deprecated url only": {
			fileJSON: `{"telemetry":{"prometheus":{"serviceMetricsURL":"` + fileURL + `"}}}`,
			expURL:   fileURL,
		},
		"file list only": {
			fileJSON: `{"telemetry":{"prometheus":{"serviceMetricsURLs":["` + fileURL + `"]}}}`,
			expURLs:  []string{fileURL},
		},
		"flag list replaces the file deprecated url": {
			fileJSON: `{"telemetry":{"prometheus":{"serviceMetricsURL":"` + fileURL + `"}}}`,
			setFlags: func(t *testing.T, p *PrometheusTelemetryFlags) {
				require.NoError(t, p.ServiceMetricsURLs.Set(flagURL))
			},
			expURL:  "",
			expURLs: []string{flagURL},
		},
		"flag list replaces the file list": {
			fileJSON: `{"telemetry":{"prometheus":{"serviceMetricsURLs":["` + fileURL + `"]}}}`,
			setFlags: func(t *testing.T, p *PrometheusTelemetryFlags) {
				require.NoError(t, p.ServiceMetricsURLs.Set(flagURL))
			},
			expURLs: []string{flagURL},
		},
		"repeated flags replace the file deprecated url": {
			fileJSON: `{"telemetry":{"prometheus":{"serviceMetricsURL":"` + fileURL + `"}}}`,
			setFlags: func(t *testing.T, p *PrometheusTelemetryFlags) {
				require.NoError(t, p.ServiceMetricsURLs.Set(flagURL))
				require.NoError(t, p.ServiceMetricsURLs.Set(envURL))
			},
			expURL:  "",
			expURLs: []string{flagURL, envURL},
		},
		"an explicit empty flag clears the file deprecated url": {
			fileJSON: `{"telemetry":{"prometheus":{"serviceMetricsURL":"` + fileURL + `"}}}`,
			setFlags: func(t *testing.T, p *PrometheusTelemetryFlags) {
				require.NoError(t, p.ServiceMetricsURLs.Set(""))
			},
			expURL:  "",
			expURLs: nil,
		},
		"an explicit empty flag clears the file list": {
			fileJSON: `{"telemetry":{"prometheus":{"serviceMetricsURLs":["` + fileURL + `"]}}}`,
			setFlags: func(t *testing.T, p *PrometheusTelemetryFlags) {
				require.NoError(t, p.ServiceMetricsURLs.Set(""))
			},
			expURLs: nil,
		},
		"an empty list in the file clears its own deprecated url": {
			fileJSON: `{"telemetry":{"prometheus":{"serviceMetricsURL":"` + fileURL + `","serviceMetricsURLs":[]}}}`,
			expURL:   "",
			expURLs:  nil,
		},
		"nothing configured": {
			fileJSON: `{}`,
		},
	}

	for name, c := range cases {
		c := c
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			configFile := filepath.Join(dir, "config.json")
			require.NoError(t, os.WriteFile(configFile, []byte(c.fileJSON), 0o600))

			opts := &FlagOpts{configFile: configFile}
			if c.setFlags != nil {
				c.setFlags(t, &opts.dataplaneConfig.Telemetry.Prometheus)
			}

			cfg, err := opts.buildDataplaneConfig(nil)
			require.NoError(t, err)

			require.Equal(t, c.expURL, cfg.Telemetry.Prometheus.ServiceMetricsURL,
				"deprecated single URL")
			require.Equal(t, c.expURLs, cfg.Telemetry.Prometheus.ServiceMetricsURLs,
				"service metrics URL list")
		})
	}
}
