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

// OmniControlPlaneSpec defines the Omni ControlPlane template document.
// +kubebuilder:validation:XValidation:rule="has(self.machines) != has(self.machineClass)",message="exactly one of machines or machineClass is required"
type OmniControlPlaneSpec struct {
	MachineSetSpecFields `json:",inline"`

	// clusterRef attaches this control plane document to one OmniCluster in the same namespace.
	// +required
	ClusterRef OmniClusterRef `json:"clusterRef"`

	// bootstrapSpec configures restore bootstrap behavior.
	// +optional
	BootstrapSpec *BootstrapSpec `json:"bootstrapSpec,omitempty"`
}

// OmniControlPlaneStatus defines the observed state of OmniControlPlane.
type OmniControlPlaneStatus struct {
	CommonStatusFields `json:",inline"`

	// clusterRef is the last cluster reference observed by the controller.
	// +optional
	ClusterRef string `json:"clusterRef,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// OmniControlPlane is the Schema for the omnicontrolplanes API
type OmniControlPlane struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of OmniControlPlane
	// +required
	Spec OmniControlPlaneSpec `json:"spec"`

	// status defines the observed state of OmniControlPlane
	// +optional
	Status OmniControlPlaneStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// OmniControlPlaneList contains a list of OmniControlPlane
type OmniControlPlaneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []OmniControlPlane `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OmniControlPlane{}, &OmniControlPlaneList{})
}
