# kplane-dev/extensions

An open-source operator and CRD catalog for extending kplane control planes with third-party services.

## Overview

This repo ships three CRDs and a controller:

| Resource | Scope | Who creates it | Purpose |
|---|---|---|---|
| `PlatformExtension` | Cluster | kplane (via catalog/) | Describes a managed extension available to all projects |
| `Extension` | Namespaced | Users | User-defined extension for a single project |
| `EnabledExtension` | Namespaced | Users | Activates an extension for one or more nested control planes |

### How it works

1. `PlatformExtension` objects are defined in `catalog/` and applied to the kplane management cluster. The operator replicates them read-only into every project VCP.
2. Users create an `EnabledExtension` in their project VCP referencing a `PlatformExtension` or their own `Extension`.
3. The operator installs the extension's CRDs into the target nested control planes.

```
GKE (management cluster)
  PlatformExtension/exe-dev  ──replicated──►  Project VCP
                                                PlatformExtension/exe-dev (read-only)
                                                EnabledExtension/exe-dev
                                                  extensionRef: PlatformExtension/exe-dev
                                                  controlPlanes: [test, staging]
                                                              │
                                              install CRDs    ▼
                                                Nested CP: test, staging
                                                  ExeVM CRD installed ✓
```

## Registering a PlatformExtension

To add a new managed extension, open a PR adding a directory under `catalog/`:

```
catalog/
  your-service/
    platformextension.yaml
```

**`platformextension.yaml`**:

```yaml
apiVersion: extensions.kplane.dev/v1alpha1
kind: PlatformExtension
metadata:
  name: your-service
spec:
  displayName: Your Service
  description: >
    One or two sentences describing what this extension provides to
    kplane control planes.
  tags:
    - Tag1
    - Tag2
  crds:
    - https://raw.githubusercontent.com/your-org/your-repo/main/config/crd/bases/your.crd.yaml
```

**Requirements for catalog PRs:**

- CRD URLs must point to a stable, versioned ref (tag or commit SHA) — not `main`
- The extension must not require cluster-admin on the project VCP
- The `name` must be globally unique (use a DNS-safe slug, e.g. `exe-dev`, `cloudflare`)
- Include a brief description in the PR of what the extension does and who maintains it

## Enabling an extension

In your project VCP, create an `EnabledExtension`:

```yaml
# Enable exe.dev for all control planes in this project
apiVersion: extensions.kplane.dev/v1alpha1
kind: EnabledExtension
metadata:
  name: exe-dev
  namespace: default
spec:
  extensionRef:
    kind: PlatformExtension
    name: exe-dev
  controlPlanes:
    - "*"   # or list specific names: [test, staging]
```

## Bringing your own extension

Users can define their own extensions without contributing to the catalog:

```yaml
apiVersion: extensions.kplane.dev/v1alpha1
kind: Extension
metadata:
  name: my-custom-crds
  namespace: default
spec:
  displayName: My Custom CRDs
  description: Internal tooling CRDs.
  crds:
    - https://raw.githubusercontent.com/my-org/my-repo/v1.2.3/config/crd/my-crd.yaml
---
apiVersion: extensions.kplane.dev/v1alpha1
kind: EnabledExtension
metadata:
  name: my-custom-crds
  namespace: default
spec:
  extensionRef:
    kind: Extension
    name: my-custom-crds
  controlPlanes:
    - production
```

## Development

```bash
# Install controller-gen
go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest

# Regenerate deepcopy methods
make generate

# Generate CRD and RBAC manifests
make manifests

# Build
make build

# Run tests
make test

# Install CRDs into current cluster
make install

# Apply catalog PlatformExtensions
make catalog
```

## Catalog

| Name | Description | Tags |
|---|---|---|
| [exe-dev](catalog/exe-dev/) | Persistent Linux VMs | VMs, Sandboxes, Linux |
| [cloudflare](catalog/cloudflare/) | DNS, Workers, Tunnels, R2 | DNS, Workers, CDN |
| [vercel](catalog/vercel/) | Deployments and preview environments | Deployments, Previews |
