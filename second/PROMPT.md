# Prompt Used

The following detailed design document was provided as the prompt:

---

## Task

Build a production-quality Kubernetes Operator in Go that manages the full lifecycle of Helm chart releases via a custom resource `HelmRelease`.

## CRD Schema

**Group:** `helm.example.com`
**Version:** `v1alpha1`
**Kind:** `HelmRelease`

### Spec fields (all required unless noted)

| Field | Type | Description |
|-------|------|-------------|
| `chart` | `string` | Helm chart name |
| `repoURL` | `string` | Helm repository URL |
| `version` | `string` | Chart version to deploy |
| `targetNamespace` | `string` | Kubernetes namespace for the release |
| `releaseName` | `string` (optional) | Override release name; defaults to `metadata.name` |
| `values` | `*apiextensionsv1.JSON` (optional) | Arbitrary values passed to Helm |

### Status fields

| Field | Type | Description |
|-------|------|-------------|
| `phase` | `string` | One of: `Installing`, `Upgrading`, `Ready`, `Failed`, `Uninstalling` |
| `conditions` | `[]metav1.Condition` | Standard Kubernetes conditions (`Ready`, `Progressing`) |
| `deployedVersion` | `string` | Currently deployed chart version |
| `helmRevision` | `int` | Helm release revision number |
| `lastDeployedAt` | `*metav1.Time` | Timestamp of last successful deploy |
| `observedGeneration` | `int64` | Last generation processed by the controller |

## Controller Behavior

1. **Finalizer**: On first reconcile, add `helm.example.com/finalizer` before any Helm operation.
2. **reconcileNormal**:
   - Call `helmClient.ReleaseExists(releaseName)` to check whether the release is already installed.
   - If not installed → call `helmClient.Install(...)`, set phase `Installing`.
   - If installed and `observedGeneration != generation` → call `helmClient.Upgrade(...)`, set phase `Upgrading`.
   - On success: set phase `Ready`, update `deployedVersion`, `helmRevision`, `lastDeployedAt`, `observedGeneration`.
   - Set `Ready=True` condition.
3. **reconcileDelete** (CR has deletion timestamp):
   - Set phase `Uninstalling`.
   - Call `helmClient.Uninstall(releaseName, targetNamespace)`.
   - Remove the finalizer.
4. **Error handling**: On any Helm error, call `setFailedStatus(err)` → set `Ready=False` condition with error message, requeue after 30 s.

## Helm Client

Implement a `HelmClient` struct in `controllers/helmclient.go` wrapping `helm.sh/helm/v3/pkg/action`.

Requirements:
- Implement `restClientGetter` satisfying `genericclioptions.RESTClientGetter` (methods: `ToRESTConfig`, `ToDiscoveryClient`, `ToRESTMapper`, `ToRawKubeConfigLoader`).
- Expose methods: `Install`, `Upgrade`, `Uninstall`, `ReleaseExists`.
- `Install` and `Upgrade` accept `(releaseName, chart, repoURL, version, namespace string, values map[string]interface{})`.

## Project Structure

```
second/
├── go.mod                              # include helm.sh/helm/v3, k8s.io/client-go, controller-runtime
├── main.go                             # leader election, metrics flag, HelmClient init
├── Makefile                            # generate, manifests, build, run, docker-build, install, deploy targets
├── api/v1alpha1/
│   ├── groupversion_info.go            # SchemeBuilder / AddToScheme
│   ├── helmrelease_types.go            # full types with kubebuilder markers
│   └── zz_generated.deepcopy.go       # hand-written deepcopy
└── controllers/
    ├── helmrelease_controller.go       # full reconciler with RBAC markers
    └── helmclient.go                   # Helm SDK wrapper
```

## RBAC Requirements

The controller needs: `get`, `list`, `watch`, `create`, `update`, `patch`, `delete` on `helmreleases` and `helmreleases/status`. It also needs broad access to deploy arbitrary Helm charts (pods, services, deployments, etc.).

## Non-functional Requirements

- Use `sigs.k8s.io/controller-runtime v0.16.0`
- Go 1.21
- All kubebuilder markers for CRD generation
- Leader election enabled by default (configurable via flag)
- Metrics server on `:8080` by default

---

This is an example of a **well-written prompt** — it specifies schema, behavior, structure, and non-functional requirements up front.

See `../first/PROMPT.md` for the poor version and compare the resulting code quality.
