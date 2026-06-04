# Release Notes

This file tracks minor and major release-line changes for
`omni-cluster-operator`.

Update `Unreleased` only when a pull request bumps the base minor or major
version in `version.json`. Patch-level fixes do not need manual changelog
entries; GitHub Releases can use generated commit notes for those.

When a GitHub Release is published for a minor or major line, move the relevant
entries under the released version heading.

## Unreleased

### Added

- Added `OmniSecretSync` for copying selected management-cluster Secrets into
  workload clusters using explicit workload-cluster kubeconfig Secrets.
- Added `OmniKubeconfigExport` for explicit, rotating workload-cluster
  service-account kubeconfig Secret exports.
- Added `OmniHelmRelease` for opt-in direct Helm release reconciliation against
  workload clusters using explicit kubeconfig Secrets.
- Added a `Stalled` condition on `OmniConnection` connection failures so GitOps
  health checks can fail fast instead of timing out while `Ready=False`.

### Changed

- Set the published release baseline to `1.0.0` for the stable operator
  release line.

### Fixed

### Removed

- Removed the rendered-addon API resources, controllers, CRDs, RBAC, samples,
  chart content, and docs. Use `OmniCluster.spec.kubernetes.manifests` for raw
  Omni manifest sync or `OmniHelmRelease` for workload-cluster Helm lifecycle.

### Breaking Changes

- Renamed the Kubernetes API group from `omni.texas-hpc.org/v1alpha1` to
  `omni.texashpc.com/v1alpha1`. Existing GitOps manifests and installed CRDs
  must be migrated to the new API group.
- The rendered-addon API resources are no longer served. Existing manifests for
  those APIs must be removed or migrated before upgrading.
