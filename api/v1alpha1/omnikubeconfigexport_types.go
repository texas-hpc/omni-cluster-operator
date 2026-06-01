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

// KubeconfigExportDeletionPolicy controls what happens to the target Secret when OmniKubeconfigExport is deleted.
// +kubebuilder:validation:Enum=Delete;Orphan
type KubeconfigExportDeletionPolicy string

const (
	KubeconfigExportDeletionPolicyDelete KubeconfigExportDeletionPolicy = "Delete"
	KubeconfigExportDeletionPolicyOrphan KubeconfigExportDeletionPolicy = "Orphan"
)

// OmniKubeconfigServiceAccountSpec configures a service-account kubeconfig request to Omni.
type OmniKubeconfigServiceAccountSpec struct {
	// user is the Kubernetes user subject embedded in the exported kubeconfig.
	// +required
	// +kubebuilder:validation:MinLength=1
	User string `json:"user"`

	// groups are the Kubernetes groups embedded in the exported kubeconfig.
	// +required
	// +kubebuilder:validation:MinItems=1
	Groups []string `json:"groups"`
}

// OmniKubeconfigExportSpec defines a managed workload-cluster kubeconfig export.
type OmniKubeconfigExportSpec struct {
	// clusterRef points to the workload OmniCluster in the same namespace.
	// +required
	ClusterRef OmniClusterRef `json:"clusterRef"`

	// targetSecretRef selects the Secret name to write kubeconfig data into.
	// +required
	TargetSecretRef LocalObjectReference `json:"targetSecretRef"`

	// serviceAccount configures a service-account kubeconfig export.
	// +required
	ServiceAccount OmniKubeconfigServiceAccountSpec `json:"serviceAccount"`

	// ttl is the requested Omni service-account kubeconfig validity period.
	// +required
	TTL metav1.Duration `json:"ttl"`

	// renewBefore requests rotation this long before expiration.
	// +optional
	RenewBefore *metav1.Duration `json:"renewBefore,omitempty"`

	// allowClusterAdmin allows requesting the privileged system:masters group.
	// +optional
	AllowClusterAdmin bool `json:"allowClusterAdmin,omitempty"`

	// deletionPolicy controls what happens to the target Secret on resource deletion.
	// +kubebuilder:default:=Delete
	// +optional
	DeletionPolicy KubeconfigExportDeletionPolicy `json:"deletionPolicy,omitempty"`
}

// OmniKubeconfigExportStatus defines the observed state of OmniKubeconfigExport.
type OmniKubeconfigExportStatus struct {
	CommonStatusFields `json:",inline"`

	// clusterRef is the last OmniCluster name observed by the controller.
	// +optional
	ClusterRef string `json:"clusterRef,omitempty"`

	// targetSecretRef is the Secret name where kubeconfig data is stored.
	// +optional
	TargetSecretRef string `json:"targetSecretRef,omitempty"`

	// kubeconfigHash is the SHA-256 hash of the last exported kubeconfig bytes.
	// +optional
	KubeconfigHash string `json:"kubeconfigHash,omitempty"`

	// lastRotationTime is when kubeconfig data was last requested from Omni.
	// +optional
	LastRotationTime *metav1.Time `json:"lastRotationTime,omitempty"`

	// expirationTime is when the currently exported kubeconfig is expected to expire.
	// +optional
	ExpirationTime *metav1.Time `json:"expirationTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=omnikubeconfigexports,singular=omnikubeconfigexport

// OmniKubeconfigExport is the Schema for managed workload-cluster kubeconfig exports.
type OmniKubeconfigExport struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of OmniKubeconfigExport
	// +required
	Spec OmniKubeconfigExportSpec `json:"spec"`

	// status defines the observed state of OmniKubeconfigExport
	// +optional
	Status OmniKubeconfigExportStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// OmniKubeconfigExportList contains a list of OmniKubeconfigExport.
type OmniKubeconfigExportList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []OmniKubeconfigExport `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OmniKubeconfigExport{}, &OmniKubeconfigExportList{})
}
