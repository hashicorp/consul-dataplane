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

	// Expose pprof debug endpoints.
	router.HandleFunc("/debug/pprof/", pprof.Index)
	router.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	router.HandleFunc("/debug/pprof/profile", pprof.Profile)
	router.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	router.HandleFunc("/debug/pprof/trace", pprof.Trace)

	// Expose default runtime metrics.
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

	// Wait for dataplane to exit, and shutdown the server.
	<-ctx.Done()
	if err := srv.Shutdown(context.Background()); err != nil {
		log.Error("error shutting down debug server", "error", err)
	}
}
