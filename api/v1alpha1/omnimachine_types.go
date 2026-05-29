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

// OmniMachineSpec defines one optional Omni Machine template document.
type OmniMachineSpec struct {
	Descriptors `json:",inline"`

	// clusterRef attaches this machine document to one OmniCluster in the same namespace.
	// +required
	ClusterRef OmniClusterRef `json:"clusterRef"`

	// machineID is the Omni machine ID. Defaults to metadata.name when omitted.
	// +optional
	MachineID string `json:"machineID,omitempty"`

	// locked prevents config updates, upgrades, and downgrades for this machine.
	// Omni allows locked machines only when used as workers.
	// +optional
	Locked bool `json:"locked,omitempty"`

	// install configures Talos installation for this static machine.
	// +optional
	Install *MachineInstallSpec `json:"install,omitempty"`

	// patches are applied to this machine.
	// +optional
	Patches []Patch `json:"patches,omitempty"`

	// systemExtensions are installed on this machine.
	// +optional
	SystemExtensions []string `json:"systemExtensions,omitempty"`

	// kernelArgs are managed for this static machine.
	// +optional
	KernelArgs []string `json:"kernelArgs,omitempty"`
}

// OmniMachineStatus defines the observed state of OmniMachine.
type OmniMachineStatus struct {
	CommonStatusFields `json:",inline"`

	// clusterRef is the last cluster reference observed by the controller.
	// +optional
	ClusterRef string `json:"clusterRef,omitempty"`

	// machineID is the Omni machine ID rendered by the controller.
	// +optional
	MachineID string `json:"machineID,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// OmniMachine is the Schema for the omnimachines API
type OmniMachine struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of OmniMachine
	// +required
	Spec OmniMachineSpec `json:"spec"`

	// status defines the observed state of OmniMachine
	// +optional
	Status OmniMachineStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// OmniMachineList contains a list of OmniMachine
type OmniMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []OmniMachine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OmniMachine{}, &OmniMachineList{})
}
