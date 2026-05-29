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

// OmniWorkersSpec defines one Omni Workers template document.
// +kubebuilder:validation:XValidation:rule="has(self.machines) != has(self.machineClass)",message="exactly one of machines or machineClass is required"
type OmniWorkersSpec struct {
	MachineSetSpecFields `json:",inline"`

	// clusterRef attaches this workers document to one OmniCluster in the same namespace.
	// +required
	ClusterRef OmniClusterRef `json:"clusterRef"`

	// workerSetName is the Omni workers document name. Defaults to metadata.name when omitted.
	// Must be unique within the cluster and cannot be control-planes.
	// +optional
	// +kubebuilder:validation:Pattern=`^[A-Za-z0-9_-]+$`
	WorkerSetName string `json:"workerSetName,omitempty"`

	// updateStrategy controls config changes across machines in this set.
	// +optional
	UpdateStrategy *UpdateStrategy `json:"updateStrategy,omitempty"`

	// upgradeStrategy controls version, extension, and kernel arg changes across machines in this set.
	// +optional
	UpgradeStrategy *UpdateStrategy `json:"upgradeStrategy,omitempty"`

	// deleteStrategy controls machine removal when this set scales down.
	// +optional
	DeleteStrategy *UpdateStrategy `json:"deleteStrategy,omitempty"`
}

// OmniWorkersStatus defines the observed state of OmniWorkers.
type OmniWorkersStatus struct {
	CommonStatusFields `json:",inline"`

	// clusterRef is the last cluster reference observed by the controller.
	// +optional
	ClusterRef string `json:"clusterRef,omitempty"`

	// workerSetName is the Omni worker set name rendered by the controller.
	// +optional
	WorkerSetName string `json:"workerSetName,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// OmniWorkers is the Schema for the omniworkers API
type OmniWorkers struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of OmniWorkers
	// +required
	Spec OmniWorkersSpec `json:"spec"`

	// status defines the observed state of OmniWorkers
	// +optional
	Status OmniWorkersStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// OmniWorkersList contains a list of OmniWorkers
type OmniWorkersList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []OmniWorkers `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OmniWorkers{}, &OmniWorkersList{})
}
