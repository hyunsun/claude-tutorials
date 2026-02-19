# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Purpose

This repository is a **prompt-quality comparison tutorial**. It demonstrates how the specificity and clarity of a prompt dramatically affects the quality of AI-generated code.

The same task — "write a Kubernetes Operator in Go that manages Helm chart releases" — is given to an AI assistant twice:

- **`first/`** — result of a **poor, vague prompt** (one sentence, no design requirements)
- **`second/`** — result of a **well-written prompt** (full design doc with schema, behavior, RBAC, and structure requirements)

Read each `PROMPT.md` to see the exact prompt used before looking at the code.

---

## Repository Structure

```
claude-tutorials/
├── CLAUDE.md            ← this file
├── first/               ← poor-prompt version
│   ├── PROMPT.md        ← the one-line prompt
│   ├── go.mod
│   ├── main.go
│   ├── api/v1alpha1/
│   │   └── helmrelease_types.go
│   └── controllers/
│       └── helmrelease_controller.go
└── second/              ← well-written-prompt version
    ├── PROMPT.md        ← the detailed design-doc prompt
    ├── go.mod
    ├── main.go
    ├── Makefile
    ├── api/v1alpha1/
    │   ├── groupversion_info.go
    │   ├── helmrelease_types.go
    │   └── zz_generated.deepcopy.go
    └── controllers/
        ├── helmrelease_controller.go
        └── helmclient.go
```

---

## Building Each Version

```bash
# Poor-prompt version — expected to surface errors (missing DeepCopy, incomplete deps)
cd first && go vet ./...

# Well-written-prompt version — expected to compile cleanly after tidy
cd second && go mod tidy && go build ./...
```

---

## Key Contrasts

| Concern | `first/` (poor prompt) | `second/` (well-written prompt) |
|---------|----------------------|--------------------------------|
| **CRD spec fields** | `chart`, `repoURL`, `version` only | + `targetNamespace`, `releaseName`, `values` |
| **Status model** | `installed: bool` | `phase`, `conditions`, `deployedVersion`, `helmRevision`, `lastDeployedAt`, `observedGeneration` |
| **DeepCopy** | Missing — won't compile | Full `zz_generated.deepcopy.go` |
| **go.mod** | Missing `helm.sh/helm/v3` and `k8s.io/client-go` | All required dependencies present |
| **Helm calls** | `fmt.Printf` + TODO | Real `helm.sh/helm/v3/pkg/action` calls via `HelmClient` |
| **Finalizer** | None — orphans Helm releases on delete | `helm.example.com/finalizer` with delete reconciliation |
| **RBAC markers** | None — generated ClusterRole is empty | Full `+kubebuilder:rbac` markers on reconciler |
| **Error handling** | No requeue on failure | `setFailedStatus` sets `Ready=False`, requeues after 30 s |
| **Leader election** | Disabled | Enabled by default via `--leader-elect` flag |
| **Metrics** | None | `:8080` metrics endpoint |
| **Makefile** | None | `generate`, `manifests`, `fmt`, `vet`, `build`, `run`, `docker-build`, `install`, `deploy` |
| **groupversion_info.go** | Inlined in types file | Separate file (standard kubebuilder layout) |
