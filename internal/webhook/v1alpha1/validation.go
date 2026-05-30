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
	"github.com/texas-hpc/omni-cluster-operator/internal/cilium"
)

const validationGroup = "omni.texas-hpc.org"

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

func validateCilium(install *omniv1alpha1.OmniCilium) field.ErrorList {
	specPath := field.NewPath("spec")
	var allErrs field.ErrorList

	if strings.TrimSpace(install.Spec.ChartVersion) == "" {
		allErrs = append(allErrs, field.Required(specPath.Child("chartVersion"), "chartVersion is required"))
	}
	if install.Spec.ChartRepository != "" {
		parsed, err := url.Parse(install.Spec.ChartRepository)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			allErrs = append(allErrs, field.Invalid(specPath.Child("chartRepository"), install.Spec.ChartRepository, "chartRepository must be an absolute URL"))
		}
	}
	if _, _, err := cilium.Values(install); err != nil {
		allErrs = append(allErrs, field.Invalid(specPath.Child("values"), install.Spec.Values, err.Error()))
	}

	return allErrs
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
