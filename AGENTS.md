# Repository Guidelines

## Project Structure & Module Organization

Multus CNI is a Go meta-plugin that attaches additional network interfaces to Kubernetes pods. `cmd/` contains executable entry points, including `multus`, `multus-daemon`, and `multus-shim`. Shared implementation lives in `pkg/`, especially `pkg/multus/` for CNI orchestration and `pkg/server/` for the thick-plugin daemon. Tests sit beside source files as `*_test.go`; shared test helpers live in `pkg/testing/`.

Deployment manifests are in `deployments/`, sample configurations in `examples/`, container build assets in `images/`, and documentation and diagrams in `docs/`. `hack/` holds build/test scripts; `e2e/` contains kind-based integration tests.

## Build, Test, and Development Commands

Use Linux and the Go version declared in `go.mod` (currently Go 1.26.0).

- `make build`: builds executables into `bin/` using `hack/build-go.sh`.
- `make test`: runs the full suite through `sudo ./hack/test-go.sh`, with race detection and coverage output in `coverage.out`.
- `go test ./pkg/types`: runs a focused package test; networking tests in other packages may require root privileges.
- `go fmt ./...` and `go vet ./...`: format Go code and check for suspicious constructs. CI also runs Revive.
- `make deps-update`: runs `go mod tidy` after intentional dependency changes.

For local cluster testing, follow `e2e/README.md`: install tools, generate manifests, run `setup_cluster.sh`, execute relevant test scripts, and clean up with `teardown.sh`. Docker and curl are prerequisites.

## Coding Style & Naming Conventions

Follow Effective Go and Go Code Review Comments. Use `gofmt` formatting with tabs, short lowercase package names, exported `MixedCaps` identifiers, and unexported `mixedCaps` identifiers. Match surrounding code and keep changes focused.

## Testing Guidelines

Tests use Go's testing package and Ginkgo v2/Gomega. Name test files `*_test.go` and suite entry points `*_suite_test.go`. Add regression coverage for behavior changes and use relevant e2e cases for networking or deployment changes. CI uploads coverage to Coveralls; no numeric coverage threshold is specified.

## Commit & Pull Request Guidelines

Use concise, imperative summaries. History includes scoped summaries such as `server: ...` and `fix(daemonset): ...`. Follow `CONTRIBUTING.md`: explain why and how in the body, wrap it at 72 characters, reference applicable issues with `Fixes #NUMBER`, and include a `Signed-off-by` footer (`git commit -s`). Open GitHub pull requests describing the resulting behavior, linked issues, and validation performed.
