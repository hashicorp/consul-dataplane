// Package telemetry is for scraping and sending Envoy metrics to HCP in OTLP format.
//
// This exists to let customers send mesh telemetry to HCP without having to deploy
// a dedicated telemetry collector. Configuration for the collection, filtering, and exporting
// of Envoy metrics is queried in the hcp.v2.TelemetryState resource in Consul.
//
// A lot of the package is concerned with converting prometheus-format metrics into their
// OTLP equivalent.
package telemetry
