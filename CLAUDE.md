# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Purpose

A production-quality Kubernetes Operator in Go that manages Helm chart releases via a `HelmRelease` custom resource. Generated from a detailed design-doc prompt — see [PROMPT.md](./PROMPT.md) for the exact prompt used.

---

## Repository Structure

```
helm-operator/
├── CLAUDE.md
├── README.md
├── PROMPT.md
├── Dockerfile
├── Makefile
├── go.mod / go.sum
├── main.go
├── api/v1alpha1/
│   ├── groupversion_info.go
│   ├── helmrelease_types.go
│   └── zz_generated.deepcopy.go
├── chart/                    ← Helm chart for deploying the operator
├── config/crd/bases/         ← generated CRD YAML (source of truth)
├── controllers/
│   ├── helmrelease_controller.go
│   └── helmclient.go
├── docs/                     ← screenshots and assets
└── web/
    ├── server.go
    └── static/index.html
```

---

## Building

```bash
# Build and vet
go mod tidy && go build ./...

# Run locally (no cluster needed for compilation)
go run ./main.go --leader-elect=false
```

## Testing

```bash
make test   # runs unit + integration tests via envtest + Ginkgo
```
