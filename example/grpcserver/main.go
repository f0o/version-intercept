// Command grpcserver runs the example Greeter service over raw gRPC
// (google.golang.org/grpc), enforced by the routergrpc server interceptors. It
// mirrors the ConnectRPC example server but demonstrates the raw-gRPC adapter:
// the interceptors negotiate the microversion from the incoming Api-Version
// metadata, reject methods outside their lifecycle, and gate request/response
// fields — all from the .proto annotations, with a single implementation.
//
// The gRPC listen address is configurable via the GRPC_ADDR environment variable
// (default :8081), and a companion HTTP endpoint serves the Prometheus /metrics
// recorded by the interceptors on METRICS_ADDR (default :8082). It shuts down
// gracefully on SIGINT/SIGTERM.
package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"

	greeterv1 "go.f0o.dev/version-intercept/example/gen-modern/greeter/v1"
	"go.f0o.dev/version-intercept/example/service/grpcimpl"
	"go.f0o.dev/version-intercept/router"
	routergrpc "go.f0o.dev/version-intercept/router/grpc"
)

// defaultAddr is the gRPC listen address used when GRPC_ADDR is not set.
const defaultAddr = ":8081"

// defaultMetricsAddr is the HTTP metrics listen address used when METRICS_ADDR
// is not set. gRPC has no HTTP endpoint of its own, so the recorded interceptor
// metrics are exposed on this companion listener.
const defaultMetricsAddr = ":8082"

// main wires the greeter gRPC service behind the routergrpc versioning
// interceptors and serves it until a termination signal arrives.
func main() {
	if err := run(); err != nil {
		log.Fatalf("grpc server error: %v", err)
	}
}

// run configures and runs the gRPC server, returning a non-nil error only on an
// unexpected startup or serving failure.
func run() error {
	reg := prometheus.NewRegistry()
	metrics, err := router.NewMetrics(reg)
	if err != nil {
		return err
	}

	headerCfg := router.DefaultHeaderConfig("greeter")
	srv := grpc.NewServer(
		grpc.UnaryInterceptor(routergrpc.UnaryServerInterceptor(
			routergrpc.WithHeaderConfig(headerCfg),
			routergrpc.WithMetrics(metrics),
		)),
		grpc.StreamInterceptor(routergrpc.StreamServerInterceptor(
			routergrpc.WithHeaderConfig(headerCfg),
			routergrpc.WithMetrics(metrics),
		)),
	)
	greeterv1.RegisterGreeterServiceServer(srv, grpcimpl.New())

	addr := defaultAddr
	if v := os.Getenv("GRPC_ADDR"); v != "" {
		addr = v
	}
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	metricsAddr := defaultMetricsAddr
	if v := os.Getenv("METRICS_ADDR"); v != "" {
		metricsAddr = v
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	metricsSrv := &http.Server{
		Addr:              metricsAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// #nosec G706 -- listen addresses are local server configuration
	log.Printf("version-intercept greeter (gRPC) listening on %s (supported 1.0..2.1), metrics at %s/metrics", addr, metricsAddr)

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(lis) }()
	go func() {
		if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		log.Print("shutdown signal received, stopping gRPC server")
		srv.GracefulStop()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := metricsSrv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return nil
	}
}
