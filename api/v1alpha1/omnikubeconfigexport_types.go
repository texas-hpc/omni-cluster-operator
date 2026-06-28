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

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	// KubeconfigClusterAdminGroup is Kubernetes' cluster-admin group.
	KubeconfigClusterAdminGroup = "system:masters"
	// KubeconfigExportDeletionPolicyDelete deletes the target Secret when the export resource is deleted.
	KubeconfigExportDeletionPolicyDelete KubeconfigExportDeletionPolicy = "Delete"
	// KubeconfigExportDeletionPolicyOrphan leaves the target Secret when the export resource is deleted.
	KubeconfigExportDeletionPolicyOrphan KubeconfigExportDeletionPolicy = "Orphan"
)

// KubeconfigExportDeletionPolicy controls target Secret cleanup on deletion.
// +kubebuilder:validation:Enum=Delete;Orphan
type KubeconfigExportDeletionPolicy string

// KubeconfigTargetSecretRef identifies the Secret key that receives generated kubeconfig data.
type KubeconfigTargetSecretRef struct {
	// name is the target Secret name in the same namespace.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// namespace is the target Secret namespace. Defaults to the OmniKubeconfigExport namespace.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Namespace string `json:"namespace,omitempty"`

	// key is the Secret data key. Defaults to kubeconfig.
	// +kubebuilder:default:=kubeconfig
	// +optional
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key,omitempty"`
}

// KubeconfigServiceAccountSpec configures the workload-cluster service account kubeconfig.
type KubeconfigServiceAccountSpec struct {
	// user is the Kubernetes username embedded in the generated kubeconfig.
	// +required
	// +kubebuilder:validation:MinLength=1
	User string `json:"user"`

	// groups are the Kubernetes groups embedded in the generated kubeconfig.
	// The system:masters group is rejected unless allowClusterAdmin is true.
	// +required
	// +kubebuilder:validation:MinItems=1
	// +listType=set
	Groups []string `json:"groups"`

	// allowClusterAdmin permits system:masters in groups. Leave false for scoped automation credentials.
	// +optional
	AllowClusterAdmin bool `json:"allowClusterAdmin,omitempty"`
}

// OmniKubeconfigExportSpec defines an explicit workload-cluster kubeconfig Secret export.
type OmniKubeconfigExportSpec struct {
	// clusterRef selects the OmniCluster whose workload-cluster kubeconfig should be exported.
	// +required
	ClusterRef OmniClusterRef `json:"clusterRef"`

	// targetSecretRef selects the Secret key that receives kubeconfig data.
	// +required
	TargetSecretRef KubeconfigTargetSecretRef `json:"targetSecretRef"`

	// serviceAccount configures the service-account kubeconfig requested from Omni.
	// +required
	ServiceAccount KubeconfigServiceAccountSpec `json:"serviceAccount"`

	// ttl is the requested service-account kubeconfig lifetime.
	// +required
	TTL metav1.Duration `json:"ttl"`

	// renewBefore rotates the kubeconfig before expiration. When omitted, rotation is due at expiration.
	// +optional
	RenewBefore *metav1.Duration `json:"renewBefore,omitempty"`

	// deletionPolicy controls whether the target Secret is deleted or orphaned when this resource is deleted.
	// +required
	DeletionPolicy KubeconfigExportDeletionPolicy `json:"deletionPolicy"`
}

// OmniKubeconfigExportStatus defines the observed state of OmniKubeconfigExport.
type OmniKubeconfigExportStatus struct {
	CommonStatusFields     `json:",inline"`
	ConnectionStatusFields `json:",inline"`

	// clusterRef is the last cluster reference observed by the controller.
	// +optional
	ClusterRef string `json:"clusterRef,omitempty"`

	// clusterName is the remote Omni cluster name.
	// +optional
	ClusterName string `json:"clusterName,omitempty"`

	// targetSecretRef is the target Secret name observed by the controller.
	// +optional
	TargetSecretRef string `json:"targetSecretRef,omitempty"`

	// targetSecretNamespace is the target Secret namespace observed by the controller.
	// +optional
	TargetSecretNamespace string `json:"targetSecretNamespace,omitempty"`

	// targetSecretKey is the target Secret data key observed by the controller.
	// +optional
	TargetSecretKey string `json:"targetSecretKey,omitempty"`

	// serviceAccountUser is the last requested service-account username.
	// +optional
	ServiceAccountUser string `json:"serviceAccountUser,omitempty"`

	// serviceAccountGroups are the last requested service-account groups.
	// +optional
	ServiceAccountGroups []string `json:"serviceAccountGroups,omitempty"`

	// kubeconfigHash is the SHA-256 hash of the kubeconfig data in the target Secret.
	// +optional
	KubeconfigHash string `json:"kubeconfigHash,omitempty"`

	// expirationTime is when the exported kubeconfig is expected to expire.
	// +optional
	ExpirationTime *metav1.Time `json:"expirationTime,omitempty"`

	// nextRotationTime is when the next rotation is due.
	// +optional
	NextRotationTime *metav1.Time `json:"nextRotationTime,omitempty"`

	// lastRotationTime is when the controller last generated and wrote kubeconfig data.
	// +optional
	LastRotationTime *metav1.Time `json:"lastRotationTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=omnikubeconfigexports,singular=omnikubeconfigexport

// OmniKubeconfigExport is the Schema for the omnikubeconfigexports API.
type OmniKubeconfigExport struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of OmniKubeconfigExport.
	// +required
	Spec OmniKubeconfigExportSpec `json:"spec"`

	// status defines the observed state of OmniKubeconfigExport.
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
