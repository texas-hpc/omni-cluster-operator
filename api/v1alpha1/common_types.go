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
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// Finalizer is attached to resources that own remote Omni lifecycle.
	Finalizer = "omni.texas-hpc.org/finalizer"

	// ConditionReady reports whether the resource is ready for use.
	ConditionReady = "Ready"
	// ConditionReachable reports whether the operator can reach an Omni connection.
	ConditionReachable = "Reachable"
	// ConditionAccepted reports whether a child template document is attached to an OmniCluster.
	ConditionAccepted = "Accepted"
	// ConditionRendered reports whether a generated artifact has been rendered and cached.
	ConditionRendered = "Rendered"
	// ConditionValidated reports whether the assembled Omni cluster template validates.
	ConditionValidated = "Validated"
	// ConditionSynced reports whether the desired template is synced to Omni.
	ConditionSynced = "Synced"

	ReasonAccepted          = "Accepted"
	ReasonConnectionReady   = "ConnectionReady"
	ReasonConnectionFailed  = "ConnectionFailed"
	ReasonDeleteFailed      = "DeleteFailed"
	ReasonDeleting          = "Deleting"
	ReasonMissingCluster    = "MissingCluster"
	ReasonMissingConnection = "MissingConnection"
	ReasonMissingSecret     = "MissingSecret"
	ReasonMissingTemplate   = "MissingTemplate"
	ReasonReconcileFailed   = "ReconcileFailed"
	ReasonRendered          = "Rendered"
	ReasonRenderFailed      = "RenderFailed"
	ReasonStatusFailed      = "StatusFailed"
	ReasonSuspended         = "Suspended"
	ReasonSyncFailed        = "SyncFailed"
	ReasonSynced            = "Synced"
	ReasonValidated         = "Validated"
	ReasonValidationFailed  = "ValidationFailed"
)

// LocalObjectReference names another Omni CR in the same namespace.
type LocalObjectReference struct {
	// name is the referenced object name.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// SecretKeySelector identifies one key in a Secret.
type SecretKeySelector struct {
	// name is the Secret name.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// key is the Secret data key.
	// +kubebuilder:default:=serviceAccountKey
	// +required
	Key string `json:"key"`
}

// OmniAuthSpec configures non-interactive Omni authentication.
type OmniAuthSpec struct {
	// serviceAccountSecretRef points at a Secret key containing the base64 Omni service account key.
	// Create this value with `omnictl serviceaccount create`.
	// +required
	ServiceAccountSecretRef SecretKeySelector `json:"serviceAccountSecretRef"`
}

// OmniConnectionRef references an OmniConnection in the same namespace.
type OmniConnectionRef = LocalObjectReference

// OmniClusterRef references an OmniCluster in the same namespace.
type OmniClusterRef = LocalObjectReference

// ClusterDeletePolicy controls what happens in Omni when an OmniCluster is deleted.
type ClusterDeletePolicy struct {
	// orphan leaves the remote Omni cluster and template-managed resources intact.
	// +optional
	Orphan bool `json:"orphan,omitempty"`

	// destroyMachines forcefully removes disconnected nodes while deleting template resources.
	// +optional
	DestroyMachines bool `json:"destroyMachines,omitempty"`
}

// Descriptors are copied to Omni resources generated from a template document.
type Descriptors struct {
	// labels are applied to the generated Omni resource.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// annotations are applied to the generated Omni resource.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// Patch is a Talos machine configuration patch reference used by Omni cluster templates.
type Patch struct {
	// file is a patch file path relative to the OmniCluster spec.templateRoot mounted in the operator.
	// Prefer inline for GitOps CRs unless the operator deployment mounts this file tree.
	// +optional
	File string `json:"file,omitempty"`

	// name is the human-readable patch name.
	// +optional
	Name string `json:"name,omitempty"`

	// idOverride overrides Omni's generated config patch ID.
	// +optional
	IDOverride string `json:"idOverride,omitempty"`

	// labels are applied to the generated config patch.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// annotations are applied to the generated config patch.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// inline is a Talos strategic machine configuration patch.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Inline *apiextensionsv1.JSON `json:"inline,omitempty"`
}

// KubernetesManifest is an Omni template Kubernetes manifest entry.
type KubernetesManifest struct {
	// name is unique within the cluster template.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// mode controls how Omni applies the manifest.
	// +kubebuilder:validation:Enum=one-time;full
	// +kubebuilder:default:=full
	// +required
	Mode string `json:"mode"`

	// inline contains Kubernetes objects to sync.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Inline []apiextensionsv1.JSON `json:"inline,omitempty"`

	// file is a manifest file path relative to the OmniCluster spec.templateRoot mounted in the operator.
	// +optional
	File string `json:"file,omitempty"`
}

// KubernetesSpec selects the Kubernetes version and optional manifests.
type KubernetesSpec struct {
	// version is the Kubernetes version to use, for example v1.35.0.
	// +required
	// +kubebuilder:validation:Pattern=`^v[0-9]+\.[0-9]+\.[0-9]+$`
	Version string `json:"version"`

	// manifests are synced to the workload cluster by Omni.
	// +optional
	Manifests []KubernetesManifest `json:"manifests,omitempty"`
}

// TalosSpec selects the Talos version.
type TalosSpec struct {
	// version is the Talos version to use, for example v1.13.2.
	// +required
	// +kubebuilder:validation:Pattern=`^v[0-9]+\.[0-9]+\.[0-9]+$`
	Version string `json:"version"`
}

// BackupConfiguration configures Omni-managed etcd backups.
type BackupConfiguration struct {
	// interval is a Go duration. Set "0" to disable automatic backups.
	// +required
	// +kubebuilder:validation:MinLength=1
	Interval string `json:"interval"`
}

// ClusterFeatures configures optional Omni cluster features.
type ClusterFeatures struct {
	// enableWorkloadProxy enables the Omni workload proxy.
	// +optional
	EnableWorkloadProxy bool `json:"enableWorkloadProxy,omitempty"`

	// useEmbeddedDiscoveryService uses Omni's embedded discovery service instead of discovery.talos.dev.
	// +optional
	UseEmbeddedDiscoveryService bool `json:"useEmbeddedDiscoveryService,omitempty"`

	// diskEncryption configures Omni as the cluster key management server.
	// +optional
	DiskEncryption bool `json:"diskEncryption,omitempty"`

	// backupConfiguration configures automatic etcd backups.
	// +optional
	BackupConfiguration *BackupConfiguration `json:"backupConfiguration,omitempty"`
}

// MachineClass selects machines from an Omni MachineClass.
type MachineClass struct {
	// name is the Omni MachineClass name.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// size is a number of machines, or one of Omni's string size keywords such as unlimited.
	// +required
	// +kubebuilder:validation:XIntOrString
	Size intstr.IntOrString `json:"size"`
}

// BootstrapSpec configures cluster restore bootstrap behavior.
type BootstrapSpec struct {
	// clusterUUID is the UUID of the cluster to restore from.
	// +required
	// +kubebuilder:validation:MinLength=1
	ClusterUUID string `json:"clusterUUID"`

	// snapshot is the snapshot file name to restore from.
	// +required
	// +kubebuilder:validation:MinLength=1
	Snapshot string `json:"snapshot"`
}

// RollingStrategy configures rolling operation parallelism.
type RollingStrategy struct {
	// maxParallelism is the maximum number of machines to operate on in parallel.
	// +kubebuilder:validation:Minimum=1
	// +optional
	MaxParallelism *int32 `json:"maxParallelism,omitempty"`
}

// UpdateStrategy controls machine-set update, upgrade, and delete behavior.
type UpdateStrategy struct {
	// type is Rolling or Unset. When Unset, Omni applies the operation at once.
	// +kubebuilder:validation:Enum=Rolling;Unset
	// +optional
	Type string `json:"type,omitempty"`

	// rolling configures rolling operation behavior.
	// +optional
	Rolling *RollingStrategy `json:"rolling,omitempty"`
}

// MachineSetSpecFields are shared by ControlPlane and Workers template documents.
type MachineSetSpecFields struct {
	Descriptors `json:",inline"`

	// machines is the explicit list of Omni machine IDs in this set. Mutually exclusive with machineClass.
	// +optional
	// +kubebuilder:validation:MinItems=1
	Machines []string `json:"machines,omitempty"`

	// machineClass selects machines from an Omni MachineClass. Mutually exclusive with machines.
	// +optional
	MachineClass *MachineClass `json:"machineClass,omitempty"`

	// patches are applied to the generated machine set.
	// +optional
	Patches []Patch `json:"patches,omitempty"`

	// systemExtensions are installed on every machine in this set.
	// +optional
	SystemExtensions []string `json:"systemExtensions,omitempty"`

	// kernelArgs are managed for static machines in this set.
	// +optional
	KernelArgs []string `json:"kernelArgs,omitempty"`
}

// MachineInstallSpec configures Talos installation for one static machine.
type MachineInstallSpec struct {
	// disk is the install disk path.
	// +required
	// +kubebuilder:validation:MinLength=1
	Disk string `json:"disk"`
}

// CommonStatusFields are embedded into CR status structs.
type CommonStatusFields struct {
	// observedGeneration is the latest generation handled by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// conditions represent the current state of the resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ConnectionStatusFields are status fields common to resources using an OmniConnection.
type ConnectionStatusFields struct {
	// connectionRef is the last OmniConnection name observed by the controller.
	// +optional
	ConnectionRef string `json:"connectionRef,omitempty"`

	// endpoint is the last Omni endpoint observed by the controller.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`
}

// SyncStatusFields describe the last cluster sync attempt.
type SyncStatusFields struct {
	// clusterName is the Omni cluster name.
	// +optional
	ClusterName string `json:"clusterName,omitempty"`

	// templateHash is the SHA-256 hash of the last rendered desired Omni template.
	// +optional
	TemplateHash string `json:"templateHash,omitempty"`

	// lastAttemptTime is when the controller last attempted a remote Omni operation.
	// +optional
	LastAttemptTime *metav1.Time `json:"lastAttemptTime,omitempty"`

	// lastSyncTime is when the controller last successfully synced Omni.
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// lastSyncOutput is a concise summary emitted by Omni template operations.
	// +optional
	LastSyncOutput string `json:"lastSyncOutput,omitempty"`

	// lastStatusOutput is the latest status output read from Omni.
	// +optional
	LastStatusOutput string `json:"lastStatusOutput,omitempty"`
}

// SetCondition upserts a condition.
func SetCondition(conditions *[]metav1.Condition, condition metav1.Condition) {
	meta.SetStatusCondition(conditions, condition)
}

// NewCondition builds a condition with the common observedGeneration value.
func NewCondition(conditionType string, status metav1.ConditionStatus, observedGeneration int64, reason, message string) metav1.Condition {
	return metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: observedGeneration,
		Reason:             reason,
		Message:            message,
	}
}

// SecretSelector converts this API type into a corev1 selector for callers that need Kubernetes helpers.
func (s SecretKeySelector) SecretSelector() corev1.SecretKeySelector {
	return corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{Name: s.Name},
		Key:                  s.Key,
	}
}
