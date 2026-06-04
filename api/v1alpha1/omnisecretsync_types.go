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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// SecretSyncDeletionPolicyDelete deletes the workload-cluster target Secret when the CR is deleted.
	SecretSyncDeletionPolicyDelete SecretSyncDeletionPolicy = "Delete"
	// SecretSyncDeletionPolicyOrphan leaves the workload-cluster target Secret when the CR is deleted.
	SecretSyncDeletionPolicyOrphan SecretSyncDeletionPolicy = "Orphan"
)

// SecretSyncDeletionPolicy controls workload-cluster Secret cleanup on deletion.
// +kubebuilder:validation:Enum=Delete;Orphan
type SecretSyncDeletionPolicy string

// SecretSyncKubeconfigSecretRef identifies the Secret key that contains workload-cluster kubeconfig data.
type SecretSyncKubeconfigSecretRef struct {
	// name is the kubeconfig Secret name in the same namespace.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// key is the Secret data key. Defaults to kubeconfig.
	// +kubebuilder:default:=kubeconfig
	// +optional
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key,omitempty"`
}

// SecretSyncSourceSecretRef identifies the management-cluster Secret to copy.
type SecretSyncSourceSecretRef struct {
	// name is the source Secret name in the same namespace.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// SecretSyncTargetSecretRef identifies the workload-cluster Secret to write.
type SecretSyncTargetSecretRef struct {
	// name is the target Secret name in the workload cluster.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// namespace is the target Secret namespace in the workload cluster.
	// +required
	// +kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace"`
}

// OmniSecretSyncSpec defines a management-cluster Secret copied to a workload cluster.
type OmniSecretSyncSpec struct {
	// clusterRef attaches this sync to one OmniCluster in the same namespace.
	// +required
	ClusterRef OmniClusterRef `json:"clusterRef"`

	// kubeconfigSecretRef selects a Secret key containing a workload-cluster kubeconfig.
	// Create this explicitly, for example with OmniKubeconfigExport.
	// +required
	KubeconfigSecretRef SecretSyncKubeconfigSecretRef `json:"kubeconfigSecretRef"`

	// sourceSecretRef selects the management-cluster Secret to copy.
	// The source Secret must be in the same namespace as this OmniSecretSync.
	// +required
	SourceSecretRef SecretSyncSourceSecretRef `json:"sourceSecretRef"`

	// targetSecretRef selects the workload-cluster Secret to create or update.
	// +required
	TargetSecretRef SecretSyncTargetSecretRef `json:"targetSecretRef"`

	// type overrides the target Secret type. When omitted, the source Secret type is used.
	// +optional
	Type corev1.SecretType `json:"type,omitempty"`

	// labels are merged into the target Secret labels in addition to operator ownership labels.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// annotations are merged into the target Secret annotations in addition to operator ownership annotations.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// createNamespace creates the workload-cluster target namespace if it is missing.
	// +optional
	CreateNamespace bool `json:"createNamespace,omitempty"`

	// deletionPolicy controls whether the workload-cluster target Secret is deleted when this CR is deleted.
	// +required
	DeletionPolicy SecretSyncDeletionPolicy `json:"deletionPolicy"`
}

// OmniSecretSyncStatus defines the observed state of OmniSecretSync.
type OmniSecretSyncStatus struct {
	CommonStatusFields `json:",inline"`

	// clusterRef is the last cluster reference observed by the controller.
	// +optional
	ClusterRef string `json:"clusterRef,omitempty"`

	// kubeconfigSecretRef is the kubeconfig Secret name observed by the controller.
	// +optional
	KubeconfigSecretRef string `json:"kubeconfigSecretRef,omitempty"`

	// kubeconfigSecretKey is the kubeconfig Secret key observed by the controller.
	// +optional
	KubeconfigSecretKey string `json:"kubeconfigSecretKey,omitempty"`

	// sourceSecretRef is the source Secret name observed by the controller.
	// +optional
	SourceSecretRef string `json:"sourceSecretRef,omitempty"`

	// targetSecretRef is the workload-cluster target Secret name observed by the controller.
	// +optional
	TargetSecretRef string `json:"targetSecretRef,omitempty"`

	// targetNamespace is the workload-cluster target Secret namespace observed by the controller.
	// +optional
	TargetNamespace string `json:"targetNamespace,omitempty"`

	// secretType is the target Secret type observed by the controller.
	// +optional
	SecretType string `json:"secretType,omitempty"`

	// secretHash is the SHA-256 hash of the synced Secret data and type.
	// +optional
	SecretHash string `json:"secretHash,omitempty"`

	// lastAttemptTime is when the controller last attempted to sync the Secret.
	// +optional
	LastAttemptTime *metav1.Time `json:"lastAttemptTime,omitempty"`

	// lastSyncTime is when the controller last completed a Secret sync successfully.
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// lastError is the last sync, cleanup, credential, or dependency error observed by the controller.
	// +optional
	LastError string `json:"lastError,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=omnisecretsyncs,singular=omnisecretsync

// OmniSecretSync is the Schema for the omnisecretsyncs API.
type OmniSecretSync struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of OmniSecretSync.
	// +required
	Spec OmniSecretSyncSpec `json:"spec"`

	// status defines the observed state of OmniSecretSync.
	// +optional
	Status OmniSecretSyncStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// OmniSecretSyncList contains a list of OmniSecretSync.
type OmniSecretSyncList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []OmniSecretSync `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OmniSecretSync{}, &OmniSecretSyncList{})
}
