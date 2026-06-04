/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"net/url"
	"slices"
	"strings"

	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation/field"

	omniv1alpha1 "github.com/texas-hpc/omni-cluster-operator/api/v1alpha1"
	"github.com/texas-hpc/omni-cluster-operator/internal/helmrelease"
)

const validationGroup = "omni.texashpc.com"

func invalid(kind, name string, allErrs field.ErrorList) error {
	if len(allErrs) == 0 {
		return nil
	}

	return errors.NewInvalid(schema.GroupKind{Group: validationGroup, Kind: kind}, name, allErrs)
}

func validateCluster(cluster *omniv1alpha1.OmniCluster) field.ErrorList {
	specPath := field.NewPath("spec")
	var allErrs field.ErrorList

	if cluster.Spec.DeletePolicy.Orphan && cluster.Spec.DeletePolicy.DestroyMachines {
		allErrs = append(allErrs, field.Invalid(specPath.Child("deletePolicy"), cluster.Spec.DeletePolicy, "orphan and destroyMachines cannot both be true"))
	}
	if cluster.Spec.SyncInterval.Duration < 0 {
		allErrs = append(allErrs, field.Invalid(specPath.Child("syncInterval"), cluster.Spec.SyncInterval.String(), "syncInterval must not be negative"))
	}

	allErrs = append(allErrs, validatePatches(specPath.Child("patches"), cluster.Spec.Patches)...)
	allErrs = append(allErrs, validateManifests(specPath.Child("kubernetes", "manifests"), cluster.Spec.Kubernetes.Manifests)...)

	return allErrs
}

func clusterWarnings(cluster *omniv1alpha1.OmniCluster) []string {
	if len(cluster.Spec.KernelArgs) == 0 {
		return nil
	}

	return []string{"cluster-level kernelArgs are valid only when every machine set uses static machines instead of machineClass"}
}

func validateControlPlane(controlPlane *omniv1alpha1.OmniControlPlane) field.ErrorList {
	return validateMachineSet(field.NewPath("spec"), controlPlane.Spec.MachineSetSpecFields)
}

func validateWorkers(workers *omniv1alpha1.OmniWorkers) field.ErrorList {
	specPath := field.NewPath("spec")
	allErrs := validateMachineSet(specPath, workers.Spec.MachineSetSpecFields)
	if workers.Spec.WorkerSetName == "control-planes" {
		allErrs = append(allErrs, field.Invalid(specPath.Child("workerSetName"), workers.Spec.WorkerSetName, "workerSetName is reserved by Omni"))
	}

	return allErrs
}

func validateMachine(machine *omniv1alpha1.OmniMachine) field.ErrorList {
	specPath := field.NewPath("spec")
	var allErrs field.ErrorList

	machineID := machine.Spec.MachineID
	if machineID == "" {
		machineID = machine.Name
	}
	if _, err := uuid.Parse(machineID); err != nil {
		allErrs = append(allErrs, field.Invalid(specPath.Child("machineID"), machineID, "machineID must be a UUID; set spec.machineID when metadata.name is not the Omni machine ID"))
	}

	allErrs = append(allErrs, validatePatches(specPath.Child("patches"), machine.Spec.Patches)...)

	return allErrs
}

func validateKubeconfigExport(item *omniv1alpha1.OmniKubeconfigExport) field.ErrorList {
	specPath := field.NewPath("spec")
	var allErrs field.ErrorList

	if strings.TrimSpace(item.Spec.ClusterRef.Name) == "" {
		allErrs = append(allErrs, field.Required(specPath.Child("clusterRef", "name"), "clusterRef.name is required"))
	}
	if strings.TrimSpace(item.Spec.TargetSecretRef.Name) == "" {
		allErrs = append(allErrs, field.Required(specPath.Child("targetSecretRef", "name"), "target Secret name is required"))
	}
	if item.Spec.TargetSecretRef.Key != "" && strings.TrimSpace(item.Spec.TargetSecretRef.Key) == "" {
		allErrs = append(allErrs, field.Required(specPath.Child("targetSecretRef", "key"), "target Secret key must not be blank"))
	}
	if strings.TrimSpace(item.Spec.ServiceAccount.User) == "" {
		allErrs = append(allErrs, field.Required(specPath.Child("serviceAccount", "user"), "service account user is required"))
	}
	if len(item.Spec.ServiceAccount.Groups) == 0 {
		allErrs = append(allErrs, field.Required(specPath.Child("serviceAccount", "groups"), "at least one service account group is required"))
	}

	seenGroups := map[string]int{}
	for i, group := range item.Spec.ServiceAccount.Groups {
		itemPath := specPath.Child("serviceAccount", "groups").Index(i)
		group = strings.TrimSpace(group)
		if group == "" {
			allErrs = append(allErrs, field.Required(itemPath, "service account group must not be blank"))
			continue
		}
		if previous, ok := seenGroups[group]; ok {
			allErrs = append(allErrs, field.Duplicate(itemPath, group), field.Duplicate(specPath.Child("serviceAccount", "groups").Index(previous), group))
		}
		seenGroups[group] = i
		if group == omniv1alpha1.KubeconfigClusterAdminGroup && !item.Spec.ServiceAccount.AllowClusterAdmin {
			allErrs = append(allErrs, field.Forbidden(itemPath, "system:masters requires serviceAccount.allowClusterAdmin: true"))
		}
	}

	if item.Spec.TTL.Duration <= 0 {
		allErrs = append(allErrs, field.Invalid(specPath.Child("ttl"), item.Spec.TTL.String(), "ttl must be greater than zero"))
	}
	if item.Spec.RenewBefore != nil {
		if item.Spec.RenewBefore.Duration <= 0 {
			allErrs = append(allErrs, field.Invalid(specPath.Child("renewBefore"), item.Spec.RenewBefore.String(), "renewBefore must be greater than zero"))
		} else if item.Spec.TTL.Duration > 0 && item.Spec.RenewBefore.Duration >= item.Spec.TTL.Duration {
			allErrs = append(allErrs, field.Invalid(specPath.Child("renewBefore"), item.Spec.RenewBefore.String(), "renewBefore must be less than ttl"))
		}
	}

	switch item.Spec.DeletionPolicy {
	case omniv1alpha1.KubeconfigExportDeletionPolicyDelete, omniv1alpha1.KubeconfigExportDeletionPolicyOrphan:
	case "":
		allErrs = append(allErrs, field.Required(specPath.Child("deletionPolicy"), "deletionPolicy is required"))
	default:
		allErrs = append(allErrs, field.NotSupported(
			specPath.Child("deletionPolicy"),
			item.Spec.DeletionPolicy,
			[]string{
				string(omniv1alpha1.KubeconfigExportDeletionPolicyDelete),
				string(omniv1alpha1.KubeconfigExportDeletionPolicyOrphan),
			},
		))
	}

	return allErrs
}

func validateHelmRelease(item *omniv1alpha1.OmniHelmRelease) field.ErrorList {
	specPath := field.NewPath("spec")
	chartPath := specPath.Child("chart")
	var allErrs field.ErrorList

	if strings.TrimSpace(item.Spec.ClusterRef.Name) == "" {
		allErrs = append(allErrs, field.Required(specPath.Child("clusterRef", "name"), "clusterRef.name is required"))
	}
	if strings.TrimSpace(item.Spec.KubeconfigSecretRef.Name) == "" {
		allErrs = append(allErrs, field.Required(specPath.Child("kubeconfigSecretRef", "name"), "kubeconfig Secret name is required"))
	}
	if item.Spec.KubeconfigSecretRef.Key != "" && strings.TrimSpace(item.Spec.KubeconfigSecretRef.Key) == "" {
		allErrs = append(allErrs, field.Required(specPath.Child("kubeconfigSecretRef", "key"), "kubeconfig Secret key must not be blank"))
	}
	if strings.TrimSpace(item.Spec.ReleaseName) != item.Spec.ReleaseName {
		allErrs = append(allErrs, field.Invalid(specPath.Child("releaseName"), item.Spec.ReleaseName, "releaseName must not include leading or trailing whitespace"))
	}
	if strings.TrimSpace(item.Spec.Namespace) != item.Spec.Namespace {
		allErrs = append(allErrs, field.Invalid(specPath.Child("namespace"), item.Spec.Namespace, "namespace must not include leading or trailing whitespace"))
	}
	if strings.TrimSpace(item.Spec.Chart.Repository) == "" {
		allErrs = append(allErrs, field.Required(chartPath.Child("repository"), "chart.repository is required"))
	} else if !isAbsoluteURL(item.Spec.Chart.Repository) {
		allErrs = append(allErrs, field.Invalid(chartPath.Child("repository"), item.Spec.Chart.Repository, "chart.repository must be an absolute URL"))
	}
	if strings.TrimSpace(item.Spec.Chart.Chart) == "" {
		allErrs = append(allErrs, field.Required(chartPath.Child("chart"), "chart.chart is required"))
	}
	if strings.TrimSpace(item.Spec.Chart.Version) == "" {
		allErrs = append(allErrs, field.Required(chartPath.Child("version"), "chart.version is required"))
	}
	if item.Spec.Timeout != nil && item.Spec.Timeout.Duration <= 0 {
		allErrs = append(allErrs, field.Invalid(specPath.Child("timeout"), item.Spec.Timeout.String(), "timeout must be greater than zero"))
	}
	if item.Spec.MaxHistory < 0 {
		allErrs = append(allErrs, field.Invalid(specPath.Child("maxHistory"), item.Spec.MaxHistory, "maxHistory must not be negative"))
	}
	switch helmrelease.DeletionPolicy(item) {
	case omniv1alpha1.HelmReleaseDeletionPolicyUninstall, omniv1alpha1.HelmReleaseDeletionPolicyOrphan:
	default:
		allErrs = append(allErrs, field.NotSupported(
			specPath.Child("deletionPolicy"),
			item.Spec.DeletionPolicy,
			[]string{
				string(omniv1alpha1.HelmReleaseDeletionPolicyUninstall),
				string(omniv1alpha1.HelmReleaseDeletionPolicyOrphan),
			},
		))
	}
	if _, err := helmrelease.Values(item); err != nil {
		allErrs = append(allErrs, field.Invalid(chartPath.Child("values"), item.Spec.Chart.Values, err.Error()))
	}

	return allErrs
}

func validateSecretSync(item *omniv1alpha1.OmniSecretSync) field.ErrorList {
	specPath := field.NewPath("spec")
	var allErrs field.ErrorList

	if strings.TrimSpace(item.Spec.ClusterRef.Name) == "" {
		allErrs = append(allErrs, field.Required(specPath.Child("clusterRef", "name"), "clusterRef.name is required"))
	}
	if strings.TrimSpace(item.Spec.KubeconfigSecretRef.Name) == "" {
		allErrs = append(allErrs, field.Required(specPath.Child("kubeconfigSecretRef", "name"), "kubeconfig Secret name is required"))
	}
	if item.Spec.KubeconfigSecretRef.Key != "" && strings.TrimSpace(item.Spec.KubeconfigSecretRef.Key) == "" {
		allErrs = append(allErrs, field.Required(specPath.Child("kubeconfigSecretRef", "key"), "kubeconfig Secret key must not be blank"))
	}
	if strings.TrimSpace(item.Spec.SourceSecretRef.Name) == "" {
		allErrs = append(allErrs, field.Required(specPath.Child("sourceSecretRef", "name"), "source Secret name is required"))
	}
	if strings.TrimSpace(item.Spec.TargetSecretRef.Name) == "" {
		allErrs = append(allErrs, field.Required(specPath.Child("targetSecretRef", "name"), "target Secret name is required"))
	}
	if strings.TrimSpace(item.Spec.TargetSecretRef.Namespace) == "" {
		allErrs = append(allErrs, field.Required(specPath.Child("targetSecretRef", "namespace"), "target Secret namespace is required"))
	}
	if item.Spec.Type != "" && strings.TrimSpace(string(item.Spec.Type)) == "" {
		allErrs = append(allErrs, field.Invalid(specPath.Child("type"), item.Spec.Type, "type must not be blank"))
	}
	switch item.Spec.DeletionPolicy {
	case omniv1alpha1.SecretSyncDeletionPolicyDelete, omniv1alpha1.SecretSyncDeletionPolicyOrphan:
	case "":
		allErrs = append(allErrs, field.Required(specPath.Child("deletionPolicy"), "deletionPolicy is required"))
	default:
		allErrs = append(allErrs, field.NotSupported(
			specPath.Child("deletionPolicy"),
			item.Spec.DeletionPolicy,
			[]string{
				string(omniv1alpha1.SecretSyncDeletionPolicyDelete),
				string(omniv1alpha1.SecretSyncDeletionPolicyOrphan),
			},
		))
	}

	return allErrs
}

func helmReleaseWarnings(item *omniv1alpha1.OmniHelmRelease) []string {
	if item.Spec.KubeconfigSecretRef.Name == "" {
		return nil
	}

	return []string{"OmniHelmRelease uses a workload-cluster kubeconfig Secret and writes directly to that cluster"}
}

func secretSyncWarnings(item *omniv1alpha1.OmniSecretSync) []string {
	if item.Spec.KubeconfigSecretRef.Name == "" {
		return nil
	}

	return []string{"OmniSecretSync uses a workload-cluster kubeconfig Secret and writes Secret data directly to that cluster"}
}

func kubeconfigExportWarnings(item *omniv1alpha1.OmniKubeconfigExport) []string {
	if !item.Spec.ServiceAccount.AllowClusterAdmin {
		return nil
	}

	for _, group := range item.Spec.ServiceAccount.Groups {
		if strings.TrimSpace(group) == omniv1alpha1.KubeconfigClusterAdminGroup {
			return []string{"system:masters exports a cluster-admin kubeconfig into the management cluster"}
		}
	}

	return nil
}

func validateConnection(connection *omniv1alpha1.OmniConnection) field.ErrorList {
	specPath := field.NewPath("spec")
	var allErrs field.ErrorList

	if strings.TrimSpace(connection.Spec.Auth.ServiceAccountSecretRef.Name) == "" {
		allErrs = append(allErrs, field.Required(specPath.Child("auth", "serviceAccountSecretRef", "name"), "service account Secret name is required"))
	}
	if strings.TrimSpace(connection.Spec.Auth.ServiceAccountSecretRef.Key) == "" {
		allErrs = append(allErrs, field.Required(specPath.Child("auth", "serviceAccountSecretRef", "key"), "service account Secret key is required"))
	}

	return allErrs
}

func connectionWarnings(connection *omniv1alpha1.OmniConnection) []string {
	if !connection.Spec.InsecureSkipTLSVerify {
		return nil
	}

	return []string{"insecureSkipTLSVerify disables Omni endpoint TLS certificate verification"}
}

func isAbsoluteURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme != "" && parsed.Host != ""
}

func validateMachineSet(specPath *field.Path, spec omniv1alpha1.MachineSetSpecFields) field.ErrorList {
	var allErrs field.ErrorList

	allErrs = append(allErrs, validateMachineClass(specPath.Child("machineClass"), spec.MachineClass)...)
	allErrs = append(allErrs, validateMachineIDs(specPath.Child("machines"), spec.Machines)...)
	allErrs = append(allErrs, validatePatches(specPath.Child("patches"), spec.Patches)...)

	if spec.MachineClass != nil && len(spec.KernelArgs) > 0 {
		allErrs = append(allErrs, field.Invalid(specPath.Child("kernelArgs"), spec.KernelArgs, "kernelArgs are supported only for static machine sets"))
	}

	return allErrs
}

func validateMachineClass(path *field.Path, machineClass *omniv1alpha1.MachineClass) field.ErrorList {
	if machineClass == nil {
		return nil
	}

	switch machineClass.Size.Type {
	case intstr.Int:
		if machineClass.Size.IntVal <= 0 {
			return field.ErrorList{field.Invalid(path.Child("size"), machineClass.Size.IntVal, "machineClass size must be greater than zero")}
		}
	case intstr.String:
		allowed := []string{"unlimited", "infinity", "∞"}
		if !slices.Contains(allowed, machineClass.Size.StrVal) {
			return field.ErrorList{field.NotSupported(path.Child("size"), machineClass.Size.StrVal, allowed)}
		}
	}

	return nil
}

func validateMachineIDs(path *field.Path, ids []string) field.ErrorList {
	var allErrs field.ErrorList
	seen := map[string]int{}

	for i, id := range ids {
		itemPath := path.Index(i)
		if _, err := uuid.Parse(id); err != nil {
			allErrs = append(allErrs, field.Invalid(itemPath, id, "machine ID must be a UUID"))
		}
		if previous, ok := seen[id]; ok {
			allErrs = append(allErrs, field.Duplicate(itemPath, id), field.Duplicate(path.Index(previous), id))
		}
		seen[id] = i
	}

	return allErrs
}

func validatePatches(path *field.Path, patches []omniv1alpha1.Patch) field.ErrorList {
	var allErrs field.ErrorList

	for i, patch := range patches {
		itemPath := path.Index(i)
		sourceCount := 0
		if patch.File != "" {
			sourceCount++
		}
		if patch.Inline != nil && len(patch.Inline.Raw) > 0 {
			sourceCount++
		}

		switch sourceCount {
		case 0:
			allErrs = append(allErrs, field.Required(itemPath, "patch requires exactly one of file or inline"))
		case 1:
		default:
			allErrs = append(allErrs, field.Invalid(itemPath, patch, "patch must not set both file and inline"))
		}
	}

	return allErrs
}

func validateManifests(path *field.Path, manifests []omniv1alpha1.KubernetesManifest) field.ErrorList {
	var allErrs field.ErrorList

	for i, manifest := range manifests {
		itemPath := path.Index(i)
		sourceCount := 0
		if manifest.File != "" {
			sourceCount++
		}
		if len(manifest.Inline) > 0 {
			sourceCount++
		}

		switch sourceCount {
		case 0:
			allErrs = append(allErrs, field.Required(itemPath, "manifest requires exactly one of file or inline"))
		case 1:
		default:
			allErrs = append(allErrs, field.Invalid(itemPath, manifest, "manifest must not set both file and inline"))
		}
	}

	return allErrs
}
