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

### Changed

### Fixed

### Removed

### Breaking Changes

- Renamed the Kubernetes API group from `omni.texas-hpc.org/v1alpha1` to
  `omni.texashpc.com/v1alpha1`. Existing GitOps manifests and installed CRDs
  must be migrated to the new API group.
