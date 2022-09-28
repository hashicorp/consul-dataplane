package consuldp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/hashicorp/consul-server-connection-manager/discovery"
	"github.com/hashicorp/consul/proto-public/pbdataplane"
	"github.com/hashicorp/go-hclog"
	"google.golang.org/grpc"

	"github.com/hashicorp/consul-dataplane/pkg/envoy"
)

type xdsServer struct {
	listener        net.Listener
	listenerAddress string
	listenerNetwork string
	gRPCServer      *grpc.Server
	exitedCh        chan struct{}
}

type httpGetter interface {
	Get(string) (*http.Response, error)
}

type metricsServer struct {
	httpServer *http.Server
	client     httpGetter
	exitedCh   chan struct{}
}

// ConsulDataplane represents the consul-dataplane process
type ConsulDataplane struct {
	logger          hclog.Logger
	cfg             *Config
	serverConn      *grpc.ClientConn
	dpServiceClient pbdataplane.DataplaneServiceClient
	xdsServer       *xdsServer
	aclToken        string
	metricsServer   *metricsServer
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
	switch {
	case cfg.Consul == nil || cfg.Consul.Addresses == "":
		return errors.New("consul addresses not specified")
	case cfg.Consul.GRPCPort == 0:
		return errors.New("consul server gRPC port not specified")
	case cfg.Service == nil:
		return errors.New("service details not specified")
	case cfg.Service.NodeID == "" && cfg.Service.NodeName == "":
		return errors.New("node name or ID not specified")
	case cfg.Service.ServiceID == "":
		return errors.New("proxy service ID not specified")
	case cfg.Envoy == nil:
		return errors.New("envoy settings not specified")
	case cfg.Envoy.AdminBindAddress == "":
		return errors.New("envoy admin bind address not specified")
	case cfg.Envoy.AdminBindPort == 0:
		return errors.New("envoy admin bind port not specified")
	case cfg.Logging == nil:
		return errors.New("logging settings not specified")
	case cfg.XDSServer.BindAddress == "":
		return errors.New("envoy xDS bind address not specified")
	case !strings.HasPrefix(cfg.XDSServer.BindAddress, "unix://") && !net.ParseIP(cfg.XDSServer.BindAddress).IsLoopback():
		return errors.New("non-local xDS bind address not allowed")

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
	}

	return nil
}

func (cdp *ConsulDataplane) Run(ctx context.Context) error {
	cdp.logger.Info("started consul-dataplane process")

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

	err = cdp.setupXDSServer()
	if err != nil {
		return err
	}
	go cdp.startXDSServer()
	defer cdp.stopXDSServer()

	bootstrapCfg, cfg, err := cdp.bootstrapConfig(ctx)
	if err != nil {
		cdp.logger.Error("failed to get bootstrap config", "error", err)
		return fmt.Errorf("failed to get bootstrap config: %w", err)
	}
	cdp.logger.Debug("generated envoy bootstrap config", "config", string(cfg))

	proxy, err := envoy.NewProxy(cdp.envoyProxyConfig(cfg))
	if err != nil {
		cdp.logger.Error("failed to create new proxy", "error", err)
		return fmt.Errorf("failed to create new proxy: %w", err)
	}
	if err := proxy.Run(); err != nil {
		cdp.logger.Error("failed to run proxy", "error", err)
		return fmt.Errorf("failed to run proxy: %w", err)
	}

	cdp.setupMetricsServer()
	if cdp.cfg.Telemetry.UseCentralConfig {
		switch {
		case bootstrapCfg.PrometheusBindAddr != "":
			go cdp.startMetricsServer()
			defer cdp.stopMetricsServer()
		case bootstrapCfg.StatsdURL != "":
			// TODO: send merged metrics
		case bootstrapCfg.DogstatsdURL != "":
			// TODO: send merged metrics
		}
	}

	doneCh := make(chan error)
	go func() {
		select {
		case <-ctx.Done():
			if err := proxy.Stop(); err != nil {
				cdp.logger.Error("failed to stop proxy", "error", err)
			}
			doneCh <- nil
		case <-proxy.Exited():
			doneCh <- errors.New("envoy proxy exited unexpectedly")
		case <-cdp.xdsServerExited():
			if err := proxy.Stop(); err != nil {
				cdp.logger.Error("failed to stop proxy", "error", err)
			}
			doneCh <- errors.New("xDS server exited unexpectedly")
		case <-cdp.metricsServerExited():
			doneCh <- errors.New("metrics server exited unexpectedly")
		}
	}()
	return <-doneCh
}

func (cdp *ConsulDataplane) envoyProxyConfig(cfg []byte) envoy.ProxyConfig {
	setConcurrency := true
	extraArgs := cdp.cfg.Envoy.ExtraArgs
	// Users could set the concurrency as an extra args. Take that as priority for best ux
	// experience.
	for _, v := range extraArgs {
		if v == "--concurrency" {
			setConcurrency = false
		}
	}
	if setConcurrency {
		extraArgs = append(extraArgs, fmt.Sprintf("--concurrency %v", cdp.cfg.Envoy.EnvoyConcurrency))
	}

	return envoy.ProxyConfig{
		Logger:          cdp.logger,
		LogJSON:         cdp.cfg.Logging.LogJSON,
		BootstrapConfig: cfg,
		ExtraArgs:       extraArgs,
	}
}
