# version-intercept example

This directory is a **self-contained, nested Go module**
(`go.f0o.dev/version-intercept/example`) that demonstrates
`version-intercept`. It is a separate module on purpose: the example depends on
things the library core does not (an h2c server via `golang.org/x/net`,
`promhttp`, and multiple generated proto snapshots). Keeping it in its own module
means consumers of the library never inherit those dependencies.

It uses a `replace` directive to point at the parent library:

```
replace go.f0o.dev/version-intercept => ../
```

## The T0 → T3 story

The example demonstrates that a client's negotiated microversion is an honest
consequence of *which proto snapshot it was compiled from* — no hardcoded version
strings:

- **T0:** a frozen 1.0 proto snapshot (`proto/legacy`) with `service_versioning.max = "1.0"`.
- **T1:** `client/legacy`, generated from T0, auto-pins to `1.0`.
- **T2:** the full 2.1 proto snapshot (`proto/modern`) with `max = "2.1"`.
- **T3:** `client/modern`, generated from T2, auto-pins to `2.1`.

The `modern` (2.1) snapshot is also the contract the example servers implement,
so the server and an up-to-date client share one generated package
(`gen-modern`) — exactly as a real server and a current client would.

Auto-pinning is done by the `autopinconnect` / `autopingrpc` client interceptors
(and the core `autopin` package they build on), which read
`service_versioning.max` from the client's OWN generated descriptor.

## Adapter surfaces exercised

The example integrates **all** of the library's adapter surfaces:

| Surface | Where |
| --- | --- |
| `routerconnect` (Connect server interceptor) | `server/` |
| `autopinconnect` (Connect client interceptor) | `client/{legacy,modern}` via `internal/exampleclient` |
| `routergrpc` (raw-gRPC server interceptors) | `grpcserver/` |
| `autopingrpc` (raw-gRPC client interceptors) | `client/grpc` |


## Layout

```
example/
  go.mod                      # nested module (replace → ../)
  proto/{legacy,modern}/      # the two annotated proto snapshots (buf configs)
                              #   legacy: frozen 1.0 (client only)
                              #   modern: 2.1, shared by the modern client AND the servers (grpc codegen on)
  gen-legacy/, gen-modern/    # generated code from those snapshots; gen-modern is what the servers link
  server/                     # the modern Connect server: both impls behind the router
  grpcserver/                 # the raw-gRPC server (routergrpc interceptors) + /metrics on :8082
  service/
    legacy/                   # 1.x behavioral implementation (Connect)
    modern/                   # 2.x behavioral implementation (Connect)
    grpcimpl/                 # raw-gRPC implementation (single impl; interceptor-gated)
  client/
    legacy/                   # T1 Connect client (auto-pins 1.0)
    modern/                   # T3 Connect client (auto-pins 2.1, + explicit 1.0 override)
    grpc/                     # raw-gRPC client (autopingrpc: auto-pin + 1.0 override)
  internal/exampleclient/     # shared Connect client wiring (h2c dialer, autopinconnect options)
```

The reusable microversion annotations proto stays in the **library**
(`../proto/versionintercept`); each proto tree here symlinks it in so `buf` can
resolve the import while it remains defined in exactly one place.

## Running

From the repository root, the `Makefile` drives everything:

```bash
make demo            # Connect + raw-gRPC demos: start servers, run all clients, tear down
make grpc-demo       # just the raw-gRPC demo (routergrpc server + autopingrpc client)
make run-server      # run the Connect server in the foreground (:8080, or $ADDR)
make run-grpc-server # run the raw-gRPC server in the foreground (:8081, or $GRPC_ADDR)
make legacy-example  # run the T1 Connect client (needs a running Connect server)
make modern-example  # run the T3 Connect client (needs a running Connect server)
make grpc-example    # run the raw-gRPC client (needs a running gRPC server)
make metrics         # curl the /metrics endpoint
```

Or directly within this module:

```bash
cd example
go run ./server                 # Connect server, in one terminal
go run ./client/legacy          # in another
go run ./client/modern
go run ./grpcserver             # raw-gRPC server (:8081), in one terminal
go run ./client/grpc            # raw-gRPC client, in another
```

## What it demonstrates

- **Auto-pinning:** each client sends the microversion baked into its own
  descriptor, with no version string in its code.
- **Field gating:** at `1.0` the server strips the 2.0 `title`/`language`/
  `served_by` fields; at `2.1` it honors them.
- **Method lifecycle:** `Farewell` (introduced at `2.0`) is rejected with
  `CodeUnimplemented` when a client negotiates `1.0`.
- **Opt-in observability:** the Connect server wires `router.WithMetrics`
  (caller-owned Prometheus registry, exposed at `/metrics`) and
  `router.WithLogger` (structured `slog` logging of rejections), plus
  `router.WithHeaderConfig`. The raw-gRPC server records the same metrics and
  exposes them on a companion HTTP listener (`METRICS_ADDR`, default `:8082`),
  since gRPC has no HTTP endpoint of its own.
- **Production hygiene:** the server uses explicit HTTP timeouts and graceful
  shutdown on SIGINT/SIGTERM.
