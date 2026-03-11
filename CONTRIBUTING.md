# Contributing to Agent Platform

Thanks for your interest in contributing! This guide will help you get started.

## Prerequisites

- Go >= 1.21
- Docker
- [kind](https://kind.sigs.k8s.io/)
- [kubebuilder](https://book.kubebuilder.io/quick-start.html#installation)
- [kustomize](https://kubectl.docs.kubernetes.io/installation/kustomize/)
- kubectl
- make

## Local Development Setup

### 1. Clone and build

```bash
git clone https://github.com/ezequiel/agent-platform.git
cd agent-platform
go build ./...
```

### 2. Run tests and lint

```bash
make test   # unit + controller tests
make lint   # golangci-lint
```

### 3. Create a local cluster

```bash
make kind-create
make install
make docker-build IMG=agent-platform:dev
kind load docker-image agent-platform:dev --name agent-platform
make deploy IMG=agent-platform:dev
```

### 4. Deploy samples and test

```bash
make deploy-samples
make test-flow
```

## Development Workflow

### Modifying CRD types

1. Edit the type definitions in `api/v1/*_types.go`
2. Regenerate:
   ```bash
   make generate    # DeepCopy methods
   make manifests   # CRD YAMLs and RBAC
   ```
3. Update controllers if the new fields affect reconciliation logic
4. Update sample manifests in `manifests/`

### Modifying the Scheduler

1. Edit `scheduler/scheduler.go`
2. Add or update tests in `scheduler/scheduler_test.go`
3. Run tests:
   ```bash
   go test ./scheduler/ -v
   ```

### Modifying Controllers

1. Edit files in `internal/controller/`
2. If you change RBAC markers, run `make manifests`
3. Rebuild and redeploy:
   ```bash
   make docker-build IMG=agent-platform:dev
   kind load docker-image agent-platform:dev --name agent-platform
   make deploy IMG=agent-platform:dev
   ```

### Testing your changes end-to-end

```bash
# Clean previous state
kubectl delete tasks --all
kubectl delete budgets --all
kubectl delete agents --all

# Redeploy
make deploy-samples
make test-flow

# Check logs
kubectl logs deploy/agent-platform-controller-manager -n agent-platform-system
```

## Code Conventions

- **Go**: Follow standard Go conventions (`gofmt`, `go vet`)
- **Kubebuilder markers**: Don't remove `// +kubebuilder:scaffold:*` comments
- **Auto-generated files**: Never edit files in `config/crd/bases/`, `config/rbac/role.yaml`, or `zz_generated.*.go`
- **Logging**: Use structured logging via `logf.FromContext(ctx)`. Start messages with capital letter, no trailing period
- **Controllers**: Keep reconciliation idempotent — safe to run multiple times

## Project Structure Quick Reference

| Path | What it is | Editable? |
|------|-----------|-----------|
| `api/v1/*_types.go` | CRD definitions | Yes |
| `api/v1/zz_generated.deepcopy.go` | Auto-generated DeepCopy | No |
| `internal/controller/*.go` | Controllers | Yes |
| `scheduler/scheduler.go` | Scheduler logic | Yes |
| `scheduler/scheduler_test.go` | Scheduler tests | Yes |
| `manifests/` | Sample CRs | Yes |
| `config/` | Kubebuilder config | No (auto-generated) |
| `cmd/main.go` | Entrypoint | Yes (carefully) |

## Areas for Contribution

See the roadmap in the README. Some ideas:

- **Budget controller**: Replace in-memory budget with Redis
- **Task queue**: Integrate NATS for async task dispatch
- **Metrics**: Add Prometheus metrics for scheduling decisions
- **Token cost tracking**: Real token counting per skill execution
- **More pool strategies**: Weighted pools, priority-based selection
- **Skill validation**: Verify Skill CRs reference valid container images
- **Tests**: Add integration tests, e2e tests with envtest

## Submitting Changes

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/my-change`
3. Make your changes
4. Ensure tests and lint pass:
   ```bash
   make test
   make lint
   ```
5. Commit with a descriptive message
6. Open a Pull Request

## Cleanup

```bash
make undeploy
make kind-delete
```
