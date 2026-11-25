# version-intercept

[![CI](https://go.f0o.dev/version-intercept/actions/workflows/ci.yml/badge.svg)](https://go.f0o.dev/version-intercept/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/go.f0o.dev/version-intercept.svg)](https://pkg.go.dev/go.f0o.dev/version-intercept)
[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](LICENSE)

OpenStack-style **microversioning** for gRPC/ConnectRPC. One schema, one endpoint;
an `Api-Version` request header selects behavior and field visibility at request
time — no forked `v1`/`v2` proto packages, no wire-format bloat.

## Problem

Behavioral evolution (pagination defaults, validation strictness, a field
becoming meaningful, a method's introduction/deprecation) doesn't warrant a
schema fork, but ships too much breakage to do in place. This library routes a
single additive schema across microversions negotiated per request, enforces
per-method/per-field lifecycle from `.proto` annotations, and lets clients pin or
opt into versions with a header.

|                | Separate `v1`/`v2` packages | Microversions (this library)                     |
| -------------- | --------------------------- | ------------------------------------------------ |
| Schema         | Forks per version           | One additive schema                              |
| Endpoint       | One per version             | One shared path                                  |
| Client opt-in  | New client + path           | A header value                                   |
| Fits           | Hard breaking redesigns     | Behavioral changes, gradual field/method rollout |

## How it works

- Client sends `Api-Version: <service> <major.minor>` (bare `<major.minor>` or
  `latest` also accepted).
- `router.Router` (an `http.Handler`) parses/negotiates the version, stores it in
  the request context, advertises `Api-Min-Version`/`Api-Max-Version`, echoes the
  served `Api-Version`, and dispatches to the handler registered for that range.
- Adapter interceptors (`router/connect`, `router/grpc`) enforce `.proto`
  microversion annotations against the negotiated version:
  - **Method lifecycle** — `introduced_in`/`removed_in` → `CodeUnimplemented`.
  - **Field gating** — request/response fields not yet introduced are cleared.
  - **Deprecation** — sets `Deprecation: true` when deprecated at the version.
  - **Streaming** — lifecycle up front, field gating on recv/send.
  - **Panic recovery** → `CodeInternal`; optional Prometheus metrics.
- Client adapters (`autopin/connect`, `autopin/grpc`) auto-pin the client's
  compiled `service_versioning.max` as the header, or override per-call.

## Install

```bash
go get go.f0o.dev/version-intercept
```

## Server

```go
import (
    "net/http"

    "connectrpc.com/connect/v2"
    "connectrpc.com/connect/v2/connecthttp"
    "go.f0o.dev/version-intercept/router"
    routerconnect "go.f0o.dev/version-intercept/router/connect"
)

r := router.New("greeter", router.WithHeaderConfig(router.DefaultHeaderConfig("greeter")))

ic := routerconnect.VersioningInterceptor(
    routerconnect.WithMetrics(m), // optional *router.Metrics
    routerconnect.WithLogger(log),
)

mount := func(impl greeterv1connect.GreeterServiceHandler) http.Handler {
    srv := connect.NewServer(ic)
    greeterv1connect.RegisterGreeterServiceHandler(srv, impl)
    mux := http.NewServeMux()
    connecthttp.Mount(mux, srv)
    return mux
}

r.Register(router.MustParseVersion("1.0"), router.Range{Min: router.Version{1, 0}, Max: router.Version{1, 9}}, mount(legacyImpl{}))
r.Register(router.MustParseVersion("2.0"), router.Range{Min: router.Version{2, 0}}, mount(modernImpl{})) // zero Max == unbounded

mux := http.NewServeMux()
mux.Handle("/"+greeterv1connect.GreeterServiceName+"/", r)
http.ListenAndServe(":8080", mux)

// In handlers:
v, _ := router.NegotiatedVersion(ctx)
```

Raw gRPC uses the same model via `router/grpc`:

```go
grpc.NewServer(
    grpc.ChainUnaryInterceptor(routergrpc.UnaryServerInterceptor(routergrpc.WithMetrics(m))),
    grpc.ChainStreamInterceptor(routergrpc.StreamServerInterceptor()),
)
```

## Client

Auto-pin to the version compiled into the client's own descriptor
(`service_versioning.max`); omit the annotation/override → no header → server
serves latest.

```go
// ConnectRPC
import (
    "connectrpc.com/connect/v2"
    "connectrpc.com/connect/v2/connecthttp"
    autopinconnect "go.f0o.dev/version-intercept/autopin/connect"
)

sd := greeterv1.File_greeter_v1_greeter_proto.Services().ByName("GreeterService")
client := greeterv1connect.NewGreeterServiceClient(
    connect.NewClient(
        connecthttp.NewTransport(httpc, base),
        autopinconnect.Interceptor(sd,
            autopinconnect.WithServiceToken("greeter"),
            autopinconnect.WithVersion("1.0"), // optional explicit pin/override
        ),
    ),
)
```

```go
// Raw gRPC
import autopingrpc "go.f0o.dev/version-intercept/autopin/grpc"

grpc.NewClient(target,
    grpc.WithUnaryInterceptor(autopingrpc.UnaryClientInterceptor(sd, autopingrpc.WithServiceToken("greeter"))),
    grpc.WithStreamInterceptor(autopingrpc.StreamClientInterceptor(sd)),
)
```

```go
// Plain net/http — stamp the header directly
import "go.f0o.dev/version-intercept/autopin"

httpc := &http.Client{Transport: autopin.AutoPinTransport("greeter", sd, nil)}
```

## Annotations

Declared inline in `.proto` (see `proto/`); parsed via
`router.MethodLifecycleOf`:

- `introduced_in` / `removed_in` — method lifecycle bounds.
- `deprecated_in` — deprecation boundary.
- per-field `introduced_in` — field gating.
- service-level `service_versioning.max` — client auto-pin source.

## Packages

| Package           | Role                                                          |
| ----------------- | ------------------------------------------------------------ |
| `router`          | Version types, `Router`, header config, annotation parsing, gating, metrics |
| `router/connect`  | ConnectRPC `VersioningInterceptor`                           |
| `router/grpc`     | gRPC server unary/stream interceptors                        |
| `autopin`         | Core `AutoPinTransport` + header helpers                     |
| `autopin/connect` | ConnectRPC client `Interceptor`                              |
| `autopin/grpc`    | gRPC client unary/stream interceptors                        |

## Demo

```bash
make demo   # runs example server + client across microversions
```

## License

GPL-3.0 — see [LICENSE](LICENSE).
