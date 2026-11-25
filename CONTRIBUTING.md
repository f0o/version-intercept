# Contributing to version-intercept

Thanks for your interest in contributing! This document explains how to get set
up and what we expect from contributions.

## Development setup

Requirements:

- Go 1.26+
- [`buf`](https://buf.build) 1.71+ (only needed to regenerate protobuf code;
  the generated code under `gen/` is committed so most contributions do not
  require network access to buf.build).

Common commands (see the `Makefile` for the full list):

```bash
go build ./...      # build
go vet ./...        # static checks
go test -race ./... # tests with the race detector
gofmt -l .          # formatting check (should print nothing)
make demo           # run the end-to-end example
```

## Coding standards

- **Formatting:** all Go code must be `gofmt`-clean.
- **Documentation:** follow the project's Go documentation standards —
  function-level GoDoc comments, no inline comments inside function bodies.
  Every exported symbol must be documented.
- **Tests:** new behavior must be covered by tests. Run `go test -race ./...`
  locally before opening a PR.
- **Linting:** CI runs `golangci-lint` and `gosec`; please run
  `golangci-lint run` locally if you can.

## Pull requests

1. Fork the repo and create a topic branch.
2. Make your change in small, focused commits.
3. Use a [Conventional Commits](https://www.conventionalcommits.org/) PR title
   (e.g. `feat(router): ...`, `fix(autopin): ...`, `docs: ...`); PR titles are
   validated automatically and used to categorize release notes.
4. Update `README.md` and `CHANGELOG.md` if you changed externally-observable
   behavior (config, commands, tools, defaults).
5. Ensure `go build`, `go vet`, `go test -race`, and `gofmt -l .` are all clean.
6. If you edit `.proto` files, run `buf lint`, `buf format -d --exit-code`, and
   `make generate`. Intentional breaking proto changes require the
   `buf skip breaking` PR label.
7. Open a PR describing the motivation and the change.

For maintainers cutting releases or managing Buf Schema Registry (BSR)
publishing, see [docs/RELEASING.md](docs/RELEASING.md).

## License

By contributing, you agree that your contributions will be licensed under the
GNU General Public License v3.0, the same license that covers this project (see
`LICENSE`).
