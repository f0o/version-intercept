<!--
PR titles are validated against Conventional Commits (e.g. `feat: ...`, `fix(router): ...`, `docs: ...`).
-->

## Summary

Describe what this PR changes and why.

## Related issues

Closes #

## Type of change

- [ ] Bug fix (`fix`)
- [ ] Feature (`feat`)
- [ ] Breaking change (`feat!:` / `fix!:` or `BREAKING CHANGE`)
- [ ] Protobuf / BSR schema change (`proto/`)
- [ ] Documentation (`docs`)
- [ ] CI / tooling (`ci` / `chore`)

## Affected modules

- [ ] Core (`.`)
- [ ] `router/connect`
- [ ] `router/grpc`
- [ ] `autopin/connect`
- [ ] `autopin/grpc`
- [ ] `example`
- [ ] `proto/` (BSR: `buf.build/f0o/version-intercept`)

## Checklist

- [ ] PR title follows Conventional Commits (`<type>[optional scope]: <description>`)
- [ ] `go vet ./...` and `go build ./...` pass across affected modules
- [ ] `go test -race ./...` passes across affected modules
- [ ] `gofmt -l .` prints nothing
- [ ] Added/updated tests for the change
- [ ] If `.proto` files changed: `buf lint` and `buf format -d --exit-code` pass, `make generate` was run, and any intentional breaking change is labeled `buf skip breaking`
- [ ] Updated `CHANGELOG.md` and `README.md` if externally-observable behavior changed
