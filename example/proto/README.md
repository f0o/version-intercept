# example proto snapshots

This directory holds **two** annotated `.proto` snapshots of the same
`greeter.v1` service. They are deliberately kept separate because two generated
Go copies of the same proto package **cannot co-link** in one binary — the
protobuf registry panics at init with a duplicate-file registration. Each snapshot
therefore generates into its own Go tree (`../gen-legacy`, `../gen-modern`) and is
linked by a different binary.

| Snapshot | `service_versioning.max` | Generates | Linked by | gRPC codegen |
| --- | --- | --- | --- | --- |
| `legacy/` | `1.0` | `../gen-legacy` | `client/legacy` (T1) | no |
| `modern/` | `2.1` | `../gen-modern` | `client/modern` (T3) **and the servers** | yes |

## Why the server uses `modern` (and there is no third tree)

The `modern` snapshot is the current, full contract. A real server and an
up-to-date client legitimately compile against the same schema, so the example
servers (`../server`, `../grpcserver`) and the modern client all link
`../gen-modern`. There is no separate server-only proto tree.

Because a server that links this snapshot also links the library, the imported
`versionintercept` annotations must resolve to the **library's** Go package (not
an example copy) so only one copy of the annotations proto links. That is done via
the `go_package` override in `modern/buf.gen.yaml`. The `modern` snapshot also
enables the `buf.build/grpc/go` plugin so the raw-gRPC server gets its stubs.

## Shared annotations

The reusable `versionintercept` annotations proto lives once in the library
(`../../proto/versionintercept`). Each snapshot here symlinks it in
(`legacy/versionintercept`, `modern/versionintercept`) so `buf` can resolve the
import while the definition stays in exactly one place.

## Regenerating

From the repository root: `make generate` (runs `buf generate` in each snapshot).
