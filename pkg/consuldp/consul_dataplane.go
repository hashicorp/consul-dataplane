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

// knownDataplaneFeatures is the ordered set of feature names that a fully-capable
// Consul server is expected to advertise. It is used by categorizeFeatures
// to produce available_features / missing_features log fields.
var knownDataplaneFeatures = []string{
	pbdataplane.DataplaneFeatures_DATAPLANE_FEATURES_WATCH_SERVERS.String(),
	pbdataplane.DataplaneFeatures_DATAPLANE_FEATURES_EDGE_CERTIFICATE_MANAGEMENT.String(),
	pbdataplane.DataplaneFeatures_DATAPLANE_FEATURES_ENVOY_BOOTSTRAP_CONFIGURATION.String(),
}

// categorizeFeatures splits the server's advertised feature map into
// "available" and "missing" slices relative to the given expected set.
// The resulting slices are suitable for structured log key-value fields.
func categorizeFeatures(expected []string, advertised map[string]bool) (available, missing []string) {
	for _, f := range expected {
		if advertised[f] {
			available = append(available, f)
		} else {
			missing = append(missing, f)
		}
	}
	return
}

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

	// isLegacyCompatMode is true when we are connected to a Consul server that
	// does not advertise full dataplane feature support. Some optional features
	// are automatically disabled or guarded to prevent failures against the
	// older API surface. Set conservatively at construction time and refined
	// once the server's actual feature set is known.
	isLegacyCompatMode bool
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
		// Conservatively pre-set legacy compat mode from the config flag so that
		// any code running before watcher.State() resolves already applies
		// compatibility guards. The value is refined once the server's actual
		// feature set is known.
		isLegacyCompatMode: cfg.Consul.EnableLegacyServerCompatibility,
	}, nil
}

// legacyCompatDisabledFeatures returns the authoritative list of consul-dataplane
// features that are intentionally disabled while in legacy-server compatibility
// mode. This is the single place to expand the list as new guards are added.
func (cdp *ConsulDataplane) legacyCompatDisabledFeatures() []string {
	if !cdp.isLegacyCompatMode {
		return nil
	}
	return []string{
		// The proxy config struct returned by older servers may be absent,
		// so central telemetry config decoding is skipped to avoid a panic.
		"central-telemetry-config",
	}
}

func validateConfig(cfg *Config) error {
	switch {
	case cfg.Consul == nil || cfg.Consul.Addresses == "":
		return errors.New("consul addresses not specified")
	case cfg.Consul.GRPCPort == 0:
		return errors.New("consul server gRPC port not specified")
	case cfg.Mode == ModeTypeSidecar && cfg.Proxy == nil:
		return errors.New("proxy details not specified")
	case cfg.Mode == ModeTypeSidecar && cfg.Proxy.ProxyID == "":
		return errors.New("proxy ID not specified")
	case cfg.Mode == ModeTypeSidecar && cfg.Envoy == nil:
		return errors.New("envoy settings not specified")
	case cfg.Mode == ModeTypeSidecar && cfg.Envoy.AdminBindAddress == "":
		return errors.New("envoy admin bind address not specified")
	case cfg.Mode == ModeTypeSidecar && cfg.Envoy.AdminBindPort == 0:
		return errors.New("envoy admin bind port not specified")
	case cfg.Logging == nil:
		return errors.New("logging settings not specified")
	case cfg.Mode == ModeTypeSidecar && cfg.XDSServer.BindAddress == "":
		return errors.New("envoy xDS bind address not specified")
	case cfg.Mode == ModeTypeSidecar && !strings.HasPrefix(cfg.XDSServer.BindAddress, "unix://") && !net.ParseIP(cfg.XDSServer.BindAddress).IsLoopback():
		return errors.New("non-local xDS bind address not allowed")
	case cfg.Mode == ModeTypeSidecar && cfg.DNSServer.Port != -1 && !net.ParseIP(cfg.DNSServer.BindAddr).IsLoopback():
		return errors.New("non-local DNS proxy bind address not allowed when running as a sidecar")
	case cfg.Mode == ModeTypeDNSProxy && cfg.Proxy != nil && (cfg.Proxy.Namespace != "" && cfg.Proxy.Namespace != "default"):
		return errors.New("namespace must be empty or set to 'default' when running in dns-proxy mode")
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
	cdp.logger.Info(fmt.Sprintf("consul-dataplane mode: %s", cdp.cfg.Mode))

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

	var serverEvalFn discovery.ServerEvalFn

	if cdp.cfg.Consul.EnableLegacyServerCompatibility {
		cdp.logger.Warn("[COMPAT] legacy-server-compat is enabled — "+
			"consul-dataplane will bypass the normal dataplane feature check to allow connection to older Consul servers. "+
			"Features not supported by the server will be automatically disabled. "+
			"This flag is intended for upgrade transitions only. "+
			"Remove it once all Consul servers are on a fully supported version.",
			"disabled_features", cdp.legacyCompatDisabledFeatures(),
		)
		// Controlled bypass: always accept the server but classify its features
		// so anomalies (e.g. a fully-capable server with the flag still on) are
		// immediately visible to the operator.
		bootstrapFeature := pbdataplane.DataplaneFeatures_DATAPLANE_FEATURES_ENVOY_BOOTSTRAP_CONFIGURATION.String()
		serverEvalFn = func(s discovery.State) bool {
			available, missing := categorizeFeatures(knownDataplaneFeatures, s.DataplaneFeatures)
			if s.DataplaneFeatures[bootstrapFeature] {
				// Anomaly: this server already has full feature support. The flag
				// is redundant — warn so the operator can clean it up.
				cdp.logger.Warn("[COMPAT] server has full dataplane feature support — "+
					"the legacy-server-compat flag is not required for this server. "+
					"Consider removing it after verifying your environment.",
					"server_address", s.Address.String(),
					"available_features", available,
					"missing_features", missing,
				)
			} else {
				// Expected path: server lacks the bootstrap feature (older server).
				cdp.logger.Info("[COMPAT] accepting server in legacy compatibility mode",
					"server_address", s.Address.String(),
					"available_features", available,
					"missing_features", missing,
				)
			}
			return true
		}
	} else {
		serverEvalFn = discovery.SupportsDataplaneFeatures(
			pbdataplane.DataplaneFeatures_DATAPLANE_FEATURES_ENVOY_BOOTSTRAP_CONFIGURATION.String(),
		)
	}

	watcher, err := discovery.NewWatcher(ctx, discovery.Config{
		Addresses:           cdp.cfg.Consul.Addresses,
		GRPCPort:            cdp.cfg.Consul.GRPCPort,
		ServerWatchDisabled: cdp.cfg.Consul.ServerWatchDisabled,
		Credentials:         creds,
		TLS:                 tls,
		ServerEvalFn:        serverEvalFn,
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

	// Refine isLegacyCompatMode using the server's actual advertised feature set.
	// The value was pre-set conservatively in NewConsulDP; here we either
	// confirm it (server lacks required features) or downgrade it (server is
	// fully capable despite the flag being set).
	if cdp.isLegacyCompatMode {
		bootstrapFeature := pbdataplane.DataplaneFeatures_DATAPLANE_FEATURES_ENVOY_BOOTSTRAP_CONFIGURATION.String()
		available, missing := categorizeFeatures(knownDataplaneFeatures, state.DataplaneFeatures)
		if state.DataplaneFeatures[bootstrapFeature] {
			// Server is fully capable — downgrade to normal operation.
			cdp.isLegacyCompatMode = false
			cdp.logger.Info("[COMPAT] server has full dataplane feature support — "+
				"compatibility guards have been lifted. "+
				"Consider removing the legacy-server-compat flag.",
				"server_address", state.Address.String(),
				"available_features", available,
				"missing_features", missing,
			)
		} else {
			// Confirmed: server lacks required features — keep compat mode active.
			cdp.logger.Warn("[COMPAT] server does not support all required dataplane features — "+
				"running in legacy compatibility mode with reduced feature set.",
				"server_address", state.Address.String(),
				"available_features", available,
				"missing_features", missing,
				"disabled_features", cdp.legacyCompatDisabledFeatures(),
			)
		}
	}

	doneCh := make(chan error)

	// if running as DNS PRoxy, xDS Server and Envoy are disabled, so
	// return before configuring them.
	if cdp.cfg.Mode == ModeTypeDNSProxy {
		// start up DNS server with the configuration from the consul-dataplane flags / environment variables since
		// envoy bootstrapping is bypassed.
		if err = cdp.startDNSProxy(ctx, cdp.cfg.DNSServer, cdp.cfg.Proxy.Namespace, cdp.cfg.Proxy.Partition); err != nil {
			cdp.logger.Error("failed to start the dns proxy", "error", err)
			return err
		}
		// Wait for context to be done in a more simplified goroutine dns-proxy mode.
		go func() {
			<-ctx.Done()
			doneCh <- nil
		}()
		return <-doneCh
	}

	// Configure xDS and Envoy configuration continues here when running in sidecar mode.
	cdp.logger.Info("configuring xDS and Envoy")
	err = cdp.setupXDSServer()
	if err != nil {
		return err
	}
	go cdp.startXDSServer(ctx)

	bootstrapParams, err := cdp.getBootstrapParams(ctx)
	if err != nil {
		cdp.logger.Error("failed to get bootstrap params", "error", err)
		return fmt.Errorf("failed to get bootstrap config: %w", err)
	}
	cdp.logger.Debug("generated envoy bootstrap params", "params", bootstrapParams)

	// start up DNS server with envoy bootstrap params.
	if err = cdp.startDNSProxy(ctx, cdp.cfg.DNSServer, bootstrapParams.Namespace, bootstrapParams.Partition); err != nil {
		cdp.logger.Error("failed to start the dns proxy", "error", err)
		return err
	}

	bootstrapCfg, cfg, err := cdp.bootstrapConfig(bootstrapParams)
	if err != nil {
		cdp.logger.Error("failed to get bootstrap config", "error", err)
		return fmt.Errorf("failed to get bootstrap config: %w", err)
	}
	cdp.logger.Debug("generated envoy bootstrap config", "config", string(cfg))

	cdp.logger.Info("configuring envoy and xDS")

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
	if err = cdp.lifecycleConfig.startLifecycleManager(ctx); err != nil {
		cdp.logger.Error("failed to start lifecycle manager", "error", err)
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
	return <-doneCh
}

func (cdp *ConsulDataplane) startDNSProxy(ctx context.Context,
	dnsConfig *DNSServerConfig, namespace, partition string) error {
	dnsClientInterface := pbdns.NewDNSServiceClient(cdp.serverConn)

	dnsServer, err := dns.NewDNSServer(dns.DNSServerParams{
		BindAddr:  dnsConfig.BindAddr,
		Port:      dnsConfig.Port,
		Client:    dnsClientInterface,
		Logger:    cdp.logger,
		Partition: partition,
		Namespace: namespace,
		Token:     cdp.aclToken,
	})
	if err == dns.ErrServerDisabled {
		cdp.logger.Info("dns server disabled: configure the Consul DNS port to enable")
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
		ExecutablePath:  cdp.cfg.Envoy.ExecutablePath,
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
