// Command server runs the example Greeter service. A single proto contract
// (greeter.v1) is served by two behavioral implementations — legacy (1.x) and
// modern (2.x) — selected at request time by the Api-Version microversion
// header. The versioning interceptor enforces method lifecycles and gates
// fields that are not visible at the negotiated microversion.
//
// The server demonstrates production hygiene: explicit HTTP timeouts, a
// caller-owned Prometheus registry, and graceful shutdown on SIGINT/SIGTERM.
// The listen address is configurable via the ADDR environment variable.
package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"go.f0o.dev/version-intercept/example/gen-modern/greeter/v1/greeterv1connect"
	"go.f0o.dev/version-intercept/example/service/legacy"
	"go.f0o.dev/version-intercept/example/service/modern"
	"go.f0o.dev/version-intercept/router"
	routerconnect "go.f0o.dev/version-intercept/router/connect"
)

// defaultAddr is the listen address used when ADDR is not set.
const defaultAddr = ":8080"

// main wires the legacy and modern implementations behind the version router
// with the versioning interceptor and serves them over h2c with timeouts and
// graceful shutdown.
func main() {
	if err := run(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// run configures and runs the server until it receives a termination signal,
// then shuts down gracefully. It returns a non-nil error only on an unexpected
// startup or serving failure.
func run() error {
	reg := prometheus.NewRegistry()
	metrics, err := router.NewMetrics(reg)
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	interceptors := connect.WithInterceptors(routerconnect.VersioningInterceptor(
		routerconnect.WithMetrics(metrics),
		routerconnect.WithLogger(logger),
	))

	servicePath, legacyHandler := greeterv1connect.NewGreeterServiceHandler(legacy.New(), interceptors)
	_, modernHandler := greeterv1connect.NewGreeterServiceHandler(modern.New(), interceptors)

	r := router.New("greeter", router.WithHeaderConfig(router.DefaultHeaderConfig("greeter")))
	r.Register(
		router.Version{Major: 1, Minor: 0},
		router.Range{Min: router.Version{Major: 1, Minor: 0}, Max: router.Version{Major: 1, Minor: 9}},
		legacyHandler,
	)
	r.Register(
		router.Version{Major: 2, Minor: 1},
		router.Range{Min: router.Version{Major: 2, Minor: 0}},
		modernHandler,
	)

	mux := http.NewServeMux()
	mux.Handle(servicePath, r)
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))

	addr := defaultAddr
	if v := os.Getenv("ADDR"); v != "" {
		addr = v
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           h2c.NewHandler(mux, &http2.Server{}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	min, max, _ := r.SupportedRange()
	log.Printf("version-intercept greeter listening on %s (supported %s..%s) path %s, metrics at /metrics", addr, min, max, servicePath)

	serveErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		log.Print("shutdown signal received, draining connections")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return <-serveErr
	}
}
