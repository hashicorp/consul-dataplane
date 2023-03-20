// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package consuldp

import "github.com/armon/go-metrics/prometheus"

var gauges = []prometheus.GaugeDefinition{
	{
		Name: []string{"envoy_connected"},
		Help: "This will either be 0 or 1 depending on whether Envoy is currently running and connected to the local xDS listeners.",
	},
}
