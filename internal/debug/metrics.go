package debug

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// EnableDebugServer starts a local HTTP server on a given port.
// By default, it will register pprof and runtime metrics endpoints.
func EnableDebugServer(ctx context.Context, addr string, port int) {
	log := hclog.FromContext(ctx).Named("debug_server")

	router := http.NewServeMux()

	// Expose pprof debug endpoints.
	router.HandleFunc("/debug/pprof/", pprof.Index)
	router.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	router.HandleFunc("/debug/pprof/profile", pprof.Profile)
	router.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	router.HandleFunc("/debug/pprof/trace", pprof.Trace)

	// Expose default runtime metrics.
	router.Handle("/debug/metrics", promhttp.Handler())

	// Configure HTTP server with sane defaults.
	timeout := 10 * time.Second
	srv := &http.Server{
		Handler:           router,
		ReadTimeout:       timeout,
		ReadHeaderTimeout: timeout,
		WriteTimeout:      timeout,
		IdleTimeout:       timeout,
	}

	go func() {
		ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", addr, port))
		if err != nil {
			log.Error("local debug server tcp error", "error", err)
			return
		}
		log.Info("starting local debug server", "address", ln.Addr().String())
		if err := srv.Serve(ln); err != http.ErrServerClosed {
			log.Error("local debug server error", "error", err)
			return
		}
	}()

	// Wait for dataplane to exit, and shutdown the server immediately.
	<-ctx.Done()
	_ = srv.Close()
}
