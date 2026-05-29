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

// OmniConnectionSpec defines one remote Omni API endpoint and its credentials.
type OmniConnectionSpec struct {
	// endpoint is the Omni API URL. Use https for managed and normal self-hosted Omni.
	// +required
	// +kubebuilder:validation:Pattern=`^(https?|grpc)://.+`
	Endpoint string `json:"endpoint"`

	// auth configures non-interactive authentication to Omni.
	// +required
	Auth OmniAuthSpec `json:"auth"`

	// insecureSkipTLSVerify disables TLS certificate verification. Use only for local development.
	// +optional
	InsecureSkipTLSVerify bool `json:"insecureSkipTLSVerify,omitempty"`
}

// OmniConnectionStatus defines the observed state of OmniConnection.
type OmniConnectionStatus struct {
	CommonStatusFields     `json:",inline"`
	ConnectionStatusFields `json:",inline"`

	// lastCheckTime is when connectivity was last checked.
	// +optional
	LastCheckTime *metav1.Time `json:"lastCheckTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// OmniConnection is the Schema for the omniconnections API
type OmniConnection struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of OmniConnection
	// +required
	Spec OmniConnectionSpec `json:"spec"`

	// status defines the observed state of OmniConnection
	// +optional
	Status OmniConnectionStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// OmniConnectionList contains a list of OmniConnection
type OmniConnectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []OmniConnection `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OmniConnection{}, &OmniConnectionList{})
}
