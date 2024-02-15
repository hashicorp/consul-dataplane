package debug

import (
	"context"
	"fmt"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// EnableDebugServer starts a local HTTP server on a given port.
// By default, it will register pprof and runtime metrics endpoints.
func EnableDebugServer(ctx context.Context, port int) {
	log := hclog.FromContext(ctx).Named("debug_server")

	router := http.NewServeMux()

	// Configure debug endpoints.
	router.HandleFunc("/debug/pprof/", pprof.Index)
	router.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	router.HandleFunc("/debug/pprof/profile", pprof.Profile)
	router.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	router.HandleFunc("/debug/pprof/trace", pprof.Trace)

	// Expose the registered metrics via HTTP.
	router.Handle("/debug/metrics", promhttp.Handler())

	addr := fmt.Sprintf(":%d", port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		log.Info("starting local debug server", "address", addr)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Error("local debug server error", "error", err)
			return
		}
	}()

	// Wait for service to exit and shutdown.
	<-ctx.Done()
	_ = srv.Shutdown(context.Background())
}
