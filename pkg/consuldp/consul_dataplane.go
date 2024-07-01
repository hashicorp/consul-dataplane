// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package consuldp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/armon/go-metrics"
	"github.com/hashicorp/consul-server-connection-manager/discovery"
	"github.com/hashicorp/consul/proto-public/pbdataplane"
	"github.com/hashicorp/consul/proto-public/pbdns"
	"github.com/hashicorp/go-hclog"
	"google.golang.org/grpc"

	"github.com/hashicorp/consul-dataplane/pkg/dns"
	"github.com/hashicorp/consul-dataplane/pkg/envoy"
	metricscache "github.com/hashicorp/consul-dataplane/pkg/metrics-cache"
)

type xdsServer struct {
	listener        net.Listener
	listenerAddress string
	listenerNetwork string
	gRPCServer      *grpc.Server
	exitedCh        chan struct{}
}

type httpClient interface {
	Get(string) (*http.Response, error)
	Post(string, string, io.Reader) (*http.Response, error)
}

// ConsulDataplane represents the consul-dataplane process
type ConsulDataplane struct {
	logger          hclog.Logger
	cfg             *Config
	serverConn      *grpc.ClientConn
	dpServiceClient pbdataplane.DataplaneServiceClient
	xdsServer       *xdsServer
	aclToken        string
	metricsConfig   *metricsConfig
	lifecycleConfig *lifecycleConfig

	resolvedProxyConfig ProxyConfig
}

// NewConsulDP creates a new instance of ConsulDataplane
func NewConsulDP(cfg *Config) (*ConsulDataplane, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	hclogLevel := hclog.LevelFromString(cfg.Logging.LogLevel)
	if hclogLevel == hclog.NoLevel {
		hclogLevel = hclog.Info
	}
	logger := hclog.New(&hclog.LoggerOptions{
		Name:       cfg.Logging.Name,
		Level:      hclogLevel,
		JSONFormat: cfg.Logging.LogJSON,
	})

	return &ConsulDataplane{
		logger: logger,
		cfg:    cfg,
	}, nil
}

func validateConfig(cfg *Config) error {
	isRunningSidecarMode := cfg.DNSServer.Enabled && cfg.XDSServer.Enabled && cfg.Envoy.Enabled
	switch {
	case cfg.Consul == nil || cfg.Consul.Addresses == "":
		return errors.New("consul addresses not specified")
	case cfg.Consul.GRPCPort == 0:
		return errors.New("consul server gRPC port not specified")
	case cfg.Proxy == nil:
		return errors.New("proxy details not specified")
	case cfg.Proxy.ProxyID == "":
		return errors.New("proxy ID not specified")
	case cfg.Envoy == nil:
		return errors.New("envoy settings not specified")
	case cfg.Envoy.Enabled && cfg.Envoy.AdminBindAddress == "":
		return errors.New("envoy admin bind address not specified")
	case cfg.Envoy.Enabled && cfg.Envoy.AdminBindPort == 0:
		return errors.New("envoy admin bind port not specified")
	case cfg.Logging == nil:
		return errors.New("logging settings not specified")
	case cfg.XDSServer.Enabled && cfg.XDSServer.BindAddress == "":
		return errors.New("envoy xDS bind address not specified")
	case cfg.XDSServer.Enabled && !strings.HasPrefix(cfg.XDSServer.BindAddress, "unix://") && !net.ParseIP(cfg.XDSServer.BindAddress).IsLoopback():
		return errors.New("non-local xDS bind address not allowed")
	case cfg.Envoy.Enabled != cfg.XDSServer.Enabled:
		return errors.New("enabled flags for envoy and xDS must match")
	case isRunningSidecarMode && cfg.DNSServer.Port != -1 && !net.ParseIP(cfg.DNSServer.BindAddr).IsLoopback():
		return errors.New("non-local DNS proxy bind address not allowed when running as sidecar")
	}

	creds := cfg.Consul.Credentials
	if creds.Type == CredentialsTypeLogin && creds.Login.BearerToken == "" && creds.Login.BearerTokenPath == "" {
		return errors.New("bearer token (or path to a file containing a bearer token) is required for login")
	}

	if cfg.Telemetry != nil {
		prom := cfg.Telemetry.Prometheus
		// If any of CA/Cert/Key are specified, make sure they are all present.
		if prom.KeyFile != "" || prom.CertFile != "" || prom.CACertsPath != "" {
			if prom.KeyFile == "" || prom.CertFile == "" || prom.CACertsPath == "" {
				return errors.New("Must provide -telemetry-prom-ca-certs-path, -telemetry-prom-cert-file, " +
					"and -telemetry-prom-key-file to enable TLS for prometheus metrics")
			}
		}

		if prom.RetentionTime <= 0 {
			return errors.New("-telemetry-prom-retention-time must be greater than zero")
		}

		if prom.ScrapePath == "" {
			return errors.New("-telemetry-prom-scrape-path must not be empty")
		}
	}

	return nil
}

func (cdp *ConsulDataplane) Run(ctx context.Context) error {
	ctx = hclog.WithContext(ctx, cdp.logger)
	cdp.logger.Info("started consul-dataplane process")

	// At startup we need to cache metrics until we have information from the bootstrap envoy config
	// that the consumer wants metrics enabled. Until then we will set our own light weight metrics
	// sink. If consumer doesn't enable the metrics the sink will set a blackhole sink. Otherwise
	// it will swap to the newly configured prometheus/dogstatsD/statsD sink.
	cacheSink := metricscache.NewSink()
	conf := metrics.DefaultConfig("")
	conf.EnableHostname = false
	_, err := metrics.NewGlobal(conf, cacheSink)
	if err != nil {
		return err
	}

	tls, err := cdp.cfg.Consul.TLS.Load()
	if err != nil {
		return err
	}

	creds, err := cdp.cfg.Consul.Credentials.ToDiscoveryCredentials()
	if err != nil {
		return err
	}

	watcher, err := discovery.NewWatcher(ctx, discovery.Config{
		Addresses:           cdp.cfg.Consul.Addresses,
		GRPCPort:            cdp.cfg.Consul.GRPCPort,
		ServerWatchDisabled: cdp.cfg.Consul.ServerWatchDisabled,
		Credentials:         creds,
		TLS:                 tls,
		ServerEvalFn: discovery.SupportsDataplaneFeatures(
			pbdataplane.DataplaneFeatures_DATAPLANE_FEATURES_ENVOY_BOOTSTRAP_CONFIGURATION.String(),
		),
	}, cdp.logger.Named("server-connection-manager"))
	if err != nil {
		return err
	}
	go watcher.Run()
	defer watcher.Stop()

	state, err := watcher.State()
	if err != nil {
		return err
	}

	cdp.logger.Info("connected to Consul server over gRPC", "initial_server_address", state.Address.String())
	cdp.serverConn = state.GRPCConn
	cdp.aclToken = state.Token
	cdp.dpServiceClient = pbdataplane.NewDataplaneServiceClient(state.GRPCConn)

	// start up DNS server if configured
	if cdp.cfg.DNSServer.Enabled {
		if err = cdp.startDNSProxy(ctx); err != nil {
			cdp.logger.Error("failed to start the dns proxy", "error", err)
			return err
		}
	}

	bootstrapCfg, cfg, err := cdp.bootstrapConfig(ctx)
	if err != nil {
		cdp.logger.Error("failed to get bootstrap config", "error", err)
		return fmt.Errorf("failed to get bootstrap config: %w", err)
	}
	cdp.logger.Debug("generated envoy bootstrap config", "config", string(cfg))

	doneCh := make(chan error)
	// start up xDS server and Envoy if configured
	if cdp.cfg.Envoy.Enabled && cdp.cfg.XDSServer.Enabled {
		err = cdp.setupXDSServer()
		if err != nil {
			return err
		}
		go cdp.startXDSServer(ctx)

		proxy, err := envoy.NewProxy(cdp.envoyProxyConfig(cfg))
		if err != nil {
			cdp.logger.Error("failed to create new proxy", "error", err)
			return fmt.Errorf("failed to create new proxy: %w", err)
		}
		if err := proxy.Run(ctx); err != nil {
			cdp.logger.Error("failed to run proxy", "error", err)
			return fmt.Errorf("failed to run proxy: %w", err)
		}

		cdp.metricsConfig = NewMetricsConfig(cdp.cfg, cacheSink)
		err = cdp.metricsConfig.startMetrics(ctx, bootstrapCfg)
		if err != nil {
			return err
		}

		cdp.lifecycleConfig = NewLifecycleConfig(cdp.cfg, proxy)
		err = cdp.lifecycleConfig.startLifecycleManager(ctx)
		if err != nil {
			return err
		}

		go func() {
			select {
			case <-ctx.Done():
				doneCh <- nil
			case err := <-proxy.Exited():
				if err != nil {
					cdp.logger.Error("envoy proxy exited with error", "error", err)
				}
				doneCh <- err
			case <-cdp.xdsServerExited():
				// Initiate graceful shutdown of Envoy, kill if error
				if err := proxy.Quit(); err != nil {
					cdp.logger.Error("failed to stop proxy, will attempt to kill", "error", err)
					if err := proxy.Kill(); err != nil {
						cdp.logger.Error("failed to kill proxy", "error", err)
					}
				}
				doneCh <- errors.New("xDS server exited unexpectedly")
			case <-cdp.metricsConfig.metricsServerExited():
				doneCh <- errors.New("metrics server exited unexpectedly")
			case <-cdp.lifecycleConfig.lifecycleServerExited():
				// Initiate graceful shutdown of Envoy, kill if error
				if err := proxy.Quit(); err != nil {
					cdp.logger.Error("failed to stop proxy", "error", err)
					if err := proxy.Kill(); err != nil {
						cdp.logger.Error("failed to kill proxy", "error", err)
					}
				}
				doneCh <- errors.New("proxy lifecycle management server exited unexpectedly")
			}
		}()
	} else {
		// Envoy and xDS servers are not configured so only listen to the done channel from context.
		go func() {
			select {
			case <-ctx.Done():
				doneCh <- nil
			}
		}()
	}

	return <-doneCh
}

func (cdp *ConsulDataplane) startDNSProxy(ctx context.Context) error {
	dnsClientInterface := pbdns.NewDNSServiceClient(cdp.serverConn)

	dnsServer, err := dns.NewDNSServer(dns.DNSServerParams{
		BindAddr:  cdp.cfg.DNSServer.BindAddr,
		Port:      cdp.cfg.DNSServer.Port,
		Client:    dnsClientInterface,
		Logger:    cdp.logger,
		Partition: cdp.resolvedProxyConfig.Partition,
		Namespace: cdp.resolvedProxyConfig.Namespace,
		Token:     cdp.aclToken,
	})
	if err == dns.ErrServerDisabled {
		cdp.logger.Info("dns proxy disabled: configure the Consul DNS port to enable")
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to create dns server: %w", err)
	}
	if err = dnsServer.Start(ctx); err != nil {
		return fmt.Errorf("failed to run the dns proxy: %w", err)
	}
	return nil
}

func (cdp *ConsulDataplane) envoyProxyConfig(cfg []byte) envoy.ProxyConfig {
	extraArgs := cdp.cfg.Envoy.ExtraArgs

	envoyArgs := map[string]interface{}{
		"--concurrency":    cdp.cfg.Envoy.EnvoyConcurrency,
		"--drain-time-s":   cdp.cfg.Envoy.EnvoyDrainTimeSeconds,
		"--drain-strategy": cdp.cfg.Envoy.EnvoyDrainStrategy,
	}

	// Users could set the Envoy concurrency, drain time, or drain strategy as
	// extra args. Prioritize values set in that way over passthrough or defaults
	// from consul-dataplane.
	for envoyArg, cdpEnvoyValue := range envoyArgs {
		for _, v := range extraArgs {
			// If found in extraArgs, skip setting value from consul-dataplane Envoy
			// config
			if v == envoyArg {
				break
			}
		}

		// If not found, append value from consul-dataplane Envoy config to extraArgs
		extraArgs = append(extraArgs, fmt.Sprintf("%s %v", envoyArg, cdpEnvoyValue))
	}

	return envoy.ProxyConfig{
		AdminAddr:       cdp.cfg.Envoy.AdminBindAddress,
		AdminBindPort:   cdp.cfg.Envoy.AdminBindPort,
		Logger:          cdp.logger,
		LogJSON:         cdp.cfg.Logging.LogJSON,
		BootstrapConfig: cfg,
		ExtraArgs:       extraArgs,
	}
}

func (cdp *ConsulDataplane) GracefulShutdown(cancel context.CancelFunc) {
	// If proxy lifecycle manager has not been initialized, cancel parent context and
	// proceed to exit rather than attempting graceful shutdown
	if cdp.lifecycleConfig != nil {
		cdp.lifecycleConfig.gracefulShutdown()
	} else {
		cancel()
	}
}
