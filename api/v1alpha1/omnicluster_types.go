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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OmniClusterSpec defines the Omni Cluster template document plus operator sync settings.
type OmniClusterSpec struct {
	Descriptors `json:",inline"`

	// connectionRef selects the OmniConnection used for this cluster and all child template documents.
	// +required
	ConnectionRef OmniConnectionRef `json:"connectionRef"`

	// clusterName is the Omni cluster name. Defaults to metadata.name when omitted.
	// Changing this creates a different cluster in Omni.
	// +optional
	// +kubebuilder:validation:Pattern=`^[A-Za-z0-9_-]+$`
	ClusterName string `json:"clusterName,omitempty"`

	// kubernetes configures the Kubernetes version and optional Omni-managed manifests.
	// +required
	Kubernetes KubernetesSpec `json:"kubernetes"`

	// talos configures the Talos version.
	// +required
	Talos TalosSpec `json:"talos"`

	// features configures optional Omni cluster features.
	// +optional
	Features *ClusterFeatures `json:"features,omitempty"`

	// patches are applied at cluster scope.
	// +optional
	Patches []Patch `json:"patches,omitempty"`

	// systemExtensions are installed on every machine in the cluster.
	// +optional
	SystemExtensions []string `json:"systemExtensions,omitempty"`

	// kernelArgs are managed for static machines in this cluster.
	// +optional
	KernelArgs []string `json:"kernelArgs,omitempty"`

	// templateRoot is an optional directory in the operator container used to resolve file-based patches
	// and Kubernetes manifests. Inline specs are recommended for fully Kubernetes-native GitOps.
	// +optional
	TemplateRoot string `json:"templateRoot,omitempty"`

	// deletePolicy controls remote Omni cleanup when this resource is deleted.
	// +optional
	DeletePolicy ClusterDeletePolicy `json:"deletePolicy,omitempty"`

	// syncInterval controls periodic reconciliation even when no Kubernetes object changed.
	// +kubebuilder:default:="5m"
	// +optional
	SyncInterval metav1.Duration `json:"syncInterval,omitempty"`

	// suspend stops remote Omni sync while preserving status and finalizers.
	// +optional
	Suspend bool `json:"suspend,omitempty"`
}

// OmniClusterStatus defines the observed state of OmniCluster.
type OmniClusterStatus struct {
	CommonStatusFields     `json:",inline"`
	ConnectionStatusFields `json:",inline"`
	SyncStatusFields       `json:",inline"`

	// controlPlaneRef is the control plane document selected for the last rendered template.
	// +optional
	ControlPlaneRef string `json:"controlPlaneRef,omitempty"`

	// workersRefs are the worker documents selected for the last rendered template.
	// +optional
	WorkersRefs []string `json:"workersRefs,omitempty"`

	// machineRefs are the machine documents selected for the last rendered template.
	// +optional
	MachineRefs []string `json:"machineRefs,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// OmniCluster is the Schema for the omniclusters API
type OmniCluster struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of OmniCluster
	// +required
	Spec OmniClusterSpec `json:"spec"`

	// status defines the observed state of OmniCluster
	// +optional
	Status OmniClusterStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// OmniClusterList contains a list of OmniCluster
type OmniClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []OmniCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OmniCluster{}, &OmniClusterList{})
}
